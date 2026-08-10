package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"
	"file-workshop/backend/internal/modules/uploads/domain"
	"file-workshop/backend/internal/platform/objectstorage"

	"github.com/google/uuid"
)

const createUploadOperation = "CREATE_UPLOAD_SESSION"

type Authorizer interface {
	EvaluatePermission(context.Context, permissiondomain.Actor, string, uuid.UUID, string, *string, bool) (permissiondomain.PermissionEvaluation, error)
}

type Config struct {
	Bucket     string
	PresignTTL time.Duration
	SessionTTL time.Duration
}

type Service struct {
	repository Repository
	transactor Transactor
	authorizer Authorizer
	storage    objectstorage.Client
	config     Config
	now        func() time.Time
}

func NewService(repository Repository, transactor Transactor, authorizer Authorizer, storage objectstorage.Client, config Config, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	if config.PresignTTL <= 0 {
		config.PresignTTL = 15 * time.Minute
	}
	if config.SessionTTL <= 0 {
		config.SessionTTL = domain.DefaultSessionTTL
	}
	return &Service{repository: repository, transactor: transactor, authorizer: authorizer, storage: storage, config: config, now: now}
}

func (s *Service) CreateSession(ctx context.Context, actor domain.Actor, input domain.CreateSessionInput) (domain.Session, error) {
	normalizedInput, expectedPartCount, err := validateCreateInput(input)
	if err != nil {
		return domain.Session{}, err
	}
	now := s.now().UTC()
	hash, err := requestHash(struct {
		SpaceID                  uuid.UUID
		FolderID                 uuid.UUID
		TargetDocumentID         *uuid.UUID
		UploadIntent             string
		FileName                 string
		DeclaredSizeBytes        int64
		DeclaredSHA256           []byte
		DeclaredMIMEType         *string
		PartSizeBytes            int64
		ExpectedCurrentVersionID *uuid.UUID
		ExpectedLockFencingToken *int64
		LockTokenHash            []byte
	}{normalizedInput.SpaceID, normalizedInput.FolderID, normalizedInput.TargetDocumentID, normalizedInput.UploadIntent, normalizedInput.FileName, normalizedInput.DeclaredSizeBytes, normalizedInput.DeclaredSHA256, normalizedInput.DeclaredMIMEType, normalizedInput.PartSizeBytes, normalizedInput.ExpectedCurrentVersionID, normalizedInput.ExpectedLockFencingToken, normalizedInput.LockTokenHash})
	if err != nil {
		return domain.Session{}, err
	}
	if err = s.authorizeCreate(ctx, actor, normalizedInput); err != nil {
		return domain.Session{}, err
	}
	sessionID, err := uuid.NewV7()
	if err != nil {
		return domain.Session{}, err
	}
	quotaReservationID, err := uuid.NewV7()
	if err != nil {
		return domain.Session{}, err
	}
	key := temporaryObjectKey(now, normalizedInput.SpaceID, sessionID)
	multipart, err := s.storage.CreateMultipartUpload(ctx, objectstorage.CreateMultipartUploadInput{
		Bucket:      s.config.Bucket,
		Key:         key,
		ContentType: stringValue(normalizedInput.DeclaredMIMEType),
		Metadata: map[string]string{
			"upload-session-id": sessionID.String(),
			"space-id":          normalizedInput.SpaceID.String(),
		},
	})
	if err != nil {
		return domain.Session{}, mapStorageError(err)
	}
	abortRemote := true
	defer func() {
		if abortRemote && strings.TrimSpace(multipart.UploadID) != "" {
			_ = s.storage.AbortMultipartUpload(context.Background(), objectstorage.AbortMultipartUploadInput{Bucket: s.config.Bucket, Key: key, UploadID: multipart.UploadID})
		}
	}()

	var result domain.Session
	createdNew := false
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		replayID, err := claimIdempotency(ctx, repository, actor.UserID, createUploadOperation, normalizedInput.IdempotencyKey, hash, now)
		if err != nil {
			return err
		}
		if replayID != nil {
			result, err = repository.GetSession(ctx, *replayID)
			return err
		}
		if err = repository.ReserveQuota(ctx, normalizedInput.SpaceID, normalizedInput.DeclaredSizeBytes, now); err != nil {
			return err
		}
		expiresAt := now.Add(s.config.SessionTTL)
		if err = repository.InsertQuotaReservation(ctx, quotaReservationID, normalizedInput.SpaceID, actor.UserID, normalizedInput.DeclaredSizeBytes, expiresAt, now); err != nil {
			return err
		}
		result, err = repository.InsertSession(ctx, domain.NewSession{
			ID:                       sessionID,
			UserID:                   actor.UserID,
			SpaceID:                  normalizedInput.SpaceID,
			FolderID:                 normalizedInput.FolderID,
			QuotaReservationID:       quotaReservationID,
			TargetDocumentID:         normalizedInput.TargetDocumentID,
			UploadIntent:             normalizedInput.UploadIntent,
			FileName:                 normalizedInput.FileName,
			NormalizedName:           domain.NormalizeName(normalizedInput.FileName),
			DeclaredSizeBytes:        normalizedInput.DeclaredSizeBytes,
			DeclaredSHA256:           normalizedInput.DeclaredSHA256,
			DeclaredMIMEType:         normalizedInput.DeclaredMIMEType,
			ProviderUploadID:         multipart.UploadID,
			TemporaryObjectKey:       key,
			PartSizeBytes:            normalizedInput.PartSizeBytes,
			ExpectedPartCount:        expectedPartCount,
			ExpectedCurrentVersionID: normalizedInput.ExpectedCurrentVersionID,
			ExpectedLockFencingToken: normalizedInput.ExpectedLockFencingToken,
			LockTokenHash:            normalizedInput.LockTokenHash,
			ExpiresAt:                expiresAt,
			CreatedAt:                now,
		})
		if err != nil {
			return err
		}
		createdNew = true
		if err = insertEvent(ctx, repository, result, "UPLOAD_SESSION_CREATED", normalizedInput.RequestID, now); err != nil {
			return err
		}
		return repository.CompleteIdempotency(ctx, actor.UserID, createUploadOperation, normalizedInput.IdempotencyKey, result.ID, domain.ResourceUploadSession, now)
	})
	if err != nil {
		return domain.Session{}, err
	}
	abortRemote = !createdNew
	return result, nil
}

func (s *Service) GetSession(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.Session, error) {
	session, err := s.repository.GetSession(ctx, id)
	if err != nil {
		return domain.Session{}, err
	}
	if err = s.requireSessionAccess(actor, session); err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (s *Service) PresignUploadPart(ctx context.Context, actor domain.Actor, id uuid.UUID, partNumber int32) (domain.PresignedPart, error) {
	if partNumber < 1 {
		return domain.PresignedPart{}, &domain.ValidationError{Field: "partNumber"}
	}
	now := s.now().UTC()
	session, err := s.repository.GetSession(ctx, id)
	if err != nil {
		return domain.PresignedPart{}, err
	}
	if err = s.requireSessionAccess(actor, session); err != nil {
		return domain.PresignedPart{}, err
	}
	if partNumber > session.ExpectedPartCount {
		return domain.PresignedPart{}, &domain.ValidationError{Field: "partNumber"}
	}
	if now.After(session.ExpiresAt) || (session.Status != domain.StatusInitiated && session.Status != domain.StatusUploading) {
		return domain.PresignedPart{}, domain.ErrConflict
	}
	if session.ProviderUploadID == nil || strings.TrimSpace(*session.ProviderUploadID) == "" {
		return domain.PresignedPart{}, domain.ErrConflict
	}
	if session.Status == domain.StatusInitiated {
		updated, err := s.repository.MarkUploading(ctx, session.ID, now)
		if err == nil {
			session = updated
		} else if !errors.Is(err, domain.ErrConflict) {
			return domain.PresignedPart{}, err
		}
	}
	contentBytes := partContentBytes(session, partNumber)
	request, err := s.storage.PresignUploadPart(ctx, objectstorage.PresignUploadPartInput{
		Bucket:       s.config.Bucket,
		Key:          session.TemporaryObjectKey,
		UploadID:     *session.ProviderUploadID,
		PartNumber:   partNumber,
		ExpiresIn:    s.config.PresignTTL,
		ContentBytes: contentBytes,
	})
	if err != nil {
		return domain.PresignedPart{}, mapStorageError(err)
	}
	return domain.PresignedPart{SessionID: session.ID, PartNumber: partNumber, Method: request.Method, URL: request.URL, Headers: request.Headers, ExpiresAt: request.ExpiresAt}, nil
}

func (s *Service) AbortSession(ctx context.Context, actor domain.Actor, id uuid.UUID, rowVersion int64, reason string, requestID uuid.UUID) (domain.Session, error) {
	if rowVersion < 1 {
		return domain.Session{}, &domain.ValidationError{Field: "rowVersion"}
	}
	if err := domain.ValidateOptionalText("reason", &reason, 512); err != nil {
		return domain.Session{}, err
	}
	now := s.now().UTC()
	current, err := s.repository.GetSession(ctx, id)
	if err != nil {
		return domain.Session{}, err
	}
	if err = s.requireSessionAccess(actor, current); err != nil {
		return domain.Session{}, err
	}
	var result domain.Session
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		locked, err := repository.GetSessionForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if locked.RowVersion != rowVersion {
			return domain.ErrVersionConflict
		}
		if locked.Status != domain.StatusInitiated && locked.Status != domain.StatusUploading && locked.Status != domain.StatusFailed {
			return domain.ErrConflict
		}
		result, err = repository.AbortSession(ctx, id, rowVersion, domain.FailureUserAborted, now)
		if err != nil {
			return err
		}
		if err = repository.ReleaseQuotaReservation(ctx, result.QuotaReservationID, result.SpaceID, result.DeclaredSizeBytes, now); err != nil {
			return err
		}
		return insertEvent(ctx, repository, result, "UPLOAD_SESSION_ABORTED", requestID, now)
	})
	if err != nil {
		return domain.Session{}, err
	}
	if result.ProviderUploadID != nil && strings.TrimSpace(*result.ProviderUploadID) != "" {
		_ = s.storage.AbortMultipartUpload(ctx, objectstorage.AbortMultipartUploadInput{Bucket: s.config.Bucket, Key: result.TemporaryObjectKey, UploadID: *result.ProviderUploadID})
	}
	return result, nil
}

func (s *Service) authorizeCreate(ctx context.Context, actor domain.Actor, input domain.CreateSessionInput) error {
	if _, err := s.repository.GetFolderContext(ctx, input.SpaceID, input.FolderID); err != nil {
		return err
	}
	resourceType, resourceID := permissiondomain.ResourceFolder, input.FolderID
	if input.UploadIntent == domain.IntentNewVersion {
		if input.TargetDocumentID == nil {
			return &domain.ValidationError{Field: "targetDocumentId"}
		}
		document, err := s.repository.GetDocumentContext(ctx, input.SpaceID, *input.TargetDocumentID)
		if err != nil {
			return err
		}
		if input.ExpectedCurrentVersionID != nil {
			if document.CurrentVersionID == nil || *document.CurrentVersionID != *input.ExpectedCurrentVersionID {
				return domain.ErrVersionConflict
			}
		}
		resourceType, resourceID = permissiondomain.ResourceDocument, document.ID
	}
	result, err := s.authorizer.EvaluatePermission(ctx, permissiondomain.Actor{UserID: actor.UserID, SessionID: actor.SessionID, Role: actor.Role}, resourceType, resourceID, domain.ActionUpload, nil, false)
	if err != nil {
		return err
	}
	if !result.Allowed {
		return domain.ErrForbidden
	}
	return nil
}

func (s *Service) requireSessionAccess(actor domain.Actor, session domain.Session) error {
	if actor.UserID == session.UserID || actor.Role == "SYSTEM_ADMIN" {
		return nil
	}
	return domain.ErrForbidden
}

func validateCreateInput(input domain.CreateSessionInput) (domain.CreateSessionInput, int32, error) {
	input.FileName = strings.TrimSpace(input.FileName)
	if err := domain.ValidateIntent(input.UploadIntent); err != nil {
		return input, 0, err
	}
	if input.UploadIntent == domain.IntentCreate && input.TargetDocumentID != nil {
		return input, 0, &domain.ValidationError{Field: "targetDocumentId"}
	}
	if input.UploadIntent == domain.IntentNewVersion && input.TargetDocumentID == nil {
		return input, 0, &domain.ValidationError{Field: "targetDocumentId"}
	}
	if err := domain.ValidateFileName(input.FileName); err != nil {
		return input, 0, err
	}
	if len(input.DeclaredSHA256) != 0 && len(input.DeclaredSHA256) != 32 {
		return input, 0, &domain.ValidationError{Field: "declaredSha256Hex"}
	}
	if len(input.LockTokenHash) != 0 && len(input.LockTokenHash) != 32 {
		return input, 0, &domain.ValidationError{Field: "lockTokenHashHex"}
	}
	if input.ExpectedLockFencingToken != nil && *input.ExpectedLockFencingToken < 1 {
		return input, 0, &domain.ValidationError{Field: "expectedLockFencingToken"}
	}
	if err := domain.ValidateOptionalText("declaredMimeType", input.DeclaredMIMEType, 256); err != nil {
		return input, 0, err
	}
	if err := validateIdempotencyKey(input.IdempotencyKey); err != nil {
		return input, 0, err
	}
	count, err := domain.ExpectedPartCount(input.DeclaredSizeBytes, input.PartSizeBytes)
	return input, count, err
}

func validateIdempotencyKey(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > 128 {
		return &domain.ValidationError{Field: "Idempotency-Key"}
	}
	return nil
}

func claimIdempotency(ctx context.Context, repository Repository, actorID uuid.UUID, operation, key string, hash []byte, now time.Time) (*uuid.UUID, error) {
	recordID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	created, err := repository.TryCreateIdempotency(ctx, recordID, actorID, operation, key, hash, now.Add(24*time.Hour), now)
	if err != nil {
		return nil, err
	}
	if created {
		return nil, nil
	}
	record, err := repository.GetIdempotency(ctx, actorID, operation, key)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(record.RequestHash, hash) {
		return nil, domain.ErrIdempotencyConflict
	}
	if record.Status == "COMPLETED" && record.ResultResourceID != nil {
		return record.ResultResourceID, nil
	}
	return nil, domain.ErrConflict
}

func requestHash(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(encoded)
	return hash[:], nil
}

func temporaryObjectKey(now time.Time, spaceID, sessionID uuid.UUID) string {
	return fmt.Sprintf("tmp/uploads/%04d/%02d/%s/%s", now.Year(), int(now.Month()), spaceID.String(), sessionID.String())
}

func partContentBytes(session domain.Session, partNumber int32) int64 {
	if partNumber < session.ExpectedPartCount {
		return session.PartSizeBytes
	}
	previous := int64(partNumber-1) * session.PartSizeBytes
	remaining := session.DeclaredSizeBytes - previous
	if remaining < 0 {
		return 0
	}
	return remaining
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func mapStorageError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, objectstorage.ErrDisabled) {
		return domain.ErrStorageUnavailable
	}
	return fmt.Errorf("%w: %v", domain.ErrStorageUnavailable, err)
}

func insertEvent(ctx context.Context, repository Repository, session domain.Session, eventType string, requestID uuid.UUID, now time.Time) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"uploadSessionId": session.ID,
		"spaceId":         session.SpaceID,
		"folderId":        session.FolderID,
		"uploadIntent":    session.UploadIntent,
		"status":          session.Status,
	})
	if err != nil {
		return err
	}
	return repository.InsertEvent(ctx, domain.Event{
		ID:               id,
		AggregateType:    domain.ResourceUploadSession,
		AggregateID:      session.ID,
		AggregateVersion: session.RowVersion,
		Type:             eventType,
		Payload:          payload,
		DeduplicationKey: fmt.Sprintf("%s:%s:%d", eventType, session.ID, session.RowVersion),
		CorrelationID:    requestID,
		CreatedAt:        now,
	})
}
