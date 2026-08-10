package application

import (
	"context"
	"errors"
	"testing"
	"time"

	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"
	"file-workshop/backend/internal/modules/uploads/domain"
	"file-workshop/backend/internal/platform/objectstorage"

	"github.com/google/uuid"
)

func TestCreateSessionStorageDisabledDoesNotWriteDatabase(t *testing.T) {
	repository := newFakeRepository()
	storage := &fakeStorage{createErr: objectstorage.ErrDisabled}
	service := NewService(repository, repository, allowAuthorizer{}, storage, Config{PresignTTL: time.Minute}, fixedNow)
	_, err := service.CreateSession(context.Background(), fakeActor(), validCreateInput())
	if !errors.Is(err, domain.ErrStorageUnavailable) {
		t.Fatalf("expected storage unavailable, got %v", err)
	}
	if repository.reserveQuotaCalled || repository.insertSessionCalled {
		t.Fatalf("database write path should not run when object storage is disabled")
	}
}

func TestCreateSessionCreatesMultipartAndDatabaseSession(t *testing.T) {
	repository := newFakeRepository()
	storage := &fakeStorage{uploadID: "provider-upload-1"}
	service := NewService(repository, repository, allowAuthorizer{}, storage, Config{PresignTTL: time.Minute}, fixedNow)
	session, err := service.CreateSession(context.Background(), fakeActor(), validCreateInput())
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	if !repository.reserveQuotaCalled || !repository.insertSessionCalled {
		t.Fatalf("expected quota reservation and upload session insert")
	}
	if session.ExpectedPartCount != 2 {
		t.Fatalf("expected 2 parts, got %d", session.ExpectedPartCount)
	}
	if session.ProviderUploadID == nil || *session.ProviderUploadID != "provider-upload-1" {
		t.Fatalf("provider upload id was not stored")
	}
	if session.TemporaryObjectKey == "" || session.TemporaryObjectKey == "demo.pdf" {
		t.Fatalf("temporary object key must be system generated, got %q", session.TemporaryObjectKey)
	}
	if storage.aborted {
		t.Fatalf("remote multipart upload should not be aborted after successful creation")
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
}

func fakeActor() domain.Actor {
	return domain.Actor{UserID: uuid.MustParse("018f1d8e-0000-7000-8000-000000000001"), SessionID: uuid.MustParse("018f1d8e-0000-7000-8000-000000000002")}
}

func validCreateInput() domain.CreateSessionInput {
	return domain.CreateSessionInput{
		SpaceID:           uuid.MustParse("018f1d8e-0000-7000-8000-000000000010"),
		FolderID:          uuid.MustParse("018f1d8e-0000-7000-8000-000000000020"),
		UploadIntent:      domain.IntentCreate,
		FileName:          "demo.pdf",
		DeclaredSizeBytes: 9 * 1024 * 1024,
		PartSizeBytes:     5 * 1024 * 1024,
		IdempotencyKey:    "upload-key-1",
		RequestID:         uuid.MustParse("018f1d8e-0000-7000-8000-000000000030"),
	}
}

type allowAuthorizer struct{}

func (allowAuthorizer) EvaluatePermission(context.Context, permissiondomain.Actor, string, uuid.UUID, string, *string, bool) (permissiondomain.PermissionEvaluation, error) {
	return permissiondomain.PermissionEvaluation{Allowed: true}, nil
}

type fakeRepository struct {
	folder              domain.FolderContext
	sessions            map[uuid.UUID]domain.Session
	idempotency         map[string]domain.IdempotencyRecord
	reserveQuotaCalled  bool
	insertSessionCalled bool
}

func newFakeRepository() *fakeRepository {
	input := validCreateInput()
	return &fakeRepository{
		folder:      domain.FolderContext{ID: input.FolderID, SpaceID: input.SpaceID},
		sessions:    map[uuid.UUID]domain.Session{},
		idempotency: map[string]domain.IdempotencyRecord{},
	}
}

func (r *fakeRepository) WithinTransaction(ctx context.Context, operation func(Repository) error) error {
	return operation(r)
}

func (r *fakeRepository) GetFolderContext(context.Context, uuid.UUID, uuid.UUID) (domain.FolderContext, error) {
	return r.folder, nil
}

func (r *fakeRepository) GetDocumentContext(context.Context, uuid.UUID, uuid.UUID) (domain.DocumentContext, error) {
	return domain.DocumentContext{}, domain.ErrNotFound
}

func (r *fakeRepository) GetSession(_ context.Context, id uuid.UUID) (domain.Session, error) {
	session, ok := r.sessions[id]
	if !ok {
		return domain.Session{}, domain.ErrNotFound
	}
	return session, nil
}

func (r *fakeRepository) GetSessionForUpdate(ctx context.Context, id uuid.UUID) (domain.Session, error) {
	return r.GetSession(ctx, id)
}

func (r *fakeRepository) TryCreateIdempotency(_ context.Context, recordID, actorID uuid.UUID, operation, key string, hash []byte, expiresAt, now time.Time) (bool, error) {
	if _, ok := r.idempotency[key]; ok {
		return false, nil
	}
	r.idempotency[key] = domain.IdempotencyRecord{RequestHash: append([]byte(nil), hash...), Status: "IN_PROGRESS"}
	return true, nil
}

func (r *fakeRepository) GetIdempotency(_ context.Context, actorID uuid.UUID, operation, key string) (domain.IdempotencyRecord, error) {
	record, ok := r.idempotency[key]
	if !ok {
		return domain.IdempotencyRecord{}, domain.ErrConflict
	}
	return record, nil
}

func (r *fakeRepository) CompleteIdempotency(_ context.Context, actorID uuid.UUID, operation, key string, resourceID uuid.UUID, resourceType string, now time.Time) error {
	record := r.idempotency[key]
	record.Status = "COMPLETED"
	record.ResultResourceID = &resourceID
	r.idempotency[key] = record
	return nil
}

func (r *fakeRepository) ReserveQuota(context.Context, uuid.UUID, int64, time.Time) error {
	r.reserveQuotaCalled = true
	return nil
}

func (r *fakeRepository) InsertQuotaReservation(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64, time.Time, time.Time) error {
	return nil
}

func (r *fakeRepository) ReleaseQuotaReservation(context.Context, uuid.UUID, uuid.UUID, int64, time.Time) error {
	return nil
}

func (r *fakeRepository) InsertSession(_ context.Context, input domain.NewSession) (domain.Session, error) {
	r.insertSessionCalled = true
	providerUploadID := input.ProviderUploadID
	session := domain.Session{
		ID:                 input.ID,
		UserID:             input.UserID,
		SpaceID:            input.SpaceID,
		FolderID:           input.FolderID,
		QuotaReservationID: input.QuotaReservationID,
		UploadIntent:       input.UploadIntent,
		FileName:           input.FileName,
		NormalizedName:     input.NormalizedName,
		DeclaredSizeBytes:  input.DeclaredSizeBytes,
		ProviderUploadID:   &providerUploadID,
		TemporaryObjectKey: input.TemporaryObjectKey,
		PartSizeBytes:      input.PartSizeBytes,
		ExpectedPartCount:  input.ExpectedPartCount,
		Status:             domain.StatusInitiated,
		ExpiresAt:          input.ExpiresAt,
		CreatedAt:          input.CreatedAt,
		UpdatedAt:          input.CreatedAt,
		RowVersion:         1,
	}
	r.sessions[session.ID] = session
	return session, nil
}

func (r *fakeRepository) MarkUploading(context.Context, uuid.UUID, time.Time) (domain.Session, error) {
	return domain.Session{}, nil
}

func (r *fakeRepository) AbortSession(context.Context, uuid.UUID, int64, string, time.Time) (domain.Session, error) {
	return domain.Session{}, nil
}

func (r *fakeRepository) InsertEvent(context.Context, domain.Event) error {
	return nil
}

type fakeStorage struct {
	uploadID  string
	createErr error
	aborted   bool
}

func (s *fakeStorage) Check(context.Context) error { return nil }

func (s *fakeStorage) CreateMultipartUpload(context.Context, objectstorage.CreateMultipartUploadInput) (objectstorage.CreateMultipartUploadOutput, error) {
	if s.createErr != nil {
		return objectstorage.CreateMultipartUploadOutput{}, s.createErr
	}
	return objectstorage.CreateMultipartUploadOutput{UploadID: s.uploadID}, nil
}

func (s *fakeStorage) PresignUploadPart(context.Context, objectstorage.PresignUploadPartInput) (objectstorage.PresignedRequest, error) {
	return objectstorage.PresignedRequest{}, nil
}

func (s *fakeStorage) CompleteMultipartUpload(context.Context, objectstorage.CompleteMultipartUploadInput) error {
	return nil
}

func (s *fakeStorage) AbortMultipartUpload(context.Context, objectstorage.AbortMultipartUploadInput) error {
	s.aborted = true
	return nil
}

func (s *fakeStorage) PresignGetObject(context.Context, objectstorage.PresignGetObjectInput) (objectstorage.PresignedRequest, error) {
	return objectstorage.PresignedRequest{}, nil
}

func (s *fakeStorage) HeadObject(context.Context, objectstorage.HeadObjectInput) (objectstorage.HeadObjectOutput, error) {
	return objectstorage.HeadObjectOutput{}, nil
}
