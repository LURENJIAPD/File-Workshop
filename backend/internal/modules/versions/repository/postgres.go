package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/versions/application"
	"file-workshop/backend/internal/modules/versions/domain"
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
		return fmt.Errorf("begin version transaction: %w", err)
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

func (r *PostgreSQL) GetDocumentContext(ctx context.Context, documentID uuid.UUID) (domain.DocumentContext, error) {
	row, err := r.queries.GetVersionDocumentContext(ctx, pgUUID(documentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DocumentContext{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.DocumentContext{}, mapDatabaseError(err)
	}
	return domain.DocumentContext{ID: uuidValue(row.DocumentID), SpaceID: uuidValue(row.SpaceID), OwnerUserID: uuidValue(row.OwnerUserID), CurrentVersionID: optionalGoogleUUID(row.CurrentVersionID), Availability: row.AvailabilityStatus, RowVersion: row.RowVersion}, nil
}

func (r *PostgreSQL) CountVersions(ctx context.Context, documentID uuid.UUID) (int64, error) {
	total, err := r.queries.CountDocumentVersions(ctx, pgUUID(documentID))
	return total, mapDatabaseError(err)
}

func (r *PostgreSQL) ListVersions(ctx context.Context, documentID uuid.UUID, page, pageSize int) ([]domain.Version, error) {
	rows, err := r.queries.ListDocumentVersions(ctx, &dbgen.ListDocumentVersionsParams{DocumentID: pgUUID(documentID), PageSize: int32(pageSize), PageOffset: int64((page - 1) * pageSize)})
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	items := make([]domain.Version, 0, len(rows))
	for _, row := range rows {
		items = append(items, versionFromRow(row))
	}
	return items, nil
}

func (r *PostgreSQL) GetVersion(ctx context.Context, documentID, versionID uuid.UUID) (domain.Version, error) {
	row, err := r.queries.GetDocumentVersion(ctx, &dbgen.GetDocumentVersionParams{DocumentID: pgUUID(documentID), DocumentVersionID: pgUUID(versionID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Version{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Version{}, mapDatabaseError(err)
	}
	return versionFromRow(row), nil
}

func (r *PostgreSQL) GetDocumentForUpdate(ctx context.Context, documentID uuid.UUID) (domain.DocumentContext, error) {
	row, err := r.queries.GetVersionDocumentForUpdate(ctx, pgUUID(documentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DocumentContext{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.DocumentContext{}, mapDatabaseError(err)
	}
	return domain.DocumentContext{ID: uuidValue(row.DocumentID), OwnerUserID: uuidValue(row.OwnerUserID), CurrentVersionID: optionalGoogleUUID(row.CurrentVersionID), Availability: row.AvailabilityStatus, RowVersion: row.RowVersion}, nil
}

func (r *PostgreSQL) InsertRestoredVersion(ctx context.Context, id, documentID, sourceVersionID, createdByUserID uuid.UUID, changeNote *string, now time.Time) (domain.Version, error) {
	row, err := r.queries.InsertRestoredDocumentVersion(ctx, &dbgen.InsertRestoredDocumentVersionParams{DocumentVersionID: pgUUID(id), DocumentID: pgUUID(documentID), RestoredFromVersionID: pgUUID(sourceVersionID), ChangeNote: optionalText(changeNote), CreatedByUserID: pgUUID(createdByUserID), CreatedAt: timestamptz(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Version{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Version{}, mapDatabaseError(err)
	}
	return versionFromRow(row), nil
}

func (r *PostgreSQL) SetCurrentVersion(ctx context.Context, documentID, versionID uuid.UUID, rowVersion int64, now time.Time) error {
	rows, err := r.queries.SetDocumentCurrentVersion(ctx, &dbgen.SetDocumentCurrentVersionParams{DocumentID: pgUUID(documentID), CurrentVersionID: pgUUID(versionID), UpdatedAt: timestamptz(now), RowVersion: rowVersion})
	if err != nil {
		return mapDatabaseError(err)
	}
	if rows != 1 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *PostgreSQL) ExpireLocks(ctx context.Context, documentID uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.ExpireDocumentLocks(ctx, &dbgen.ExpireDocumentLocksParams{DocumentID: pgUUID(documentID), Now: timestamptz(now)}))
}

func (r *PostgreSQL) EnsureLockCounter(ctx context.Context, documentID uuid.UUID, now time.Time) error {
	return mapDatabaseError(r.queries.EnsureDocumentLockCounter(ctx, &dbgen.EnsureDocumentLockCounterParams{DocumentID: pgUUID(documentID), UpdatedAt: timestamptz(now)}))
}

func (r *PostgreSQL) IncrementLockCounter(ctx context.Context, documentID uuid.UUID, now time.Time) (int64, error) {
	value, err := r.queries.IncrementDocumentLockCounter(ctx, &dbgen.IncrementDocumentLockCounterParams{DocumentID: pgUUID(documentID), UpdatedAt: timestamptz(now)})
	return value, mapDatabaseError(err)
}

func (r *PostgreSQL) InsertLock(ctx context.Context, id, documentID, userID uuid.UUID, tokenHash []byte, fencingToken int64, source string, acquiredAt, expiresAt time.Time) (domain.Lock, error) {
	row, err := r.queries.InsertDocumentLock(ctx, &dbgen.InsertDocumentLockParams{DocumentLockID: pgUUID(id), DocumentID: pgUUID(documentID), UserID: pgUUID(userID), TokenHash: tokenHash, FencingToken: fencingToken, Source: source, AcquiredAt: timestamptz(acquiredAt), ExpiresAt: timestamptz(expiresAt)})
	if err != nil {
		return domain.Lock{}, mapDatabaseError(err)
	}
	return lockFromRow(row), nil
}

func (r *PostgreSQL) GetActiveLock(ctx context.Context, documentID uuid.UUID) (*domain.Lock, error) {
	row, err := r.queries.GetActiveDocumentLock(ctx, pgUUID(documentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	lock := lockFromRow(row)
	return &lock, nil
}

func (r *PostgreSQL) GetActiveLockForUpdate(ctx context.Context, documentID uuid.UUID) (*domain.Lock, error) {
	row, err := r.queries.GetActiveDocumentLockForUpdate(ctx, pgUUID(documentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	lock := lockFromRow(row)
	return &lock, nil
}

func (r *PostgreSQL) HeartbeatLock(ctx context.Context, documentID uuid.UUID, tokenHash []byte, rowVersion int64, userID uuid.UUID, now, expiresAt time.Time) (domain.Lock, error) {
	row, err := r.queries.HeartbeatDocumentLock(ctx, &dbgen.HeartbeatDocumentLockParams{DocumentID: pgUUID(documentID), TokenHash: tokenHash, RowVersion: rowVersion, HeartbeatAt: timestamptz(now), ExpiresAt: timestamptz(expiresAt), UserID: pgUUID(userID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Lock{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.Lock{}, mapDatabaseError(err)
	}
	return lockFromRow(row), nil
}

func (r *PostgreSQL) ReleaseLock(ctx context.Context, documentID uuid.UUID, tokenHash []byte, rowVersion int64, userID uuid.UUID, now time.Time, reason *string) (domain.Lock, error) {
	row, err := r.queries.ReleaseDocumentLock(ctx, &dbgen.ReleaseDocumentLockParams{DocumentID: pgUUID(documentID), TokenHash: tokenHash, RowVersion: rowVersion, UserID: pgUUID(userID), ReleasedAt: timestamptz(now), ReleasedByUserID: pgUUID(userID), ReleaseReason: optionalText(reason)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Lock{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.Lock{}, mapDatabaseError(err)
	}
	return lockFromRow(row), nil
}

func (r *PostgreSQL) ForceReleaseLock(ctx context.Context, documentID uuid.UUID, rowVersion int64, userID uuid.UUID, now time.Time, reason string) (domain.Lock, error) {
	row, err := r.queries.ForceReleaseDocumentLock(ctx, &dbgen.ForceReleaseDocumentLockParams{DocumentID: pgUUID(documentID), RowVersion: rowVersion, ReleasedAt: timestamptz(now), ReleasedByUserID: pgUUID(userID), ReleaseReason: pgtype.Text{String: reason, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Lock{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.Lock{}, mapDatabaseError(err)
	}
	return lockFromRow(row), nil
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
	return mapDatabaseError(r.queries.InsertVersionOutboxEvent(ctx, &dbgen.InsertVersionOutboxEventParams{OutboxEventID: pgUUID(event.ID), AggregateType: event.AggregateType, AggregateID: pgUUID(event.AggregateID), AggregateVersion: event.AggregateVersion, EventType: event.Type, PayloadJson: event.Payload, DeduplicationKey: event.DeduplicationKey, CorrelationID: pgUUID(event.CorrelationID), AvailableAt: timestamptz(event.CreatedAt)}))
}

func versionFromRow(row *dbgen.DocumentVersion) domain.Version {
	return domain.Version{ID: uuidValue(row.DocumentVersionID), DocumentID: uuidValue(row.DocumentID), VersionNumber: row.VersionNumber, StorageObjectID: uuidValue(row.StorageObjectID), SizeBytes: row.SizeBytes, SHA256: append([]byte(nil), row.Sha256...), MIMEType: row.MimeType, ChangeNote: optionalString(row.ChangeNote), SourceType: row.SourceType, RestoredFromVersionID: optionalGoogleUUID(row.RestoredFromVersionID), CreatedByUserID: uuidValue(row.CreatedByUserID), CreatedAt: row.CreatedAt.Time.UTC()}
}

func lockFromRow(row *dbgen.DocumentLock) domain.Lock {
	return domain.Lock{ID: uuidValue(row.DocumentLockID), DocumentID: uuidValue(row.DocumentID), UserID: uuidValue(row.UserID), FencingToken: row.FencingToken, Source: row.Source, Status: row.Status, AcquiredAt: row.AcquiredAt.Time.UTC(), HeartbeatAt: row.HeartbeatAt.Time.UTC(), ExpiresAt: row.ExpiresAt.Time.UTC(), ReleasedAt: optionalTime(row.ReleasedAt), ReleasedByUserID: optionalGoogleUUID(row.ReleasedByUserID), ReleaseReason: optionalString(row.ReleaseReason), CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(), RowVersion: row.RowVersion}
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
			return domain.ErrNotFound
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

func optionalGoogleUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
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
	text := value.String
	return &text
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
