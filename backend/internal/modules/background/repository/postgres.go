package repository

import (
	"context"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/background/domain"
	"file-workshop/backend/internal/platform/database/dbgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type PostgreSQL struct {
	queries *dbgen.Queries
}

func NewPostgreSQL(db dbgen.DBTX) *PostgreSQL {
	return &PostgreSQL{queries: dbgen.New(db)}
}

func (r *PostgreSQL) ClaimOutboxEventsByType(ctx context.Context, eventType, workerID string, batchSize int32, leaseUntil time.Time, now time.Time) ([]domain.OutboxEvent, error) {
	rows, err := r.queries.ClaimOutboxEventsByType(ctx, &dbgen.ClaimOutboxEventsByTypeParams{EventType: eventType, LockedBy: workerID, BatchSize: batchSize, LeaseUntil: timestamptz(leaseUntil), Now: timestamptz(now)})
	if err != nil {
		return nil, err
	}
	result := make([]domain.OutboxEvent, 0, len(rows))
	for _, row := range rows {
		event, err := outboxEvent(row)
		if err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, nil
}

func (r *PostgreSQL) MarkOutboxEventPublished(ctx context.Context, event domain.OutboxEvent, workerID string, now time.Time) (bool, error) {
	rows, err := r.queries.MarkOutboxEventPublished(ctx, &dbgen.MarkOutboxEventPublishedParams{OutboxEventID: pgUUID(event.ID), LockedBy: workerID, RowVersion: event.RowVersion, PublishedAt: timestamptz(now)})
	return rows == 1, err
}

func (r *PostgreSQL) MarkOutboxEventFailed(ctx context.Context, event domain.OutboxEvent, workerID string, code, summary string, nextRetryAt time.Time, now time.Time) (bool, error) {
	rows, err := r.queries.MarkOutboxEventFailed(ctx, &dbgen.MarkOutboxEventFailedParams{OutboxEventID: pgUUID(event.ID), LockedBy: workerID, RowVersion: event.RowVersion, LastErrorCode: code, LastErrorSummary: summary, NextRetryAt: timestamptz(nextRetryAt), Now: timestamptz(now)})
	return rows == 1, err
}

func (r *PostgreSQL) MarkOutboxEventDead(ctx context.Context, event domain.OutboxEvent, workerID string, code, summary string, now time.Time) (bool, error) {
	rows, err := r.queries.MarkOutboxEventDead(ctx, &dbgen.MarkOutboxEventDeadParams{OutboxEventID: pgUUID(event.ID), LockedBy: workerID, RowVersion: event.RowVersion, LastErrorCode: code, LastErrorSummary: summary, Now: timestamptz(now)})
	return rows == 1, err
}

func (r *PostgreSQL) RenewOutboxEventLease(ctx context.Context, event domain.OutboxEvent, workerID string, leaseUntil time.Time, now time.Time) (bool, error) {
	rows, err := r.queries.RenewOutboxEventLease(ctx, &dbgen.RenewOutboxEventLeaseParams{OutboxEventID: pgUUID(event.ID), LockedBy: workerID, RowVersion: event.RowVersion, LeaseUntil: timestamptz(leaseUntil), Now: timestamptz(now)})
	return rows == 1, err
}

func (r *PostgreSQL) CountOutboxEventsByStatus(ctx context.Context) ([]domain.OutboxStatusCount, error) {
	rows, err := r.queries.CountOutboxEventsByStatus(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.OutboxStatusCount, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.OutboxStatusCount{Status: row.Status, Count: row.Count})
	}
	return result, nil
}

func outboxEvent(row *dbgen.OutboxEvent) (domain.OutboxEvent, error) {
	if row == nil {
		return domain.OutboxEvent{}, fmt.Errorf("outbox event row is nil")
	}
	id, err := googleUUID(row.OutboxEventID)
	if err != nil {
		return domain.OutboxEvent{}, fmt.Errorf("outbox_event_id: %w", err)
	}
	aggregateID, err := googleUUID(row.AggregateID)
	if err != nil {
		return domain.OutboxEvent{}, fmt.Errorf("aggregate_id: %w", err)
	}
	return domain.OutboxEvent{
		ID:                 id,
		AggregateType:      row.AggregateType,
		AggregateID:        aggregateID,
		AggregateVersion:   row.AggregateVersion,
		EventType:          row.EventType,
		EventSchemaVersion: row.EventSchemaVersion,
		PayloadJSON:        append([]byte(nil), row.PayloadJson...),
		DeduplicationKey:   row.DeduplicationKey,
		CorrelationID:      optionalGoogleUUID(row.CorrelationID),
		CausationID:        optionalGoogleUUID(row.CausationID),
		Priority:           row.Priority,
		Status:             row.Status,
		AttemptCount:       row.AttemptCount,
		MaxAttempts:        row.MaxAttempts,
		AvailableAt:        row.AvailableAt.Time,
		LockedBy:           optionalString(row.LockedBy),
		LockedAt:           optionalTime(row.LockedAt),
		LeaseUntil:         optionalTime(row.LeaseUntil),
		NextRetryAt:        optionalTime(row.NextRetryAt),
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
		PublishedAt:        optionalTime(row.PublishedAt),
		LastErrorCode:      optionalString(row.LastErrorCode),
		LastErrorSummary:   optionalString(row.LastErrorSummary),
		RowVersion:         row.RowVersion,
	}, nil
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
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
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}
