package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"file-workshop/backend/internal/modules/lifecycle/domain"
	permissiondomain "file-workshop/backend/internal/modules/permissions/domain"

	"github.com/google/uuid"
)

func TestTrashEntryCreatesRecycleItemAndInvalidatesShares(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	actor := testActor()
	parentID := uuid.Must(uuid.NewV7())
	entry := testEntry(parentID)
	repository := newFakeRepository()
	repository.entries[entry.ID] = entry
	service := NewService(repository, repository, allowAuthorizer{}, func() time.Time { return now })

	item, err := service.TrashEntry(context.Background(), actor, entry.ID, domain.TrashInput{
		RowVersion:     entry.RowVersion,
		IdempotencyKey: "trash-entry-001",
		RequestID:      uuid.Must(uuid.NewV7()),
	})
	if err != nil {
		t.Fatalf("TrashEntry returned error: %v", err)
	}
	if item.EntryID != entry.ID || item.Status != domain.RecycleStatusActive {
		t.Fatalf("unexpected recycle item: %+v", item)
	}
	if item.ExpiresAt.Sub(item.DeletedAt) != domain.DefaultRecycleRetention {
		t.Fatalf("unexpected recycle retention: %s", item.ExpiresAt.Sub(item.DeletedAt))
	}
	if repository.entries[entry.ID].LifecycleStatus != domain.LifecycleTrashed {
		t.Fatalf("entry lifecycle was not trashed: %+v", repository.entries[entry.ID])
	}
	if !repository.sharesUnavailable || !repository.securityEpochTouched || len(repository.events) != 1 {
		t.Fatalf("expected shares/security/event side effects, got shares=%v security=%v events=%d", repository.sharesUnavailable, repository.securityEpochTouched, len(repository.events))
	}
}

func TestTrashEntryRejectsRootFolder(t *testing.T) {
	entry := testEntry(uuid.Nil)
	entry.IsRoot = true
	repository := newFakeRepository()
	repository.entries[entry.ID] = entry
	service := NewService(repository, repository, allowAuthorizer{}, time.Now)

	_, err := service.TrashEntry(context.Background(), testActor(), entry.ID, domain.TrashInput{RowVersion: entry.RowVersion, IdempotencyKey: "trash-root", RequestID: uuid.Must(uuid.NewV7())})
	if !errors.Is(err, domain.ErrRootOperation) {
		t.Fatalf("expected ErrRootOperation, got %v", err)
	}
}

func TestRestoreRecycleItemRejectsNameConflict(t *testing.T) {
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	parentID := uuid.Must(uuid.NewV7())
	entry := testEntry(parentID)
	entry.LifecycleStatus = domain.LifecycleTrashed
	recycleID := uuid.Must(uuid.NewV7())
	repository := newFakeRepository()
	repository.entries[entry.ID] = entry
	repository.entries[parentID] = domain.Entry{ID: parentID, SpaceID: entry.SpaceID, EntryType: domain.EntryTypeFolder, Name: "parent", NormalizedName: "parent", PathCache: ptr("/parent"), Depth: 1, LifecycleStatus: domain.LifecycleActive, RowVersion: 1}
	repository.recycle[recycleID] = domain.RecycleItem{ID: recycleID, EntryID: entry.ID, EntryType: entry.EntryType, OriginalSpaceID: entry.SpaceID, OriginalParentFolderID: &parentID, OriginalName: entry.Name, CurrentName: entry.Name, LifecycleStatus: entry.LifecycleStatus, DeletedByUserID: uuid.Must(uuid.NewV7()), DeletedAt: now, ExpiresAt: now.Add(domain.DefaultRecycleRetention), Status: domain.RecycleStatusActive, RowVersion: 7}
	repository.nameExists = true
	service := NewService(repository, repository, allowAuthorizer{}, func() time.Time { return now })

	_, err := service.RestoreRecycleItem(context.Background(), testActor(), recycleID, domain.RestoreInput{RowVersion: 7, RequestID: uuid.Must(uuid.NewV7())})
	if !errors.Is(err, domain.ErrNameConflict) {
		t.Fatalf("expected ErrNameConflict, got %v", err)
	}
}

func TestPurgeRecycleItemRejectsActiveLegalHold(t *testing.T) {
	now := time.Date(2026, 8, 10, 11, 0, 0, 0, time.UTC)
	parentID := uuid.Must(uuid.NewV7())
	entry := testEntry(parentID)
	entry.LifecycleStatus = domain.LifecycleTrashed
	recycleID := uuid.Must(uuid.NewV7())
	repository := newFakeRepository()
	repository.entries[entry.ID] = entry
	repository.recycle[recycleID] = domain.RecycleItem{ID: recycleID, EntryID: entry.ID, EntryType: entry.EntryType, OriginalSpaceID: entry.SpaceID, OriginalParentFolderID: &parentID, OriginalName: entry.Name, CurrentName: entry.Name, LifecycleStatus: entry.LifecycleStatus, DeletedByUserID: uuid.Must(uuid.NewV7()), DeletedAt: now, ExpiresAt: now.Add(domain.DefaultRecycleRetention), Status: domain.RecycleStatusActive, RowVersion: 4}
	repository.legalHold = true
	service := NewService(repository, repository, allowAuthorizer{}, func() time.Time { return now })

	_, err := service.PurgeRecycleItem(context.Background(), testActor(), recycleID, domain.PurgeInput{Reason: "管理员确认清理", RowVersion: 4, RequestID: uuid.Must(uuid.NewV7())})
	if !errors.Is(err, domain.ErrLegalHoldActive) {
		t.Fatalf("expected ErrLegalHoldActive, got %v", err)
	}
}

type fakeRepository struct {
	entries              map[uuid.UUID]domain.Entry
	recycle              map[uuid.UUID]domain.RecycleItem
	idempotency          map[string]domain.IdempotencyRecord
	events               []domain.Event
	nameExists           bool
	legalHold            bool
	sharesUnavailable    bool
	securityEpochTouched bool
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{entries: map[uuid.UUID]domain.Entry{}, recycle: map[uuid.UUID]domain.RecycleItem{}, idempotency: map[string]domain.IdempotencyRecord{}}
}

func (r *fakeRepository) WithinTransaction(_ context.Context, operation func(Repository) error) error {
	return operation(r)
}

func (r *fakeRepository) GetEntryForUpdate(_ context.Context, entryID uuid.UUID) (domain.Entry, error) {
	entry, ok := r.entries[entryID]
	if !ok {
		return domain.Entry{}, domain.ErrNotFound
	}
	return entry, nil
}

func (r *fakeRepository) GetFolderForUpdate(ctx context.Context, folderID uuid.UUID) (domain.Entry, error) {
	return r.GetEntryForUpdate(ctx, folderID)
}

func (r *fakeRepository) NameExists(context.Context, uuid.UUID, *uuid.UUID, string, uuid.UUID) (bool, error) {
	return r.nameExists, nil
}

func (r *fakeRepository) TrashEntrySubtree(_ context.Context, rootID uuid.UUID, at time.Time) (int64, error) {
	entry := r.entries[rootID]
	entry.LifecycleStatus = domain.LifecycleTrashed
	entry.UpdatedAt = at
	entry.RowVersion++
	r.entries[rootID] = entry
	return 1, nil
}

func (r *fakeRepository) MoveRestoreRoot(_ context.Context, entryID uuid.UUID, parentFolderID *uuid.UUID, name, normalizedName string, path *string, depth int32, at time.Time) (domain.Entry, error) {
	entry := r.entries[entryID]
	entry.ParentFolderID = parentFolderID
	entry.Name = name
	entry.NormalizedName = normalizedName
	entry.PathCache = path
	entry.Depth = depth
	entry.UpdatedAt = at
	entry.RowVersion++
	r.entries[entryID] = entry
	return entry, nil
}

func (r *fakeRepository) RestoreEntrySubtree(_ context.Context, rootID uuid.UUID, at time.Time) (int64, error) {
	entry := r.entries[rootID]
	entry.LifecycleStatus = domain.LifecycleActive
	entry.UpdatedAt = at
	entry.RowVersion++
	r.entries[rootID] = entry
	return 1, nil
}

func (r *fakeRepository) UpdateDescendantPaths(context.Context, uuid.UUID, string, int32, time.Time) error {
	return nil
}

func (r *fakeRepository) MarkEntrySubtreePurging(_ context.Context, rootID uuid.UUID, at time.Time) (int64, error) {
	entry := r.entries[rootID]
	entry.LifecycleStatus = domain.LifecyclePurging
	entry.DeletedAt = &at
	entry.UpdatedAt = at
	entry.RowVersion++
	r.entries[rootID] = entry
	return 1, nil
}

func (r *fakeRepository) MarkSharesSourceUnavailable(context.Context, uuid.UUID, time.Time) error {
	r.sharesUnavailable = true
	return nil
}

func (r *fakeRepository) TouchSpaceSecurityEpoch(context.Context, uuid.UUID, time.Time) error {
	r.securityEpochTouched = true
	return nil
}

func (r *fakeRepository) ActiveLegalHoldExists(context.Context, uuid.UUID) (bool, error) {
	return r.legalHold, nil
}

func (r *fakeRepository) InsertRecycleItem(_ context.Context, recycleID uuid.UUID, entry domain.Entry, deletedBy uuid.UUID, deletedAt, expiresAt time.Time) (domain.RecycleItem, error) {
	item := domain.RecycleItem{ID: recycleID, EntryID: entry.ID, EntryType: entry.EntryType, OriginalSpaceID: entry.SpaceID, OriginalParentFolderID: entry.ParentFolderID, OriginalName: entry.Name, CurrentName: entry.Name, LifecycleStatus: domain.LifecycleTrashed, DeletedByUserID: deletedBy, DeletedAt: deletedAt, ExpiresAt: expiresAt, Status: domain.RecycleStatusActive, CreatedAt: deletedAt, UpdatedAt: deletedAt, RowVersion: 1}
	r.recycle[recycleID] = item
	return item, nil
}

func (r *fakeRepository) GetRecycleItem(_ context.Context, recycleID uuid.UUID) (domain.RecycleItem, error) {
	item, ok := r.recycle[recycleID]
	if !ok {
		return domain.RecycleItem{}, domain.ErrNotFound
	}
	return item, nil
}

func (r *fakeRepository) GetRecycleItemForUpdate(ctx context.Context, recycleID uuid.UUID) (domain.RecycleItem, error) {
	return r.GetRecycleItem(ctx, recycleID)
}

func (r *fakeRepository) CountRecycleItems(context.Context, *uuid.UUID) (int64, error) {
	return int64(len(r.recycle)), nil
}

func (r *fakeRepository) ListRecycleItems(context.Context, *uuid.UUID, int, int) ([]domain.RecycleItem, error) {
	items := make([]domain.RecycleItem, 0, len(r.recycle))
	for _, item := range r.recycle {
		items = append(items, item)
	}
	return items, nil
}

func (r *fakeRepository) RestoreRecycleItem(_ context.Context, recycleID uuid.UUID, restoredToFolderID uuid.UUID, rowVersion int64, at time.Time) (domain.RecycleItem, error) {
	item := r.recycle[recycleID]
	if item.RowVersion != rowVersion {
		return domain.RecycleItem{}, domain.ErrVersionConflict
	}
	item.Status = domain.RecycleStatusRestored
	item.RestoredToFolderID = &restoredToFolderID
	item.RestoredAt = &at
	item.UpdatedAt = at
	item.RowVersion++
	r.recycle[recycleID] = item
	return item, nil
}

func (r *fakeRepository) MarkRecycleItemPurging(_ context.Context, recycleID uuid.UUID, rowVersion int64, at time.Time) (domain.RecycleItem, error) {
	item := r.recycle[recycleID]
	if item.RowVersion != rowVersion {
		return domain.RecycleItem{}, domain.ErrVersionConflict
	}
	item.Status = domain.RecycleStatusPurging
	item.UpdatedAt = at
	item.RowVersion++
	r.recycle[recycleID] = item
	return item, nil
}

func (r *fakeRepository) TryCreateIdempotency(_ context.Context, _ uuid.UUID, actorID uuid.UUID, operation, key string, hash []byte, _ time.Time, _ time.Time) (bool, error) {
	storageKey := actorID.String() + ":" + operation + ":" + key
	if _, ok := r.idempotency[storageKey]; ok {
		return false, nil
	}
	r.idempotency[storageKey] = domain.IdempotencyRecord{RequestHash: hash, Status: "PROCESSING"}
	return true, nil
}

func (r *fakeRepository) GetIdempotency(_ context.Context, actorID uuid.UUID, operation, key string) (domain.IdempotencyRecord, error) {
	record, ok := r.idempotency[actorID.String()+":"+operation+":"+key]
	if !ok {
		return domain.IdempotencyRecord{}, domain.ErrConflict
	}
	return record, nil
}

func (r *fakeRepository) CompleteIdempotency(_ context.Context, actorID uuid.UUID, operation, key string, resourceID uuid.UUID, _ string, _ time.Time) error {
	storageKey := actorID.String() + ":" + operation + ":" + key
	record := r.idempotency[storageKey]
	record.Status = "COMPLETED"
	record.ResultResourceID = &resourceID
	r.idempotency[storageKey] = record
	return nil
}

func (r *fakeRepository) InsertEvent(_ context.Context, event domain.Event) error {
	r.events = append(r.events, event)
	return nil
}

type allowAuthorizer struct{}

func (allowAuthorizer) EvaluatePermission(context.Context, permissiondomain.Actor, string, uuid.UUID, string, *string, bool) (permissiondomain.PermissionEvaluation, error) {
	return permissiondomain.PermissionEvaluation{Allowed: true}, nil
}

func testActor() domain.Actor {
	return domain.Actor{UserID: uuid.Must(uuid.NewV7()), SessionID: uuid.Must(uuid.NewV7()), Role: "USER"}
}

func testEntry(parentID uuid.UUID) domain.Entry {
	entryID := uuid.Must(uuid.NewV7())
	spaceID := uuid.Must(uuid.NewV7())
	var parent *uuid.UUID
	if parentID != uuid.Nil {
		parent = &parentID
	}
	return domain.Entry{ID: entryID, SpaceID: spaceID, ParentFolderID: parent, EntryType: domain.EntryTypeDocument, Name: "设计说明.docx", NormalizedName: "设计说明.docx", PathCache: ptr("/设计说明.docx"), Depth: 1, LifecycleStatus: domain.LifecycleActive, CreatedByUserID: uuid.Must(uuid.NewV7()), RowVersion: 3}
}

func ptr(value string) *string {
	return &value
}

func TestTrashEntryIdempotencyRejectsDifferentBody(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	actor := testActor()
	parentID := uuid.Must(uuid.NewV7())
	entry := testEntry(parentID)
	repository := newFakeRepository()
	repository.entries[entry.ID] = entry
	service := NewService(repository, repository, allowAuthorizer{}, func() time.Time { return now })
	key := "trash-entry-reused-key"

	_, err := service.TrashEntry(context.Background(), actor, entry.ID, domain.TrashInput{RowVersion: entry.RowVersion, IdempotencyKey: key, RequestID: uuid.Must(uuid.NewV7())})
	if err != nil {
		t.Fatalf("first TrashEntry returned error: %v", err)
	}
	record, err := repository.GetIdempotency(context.Background(), actor.UserID, trashOperation, key)
	if err != nil {
		t.Fatalf("expected idempotency record: %v", err)
	}
	record.Status = "PROCESSING"
	repository.idempotency[actor.UserID.String()+":"+trashOperation+":"+key] = record

	_, err = service.TrashEntry(context.Background(), actor, entry.ID, domain.TrashInput{RowVersion: entry.RowVersion + 1, IdempotencyKey: key, RequestID: uuid.Must(uuid.NewV7())})
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("expected ErrIdempotencyConflict, got %v", err)
	}
}
