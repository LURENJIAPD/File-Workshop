package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/lifecycle/application"
	"file-workshop/backend/internal/modules/lifecycle/domain"
	"file-workshop/backend/internal/platform/database/dbgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgreSQL struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func NewPostgreSQL(pool *pgxpool.Pool) *PostgreSQL {
	return &PostgreSQL{pool: pool, queries: dbgen.New(pool)}
}

func (r *PostgreSQL) WithinTransaction(ctx context.Context, operation func(application.Repository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin lifecycle transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	repository := &PostgreSQL{pool: r.pool, queries: r.queries.WithTx(tx)}
	if err = operation(repository); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return mapDatabaseError(err)
	}
	return nil
}

func (r *PostgreSQL) GetEntryForUpdate(ctx context.Context, entryID uuid.UUID) (domain.Entry, error) {
	row, err := r.queries.GetLifecycleEntryForUpdate(ctx, pgUUID(entryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Entry{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Entry{}, mapDatabaseError(err)
	}
	return entryFromLifecycleRow(row), nil
}

func (r *PostgreSQL) GetFolderForUpdate(ctx context.Context, folderID uuid.UUID) (domain.Entry, error) {
	row, err := r.queries.GetLifecycleFolderForUpdate(ctx, pgUUID(folderID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Entry{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Entry{}, mapDatabaseError(err)
	}
	return entryFromFolderRow(row), nil
}

func (r *PostgreSQL) NameExists(ctx context.Context, spaceID uuid.UUID, parentFolderID *uuid.UUID, normalizedName string, excludeID uuid.UUID) (bool, error) {
	value, err := r.queries.LifecycleNameExists(ctx, &dbgen.LifecycleNameExistsParams{SpaceID: pgUUID(spaceID), Column2: optionalUUID(parentFolderID), NormalizedName: normalizedName, NamespaceEntryID: pgUUID(excludeID)})
	return value, mapDatabaseError(err)
}

func (r *PostgreSQL) TrashEntrySubtree(ctx context.Context, rootID uuid.UUID, at time.Time) (int64, error) {
	count, err := r.queries.TrashLifecycleEntrySubtree(ctx, &dbgen.TrashLifecycleEntrySubtreeParams{RootID: pgUUID(rootID), UpdatedAt: timestamptz(at)})
	return count, mapDatabaseError(err)
}

func (r *PostgreSQL) MoveRestoreRoot(ctx context.Context, entryID uuid.UUID, parentFolderID *uuid.UUID, name, normalizedName string, path *string, depth int32, at time.Time) (domain.Entry, error) {
	row, err := r.queries.MoveRestoreLifecycleRoot(ctx, &dbgen.MoveRestoreLifecycleRootParams{NamespaceEntryID: pgUUID(entryID), ParentFolderID: optionalUUID(parentFolderID), Name: name, NormalizedName: normalizedName, PathCache: optionalText(path), Depth: depth, UpdatedAt: timestamptz(at)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Entry{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.Entry{}, mapDatabaseError(err)
	}
	return entryFromNamespace(row), nil
}

func (r *PostgreSQL) RestoreEntrySubtree(ctx context.Context, rootID uuid.UUID, at time.Time) (int64, error) {
	count, err := r.queries.RestoreLifecycleEntrySubtree(ctx, &dbgen.RestoreLifecycleEntrySubtreeParams{UpdatedAt: timestamptz(at), RootID: pgUUID(rootID)})
	return count, mapDatabaseError(err)
}

func (r *PostgreSQL) UpdateDescendantPaths(ctx context.Context, rootID uuid.UUID, rootPath string, rootDepth int32, at time.Time) error {
	return mapDatabaseError(r.queries.UpdateLifecycleDescendantPaths(ctx, &dbgen.UpdateLifecycleDescendantPathsParams{RootPath: rootPath, RootDepth: rootDepth, RootID: pgUUID(rootID), UpdatedAt: timestamptz(at)}))
}

func (r *PostgreSQL) MarkEntrySubtreePurging(ctx context.Context, rootID uuid.UUID, at time.Time) (int64, error) {
	count, err := r.queries.MarkLifecycleEntrySubtreePurging(ctx, &dbgen.MarkLifecycleEntrySubtreePurgingParams{RootID: pgUUID(rootID), DeletedAt: timestamptz(at)})
	return count, mapDatabaseError(err)
}

func (r *PostgreSQL) MarkSharesSourceUnavailable(ctx context.Context, rootID uuid.UUID, at time.Time) error {
	return mapDatabaseError(r.queries.MarkSharesSourceUnavailableForEntrySubtree(ctx, &dbgen.MarkSharesSourceUnavailableForEntrySubtreeParams{RootID: pgUUID(rootID), UpdatedAt: timestamptz(at)}))
}

func (r *PostgreSQL) TouchSpaceSecurityEpoch(ctx context.Context, spaceID uuid.UUID, at time.Time) error {
	return mapDatabaseError(r.queries.TouchLifecycleSpaceSecurityEpoch(ctx, &dbgen.TouchLifecycleSpaceSecurityEpochParams{SpaceID: pgUUID(spaceID), UpdatedAt: timestamptz(at)}))
}

func (r *PostgreSQL) ActiveLegalHoldExists(ctx context.Context, rootID uuid.UUID) (bool, error) {
	value, err := r.queries.ActiveLegalHoldExistsForEntrySubtree(ctx, pgUUID(rootID))
	return value, mapDatabaseError(err)
}

func (r *PostgreSQL) InsertRecycleItem(ctx context.Context, recycleID uuid.UUID, entry domain.Entry, deletedBy uuid.UUID, deletedAt, expiresAt time.Time) (domain.RecycleItem, error) {
	row, err := r.queries.InsertRecycleItem(ctx, &dbgen.InsertRecycleItemParams{RecycleItemID: pgUUID(recycleID), NamespaceEntryID: pgUUID(entry.ID), OriginalSpaceID: pgUUID(entry.SpaceID), OriginalParentFolderID: optionalUUID(entry.ParentFolderID), OriginalName: entry.Name, DeletedByUserID: pgUUID(deletedBy), DeletedAt: timestamptz(deletedAt), ExpiresAt: timestamptz(expiresAt)})
	if err != nil {
		return domain.RecycleItem{}, mapDatabaseError(err)
	}
	return domain.RecycleItem{ID: uuidValue(row.RecycleItemID), EntryID: uuidValue(row.NamespaceEntryID), EntryType: entry.EntryType, OriginalSpaceID: uuidValue(row.OriginalSpaceID), OriginalParentFolderID: optionalGoogleUUID(row.OriginalParentFolderID), OriginalName: row.OriginalName, CurrentName: entry.Name, LifecycleStatus: domain.LifecycleTrashed, DeletedByUserID: uuidValue(row.DeletedByUserID), DeletedAt: row.DeletedAt.Time, ExpiresAt: row.ExpiresAt.Time, Status: row.Status, RestoredToFolderID: optionalGoogleUUID(row.RestoredToFolderID), RestoredAt: optionalTime(row.RestoredAt), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, RowVersion: row.RowVersion}, nil
}

func (r *PostgreSQL) GetRecycleItem(ctx context.Context, recycleID uuid.UUID) (domain.RecycleItem, error) {
	row, err := r.queries.GetRecycleItemWithEntry(ctx, pgUUID(recycleID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RecycleItem{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.RecycleItem{}, mapDatabaseError(err)
	}
	return recycleFromRow(row), nil
}

func (r *PostgreSQL) GetRecycleItemForUpdate(ctx context.Context, recycleID uuid.UUID) (domain.RecycleItem, error) {
	row, err := r.queries.GetRecycleItemWithEntryForUpdate(ctx, pgUUID(recycleID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RecycleItem{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.RecycleItem{}, mapDatabaseError(err)
	}
	return recycleFromRow(row), nil
}

func (r *PostgreSQL) CountRecycleItems(ctx context.Context, spaceID *uuid.UUID) (int64, error) {
	total, err := r.queries.CountRecycleItems(ctx, optionalUUID(spaceID))
	return total, mapDatabaseError(err)
}

func (r *PostgreSQL) ListRecycleItems(ctx context.Context, spaceID *uuid.UUID, page, pageSize int) ([]domain.RecycleItem, error) {
	rows, err := r.queries.ListRecycleItems(ctx, &dbgen.ListRecycleItemsParams{SpaceID: optionalUUID(spaceID), PageOffset: pageOffset(page, pageSize), PageSize: int32(pageSize)})
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	items := make([]domain.RecycleItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, recycleFromRow(row))
	}
	return items, nil
}

func (r *PostgreSQL) RestoreRecycleItem(ctx context.Context, recycleID uuid.UUID, restoredToFolderID uuid.UUID, rowVersion int64, at time.Time) (domain.RecycleItem, error) {
	if _, err := r.queries.RestoreRecycleItem(ctx, &dbgen.RestoreRecycleItemParams{RecycleItemID: pgUUID(recycleID), RestoredToFolderID: pgUUID(restoredToFolderID), RestoredAt: timestamptz(at), RowVersion: rowVersion}); errors.Is(err, pgx.ErrNoRows) {
		return domain.RecycleItem{}, domain.ErrVersionConflict
	} else if err != nil {
		return domain.RecycleItem{}, mapDatabaseError(err)
	}
	return r.GetRecycleItem(ctx, recycleID)
}

func (r *PostgreSQL) MarkRecycleItemPurging(ctx context.Context, recycleID uuid.UUID, rowVersion int64, at time.Time) (domain.RecycleItem, error) {
	if _, err := r.queries.MarkRecycleItemPurging(ctx, &dbgen.MarkRecycleItemPurgingParams{RecycleItemID: pgUUID(recycleID), RowVersion: rowVersion, UpdatedAt: timestamptz(at)}); errors.Is(err, pgx.ErrNoRows) {
		return domain.RecycleItem{}, domain.ErrVersionConflict
	} else if err != nil {
		return domain.RecycleItem{}, mapDatabaseError(err)
	}
	return r.GetRecycleItem(ctx, recycleID)
}

func (r *PostgreSQL) TryCreateIdempotency(ctx context.Context, recordID, actorID uuid.UUID, operation, key string, hash []byte, expiresAt, now time.Time) (bool, error) {
	rows, err := r.queries.TryCreateLifecycleIdempotency(ctx, &dbgen.TryCreateLifecycleIdempotencyParams{IdempotencyRecordID: pgUUID(recordID), UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key, RequestHash: hash, ExpiresAt: timestamptz(expiresAt), CreatedAt: timestamptz(now)})
	return rows == 1, mapDatabaseError(err)
}

func (r *PostgreSQL) GetIdempotency(ctx context.Context, actorID uuid.UUID, operation, key string) (domain.IdempotencyRecord, error) {
	row, err := r.queries.GetLifecycleIdempotency(ctx, &dbgen.GetLifecycleIdempotencyParams{UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.IdempotencyRecord{}, domain.ErrConflict
	}
	if err != nil {
		return domain.IdempotencyRecord{}, mapDatabaseError(err)
	}
	return domain.IdempotencyRecord{RequestHash: row.RequestHash, Status: row.Status, ResultResourceID: optionalGoogleUUID(row.ResultResourceID)}, nil
}

func (r *PostgreSQL) CompleteIdempotency(ctx context.Context, actorID uuid.UUID, operation, key string, resourceID uuid.UUID, resourceType string, at time.Time) error {
	return mapDatabaseError(r.queries.CompleteLifecycleIdempotency(ctx, &dbgen.CompleteLifecycleIdempotencyParams{UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key, ResultResourceID: pgUUID(resourceID), ResultResourceType: pgtype.Text{String: resourceType, Valid: true}, CompletedAt: timestamptz(at)}))
}

func (r *PostgreSQL) InsertEvent(ctx context.Context, event domain.Event) error {
	return mapDatabaseError(r.queries.InsertLifecycleOutboxEvent(ctx, &dbgen.InsertLifecycleOutboxEventParams{OutboxEventID: pgUUID(event.ID), AggregateType: event.AggregateType, AggregateID: pgUUID(event.AggregateID), AggregateVersion: event.AggregateVersion, EventType: event.Type, PayloadJson: event.Payload, DeduplicationKey: event.DeduplicationKey, CorrelationID: pgUUID(event.CorrelationID), AvailableAt: timestamptz(event.CreatedAt)}))
}

func entryFromLifecycleRow(row *dbgen.GetLifecycleEntryForUpdateRow) domain.Entry {
	return domain.Entry{ID: uuidValue(row.NamespaceEntryID), SpaceID: uuidValue(row.SpaceID), ParentFolderID: optionalGoogleUUID(row.ParentFolderID), EntryType: row.EntryType, Name: row.Name, NormalizedName: row.NormalizedName, PathCache: optionalString(row.PathCache), Depth: row.Depth, LifecycleStatus: row.LifecycleStatus, CreatedByUserID: uuidValue(row.CreatedByUserID), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, DeletedAt: optionalTime(row.DeletedAt), RowVersion: row.RowVersion, IsRoot: row.IsRoot}
}

func entryFromFolderRow(row *dbgen.GetLifecycleFolderForUpdateRow) domain.Entry {
	return domain.Entry{ID: uuidValue(row.NamespaceEntryID), SpaceID: uuidValue(row.SpaceID), ParentFolderID: optionalGoogleUUID(row.ParentFolderID), EntryType: row.EntryType, Name: row.Name, NormalizedName: row.NormalizedName, PathCache: optionalString(row.PathCache), Depth: row.Depth, LifecycleStatus: row.LifecycleStatus, CreatedByUserID: uuidValue(row.CreatedByUserID), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, DeletedAt: optionalTime(row.DeletedAt), RowVersion: row.RowVersion, IsRoot: row.IsRoot}
}

func entryFromNamespace(row *dbgen.NamespaceEntry) domain.Entry {
	return domain.Entry{ID: uuidValue(row.NamespaceEntryID), SpaceID: uuidValue(row.SpaceID), ParentFolderID: optionalGoogleUUID(row.ParentFolderID), EntryType: row.EntryType, Name: row.Name, NormalizedName: row.NormalizedName, PathCache: optionalString(row.PathCache), Depth: row.Depth, LifecycleStatus: row.LifecycleStatus, CreatedByUserID: uuidValue(row.CreatedByUserID), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, DeletedAt: optionalTime(row.DeletedAt), RowVersion: row.RowVersion}
}

func recycleFromRow(row any) domain.RecycleItem {
	switch value := row.(type) {
	case *dbgen.GetRecycleItemWithEntryRow:
		return recycleFromFields(value.RecycleItemID, value.NamespaceEntryID, value.EntryType, value.OriginalSpaceID, value.OriginalParentFolderID, value.OriginalName, value.CurrentName, value.LifecycleStatus, value.DeletedByUserID, value.DeletedAt, value.ExpiresAt, value.Status, value.RestoredToFolderID, value.RestoredAt, value.CreatedAt, value.UpdatedAt, value.RowVersion)
	case *dbgen.GetRecycleItemWithEntryForUpdateRow:
		return recycleFromFields(value.RecycleItemID, value.NamespaceEntryID, value.EntryType, value.OriginalSpaceID, value.OriginalParentFolderID, value.OriginalName, value.CurrentName, value.LifecycleStatus, value.DeletedByUserID, value.DeletedAt, value.ExpiresAt, value.Status, value.RestoredToFolderID, value.RestoredAt, value.CreatedAt, value.UpdatedAt, value.RowVersion)
	case *dbgen.ListRecycleItemsRow:
		return recycleFromFields(value.RecycleItemID, value.NamespaceEntryID, value.EntryType, value.OriginalSpaceID, value.OriginalParentFolderID, value.OriginalName, value.CurrentName, value.LifecycleStatus, value.DeletedByUserID, value.DeletedAt, value.ExpiresAt, value.Status, value.RestoredToFolderID, value.RestoredAt, value.CreatedAt, value.UpdatedAt, value.RowVersion)
	default:
		return domain.RecycleItem{}
	}
}

func recycleFromFields(id, entryID pgtype.UUID, entryType string, spaceID, parentID pgtype.UUID, originalName, currentName, lifecycle string, deletedBy pgtype.UUID, deletedAt, expiresAt pgtype.Timestamptz, status string, restoredTo pgtype.UUID, restoredAt, createdAt, updatedAt pgtype.Timestamptz, rowVersion int64) domain.RecycleItem {
	return domain.RecycleItem{ID: uuidValue(id), EntryID: uuidValue(entryID), EntryType: entryType, OriginalSpaceID: uuidValue(spaceID), OriginalParentFolderID: optionalGoogleUUID(parentID), OriginalName: originalName, CurrentName: currentName, LifecycleStatus: lifecycle, DeletedByUserID: uuidValue(deletedBy), DeletedAt: deletedAt.Time, ExpiresAt: expiresAt.Time, Status: status, RestoredToFolderID: optionalGoogleUUID(restoredTo), RestoredAt: optionalTime(restoredAt), CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time, RowVersion: rowVersion}
}

func mapDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return domain.ErrNameConflict
		case "23514":
			return domain.ErrConflict
		}
	}
	return err
}

func pgUUID(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: true}
}

func uuidValue(value pgtype.UUID) uuid.UUID {
	if !value.Valid {
		return uuid.Nil
	}
	return uuid.UUID(value.Bytes)
}

func optionalUUID(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return pgUUID(*value)
}

func optionalGoogleUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	converted := uuid.UUID(value.Bytes)
	return &converted
}

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func pageOffset(page, pageSize int) int64 {
	return int64(page-1) * int64(pageSize)
}
