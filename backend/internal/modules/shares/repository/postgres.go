package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/shares/application"
	"file-workshop/backend/internal/modules/shares/domain"
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
		return fmt.Errorf("begin share transaction: %w", err)
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

func (r *PostgreSQL) GetSourceResource(ctx context.Context, sourceType string, sourceID uuid.UUID) (domain.SourceResource, error) {
	row, err := r.queries.GetShareSourceResource(ctx, &dbgen.GetShareSourceResourceParams{SourceType: sourceType, SourceID: pgUUID(sourceID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SourceResource{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.SourceResource{}, mapDatabaseError(err)
	}
	return domain.SourceResource{
		ID:               uuidValue(row.ResourceID),
		Type:             row.ResourceType,
		SpaceID:          uuidValue(row.SpaceID),
		Name:             row.Name,
		SpaceType:        row.SpaceType,
		SpaceOwnerUserID: optionalGoogleUUID(row.SpaceOwnerUserID),
		OrganizationID:   optionalGoogleUUID(row.OrganizationID),
		OwnerUserID:      optionalGoogleUUID(row.OwnerUserID),
		InheritanceMode:  row.InheritanceMode,
		ACLVersion:       row.AclVersion,
		RowVersion:       row.RowVersion,
	}, nil
}

func (r *PostgreSQL) GetShare(ctx context.Context, shareID uuid.UUID) (domain.Share, error) {
	row, err := r.queries.GetShareWithActions(ctx, pgUUID(shareID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Share{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Share{}, mapDatabaseError(err)
	}
	return shareFromRow(row), nil
}

func (r *PostgreSQL) GetShareForUpdate(ctx context.Context, shareID uuid.UUID) (domain.Share, error) {
	row, err := r.queries.GetShareWithActionsForUpdate(ctx, pgUUID(shareID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Share{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Share{}, mapDatabaseError(err)
	}
	return shareFromRow(row), nil
}

func (r *PostgreSQL) InsertShare(ctx context.Context, value domain.Share, at time.Time) (domain.Share, error) {
	row, err := r.queries.InsertShare(ctx, &dbgen.InsertShareParams{
		ShareID:              pgUUID(value.ID),
		SourceDocumentID:     optionalUUID(value.SourceDocumentID),
		SourceFolderID:       optionalUUID(value.SourceFolderID),
		CreatorUserID:        pgUUID(value.CreatorUserID),
		TargetKind:           value.TargetKind,
		TargetUserID:         optionalUUID(value.TargetUserID),
		TargetOrganizationID: optionalUUID(value.TargetOrganizationID),
		TargetSpaceID:        optionalUUID(value.TargetSpaceID),
		TokenHash:            value.TokenHash,
		AllowReshare:         value.AllowReshare,
		ValidFrom:            timestamptz(value.ValidFrom),
		ValidUntil:           optionalTime(value.ValidUntil),
		CreatedAt:            timestamptz(at),
	})
	if err != nil {
		return domain.Share{}, mapDatabaseError(err)
	}
	for _, action := range value.Actions {
		if err = r.queries.InsertShareAction(ctx, &dbgen.InsertShareActionParams{ShareID: pgUUID(value.ID), Action: action, CreatedAt: timestamptz(at)}); err != nil {
			return domain.Share{}, mapDatabaseError(err)
		}
	}
	result := shareFromModel(row, value.Actions)
	result.TokenHash = value.TokenHash
	return result, nil
}

func (r *PostgreSQL) UpdateShare(ctx context.Context, shareID uuid.UUID, actions []string, allowReshare bool, validUntil *time.Time, rowVersion int64, at time.Time) (domain.Share, error) {
	if err := r.queries.DeleteShareActions(ctx, pgUUID(shareID)); err != nil {
		return domain.Share{}, mapDatabaseError(err)
	}
	row, err := r.queries.UpdateShare(ctx, &dbgen.UpdateShareParams{ShareID: pgUUID(shareID), AllowReshare: allowReshare, ValidUntil: optionalTime(validUntil), UpdatedAt: timestamptz(at), RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Share{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.Share{}, mapDatabaseError(err)
	}
	for _, action := range actions {
		if err = r.queries.InsertShareAction(ctx, &dbgen.InsertShareActionParams{ShareID: pgUUID(shareID), Action: action, CreatedAt: timestamptz(at)}); err != nil {
			return domain.Share{}, mapDatabaseError(err)
		}
	}
	return shareFromModel(row, actions), nil
}

func (r *PostgreSQL) RevokeShare(ctx context.Context, shareID, actorID uuid.UUID, reason string, rowVersion int64, at time.Time) (domain.Share, error) {
	current, err := r.GetShare(ctx, shareID)
	if err != nil {
		return domain.Share{}, err
	}
	row, err := r.queries.RevokeShare(ctx, &dbgen.RevokeShareParams{ShareID: pgUUID(shareID), RevokedByUserID: pgUUID(actorID), RevokeReason: pgtype.Text{String: reason, Valid: true}, RevokedAt: timestamptz(at), RowVersion: rowVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Share{}, domain.ErrVersionConflict
	}
	if err != nil {
		return domain.Share{}, mapDatabaseError(err)
	}
	result := shareFromModel(row, current.Actions)
	result.TokenHash = current.TokenHash
	return result, nil
}

func (r *PostgreSQL) ExpireShares(ctx context.Context, at time.Time) error {
	return mapDatabaseError(r.queries.ExpireShares(ctx, timestamptz(at)))
}

func (r *PostgreSQL) CountCreated(ctx context.Context, userID uuid.UUID) (int64, error) {
	total, err := r.queries.CountCreatedShares(ctx, pgUUID(userID))
	return total, mapDatabaseError(err)
}

func (r *PostgreSQL) ListCreated(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]domain.Share, error) {
	rows, err := r.queries.ListCreatedShares(ctx, &dbgen.ListCreatedSharesParams{CreatorUserID: pgUUID(userID), PageOffset: pageOffset(page, pageSize), PageSize: int32(pageSize)})
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	items := make([]domain.Share, 0, len(rows))
	for _, row := range rows {
		items = append(items, shareFromRow(row))
	}
	return items, nil
}

func (r *PostgreSQL) CountReceived(ctx context.Context, userID uuid.UUID, organizationIDs []uuid.UUID, at time.Time) (int64, error) {
	total, err := r.queries.CountReceivedShares(ctx, &dbgen.CountReceivedSharesParams{EffectiveAt: timestamptz(at), UserID: pgUUID(userID), OrganizationIds: pgUUIDs(organizationIDs)})
	return total, mapDatabaseError(err)
}

func (r *PostgreSQL) ListReceived(ctx context.Context, userID uuid.UUID, organizationIDs []uuid.UUID, page, pageSize int, at time.Time) ([]domain.Share, error) {
	rows, err := r.queries.ListReceivedShares(ctx, &dbgen.ListReceivedSharesParams{EffectiveAt: timestamptz(at), UserID: pgUUID(userID), OrganizationIds: pgUUIDs(organizationIDs), PageOffset: pageOffset(page, pageSize), PageSize: int32(pageSize)})
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	items := make([]domain.Share, 0, len(rows))
	for _, row := range rows {
		items = append(items, shareFromRow(row))
	}
	return items, nil
}

func (r *PostgreSQL) ListActiveUserOrganizations(ctx context.Context, userID uuid.UUID, at time.Time) ([]uuid.UUID, error) {
	rows, err := r.queries.ListActivePermissionUserOrganizations(ctx, &dbgen.ListActivePermissionUserOrganizationsParams{UserID: pgUUID(userID), EffectiveFrom: timestamptz(at)})
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	items := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		items = append(items, uuidValue(row))
	}
	return items, nil
}

func (r *PostgreSQL) IncrementShareVersions(ctx context.Context, share domain.Share, at time.Time) error {
	if share.TargetUserID != nil {
		return mapDatabaseError(r.queries.IncrementSharePrincipalVersion(ctx, &dbgen.IncrementSharePrincipalVersionParams{UserID: pgUUID(*share.TargetUserID), UpdatedAt: timestamptz(at)}))
	}
	if share.TargetOrganizationID != nil {
		return mapDatabaseError(r.queries.IncrementShareOrganizationVersion(ctx, &dbgen.IncrementShareOrganizationVersionParams{DescendantOrganizationID: pgUUID(*share.TargetOrganizationID), UpdatedAt: timestamptz(at)}))
	}
	return nil
}

func (r *PostgreSQL) TryCreateIdempotency(ctx context.Context, recordID, actorID uuid.UUID, operation, key string, hash []byte, expiresAt, now time.Time) (bool, error) {
	rows, err := r.queries.TryCreateShareIdempotency(ctx, &dbgen.TryCreateShareIdempotencyParams{IdempotencyRecordID: pgUUID(recordID), UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key, RequestHash: hash, ExpiresAt: timestamptz(expiresAt), CreatedAt: timestamptz(now)})
	return rows == 1, mapDatabaseError(err)
}

func (r *PostgreSQL) GetIdempotency(ctx context.Context, actorID uuid.UUID, operation, key string) (domain.IdempotencyRecord, error) {
	row, err := r.queries.GetShareIdempotency(ctx, &dbgen.GetShareIdempotencyParams{UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.IdempotencyRecord{}, domain.ErrConflict
	}
	if err != nil {
		return domain.IdempotencyRecord{}, mapDatabaseError(err)
	}
	return domain.IdempotencyRecord{RequestHash: row.RequestHash, Status: row.Status, ResultResourceID: optionalGoogleUUID(row.ResultResourceID)}, nil
}

func (r *PostgreSQL) CompleteIdempotency(ctx context.Context, actorID uuid.UUID, operation, key string, resourceID uuid.UUID, resourceType string, at time.Time) error {
	return mapDatabaseError(r.queries.CompleteShareIdempotency(ctx, &dbgen.CompleteShareIdempotencyParams{UserID: pgUUID(actorID), Operation: operation, IdempotencyKey: key, ResultResourceID: pgUUID(resourceID), ResultResourceType: pgtype.Text{String: resourceType, Valid: true}, CompletedAt: timestamptz(at)}))
}

func (r *PostgreSQL) InsertEvent(ctx context.Context, event domain.Event) error {
	return mapDatabaseError(r.queries.InsertShareOutboxEvent(ctx, &dbgen.InsertShareOutboxEventParams{OutboxEventID: pgUUID(event.ID), AggregateType: event.AggregateType, AggregateID: pgUUID(event.AggregateID), AggregateVersion: event.AggregateVersion, EventType: event.Type, PayloadJson: event.Payload, DeduplicationKey: event.DeduplicationKey, CorrelationID: pgUUID(event.CorrelationID), AvailableAt: timestamptz(event.CreatedAt)}))
}

type shareRow interface {
	GetShareID() pgtype.UUID
	GetSourceDocumentID() pgtype.UUID
	GetSourceFolderID() pgtype.UUID
	GetCreatorUserID() pgtype.UUID
	GetTargetKind() string
	GetTargetUserID() pgtype.UUID
	GetTargetOrganizationID() pgtype.UUID
	GetTargetSpaceID() pgtype.UUID
	GetTokenHash() []byte
	GetAllowReshare() bool
	GetValidFrom() pgtype.Timestamptz
	GetValidUntil() pgtype.Timestamptz
	GetStatus() string
	GetCreatedAt() pgtype.Timestamptz
	GetUpdatedAt() pgtype.Timestamptz
	GetRevokedAt() pgtype.Timestamptz
	GetRevokedByUserID() pgtype.UUID
	GetRevokeReason() pgtype.Text
	GetRowVersion() int64
	GetActions() []string
}

func shareFromRow(row any) domain.Share {
	switch value := row.(type) {
	case *dbgen.GetShareWithActionsRow:
		return shareFromFields(value.ShareID, value.SourceDocumentID, value.SourceFolderID, value.CreatorUserID, value.TargetKind, value.TargetUserID, value.TargetOrganizationID, value.TargetSpaceID, value.TokenHash, value.AllowReshare, value.ValidFrom, value.ValidUntil, value.Status, value.CreatedAt, value.UpdatedAt, value.RevokedAt, value.RevokedByUserID, value.RevokeReason, value.RowVersion, value.Actions)
	case *dbgen.GetShareWithActionsForUpdateRow:
		return shareFromFields(value.ShareID, value.SourceDocumentID, value.SourceFolderID, value.CreatorUserID, value.TargetKind, value.TargetUserID, value.TargetOrganizationID, value.TargetSpaceID, value.TokenHash, value.AllowReshare, value.ValidFrom, value.ValidUntil, value.Status, value.CreatedAt, value.UpdatedAt, value.RevokedAt, value.RevokedByUserID, value.RevokeReason, value.RowVersion, value.Actions)
	case *dbgen.ListCreatedSharesRow:
		return shareFromFields(value.ShareID, value.SourceDocumentID, value.SourceFolderID, value.CreatorUserID, value.TargetKind, value.TargetUserID, value.TargetOrganizationID, value.TargetSpaceID, value.TokenHash, value.AllowReshare, value.ValidFrom, value.ValidUntil, value.Status, value.CreatedAt, value.UpdatedAt, value.RevokedAt, value.RevokedByUserID, value.RevokeReason, value.RowVersion, value.Actions)
	case *dbgen.ListReceivedSharesRow:
		return shareFromFields(value.ShareID, value.SourceDocumentID, value.SourceFolderID, value.CreatorUserID, value.TargetKind, value.TargetUserID, value.TargetOrganizationID, value.TargetSpaceID, value.TokenHash, value.AllowReshare, value.ValidFrom, value.ValidUntil, value.Status, value.CreatedAt, value.UpdatedAt, value.RevokedAt, value.RevokedByUserID, value.RevokeReason, value.RowVersion, value.Actions)
	default:
		return domain.Share{}
	}
}

func shareFromModel(value *dbgen.Share, actions []string) domain.Share {
	return shareFromFields(value.ShareID, value.SourceDocumentID, value.SourceFolderID, value.CreatorUserID, value.TargetKind, value.TargetUserID, value.TargetOrganizationID, value.TargetSpaceID, value.TokenHash, value.AllowReshare, value.ValidFrom, value.ValidUntil, value.Status, value.CreatedAt, value.UpdatedAt, value.RevokedAt, value.RevokedByUserID, value.RevokeReason, value.RowVersion, actions)
}

func shareFromFields(id, sourceDocumentID, sourceFolderID, creatorUserID pgtype.UUID, targetKind string, targetUserID, targetOrganizationID, targetSpaceID pgtype.UUID, tokenHash []byte, allowReshare bool, validFrom, validUntil pgtype.Timestamptz, status string, createdAt, updatedAt, revokedAt pgtype.Timestamptz, revokedByUserID pgtype.UUID, revokeReason pgtype.Text, rowVersion int64, actions []string) domain.Share {
	return domain.Share{ID: uuidValue(id), SourceDocumentID: optionalGoogleUUID(sourceDocumentID), SourceFolderID: optionalGoogleUUID(sourceFolderID), CreatorUserID: uuidValue(creatorUserID), TargetKind: targetKind, TargetUserID: optionalGoogleUUID(targetUserID), TargetOrganizationID: optionalGoogleUUID(targetOrganizationID), TargetSpaceID: optionalGoogleUUID(targetSpaceID), TokenHash: tokenHash, AllowReshare: allowReshare, Actions: actions, ValidFrom: validFrom.Time, ValidUntil: optionalGoTime(validUntil), Status: status, CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time, RevokedAt: optionalGoTime(revokedAt), RevokedByUserID: optionalGoogleUUID(revokedByUserID), RevokeReason: optionalString(revokeReason), RowVersion: rowVersion}
}

func mapDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23514":
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

func optionalTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamptz(*value)
}

func optionalGoTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func optionalString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func pgUUIDs(values []uuid.UUID) []pgtype.UUID {
	result := make([]pgtype.UUID, 0, len(values))
	for _, value := range values {
		result = append(result, pgUUID(value))
	}
	return result
}

func pageOffset(page, pageSize int) int64 {
	return int64(page-1) * int64(pageSize)
}
