package application

import (
	"bytes"
	"context"
	"testing"
	"time"

	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"
	"file-workshop/backend/internal/modules/versions/domain"

	"github.com/google/uuid"
)

func TestAcquireLockReturnsTokenAndFencingToken(t *testing.T) {
	repository := newFakeRepository()
	service := NewService(repository, repository, allowAuthorizer{}, fixedNow)
	result, err := service.AcquireLock(context.Background(), fakeActor(), fakeDocumentID, domain.AcquireLockInput{Source: domain.LockSourceWeb, RequestID: fakeRequestID})
	if err != nil {
		t.Fatalf("acquire lock failed: %v", err)
	}
	if result.LockToken == "" {
		t.Fatalf("lock token should be returned once on acquire")
	}
	if result.Lock.FencingToken != 1 {
		t.Fatalf("expected fencing token 1, got %d", result.Lock.FencingToken)
	}
	if len(repository.lastTokenHash) != 32 || bytes.Contains(repository.lastTokenHash, []byte(result.LockToken)) {
		t.Fatalf("repository should receive sha256 token hash, not raw token")
	}
}

func TestAcquireLockRejectsExistingActiveLock(t *testing.T) {
	repository := newFakeRepository()
	repository.activeLock = &domain.Lock{ID: uuid.MustParse("0198b100-0000-7000-8000-000000000090"), DocumentID: fakeDocumentID, UserID: fakeActor().UserID, Status: domain.LockStatusActive, ExpiresAt: fixedNow().Add(time.Minute), RowVersion: 1}
	service := NewService(repository, repository, allowAuthorizer{}, fixedNow)
	_, err := service.AcquireLock(context.Background(), fakeActor(), fakeDocumentID, domain.AcquireLockInput{Source: domain.LockSourceWeb, RequestID: fakeRequestID})
	if err != domain.ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
}

func TestRestoreVersionCreatesNewCurrentVersion(t *testing.T) {
	repository := newFakeRepository()
	service := NewService(repository, repository, allowAuthorizer{}, fixedNow)
	note := "恢复历史版本"
	version, err := service.RestoreVersion(context.Background(), fakeActor(), fakeDocumentID, fakeVersionID, domain.RestoreVersionInput{RowVersion: 3, ChangeNote: &note, IdempotencyKey: "restore-key-1", RequestID: fakeRequestID})
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if version.SourceType != "RESTORE" || version.RestoredFromVersionID == nil || *version.RestoredFromVersionID != fakeVersionID {
		t.Fatalf("restore should create a new RESTORE version from source")
	}
	if repository.currentVersionID == nil || *repository.currentVersionID != version.ID {
		t.Fatalf("restored version should become current")
	}
}

func TestRestoreVersionRejectsActiveLock(t *testing.T) {
	repository := newFakeRepository()
	repository.activeLock = &domain.Lock{ID: uuid.MustParse("0198b100-0000-7000-8000-000000000091"), DocumentID: fakeDocumentID, UserID: fakeActor().UserID, Status: domain.LockStatusActive, ExpiresAt: fixedNow().Add(time.Minute), RowVersion: 1}
	service := NewService(repository, repository, allowAuthorizer{}, fixedNow)
	_, err := service.RestoreVersion(context.Background(), fakeActor(), fakeDocumentID, fakeVersionID, domain.RestoreVersionInput{RowVersion: 3, IdempotencyKey: "restore-key-2", RequestID: fakeRequestID})
	if err != domain.ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
}

func TestForceReleaseLockRequiresSystemAdmin(t *testing.T) {
	repository := newFakeRepository()
	service := NewService(repository, repository, allowAuthorizer{}, fixedNow)
	_, err := service.ForceReleaseLock(context.Background(), fakeActor(), fakeDocumentID, domain.ForceReleaseLockInput{RowVersion: 1, Reason: "管理员释放异常锁", RequestID: fakeRequestID})
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

var (
	fakeDocumentID = uuid.MustParse("0198b100-0000-7000-8000-000000000001")
	fakeVersionID  = uuid.MustParse("0198b100-0000-7000-8000-000000000002")
	fakeRequestID  = uuid.MustParse("0198b100-0000-7000-8000-000000000003")
	fakeStorageID  = uuid.MustParse("0198b100-0000-7000-8000-000000000004")
)

func fixedNow() time.Time {
	return time.Date(2026, 8, 10, 11, 0, 0, 0, time.UTC)
}

func fakeActor() domain.Actor {
	return domain.Actor{UserID: uuid.MustParse("0198b100-0000-7000-8000-000000000010"), SessionID: uuid.MustParse("0198b100-0000-7000-8000-000000000011")}
}

type allowAuthorizer struct{}

func (allowAuthorizer) EvaluatePermission(context.Context, permissiondomain.Actor, string, uuid.UUID, string, *string, bool) (permissiondomain.PermissionEvaluation, error) {
	return permissiondomain.PermissionEvaluation{Allowed: true}, nil
}

type fakeRepository struct {
	document         domain.DocumentContext
	versions         map[uuid.UUID]domain.Version
	currentVersionID *uuid.UUID
	activeLock       *domain.Lock
	lastTokenHash    []byte
	idempotency      map[string]domain.IdempotencyRecord
}

func newFakeRepository() *fakeRepository {
	sha := bytes.Repeat([]byte{1}, 32)
	return &fakeRepository{
		document:    domain.DocumentContext{ID: fakeDocumentID, OwnerUserID: fakeActor().UserID, RowVersion: 3},
		versions:    map[uuid.UUID]domain.Version{fakeVersionID: {ID: fakeVersionID, DocumentID: fakeDocumentID, VersionNumber: 1, StorageObjectID: fakeStorageID, SizeBytes: 100, SHA256: sha, MIMEType: "application/pdf", SourceType: "WEB", CreatedByUserID: fakeActor().UserID, CreatedAt: fixedNow()}},
		idempotency: map[string]domain.IdempotencyRecord{},
	}
}

func (r *fakeRepository) WithinTransaction(ctx context.Context, operation func(Repository) error) error {
	return operation(r)
}

func (r *fakeRepository) GetDocumentContext(context.Context, uuid.UUID) (domain.DocumentContext, error) {
	return r.document, nil
}

func (r *fakeRepository) CountVersions(context.Context, uuid.UUID) (int64, error) {
	return int64(len(r.versions)), nil
}

func (r *fakeRepository) ListVersions(context.Context, uuid.UUID, int, int) ([]domain.Version, error) {
	items := make([]domain.Version, 0, len(r.versions))
	for _, value := range r.versions {
		items = append(items, value)
	}
	return items, nil
}

func (r *fakeRepository) GetVersion(_ context.Context, _ uuid.UUID, versionID uuid.UUID) (domain.Version, error) {
	value, ok := r.versions[versionID]
	if !ok {
		return domain.Version{}, domain.ErrNotFound
	}
	return value, nil
}

func (r *fakeRepository) GetDocumentForUpdate(context.Context, uuid.UUID) (domain.DocumentContext, error) {
	return r.document, nil
}

func (r *fakeRepository) InsertRestoredVersion(_ context.Context, id, documentID, sourceVersionID, createdByUserID uuid.UUID, changeNote *string, now time.Time) (domain.Version, error) {
	source := r.versions[sourceVersionID]
	version := domain.Version{ID: id, DocumentID: documentID, VersionNumber: source.VersionNumber + 1, StorageObjectID: source.StorageObjectID, SizeBytes: source.SizeBytes, SHA256: source.SHA256, MIMEType: source.MIMEType, ChangeNote: changeNote, SourceType: "RESTORE", RestoredFromVersionID: &sourceVersionID, CreatedByUserID: createdByUserID, CreatedAt: now}
	r.versions[id] = version
	return version, nil
}

func (r *fakeRepository) SetCurrentVersion(_ context.Context, _ uuid.UUID, versionID uuid.UUID, _ int64, _ time.Time) error {
	r.currentVersionID = &versionID
	return nil
}

func (r *fakeRepository) ExpireLocks(context.Context, uuid.UUID, time.Time) error { return nil }

func (r *fakeRepository) EnsureLockCounter(context.Context, uuid.UUID, time.Time) error { return nil }

func (r *fakeRepository) IncrementLockCounter(context.Context, uuid.UUID, time.Time) (int64, error) {
	return 1, nil
}

func (r *fakeRepository) InsertLock(_ context.Context, id, documentID, userID uuid.UUID, tokenHash []byte, fencingToken int64, source string, acquiredAt, expiresAt time.Time) (domain.Lock, error) {
	r.lastTokenHash = append([]byte(nil), tokenHash...)
	lock := domain.Lock{ID: id, DocumentID: documentID, UserID: userID, FencingToken: fencingToken, Source: source, Status: domain.LockStatusActive, AcquiredAt: acquiredAt, HeartbeatAt: acquiredAt, ExpiresAt: expiresAt, CreatedAt: acquiredAt, UpdatedAt: acquiredAt, RowVersion: 1}
	r.activeLock = &lock
	return lock, nil
}

func (r *fakeRepository) GetActiveLock(context.Context, uuid.UUID) (*domain.Lock, error) {
	return r.activeLock, nil
}

func (r *fakeRepository) GetActiveLockForUpdate(context.Context, uuid.UUID) (*domain.Lock, error) {
	return r.activeLock, nil
}

func (r *fakeRepository) HeartbeatLock(context.Context, uuid.UUID, []byte, int64, uuid.UUID, time.Time, time.Time) (domain.Lock, error) {
	return domain.Lock{}, nil
}

func (r *fakeRepository) ReleaseLock(context.Context, uuid.UUID, []byte, int64, uuid.UUID, time.Time, *string) (domain.Lock, error) {
	return domain.Lock{}, nil
}

func (r *fakeRepository) ForceReleaseLock(context.Context, uuid.UUID, int64, uuid.UUID, time.Time, string) (domain.Lock, error) {
	return domain.Lock{}, nil
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

func (r *fakeRepository) InsertEvent(context.Context, domain.Event) error { return nil }
