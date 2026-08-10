package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	backgroundapplication "file-workshop/backend/internal/modules/background/application"
	backgrounddomain "file-workshop/backend/internal/modules/background/domain"
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
	if !errors.Is(err, domain.ErrPreservationHoldActive) {
		t.Fatalf("expected ErrPreservationHoldActive, got %v", err)
	}
}

func TestScanExpiredRecycleItemsEnqueuesPurgeJobsAndSkipsLegalHold(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 30, 0, 0, time.UTC)
	actor := testActor()
	actor.Role = domain.SystemRoleAdmin
	first := expiredRecycleItem(now, domain.EntryTypeDocument)
	second := expiredRecycleItem(now, domain.EntryTypeFolder)
	repository := newFakeRepository()
	repository.expiredRecycle = []domain.RecycleItem{first, second}
	repository.legalHoldByEntry = map[uuid.UUID]bool{second.EntryID: true}
	enqueuer := &fakeJobEnqueuer{}
	service := NewService(repository, repository, allowAuthorizer{}, func() time.Time { return now })
	service.SetJobEnqueuer(enqueuer)

	result, err := service.ScanExpiredRecycleItems(context.Background(), actor, domain.ExpiredScanInput{BatchSize: 2, RequestID: uuid.Must(uuid.NewV7())})
	if err != nil {
		t.Fatalf("ScanExpiredRecycleItems returned error: %v", err)
	}
	if result.Scanned != 2 || result.Enqueued != 1 || result.SkippedPreservationHold != 1 || result.JobType != domain.LifecyclePurgeJobType {
		t.Fatalf("unexpected scan result: %+v", result)
	}
	if len(enqueuer.commands) != 1 {
		t.Fatalf("expected one enqueue command, got %d", len(enqueuer.commands))
	}
	command := enqueuer.commands[0]
	if command.JobType != domain.LifecyclePurgeJobType || command.DeduplicationKey != domain.LifecyclePurgeJobType+":"+first.ID.String() || command.MaxAttempts != 10 || command.PayloadSchemaVersion != 1 {
		t.Fatalf("unexpected enqueue command: %+v", command)
	}
	if command.TargetDocumentID == nil || *command.TargetDocumentID != first.EntryID {
		t.Fatalf("expected document target id, got %+v", command.TargetDocumentID)
	}
	var payload map[string]any
	if err := json.Unmarshal(command.PayloadJSON, &payload); err != nil {
		t.Fatalf("invalid payload JSON: %v", err)
	}
	if payload["requestedBy"] != "expired-recycle-scan" || payload["entryType"] != domain.EntryTypeDocument {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestScanExpiredRecycleItemsRejectsNonAdmin(t *testing.T) {
	service := NewService(newFakeRepository(), newFakeRepository(), allowAuthorizer{}, time.Now)
	service.SetJobEnqueuer(&fakeJobEnqueuer{})
	_, err := service.ScanExpiredRecycleItems(context.Background(), testActor(), domain.ExpiredScanInput{})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestScanExpiredRecycleItemsRejectsInvalidBatchSize(t *testing.T) {
	actor := testActor()
	actor.Role = domain.SystemRoleAdmin
	repository := newFakeRepository()
	service := NewService(repository, repository, allowAuthorizer{}, time.Now)
	service.SetJobEnqueuer(&fakeJobEnqueuer{})
	_, err := service.ScanExpiredRecycleItems(context.Background(), actor, domain.ExpiredScanInput{BatchSize: domain.MaxExpiredScanBatchSize + 1})
	var validationError *domain.ValidationError
	if !errors.As(err, &validationError) || validationError.Field != "batchSize" {
		t.Fatalf("expected batchSize validation error, got %v", err)
	}
}

func TestPlacePreservationHoldCreatesHoldAndEvent(t *testing.T) {
	now := time.Date(2026, 8, 10, 13, 0, 0, 0, time.UTC)
	actor := testActor()
	actor.Role = domain.SystemRoleAdmin
	document := testDocumentRef()
	repository := newFakeRepository()
	repository.documents[document.ID] = document
	service := NewService(repository, repository, allowAuthorizer{}, func() time.Time { return now })

	hold, err := service.PlacePreservationHold(context.Background(), actor, document.ID, domain.PlacePreservationHoldInput{CaseReference: "QA-2026-0001", Reason: "质量追溯资料需要保全", IdempotencyKey: "preserve-001", RequestID: uuid.Must(uuid.NewV7())})
	if err != nil {
		t.Fatalf("PlacePreservationHold returned error: %v", err)
	}
	if hold.DocumentID != document.ID || hold.Status != domain.PreservationHoldStatusActive || hold.CaseReference != "QA-2026-0001" {
		t.Fatalf("unexpected preservation hold: %+v", hold)
	}
	if !repository.securityEpochTouched || len(repository.events) != 1 || repository.events[0].Type != "PRESERVATION_HOLD_PLACED" {
		t.Fatalf("expected security epoch touch and placement event, security=%v events=%+v", repository.securityEpochTouched, repository.events)
	}
}

func TestReleasePreservationHoldMarksReleasedAndWritesEvent(t *testing.T) {
	now := time.Date(2026, 8, 10, 14, 0, 0, 0, time.UTC)
	actor := testActor()
	actor.Role = domain.SystemRoleAdmin
	document := testDocumentRef()
	hold := testPreservationHold(document.ID, actor.UserID, now.Add(-time.Hour))
	repository := newFakeRepository()
	repository.documents[document.ID] = document
	repository.preservationHolds[hold.ID] = hold
	service := NewService(repository, repository, allowAuthorizer{}, func() time.Time { return now })

	result, err := service.ReleasePreservationHold(context.Background(), actor, hold.ID, domain.ReleasePreservationHoldInput{Reason: "质量追溯已关闭", RowVersion: hold.RowVersion, RequestID: uuid.Must(uuid.NewV7())})
	if err != nil {
		t.Fatalf("ReleasePreservationHold returned error: %v", err)
	}
	if result.Status != domain.PreservationHoldStatusReleased || result.ReleasedByUserID == nil || *result.ReleasedByUserID != actor.UserID || result.ReleaseReason == nil || *result.ReleaseReason != "质量追溯已关闭" {
		t.Fatalf("unexpected released hold: %+v", result)
	}
	if len(repository.events) != 1 || repository.events[0].Type != "PRESERVATION_HOLD_RELEASED" {
		t.Fatalf("expected release event, got %+v", repository.events)
	}
}

func TestPreservationHoldRequiresSystemAdmin(t *testing.T) {
	service := NewService(newFakeRepository(), newFakeRepository(), allowAuthorizer{}, time.Now)
	_, err := service.ListPreservationHolds(context.Background(), testActor(), domain.PreservationHoldListFilter{})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

type fakeRepository struct {
	entries              map[uuid.UUID]domain.Entry
	recycle              map[uuid.UUID]domain.RecycleItem
	expiredRecycle       []domain.RecycleItem
	documents            map[uuid.UUID]domain.DocumentRef
	preservationHolds    map[uuid.UUID]domain.PreservationHold
	idempotency          map[string]domain.IdempotencyRecord
	events               []domain.Event
	nameExists           bool
	legalHold            bool
	legalHoldByEntry     map[uuid.UUID]bool
	sharesUnavailable    bool
	securityEpochTouched bool
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{entries: map[uuid.UUID]domain.Entry{}, recycle: map[uuid.UUID]domain.RecycleItem{}, documents: map[uuid.UUID]domain.DocumentRef{}, preservationHolds: map[uuid.UUID]domain.PreservationHold{}, idempotency: map[string]domain.IdempotencyRecord{}}
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

func (r *fakeRepository) ActiveLegalHoldExists(_ context.Context, rootID uuid.UUID) (bool, error) {
	if r.legalHoldByEntry != nil {
		return r.legalHoldByEntry[rootID], nil
	}
	return r.legalHold, nil
}

func (r *fakeRepository) GetDocumentRef(_ context.Context, documentID uuid.UUID) (domain.DocumentRef, error) {
	document, ok := r.documents[documentID]
	if !ok {
		return domain.DocumentRef{}, domain.ErrNotFound
	}
	return document, nil
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

func (r *fakeRepository) ListExpiredActiveRecycleItems(context.Context, time.Time, int) ([]domain.RecycleItem, error) {
	return append([]domain.RecycleItem(nil), r.expiredRecycle...), nil
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

func (r *fakeRepository) InsertPreservationHold(_ context.Context, hold domain.PreservationHold) (domain.PreservationHold, error) {
	hold.Status = domain.PreservationHoldStatusActive
	hold.RowVersion = 1
	hold.CreatedAt = hold.PlacedAt
	hold.UpdatedAt = hold.PlacedAt
	r.preservationHolds[hold.ID] = hold
	return hold, nil
}

func (r *fakeRepository) GetPreservationHold(_ context.Context, id uuid.UUID) (domain.PreservationHold, error) {
	hold, ok := r.preservationHolds[id]
	if !ok {
		return domain.PreservationHold{}, domain.ErrNotFound
	}
	return hold, nil
}

func (r *fakeRepository) GetPreservationHoldForUpdate(ctx context.Context, id uuid.UUID) (domain.PreservationHold, error) {
	return r.GetPreservationHold(ctx, id)
}

func (r *fakeRepository) CountPreservationHolds(context.Context, domain.PreservationHoldListFilter) (int64, error) {
	return int64(len(r.preservationHolds)), nil
}

func (r *fakeRepository) ListPreservationHolds(context.Context, domain.PreservationHoldListFilter) ([]domain.PreservationHold, error) {
	items := make([]domain.PreservationHold, 0, len(r.preservationHolds))
	for _, hold := range r.preservationHolds {
		items = append(items, hold)
	}
	return items, nil
}

func (r *fakeRepository) ReleasePreservationHold(_ context.Context, id uuid.UUID, releasedBy uuid.UUID, reason string, rowVersion int64, at time.Time) (domain.PreservationHold, error) {
	hold, ok := r.preservationHolds[id]
	if !ok {
		return domain.PreservationHold{}, domain.ErrNotFound
	}
	if hold.RowVersion != rowVersion || hold.Status != domain.PreservationHoldStatusActive {
		return domain.PreservationHold{}, domain.ErrVersionConflict
	}
	hold.Status = domain.PreservationHoldStatusReleased
	hold.ReleasedByUserID = &releasedBy
	hold.ReleasedAt = &at
	hold.ReleaseReason = &reason
	hold.UpdatedAt = at
	hold.RowVersion++
	r.preservationHolds[id] = hold
	return hold, nil
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

func testDocumentRef() domain.DocumentRef {
	return domain.DocumentRef{ID: uuid.Must(uuid.NewV7()), SpaceID: uuid.Must(uuid.NewV7()), Name: "检验报告.pdf", LifecycleStatus: domain.LifecycleActive}
}

func testPreservationHold(documentID, placedBy uuid.UUID, at time.Time) domain.PreservationHold {
	return domain.PreservationHold{ID: uuid.Must(uuid.NewV7()), DocumentID: documentID, CaseReference: "QA-2026-0001", Reason: "质量追溯资料需要保全", Status: domain.PreservationHoldStatusActive, PlacedByUserID: placedBy, PlacedAt: at, CreatedAt: at, UpdatedAt: at, RowVersion: 1}
}

func expiredRecycleItem(now time.Time, entryType string) domain.RecycleItem {
	entryID := uuid.Must(uuid.NewV7())
	parentID := uuid.Must(uuid.NewV7())
	return domain.RecycleItem{
		ID:                     uuid.Must(uuid.NewV7()),
		EntryID:                entryID,
		EntryType:              entryType,
		OriginalSpaceID:        uuid.Must(uuid.NewV7()),
		OriginalParentFolderID: &parentID,
		OriginalName:           "expired.dat",
		CurrentName:            "expired.dat",
		LifecycleStatus:        domain.LifecycleTrashed,
		DeletedByUserID:        uuid.Must(uuid.NewV7()),
		DeletedAt:              now.Add(-domain.DefaultRecycleRetention),
		ExpiresAt:              now.Add(-time.Minute),
		Status:                 domain.RecycleStatusActive,
		CreatedAt:              now.Add(-domain.DefaultRecycleRetention),
		UpdatedAt:              now.Add(-domain.DefaultRecycleRetention),
		RowVersion:             1,
	}
}

type fakeJobEnqueuer struct {
	commands []backgroundapplication.EnqueueJobCommand
}

func (e *fakeJobEnqueuer) EnqueueJob(_ context.Context, command backgroundapplication.EnqueueJobCommand) (backgrounddomain.BackgroundJob, error) {
	e.commands = append(e.commands, command)
	return backgrounddomain.BackgroundJob{ID: uuid.Must(uuid.NewV7()), JobType: command.JobType, PayloadSchemaVersion: command.PayloadSchemaVersion, PayloadJSON: command.PayloadJSON, DeduplicationKey: command.DeduplicationKey, Priority: command.Priority, Status: backgrounddomain.JobStatusPending, MaxAttempts: command.MaxAttempts, AvailableAt: command.AvailableAt, RowVersion: 1}, nil
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
