package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/files/application"
	"file-workshop/backend/internal/modules/files/domain"
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
		return fmt.Errorf("begin file transaction: %w", err)
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

func (r *PostgreSQL) GetSpaceDirectoryInfo(ctx context.Context, id uuid.UUID) (domain.SpaceDirectoryInfo, error) {
	row, err := r.queries.GetFileSpaceDirectoryInfo(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SpaceDirectoryInfo{}, domain.ErrSpaceNotFound
	}
	if err != nil {
		return domain.SpaceDirectoryInfo{}, mapDatabaseError(err)
	}
	return spaceInfo(row.SpaceID, row.SpaceType, row.OwnerUserID, row.OrganizationID, row.RootFolderID, row.Status, row.RowVersion)
}

func (r *PostgreSQL) GetSpaceDirectoryInfoForUpdate(ctx context.Context, id uuid.UUID) (domain.SpaceDirectoryInfo, error) {
	row, err := r.queries.GetFileSpaceDirectoryInfoForUpdate(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SpaceDirectoryInfo{}, domain.ErrSpaceNotFound
	}
	if err != nil {
		return domain.SpaceDirectoryInfo{}, mapDatabaseError(err)
	}
	return spaceInfo(row.SpaceID, row.SpaceType, row.OwnerUserID, row.OrganizationID, row.RootFolderID, row.Status, row.RowVersion)
}

func (r *PostgreSQL) GetEntry(ctx context.Context, id uuid.UUID) (domain.NamespaceEntry, error) {
	row, err := r.queries.GetFileNamespaceEntry(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NamespaceEntry{}, domain.ErrEntryNotFound
	}
	if err != nil {
		return domain.NamespaceEntry{}, mapDatabaseError(err)
	}
	return entryFromGet(row)
}

func (r *PostgreSQL) GetEntryForUpdate(ctx context.Context, id uuid.UUID) (domain.NamespaceEntry, error) {
	row, err := r.queries.GetFileNamespaceEntryForUpdate(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NamespaceEntry{}, domain.ErrEntryNotFound
	}
	if err != nil {
		return domain.NamespaceEntry{}, mapDatabaseError(err)
	}
	return entryFromGetForUpdate(row)
}

func (r *PostgreSQL) ListEntries(ctx context.Context, filter domain.EntryListFilter) (domain.EntryListResult, error) {
	params := &dbgen.CountFileChildEntriesParams{SpaceID: pgUUID(filter.SpaceID), ParentFolderID: pgUUID(*filter.ParentFolderID), EntryType: optionalText(filter.EntryType), LifecycleStatus: optionalText(filter.LifecycleStatus)}
	total, err := r.queries.CountFileChildEntries(ctx, params)
	if err != nil {
		return domain.EntryListResult{}, mapDatabaseError(err)
	}
	rows, err := r.queries.ListFileChildEntries(ctx, &dbgen.ListFileChildEntriesParams{SpaceID: params.SpaceID, ParentFolderID: params.ParentFolderID, EntryType: params.EntryType, LifecycleStatus: params.LifecycleStatus, PageSize: int32(filter.PageSize), PageOffset: pageOffset(filter.Page, filter.PageSize)})
	if err != nil {
		return domain.EntryListResult{}, mapDatabaseError(err)
	}
	items := make([]domain.NamespaceEntry, 0, len(rows))
	for _, row := range rows {
		item, err := entryFromList(row)
		if err != nil {
			return domain.EntryListResult{}, err
		}
		items = append(items, item)
	}
	return domain.EntryListResult{Items: items, SpaceID: filter.SpaceID, ParentFolderID: filter.ParentFolderID, Page: filter.Page, PageSize: filter.PageSize, Total: total}, nil
}

func (r *PostgreSQL) InsertNamespaceEntry(ctx context.Context, input domain.NewNamespaceEntry) (domain.NamespaceEntry, error) {
	row, err := r.queries.InsertFileNamespaceEntry(ctx, &dbgen.InsertFileNamespaceEntryParams{NamespaceEntryID: pgUUID(input.ID), SpaceID: pgUUID(input.SpaceID), ParentFolderID: optionalUUID(input.ParentFolderID), EntryType: input.EntryType, Name: input.Name, NormalizedName: input.NormalizedName, PathCache: pgtype.Text{String: input.PathCache, Valid: true}, Depth: input.Depth, CreatedByUserID: pgUUID(input.CreatedByUserID), CreatedAt: timestamptz(input.CreatedAt)})
	if err != nil {
		return domain.NamespaceEntry{}, mapDatabaseError(err)
	}
	id, err := googleUUID(row.NamespaceEntryID)
	if err != nil {
		return domain.NamespaceEntry{}, err
	}
	return domain.NamespaceEntry{ID: id, SpaceID: uuidValue(row.SpaceID), ParentFolderID: optionalGoogleUUID(row.ParentFolderID), EntryType: row.EntryType, Name: row.Name, NormalizedName: row.NormalizedName, PathCache: optionalString(row.PathCache), Depth: row.Depth, LifecycleStatus: row.LifecycleStatus, CreatedByUserID: uuidValue(row.CreatedByUserID), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(), DeletedAt: optionalTime(row.DeletedAt), RowVersion: row.RowVersion}, nil
}

func (r *PostgreSQL) InsertFolder(ctx context.Context, id uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.InsertFileFolder(ctx, &dbgen.InsertFileFolderParams{FolderID: pgUUID(id), CreatedAt: timestamptz(now)}))
}

func (r *PostgreSQL) InsertDocument(ctx context.Context, input domain.NewDocument) (domain.NamespaceEntry, error) {
	if _, err := r.queries.InsertFileDocument(ctx, &dbgen.InsertFileDocumentParams{DocumentID: pgUUID(input.ID), OwnerUserID: pgUUID(input.OwnerUserID), AvailabilityStatus: input.AvailabilityStatus, ExtensionNormalized: optionalText(input.ExtensionNormalized), Classification: optionalText(input.Classification), MetadataJson: input.MetadataJSON, CreatedAt: timestamptz(input.CreatedAt)}); err != nil {
		return domain.NamespaceEntry{}, mapDatabaseError(err)
	}
	return r.GetEntry(ctx, input.ID)
}

func (r *PostgreSQL) UpdateSpaceRootFolder(ctx context.Context, spaceID, rootID uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.UpdateFileSpaceRootFolder(ctx, &dbgen.UpdateFileSpaceRootFolderParams{SpaceID: pgUUID(spaceID), RootFolderID: optionalUUID(&rootID), UpdatedAt: timestamptz(now)}))
}

func (r *PostgreSQL) RenameEntry(ctx context.Context, id uuid.UUID, name, normalized string, extension *string, rowVersion int64, now time.Time) (domain.NamespaceEntry, error) {
	row, err := r.queries.RenameFileNamespaceEntry(ctx, &dbgen.RenameFileNamespaceEntryParams{NamespaceEntryID: pgUUID(id), Name: name, NormalizedName: normalized, UpdatedAt: timestamptz(now), RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NamespaceEntry{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.NamespaceEntry{}, mapDatabaseError(err)
	}
	if row.EntryType == domain.EntryTypeDocument {
		if err = r.queries.UpdateFileDocumentExtension(ctx, &dbgen.UpdateFileDocumentExtensionParams{DocumentID: pgUUID(id), ExtensionNormalized: optionalText(extension), UpdatedAt: timestamptz(now)}); err != nil {
			return domain.NamespaceEntry{}, mapDatabaseError(err)
		}
	}
	return r.GetEntry(ctx, id)
}

func (r *PostgreSQL) MoveEntry(ctx context.Context, id, parentID uuid.UUID, path string, depth int32, rowVersion int64, now time.Time) (domain.NamespaceEntry, error) {
	row, err := r.queries.MoveFileNamespaceEntry(ctx, &dbgen.MoveFileNamespaceEntryParams{NamespaceEntryID: pgUUID(id), ParentFolderID: optionalUUID(&parentID), PathCache: pgtype.Text{String: path, Valid: true}, Depth: depth, UpdatedAt: timestamptz(now), RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NamespaceEntry{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.NamespaceEntry{}, mapDatabaseError(err)
	}
	return r.GetEntry(ctx, uuidValue(row.NamespaceEntryID))
}

func (r *PostgreSQL) UpdateDescendantPaths(ctx context.Context, rootID uuid.UUID, rootPath string, rootDepth int32, now time.Time) error {
	return mapDatabaseError(r.queries.UpdateFileDescendantPaths(ctx, &dbgen.UpdateFileDescendantPathsParams{RootID: pgUUID(rootID), RootPath: rootPath, RootDepth: int32(rootDepth), UpdatedAt: timestamptz(now)}))
}

func (r *PostgreSQL) FolderIsDescendantOf(ctx context.Context, folderID, ancestorID uuid.UUID) (bool, error) {
	value, err := r.queries.FileFolderIsDescendantOf(ctx, &dbgen.FileFolderIsDescendantOfParams{FolderID: pgUUID(folderID), AncestorFolderID: pgUUID(ancestorID)})
	return value, mapDatabaseError(err)
}

func (r *PostgreSQL) TouchSpaceSecurityEpoch(ctx context.Context, spaceID uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.TouchFileSpaceSecurityEpoch(ctx, &dbgen.TouchFileSpaceSecurityEpochParams{SpaceID: pgUUID(spaceID), UpdatedAt: timestamptz(now)}))
}

func (r *PostgreSQL) TryCreateIdempotency(ctx context.Context, recordID, actorID uuid.UUID, operation, key string, hash []byte, expiresAt, now time.Time) (bool, error) {
	rows, err := r.queries.TryCreateFileIdempotencyRecord(ctx, &dbgen.TryCreateFileIdempotencyRecordParams{IdempotencyRecordID: pgUUID(recordID), UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key, RequestHash: hash, ExpiresAt: timestamptz(expiresAt), CreatedAt: timestamptz(now)})
	return rows == 1, mapDatabaseError(err)
}

func (r *PostgreSQL) GetIdempotency(ctx context.Context, actorID uuid.UUID, operation, key string) (domain.IdempotencyRecord, error) {
	row, err := r.queries.GetFileIdempotencyRecord(ctx, &dbgen.GetFileIdempotencyRecordParams{UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key})
	if err != nil {
		return domain.IdempotencyRecord{}, mapDatabaseError(err)
	}
	return domain.IdempotencyRecord{RequestHash: row.RequestHash, Status: row.Status, ResultResourceID: optionalGoogleUUID(row.ResultResourceID)}, nil
}

func (r *PostgreSQL) CompleteIdempotency(ctx context.Context, actorID uuid.UUID, operation, key string, resourceID uuid.UUID, resourceType string, now time.Time) error {
	return mapDatabaseError(r.queries.CompleteFileIdempotencyRecord(ctx, &dbgen.CompleteFileIdempotencyRecordParams{UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key, ResultResourceID: pgUUID(resourceID), ResultResourceType: pgtype.Text{String: resourceType, Valid: true}, CompletedAt: timestamptz(now)}))
}

func (r *PostgreSQL) InsertEvent(ctx context.Context, event domain.Event) error {
	return mapDatabaseError(r.queries.InsertFileOutboxEvent(ctx, &dbgen.InsertFileOutboxEventParams{OutboxEventID: pgUUID(event.ID), AggregateType: event.AggregateType, AggregateID: pgUUID(event.AggregateID), AggregateVersion: event.AggregateVersion, EventType: event.Type, PayloadJson: event.Payload, DeduplicationKey: event.DeduplicationKey, CorrelationID: pgUUID(event.CorrelationID), AvailableAt: timestamptz(event.CreatedAt)}))
}

func spaceInfo(id pgtype.UUID, spaceType string, owner, organization, root pgtype.UUID, status string, rowVersion int64) (domain.SpaceDirectoryInfo, error) {
	return domain.SpaceDirectoryInfo{ID: uuidValue(id), SpaceType: spaceType, OwnerUserID: optionalGoogleUUID(owner), OrganizationID: optionalGoogleUUID(organization), RootFolderID: optionalGoogleUUID(root), Status: status, RowVersion: rowVersion}, nil
}

func entryFromGet(row *dbgen.GetFileNamespaceEntryRow) (domain.NamespaceEntry, error) {
	return entry(row.NamespaceEntryID, row.SpaceID, row.ParentFolderID, row.EntryType, row.Name, row.NormalizedName, row.PathCache, row.Depth, row.LifecycleStatus, row.CreatedByUserID, row.CreatedAt, row.UpdatedAt, row.DeletedAt, row.RowVersion, row.IsRoot, row.FolderInheritanceMode, row.FolderAclVersion, row.OwnerUserID, row.CurrentVersionID, row.AvailabilityStatus, row.ExtensionNormalized, row.DocumentInheritanceMode, row.DocumentAclVersion, row.Classification, row.MetadataSchemaVersion, row.MetadataJson)
}

func entryFromGetForUpdate(row *dbgen.GetFileNamespaceEntryForUpdateRow) (domain.NamespaceEntry, error) {
	return entry(row.NamespaceEntryID, row.SpaceID, row.ParentFolderID, row.EntryType, row.Name, row.NormalizedName, row.PathCache, row.Depth, row.LifecycleStatus, row.CreatedByUserID, row.CreatedAt, row.UpdatedAt, row.DeletedAt, row.RowVersion, row.IsRoot, row.FolderInheritanceMode, row.FolderAclVersion, row.OwnerUserID, row.CurrentVersionID, row.AvailabilityStatus, row.ExtensionNormalized, row.DocumentInheritanceMode, row.DocumentAclVersion, row.Classification, row.MetadataSchemaVersion, row.MetadataJson)
}

func entryFromList(row *dbgen.ListFileChildEntriesRow) (domain.NamespaceEntry, error) {
	return entry(row.NamespaceEntryID, row.SpaceID, row.ParentFolderID, row.EntryType, row.Name, row.NormalizedName, row.PathCache, row.Depth, row.LifecycleStatus, row.CreatedByUserID, row.CreatedAt, row.UpdatedAt, row.DeletedAt, row.RowVersion, row.IsRoot, row.FolderInheritanceMode, row.FolderAclVersion, row.OwnerUserID, row.CurrentVersionID, row.AvailabilityStatus, row.ExtensionNormalized, row.DocumentInheritanceMode, row.DocumentAclVersion, row.Classification, row.MetadataSchemaVersion, row.MetadataJson)
}

func entry(idValue, spaceIDValue, parentIDValue pgtype.UUID, entryType, name, normalized string, path pgtype.Text, depth int32, lifecycle string, creatorValue pgtype.UUID, createdAt, updatedAt pgtype.Timestamptz, deletedAt pgtype.Timestamptz, rowVersion int64, isRoot bool, folderMode pgtype.Text, folderACL pgtype.Int8, ownerValue, currentVersionValue pgtype.UUID, availability, extension, documentMode pgtype.Text, documentACL pgtype.Int8, classification pgtype.Text, metadataVersion pgtype.Int4, metadata json.RawMessage) (domain.NamespaceEntry, error) {
	id, err := googleUUID(idValue)
	if err != nil {
		return domain.NamespaceEntry{}, err
	}
	creator, err := googleUUID(creatorValue)
	if err != nil {
		return domain.NamespaceEntry{}, err
	}
	result := domain.NamespaceEntry{ID: id, SpaceID: uuidValue(spaceIDValue), ParentFolderID: optionalGoogleUUID(parentIDValue), EntryType: entryType, Name: name, NormalizedName: normalized, PathCache: optionalString(path), Depth: depth, LifecycleStatus: lifecycle, CreatedByUserID: creator, CreatedAt: createdAt.Time.UTC(), UpdatedAt: updatedAt.Time.UTC(), DeletedAt: optionalTime(deletedAt), RowVersion: rowVersion, IsRoot: isRoot}
	if entryType == domain.EntryTypeFolder {
		result.InheritanceMode = optionalString(folderMode)
		if folderACL.Valid {
			value := folderACL.Int64
			result.ACLVersion = &value
		}
	}
	if entryType == domain.EntryTypeDocument {
		result.OwnerUserID = optionalGoogleUUID(ownerValue)
		result.CurrentVersionID = optionalGoogleUUID(currentVersionValue)
		result.AvailabilityStatus = optionalString(availability)
		result.ExtensionNormalized = optionalString(extension)
		result.InheritanceMode = optionalString(documentMode)
		if documentACL.Valid {
			value := documentACL.Int64
			result.ACLVersion = &value
		}
		result.Classification = optionalString(classification)
		if metadataVersion.Valid {
			value := metadataVersion.Int32
			result.MetadataSchemaVersion = &value
		}
		if len(metadata) > 0 {
			result.MetadataJSON = append([]byte(nil), metadata...)
		}
	}
	return result, nil
}

func mapDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return domain.ErrConflict
		case "23503":
			return domain.ErrEntryNotFound
		case "23514", "23P01":
			return domain.ErrInvalidInput
		}
	}
	return err
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func uuidValue(value pgtype.UUID) uuid.UUID {
	return uuid.UUID(value.Bytes)
}

func googleUUID(value pgtype.UUID) (uuid.UUID, error) {
	if !value.Valid {
		return uuid.Nil, fmt.Errorf("uuid is null")
	}
	return uuid.UUID(value.Bytes), nil
}

func optionalGoogleUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
}

func optionalUUID(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return pgUUID(*value)
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	converted := value.Time.UTC()
	return &converted
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func pageOffset(page, pageSize int) int64 {
	return int64((page - 1) * pageSize)
}
