package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	backgroundapplication "file-workshop/backend/internal/modules/background/application"
	backgrounddomain "file-workshop/backend/internal/modules/background/domain"
	"file-workshop/backend/internal/modules/lifecycle/domain"
	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"

	"github.com/google/uuid"
)

const (
	trashOperation = "TRASH_DIRECTORY_ENTRY"
	idempotencyTTL = 24 * time.Hour
)

type Authorizer interface {
	EvaluatePermission(context.Context, permissiondomain.Actor, string, uuid.UUID, string, *string, bool) (permissiondomain.PermissionEvaluation, error)
}

type JobEnqueuer interface {
	EnqueueJob(context.Context, backgroundapplication.EnqueueJobCommand) (backgrounddomain.BackgroundJob, error)
}

type Service struct {
	repository  Repository
	transactor  Transactor
	authorizer  Authorizer
	jobEnqueuer JobEnqueuer
	now         func() time.Time
}

func NewService(repository Repository, transactor Transactor, authorizer Authorizer, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, transactor: transactor, authorizer: authorizer, now: now}
}

func (s *Service) SetJobEnqueuer(enqueuer JobEnqueuer) {
	s.jobEnqueuer = enqueuer
}

func (s *Service) TrashEntry(ctx context.Context, actor domain.Actor, entryID uuid.UUID, input domain.TrashInput) (domain.RecycleItem, error) {
	if input.RowVersion < 1 {
		return domain.RecycleItem{}, &domain.ValidationError{Field: "rowVersion"}
	}
	if err := validateIdempotencyKey(input.IdempotencyKey); err != nil {
		return domain.RecycleItem{}, err
	}
	now := s.now().UTC()
	hash, err := requestHash(struct {
		EntryID    uuid.UUID
		RowVersion int64
		Reason     *string
	}{entryID, input.RowVersion, input.Reason})
	if err != nil {
		return domain.RecycleItem{}, err
	}
	var result domain.RecycleItem
	err = s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		replayID, err := claimIdempotency(ctx, repository, actor.UserID, trashOperation, input.IdempotencyKey, hash, now)
		if err != nil {
			return err
		}
		if replayID != nil {
			result, err = repository.GetRecycleItem(ctx, *replayID)
			return err
		}
		entry, err := repository.GetEntryForUpdate(ctx, entryID)
		if err != nil {
			return err
		}
		if entry.IsRoot {
			return domain.ErrRootOperation
		}
		if entry.RowVersion != input.RowVersion || entry.LifecycleStatus != domain.LifecycleActive && entry.LifecycleStatus != domain.LifecycleArchived {
			return domain.ErrVersionConflict
		}
		if err = s.requirePermission(ctx, actor, entry.EntryType, entry.ID, domain.ActionDelete); err != nil {
			return err
		}
		recycleID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		result, err = repository.InsertRecycleItem(ctx, recycleID, entry, actor.UserID, now, now.Add(domain.DefaultRecycleRetention))
		if err != nil {
			return err
		}
		if _, err = repository.TrashEntrySubtree(ctx, entry.ID, now); err != nil {
			return err
		}
		if err = repository.MarkSharesSourceUnavailable(ctx, entry.ID, now); err != nil {
			return err
		}
		if err = repository.TouchSpaceSecurityEpoch(ctx, entry.SpaceID, now); err != nil {
			return err
		}
		if err = insertEvent(ctx, repository, "ENTRY_TRASHED", result, input.RequestID, now); err != nil {
			return err
		}
		return repository.CompleteIdempotency(ctx, actor.UserID, trashOperation, input.IdempotencyKey, result.ID, domain.ResourceRecycleItem, now)
	})
	return result, err
}

func (s *Service) ListRecycleItems(ctx context.Context, actor domain.Actor, filter domain.ListFilter) (domain.ListResult, error) {
	page, pageSize, err := domain.NormalizePage(filter.Page, filter.PageSize)
	if err != nil {
		return domain.ListResult{}, err
	}
	items, err := s.repository.ListRecycleItems(ctx, filter.SpaceID, page, pageSize)
	if err != nil {
		return domain.ListResult{}, err
	}
	visible := items[:0]
	for _, item := range items {
		if err = s.requirePermission(ctx, actor, item.EntryType, item.EntryID, domain.ActionRestore); err == nil {
			visible = append(visible, item)
		}
	}
	return domain.ListResult{Items: visible, Page: page, PageSize: pageSize, Total: int64(len(visible))}, nil
}

func (s *Service) RestoreRecycleItem(ctx context.Context, actor domain.Actor, recycleItemID uuid.UUID, input domain.RestoreInput) (domain.RecycleItem, error) {
	if input.RowVersion < 1 {
		return domain.RecycleItem{}, &domain.ValidationError{Field: "rowVersion"}
	}
	now := s.now().UTC()
	var result domain.RecycleItem
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		item, err := repository.GetRecycleItemForUpdate(ctx, recycleItemID)
		if err != nil {
			return err
		}
		if item.RowVersion != input.RowVersion || item.Status != domain.RecycleStatusActive {
			return domain.ErrVersionConflict
		}
		entry, err := repository.GetEntryForUpdate(ctx, item.EntryID)
		if err != nil {
			return err
		}
		if entry.LifecycleStatus != domain.LifecycleTrashed {
			return domain.ErrConflict
		}
		if err = s.requirePermission(ctx, actor, entry.EntryType, entry.ID, domain.ActionRestore); err != nil {
			return err
		}
		targetParentID := item.OriginalParentFolderID
		if input.TargetParentFolderID != nil {
			targetParentID = input.TargetParentFolderID
		}
		if targetParentID == nil {
			return domain.ErrNotFound
		}
		parent, err := repository.GetFolderForUpdate(ctx, *targetParentID)
		if err != nil {
			return err
		}
		if parent.SpaceID != item.OriginalSpaceID || parent.LifecycleStatus != domain.LifecycleActive {
			return domain.ErrNotFound
		}
		if err = s.requirePermission(ctx, actor, parent.EntryType, parent.ID, targetCreateAction(entry.EntryType)); err != nil {
			return err
		}
		name := item.OriginalName
		if input.Name != nil {
			name = strings.TrimSpace(*input.Name)
		}
		if err = domain.ValidateEntryName(name); err != nil {
			return err
		}
		normalized := domain.NormalizeName(name)
		exists, err := repository.NameExists(ctx, parent.SpaceID, &parent.ID, normalized, entry.ID)
		if err != nil {
			return err
		}
		if exists {
			return domain.ErrNameConflict
		}
		rootPath := childPath(parent.PathCache, name)
		root, err := repository.MoveRestoreRoot(ctx, entry.ID, &parent.ID, name, normalized, &rootPath, parent.Depth+1, now)
		if err != nil {
			return err
		}
		if _, err = repository.RestoreEntrySubtree(ctx, entry.ID, now); err != nil {
			return err
		}
		if root.EntryType == domain.EntryTypeFolder {
			if err = repository.UpdateDescendantPaths(ctx, root.ID, stringValue(root.PathCache), root.Depth, now); err != nil {
				return err
			}
		}
		result, err = repository.RestoreRecycleItem(ctx, recycleItemID, parent.ID, input.RowVersion, now)
		if err != nil {
			return err
		}
		if err = repository.TouchSpaceSecurityEpoch(ctx, item.OriginalSpaceID, now); err != nil {
			return err
		}
		return insertEvent(ctx, repository, "ENTRY_RESTORED", result, input.RequestID, now)
	})
	return result, err
}

func (s *Service) PurgeRecycleItem(ctx context.Context, actor domain.Actor, recycleItemID uuid.UUID, input domain.PurgeInput) (domain.RecycleItem, error) {
	if input.RowVersion < 1 {
		return domain.RecycleItem{}, &domain.ValidationError{Field: "rowVersion"}
	}
	if err := domain.ValidateReason(input.Reason); err != nil {
		return domain.RecycleItem{}, err
	}
	now := s.now().UTC()
	var result domain.RecycleItem
	err := s.transactor.WithinTransaction(ctx, func(repository Repository) error {
		item, err := repository.GetRecycleItemForUpdate(ctx, recycleItemID)
		if err != nil {
			return err
		}
		if item.RowVersion != input.RowVersion || item.Status != domain.RecycleStatusActive {
			return domain.ErrVersionConflict
		}
		entry, err := repository.GetEntryForUpdate(ctx, item.EntryID)
		if err != nil {
			return err
		}
		if err = s.requirePermission(ctx, actor, entry.EntryType, entry.ID, domain.ActionPurge); err != nil {
			return err
		}
		hold, err := repository.ActiveLegalHoldExists(ctx, entry.ID)
		if err != nil {
			return err
		}
		if hold {
			return domain.ErrLegalHoldActive
		}
		if _, err = repository.MarkEntrySubtreePurging(ctx, entry.ID, now); err != nil {
			return err
		}
		result, err = repository.MarkRecycleItemPurging(ctx, recycleItemID, input.RowVersion, now)
		if err != nil {
			return err
		}
		if err = repository.TouchSpaceSecurityEpoch(ctx, item.OriginalSpaceID, now); err != nil {
			return err
		}
		return insertEvent(ctx, repository, "ENTRY_PURGE_REQUESTED", result, input.RequestID, now)
	})
	return result, err
}

func (s *Service) ScanExpiredRecycleItems(ctx context.Context, actor domain.Actor, input domain.ExpiredScanInput) (domain.ExpiredScanResult, error) {
	if actor.Role != domain.SystemRoleAdmin {
		return domain.ExpiredScanResult{}, domain.ErrForbidden
	}
	if s.jobEnqueuer == nil {
		return domain.ExpiredScanResult{}, domain.ErrConflict
	}
	batchSize := input.BatchSize
	if batchSize == 0 {
		batchSize = domain.DefaultExpiredScanBatchSize
	}
	if batchSize < 1 || batchSize > domain.MaxExpiredScanBatchSize {
		return domain.ExpiredScanResult{}, &domain.ValidationError{Field: "batchSize"}
	}
	now := s.now().UTC()
	items, err := s.repository.ListExpiredActiveRecycleItems(ctx, now, batchSize)
	if err != nil {
		return domain.ExpiredScanResult{}, err
	}
	result := domain.ExpiredScanResult{Scanned: len(items), JobType: domain.LifecyclePurgeJobType}
	for _, item := range items {
		hold, err := s.repository.ActiveLegalHoldExists(ctx, item.EntryID)
		if err != nil {
			return domain.ExpiredScanResult{}, err
		}
		if hold {
			result.SkippedLegalHold++
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"recycleItemId": item.ID,
			"entryId":       item.EntryID,
			"entryType":     item.EntryType,
			"expiresAt":     item.ExpiresAt,
			"requestedBy":   "expired-recycle-scan",
			"requestId":     input.RequestID,
		})
		if err != nil {
			return domain.ExpiredScanResult{}, err
		}
		var documentID *uuid.UUID
		if item.EntryType == domain.EntryTypeDocument {
			documentID = &item.EntryID
		}
		if _, err = s.jobEnqueuer.EnqueueJob(ctx, backgroundapplication.EnqueueJobCommand{
			JobType:              domain.LifecyclePurgeJobType,
			TargetDocumentID:     documentID,
			PayloadSchemaVersion: 1,
			PayloadJSON:          payload,
			DeduplicationKey:     fmt.Sprintf("%s:%s", domain.LifecyclePurgeJobType, item.ID),
			Priority:             50,
			MaxAttempts:          10,
			AvailableAt:          now,
		}); err != nil {
			return domain.ExpiredScanResult{}, err
		}
		result.Enqueued++
	}
	return result, nil
}

func (s *Service) requirePermission(ctx context.Context, actor domain.Actor, resourceType string, resourceID uuid.UUID, action string) error {
	result, err := s.authorizer.EvaluatePermission(ctx, permissiondomain.Actor{UserID: actor.UserID, SessionID: actor.SessionID, Role: actor.Role}, resourceType, resourceID, action, nil, false)
	if err != nil {
		return err
	}
	if !result.Allowed {
		return domain.ErrForbidden
	}
	return nil
}

func targetCreateAction(entryType string) string {
	if entryType == domain.EntryTypeFolder {
		return domain.ActionCreateFolder
	}
	return domain.ActionUpload
}

func childPath(parentPath *string, name string) string {
	if parentPath == nil || *parentPath == "" {
		return "/" + name
	}
	return *parentPath + "/" + name
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
	created, err := repository.TryCreateIdempotency(ctx, recordID, actorID, operation, key, hash, now.Add(idempotencyTTL), now)
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

func insertEvent(ctx context.Context, repository Repository, eventType string, item domain.RecycleItem, requestID uuid.UUID, now time.Time) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"recycleItemId": item.ID, "entryId": item.EntryID, "entryType": item.EntryType, "status": item.Status})
	if err != nil {
		return err
	}
	return repository.InsertEvent(ctx, domain.Event{ID: id, AggregateType: domain.ResourceRecycleItem, AggregateID: item.ID, AggregateVersion: item.RowVersion, Type: eventType, Payload: payload, DeduplicationKey: fmt.Sprintf("%s:%s:%d", eventType, item.ID, item.RowVersion), CorrelationID: requestID, CreatedAt: now})
}
