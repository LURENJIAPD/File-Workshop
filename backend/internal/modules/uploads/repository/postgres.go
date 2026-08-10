package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/uploads/application"
	"file-workshop/backend/internal/modules/uploads/domain"
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
		return fmt.Errorf("begin upload transaction: %w", err)
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

func (r *PostgreSQL) GetFolderContext(ctx context.Context, spaceID, folderID uuid.UUID) (domain.FolderContext, error) {
	row, err := r.queries.GetUploadFolderContext(ctx, &dbgen.GetUploadFolderContextParams{FolderID: pgUUID(folderID), SpaceID: pgUUID(spaceID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FolderContext{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.FolderContext{}, mapDatabaseError(err)
	}
	return domain.FolderContext{ID: uuidValue(row.FolderID), SpaceID: uuidValue(row.SpaceID)}, nil
}

func (r *PostgreSQL) GetDocumentContext(ctx context.Context, spaceID, documentID uuid.UUID) (domain.DocumentContext, error) {
	row, err := r.queries.GetUploadDocumentContext(ctx, &dbgen.GetUploadDocumentContextParams{DocumentID: pgUUID(documentID), SpaceID: pgUUID(spaceID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DocumentContext{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.DocumentContext{}, mapDatabaseError(err)
	}
	return domain.DocumentContext{ID: uuidValue(row.DocumentID), SpaceID: uuidValue(row.SpaceID), CurrentVersionID: optionalGoogleUUID(row.CurrentVersionID), Availability: row.AvailabilityStatus, RowVersion: row.RowVersion}, nil
}

func (r *PostgreSQL) GetSession(ctx context.Context, id uuid.UUID) (domain.Session, error) {
	row, err := r.queries.GetUploadSession(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Session{}, mapDatabaseError(err)
	}
	return sessionFromRow(row), nil
}

func (r *PostgreSQL) GetSessionForUpdate(ctx context.Context, id uuid.UUID) (domain.Session, error) {
	row, err := r.queries.GetUploadSessionForUpdate(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Session{}, mapDatabaseError(err)
	}
	return sessionFromRow(row), nil
}

func (r *PostgreSQL) TryCreateIdempotency(ctx context.Context, recordID, actorID uuid.UUID, operation, key string, hash []byte, expiresAt, now time.Time) (bool, error) {
	rows, err := r.queries.TryCreateFileIdempotencyRecord(ctx, &dbgen.TryCreateFileIdempotencyRecordParams{IdempotencyRecordID: pgUUID(recordID), UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key, RequestHash: hash, ExpiresAt: timestamptz(expiresAt), CreatedAt: timestamptz(now)})
	return rows == 1, mapDatabaseError(err)
}

func (r *PostgreSQL) GetIdempotency(ctx context.Context, actorID uuid.UUID, operation, key string) (domain.IdempotencyRecord, error) {
	row, err := r.queries.GetFileIdempotencyRecord(ctx, &dbgen.GetFileIdempotencyRecordParams{UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.IdempotencyRecord{}, domain.ErrConflict
	}
	if err != nil {
		return domain.IdempotencyRecord{}, mapDatabaseError(err)
	}
	return domain.IdempotencyRecord{RequestHash: row.RequestHash, Status: row.Status, ResultResourceID: optionalGoogleUUID(row.ResultResourceID)}, nil
}

func (r *PostgreSQL) CompleteIdempotency(ctx context.Context, actorID uuid.UUID, operation, key string, resourceID uuid.UUID, resourceType string, now time.Time) error {
	return mapDatabaseError(r.queries.CompleteFileIdempotencyRecord(ctx, &dbgen.CompleteFileIdempotencyRecordParams{UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key, ResultResourceID: pgUUID(resourceID), ResultResourceType: pgtype.Text{String: resourceType, Valid: true}, CompletedAt: timestamptz(now)}))
}

func (r *PostgreSQL) ReserveQuota(ctx context.Context, spaceID uuid.UUID, bytes int64, now time.Time) error {
	_, err := r.queries.ReserveSpaceQuota(ctx, &dbgen.ReserveSpaceQuotaParams{SpaceID: pgUUID(spaceID), ReservedBytes: bytes, UpdatedAt: timestamptz(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrQuotaExceeded
	}
	return mapDatabaseError(err)
}

func (r *PostgreSQL) InsertQuotaReservation(ctx context.Context, id, spaceID, userID uuid.UUID, bytes int64, expiresAt, now time.Time) error {
	_, err := r.queries.InsertQuotaReservation(ctx, &dbgen.InsertQuotaReservationParams{QuotaReservationID: pgUUID(id), SpaceID: pgUUID(spaceID), UserID: pgUUID(userID), ReservedBytes: bytes, ExpiresAt: timestamptz(expiresAt), CreatedAt: timestamptz(now)})
	return mapDatabaseError(err)
}

func (r *PostgreSQL) ReleaseQuotaReservation(ctx context.Context, id, spaceID uuid.UUID, bytes int64, now time.Time) error {
	reservation, err := r.queries.GetQuotaReservationForUpdate(ctx, pgUUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrConflict
	}
	if err != nil {
		return mapDatabaseError(err)
	}
	if uuidValue(reservation.SpaceID) != spaceID || reservation.Status != "ACTIVE" {
		return domain.ErrConflict
	}
	rows, err := r.queries.ReleaseSpaceQuotaReservation(ctx, &dbgen.ReleaseSpaceQuotaReservationParams{SpaceID: pgUUID(spaceID), ReservedBytes: bytes, UpdatedAt: timestamptz(now)})
	if err != nil {
		return mapDatabaseError(err)
	}
	if rows != 1 {
		return domain.ErrConflict
	}
	_, err = r.queries.MarkQuotaReservationReleased(ctx, &dbgen.MarkQuotaReservationReleasedParams{QuotaReservationID: pgUUID(id), Status: "RELEASED", ReleasedAt: timestamptz(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrConflict
	}
	return mapDatabaseError(err)
}

func (r *PostgreSQL) InsertSession(ctx context.Context, input domain.NewSession) (domain.Session, error) {
	row, err := r.queries.InsertUploadSession(ctx, &dbgen.InsertUploadSessionParams{
		UploadSessionID:          pgUUID(input.ID),
		UserID:                   pgUUID(input.UserID),
		SpaceID:                  pgUUID(input.SpaceID),
		FolderID:                 pgUUID(input.FolderID),
		QuotaReservationID:       pgUUID(input.QuotaReservationID),
		TargetDocumentID:         optionalUUID(input.TargetDocumentID),
		UploadIntent:             input.UploadIntent,
		FileName:                 input.FileName,
		NormalizedName:           input.NormalizedName,
		DeclaredSizeBytes:        input.DeclaredSizeBytes,
		DeclaredSha256:           optionalBytes(input.DeclaredSHA256),
		DeclaredMimeType:         optionalText(input.DeclaredMIMEType),
		ProviderUploadID:         pgtype.Text{String: input.ProviderUploadID, Valid: stringsTrimmedValid(input.ProviderUploadID)},
		TemporaryObjectKey:       input.TemporaryObjectKey,
		PartSizeBytes:            input.PartSizeBytes,
		ExpectedPartCount:        input.ExpectedPartCount,
		ExpectedCurrentVersionID: optionalUUID(input.ExpectedCurrentVersionID),
		ExpectedLockFencingToken: optionalInt8(input.ExpectedLockFencingToken),
		LockTokenHash:            optionalBytes(input.LockTokenHash),
		ExpiresAt:                timestamptz(input.ExpiresAt),
		CreatedAt:                timestamptz(input.CreatedAt),
	})
	if err != nil {
		return domain.Session{}, mapDatabaseError(err)
	}
	return sessionFromRow(row), nil
}

func (r *PostgreSQL) MarkUploading(ctx context.Context, id uuid.UUID, now time.Time) (domain.Session, error) {
	row, err := r.queries.MarkUploadSessionUploading(ctx, &dbgen.MarkUploadSessionUploadingParams{UploadSessionID: pgUUID(id), UpdatedAt: timestamptz(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, domain.ErrConflict
	}
	if err != nil {
		return domain.Session{}, mapDatabaseError(err)
	}
	return sessionFromRow(row), nil
}

func (r *PostgreSQL) AbortSession(ctx context.Context, id uuid.UUID, rowVersion int64, failureCode string, now time.Time) (domain.Session, error) {
	row, err := r.queries.AbortUploadSession(ctx, &dbgen.AbortUploadSessionParams{UploadSessionID: pgUUID(id), RowVersion: rowVersion, FailureCode: pgtype.Text{String: failureCode, Valid: true}, UpdatedAt: timestamptz(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Session{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.Session{}, mapDatabaseError(err)
	}
	return sessionFromRow(row), nil
}

func (r *PostgreSQL) InsertEvent(ctx context.Context, event domain.Event) error {
	return mapDatabaseError(r.queries.InsertFileOutboxEvent(ctx, &dbgen.InsertFileOutboxEventParams{OutboxEventID: pgUUID(event.ID), AggregateType: event.AggregateType, AggregateID: pgUUID(event.AggregateID), AggregateVersion: event.AggregateVersion, EventType: event.Type, PayloadJson: event.Payload, DeduplicationKey: event.DeduplicationKey, CorrelationID: pgUUID(event.CorrelationID), AvailableAt: timestamptz(event.CreatedAt)}))
}

func sessionFromRow(row *dbgen.UploadSession) domain.Session {
	return domain.Session{
		ID:                       uuidValue(row.UploadSessionID),
		UserID:                   uuidValue(row.UserID),
		SpaceID:                  uuidValue(row.SpaceID),
		FolderID:                 uuidValue(row.FolderID),
		QuotaReservationID:       uuidValue(row.QuotaReservationID),
		TargetDocumentID:         optionalGoogleUUID(row.TargetDocumentID),
		UploadIntent:             row.UploadIntent,
		FileName:                 row.FileName,
		NormalizedName:           row.NormalizedName,
		DeclaredSizeBytes:        row.DeclaredSizeBytes,
		DeclaredSHA256:           optionalBytes(row.DeclaredSha256),
		DeclaredMIMEType:         optionalString(row.DeclaredMimeType),
		ProviderUploadID:         optionalString(row.ProviderUploadID),
		TemporaryObjectKey:       row.TemporaryObjectKey,
		PartSizeBytes:            row.PartSizeBytes,
		ExpectedPartCount:        row.ExpectedPartCount,
		ExpectedCurrentVersionID: optionalGoogleUUID(row.ExpectedCurrentVersionID),
		ExpectedLockFencingToken: optionalInt64(row.ExpectedLockFencingToken),
		LockTokenHash:            optionalBytes(row.LockTokenHash),
		Status:                   row.Status,
		ExpiresAt:                row.ExpiresAt.Time.UTC(),
		CreatedAt:                row.CreatedAt.Time.UTC(),
		UpdatedAt:                row.UpdatedAt.Time.UTC(),
		CompletedAt:              optionalTime(row.CompletedAt),
		FailureCode:              optionalString(row.FailureCode),
		ResultDocumentID:         optionalGoogleUUID(row.ResultDocumentID),
		ResultVersionID:          optionalGoogleUUID(row.ResultVersionID),
		RowVersion:               row.RowVersion,
	}
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

func optionalUUID(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return pgUUID(*value)
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

func optionalInt8(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func optionalInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func optionalBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	return append([]byte(nil), value...)
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

func stringsTrimmedValid(value string) bool {
	return value != ""
}
