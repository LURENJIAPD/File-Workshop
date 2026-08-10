package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"
	"file-workshop/backend/internal/modules/versions/domain"

	"github.com/google/uuid"
)

const restoreOperation = "RESTORE_DOCUMENT_VERSION"

type Authorizer interface {
	EvaluatePermission(context.Context, permissiondomain.Actor, string, uuid.UUID, string, *string, bool) (permissiondomain.PermissionEvaluation, error)
}

type Service struct {
	repository Repository
	transactor Transactor
	authorizer Authorizer
	now        func() time.Time
}

func NewService(repository Repository, transactor Transactor, authorizer Authorizer, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, transactor: transactor, authorizer: authorizer, now: now}
}

func (s *Service) ListVersions(ctx context.Context, actor domain.Actor, documentID uuid.UUID, page, pageSize int) (domain.VersionListResult, error) {
	page, pageSize, err := domain.NormalizePage(page, pageSize)
	if err != nil {
		return domain.VersionListResult{}, err
	}
	if err = s.requirePermission(ctx, actor, documentID, domain.ActionReadMetadata); err != nil {
		return domain.VersionListResult{}, err
	}
	total, err := s.repository.CountVersions(ctx, documentID)
	if err != nil {
		return domain.VersionListResult{}, err
	}
	items, err := s.repository.ListVersions(ctx, documentID, page, pageSize)
	if err != nil {
		return domain.VersionListResult{}, err
	}
	return domain.VersionListResult{Items: items, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *Service) RestoreVersion(ctx context.Context, actor domain.Actor, documentID, versionID uuid.UUID, input domain.RestoreVersionInput) (domain.Version, error) {
	if input.RowVersion < 1 {
		return domain.Version{}, &domain.ValidationError{Field: "rowVersion"}
	}
	if err := domain.ValidateChangeNote(input.ChangeNote); err != nil {
		return domain.Version{}, err
	}
	if err := validateIdempotencyKey(input.IdempotencyKey); err != nil {
		return domain.Version{}, err
	}
	if err := s.requirePermission(ctx, actor, documentID, domain.ActionManageVersion); err != nil {
		return domain.Version{}, err
	}
	now := s.now().UTC()
	hash, err := requestHash(struct {
		DocumentID uuid.UUID
		VersionID  uuid.UUID
		RowVersion int64
		ChangeNote *string
	}{documentID, versionID, input.RowVersion, input.ChangeNote})
	if err != nil {
		return domain.Version{}, err
	}
	var result domain.Version
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		replayID, err := claimIdempotency(ctx, repository, actor.UserID, restoreOperation, input.IdempotencyKey, hash, now)
		if err != nil {
			return err
		}
		if replayID != nil {
			result, err = repository.GetVersion(ctx, documentID, *replayID)
			return err
		}
		document, err := repository.GetDocumentForUpdate(ctx, documentID)
		if err != nil {
			return err
		}
		if document.RowVersion != input.RowVersion {
			return domain.ErrVersionConflict
		}
		if err := repository.ExpireLocks(ctx, documentID, now); err != nil {
			return err
		}
		if active, err := repository.GetActiveLockForUpdate(ctx, documentID); err != nil {
			return err
		} else if active != nil {
			return domain.ErrLocked
		}
		if _, err = repository.GetVersion(ctx, documentID, versionID); err != nil {
			return err
		}
		resultID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		result, err = repository.InsertRestoredVersion(ctx, resultID, documentID, versionID, actor.UserID, input.ChangeNote, now)
		if err != nil {
			return err
		}
		if err = repository.SetCurrentVersion(ctx, documentID, result.ID, input.RowVersion, now); err != nil {
			return err
		}
		if err = insertEvent(ctx, repository, "DOCUMENT_VERSION_RESTORED", domain.ResourceDocumentVersion, result.ID, result.VersionNumber, input.RequestID, now); err != nil {
			return err
		}
		return repository.CompleteIdempotency(ctx, actor.UserID, restoreOperation, input.IdempotencyKey, result.ID, domain.ResourceDocumentVersion, now)
	})
	return result, err
}

func (s *Service) GetLock(ctx context.Context, actor domain.Actor, documentID uuid.UUID) (*domain.Lock, error) {
	if err := s.requirePermission(ctx, actor, documentID, domain.ActionReadMetadata); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	_ = s.repository.ExpireLocks(ctx, documentID, now)
	return s.repository.GetActiveLock(ctx, documentID)
}

func (s *Service) AcquireLock(ctx context.Context, actor domain.Actor, documentID uuid.UUID, input domain.AcquireLockInput) (domain.AcquiredLock, error) {
	if err := domain.ValidateLockSource(input.Source); err != nil {
		return domain.AcquiredLock{}, err
	}
	if err := s.requirePermission(ctx, actor, documentID, domain.ActionLock); err != nil {
		return domain.AcquiredLock{}, err
	}
	now := s.now().UTC()
	ttl, err := normalizedTTL(input.TTLSeconds)
	if err != nil {
		return domain.AcquiredLock{}, err
	}
	token, hash, err := newLockToken()
	if err != nil {
		return domain.AcquiredLock{}, err
	}
	lockID, err := uuid.NewV7()
	if err != nil {
		return domain.AcquiredLock{}, err
	}
	var lock domain.Lock
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		if _, err := repository.GetDocumentForUpdate(ctx, documentID); err != nil {
			return err
		}
		if err := repository.ExpireLocks(ctx, documentID, now); err != nil {
			return err
		}
		if active, err := repository.GetActiveLockForUpdate(ctx, documentID); err != nil {
			return err
		} else if active != nil {
			return domain.ErrLocked
		}
		if err := repository.EnsureLockCounter(ctx, documentID, now); err != nil {
			return err
		}
		fencing, err := repository.IncrementLockCounter(ctx, documentID, now)
		if err != nil {
			return err
		}
		lock, err = repository.InsertLock(ctx, lockID, documentID, actor.UserID, hash, fencing, input.Source, now, now.Add(ttl))
		if err != nil {
			return err
		}
		return insertEvent(ctx, repository, "DOCUMENT_LOCKED", domain.ResourceDocumentLock, lock.ID, lock.RowVersion, input.RequestID, now)
	})
	if err != nil {
		return domain.AcquiredLock{}, err
	}
	return domain.AcquiredLock{Lock: lock, LockToken: token}, nil
}

func (s *Service) HeartbeatLock(ctx context.Context, actor domain.Actor, documentID uuid.UUID, input domain.HeartbeatLockInput) (domain.Lock, error) {
	if input.RowVersion < 1 {
		return domain.Lock{}, &domain.ValidationError{Field: "rowVersion"}
	}
	ttl, err := normalizedTTL(input.TTLSeconds)
	if err != nil {
		return domain.Lock{}, err
	}
	hash, err := tokenHash(input.LockToken)
	if err != nil {
		return domain.Lock{}, err
	}
	now := s.now().UTC()
	return s.repository.HeartbeatLock(ctx, documentID, hash, input.RowVersion, actor.UserID, now, now.Add(ttl))
}

func (s *Service) ReleaseLock(ctx context.Context, actor domain.Actor, documentID uuid.UUID, input domain.ReleaseLockInput) (domain.Lock, error) {
	if input.RowVersion < 1 {
		return domain.Lock{}, &domain.ValidationError{Field: "rowVersion"}
	}
	if err := domain.ValidateReason(input.Reason); err != nil {
		return domain.Lock{}, err
	}
	hash, err := tokenHash(input.LockToken)
	if err != nil {
		return domain.Lock{}, err
	}
	now := s.now().UTC()
	lock, err := s.repository.ReleaseLock(ctx, documentID, hash, input.RowVersion, actor.UserID, now, input.Reason)
	if err != nil {
		return domain.Lock{}, err
	}
	_ = insertEvent(ctx, s.repository, "DOCUMENT_UNLOCKED", domain.ResourceDocumentLock, lock.ID, lock.RowVersion, input.RequestID, now)
	return lock, nil
}

func (s *Service) ForceReleaseLock(ctx context.Context, actor domain.Actor, documentID uuid.UUID, input domain.ForceReleaseLockInput) (domain.Lock, error) {
	if input.RowVersion < 1 {
		return domain.Lock{}, &domain.ValidationError{Field: "rowVersion"}
	}
	reason := strings.TrimSpace(input.Reason)
	if err := domain.ValidateReason(&reason); err != nil {
		return domain.Lock{}, err
	}
	if actor.Role != domain.SystemRoleAdmin {
		return domain.Lock{}, domain.ErrForbidden
	}
	if err := s.requirePermission(ctx, actor, documentID, domain.ActionLock); err != nil {
		return domain.Lock{}, err
	}
	now := s.now().UTC()
	lock, err := s.repository.ForceReleaseLock(ctx, documentID, input.RowVersion, actor.UserID, now, reason)
	if err != nil {
		return domain.Lock{}, err
	}
	_ = insertEvent(ctx, s.repository, "DOCUMENT_FORCE_UNLOCKED", domain.ResourceDocumentLock, lock.ID, lock.RowVersion, input.RequestID, now)
	return lock, nil
}

func (s *Service) requirePermission(ctx context.Context, actor domain.Actor, documentID uuid.UUID, action string) error {
	if _, err := s.repository.GetDocumentContext(ctx, documentID); err != nil {
		return err
	}
	result, err := s.authorizer.EvaluatePermission(ctx, permissiondomain.Actor{UserID: actor.UserID, SessionID: actor.SessionID, Role: actor.Role}, permissiondomain.ResourceDocument, documentID, action, nil, false)
	if err != nil {
		return err
	}
	if !result.Allowed {
		return domain.ErrForbidden
	}
	return nil
}

func normalizedTTL(value *int) (time.Duration, error) {
	if value == nil {
		return domain.DefaultLockTTL, nil
	}
	ttl := time.Duration(*value) * time.Second
	if ttl < domain.MinLockTTL || ttl > domain.MaxLockTTL {
		return 0, &domain.ValidationError{Field: "ttlSeconds"}
	}
	return ttl, nil
}

func newLockToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash, err := tokenHash(token)
	return token, hash, err
}

func tokenHash(value string) ([]byte, error) {
	if strings.TrimSpace(value) == "" {
		return nil, &domain.ValidationError{Field: "lockToken"}
	}
	sum := sha256.Sum256([]byte(value))
	return sum[:], nil
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

func insertEvent(ctx context.Context, repository Repository, eventType, aggregateType string, aggregateID uuid.UUID, version int64, requestID uuid.UUID, now time.Time) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"resourceId": aggregateID, "eventType": eventType})
	if err != nil {
		return err
	}
	return repository.InsertEvent(ctx, domain.Event{ID: id, AggregateType: aggregateType, AggregateID: aggregateID, AggregateVersion: version, Type: eventType, Payload: payload, DeduplicationKey: fmt.Sprintf("%s:%s:%d", eventType, aggregateID, version), CorrelationID: requestID, CreatedAt: now})
}
