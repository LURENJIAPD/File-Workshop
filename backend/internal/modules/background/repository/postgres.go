package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"file-workshop/backend/internal/modules/background/domain"
	"file-workshop/backend/internal/platform/database/dbgen"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func (r *PostgreSQL) CountOutboxFailuresByErrorCode(ctx context.Context) ([]domain.FailureSummaryItem, error) {
	rows, err := r.queries.CountOutboxFailuresByErrorCode(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.FailureSummaryItem, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.FailureSummaryItem{ErrorCode: row.LastErrorCode.String, Count: row.Count, LatestAt: row.LatestAt.Time})
	}
	return result, nil
}

func (r *PostgreSQL) ListOutboxEvents(ctx context.Context, filter domain.OutboxListFilter) (domain.OutboxListResult, error) {
	status, eventType := nullableText(filter.Status), nullableText(filter.EventType)
	total, err := r.queries.CountOutboxEvents(ctx, &dbgen.CountOutboxEventsParams{Status: status, EventType: eventType})
	if err != nil {
		return domain.OutboxListResult{}, err
	}
	rows, err := r.queries.ListOutboxEvents(ctx, &dbgen.ListOutboxEventsParams{Status: status, EventType: eventType, PageOffset: int64((filter.Page - 1) * filter.PageSize), PageSize: int32(filter.PageSize)})
	if err != nil {
		return domain.OutboxListResult{}, err
	}
	items := make([]domain.OutboxEvent, 0, len(rows))
	for _, row := range rows {
		item, err := outboxEvent(row)
		if err != nil {
			return domain.OutboxListResult{}, err
		}
		items = append(items, item)
	}
	return domain.OutboxListResult{Items: items, Page: filter.Page, PageSize: filter.PageSize, Total: total}, nil
}

func (r *PostgreSQL) RetryOutboxEvent(ctx context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.OutboxEvent, error) {
	row, err := r.queries.RetryOutboxEvent(ctx, &dbgen.RetryOutboxEventParams{OutboxEventID: pgUUID(id), RowVersion: rowVersion, Reason: reason, AvailableAt: timestamptz(now)})
	if err != nil {
		return domain.OutboxEvent{}, r.classifyOutboxRetryError(ctx, id, err)
	}
	return outboxEvent(row)
}

func (r *PostgreSQL) EnqueueJob(ctx context.Context, input domain.EnqueueJobInput, now time.Time) (domain.BackgroundJob, error) {
	row, err := r.queries.InsertBackgroundJob(ctx, &dbgen.InsertBackgroundJobParams{
		BackgroundJobID:         pgUUID(input.ID),
		JobType:                 input.JobType,
		TargetDocumentID:        optionalUUID(input.TargetDocumentID),
		TargetDocumentVersionID: optionalUUID(input.TargetDocumentVersionID),
		TargetStorageObjectID:   optionalUUID(input.TargetStorageObjectID),
		PayloadSchemaVersion:    input.PayloadSchemaVersion,
		PayloadJson:             input.PayloadJSON,
		DeduplicationKey:        input.DeduplicationKey,
		Priority:                input.Priority,
		MaxAttempts:             input.MaxAttempts,
		AvailableAt:             timestamptz(input.AvailableAt),
		CreatedAt:               timestamptz(now),
	})
	if err != nil {
		return domain.BackgroundJob{}, err
	}
	return backgroundJob(row)
}

func (r *PostgreSQL) ClaimBackgroundJobsByType(ctx context.Context, jobType, workerID string, batchSize int32, leaseUntil time.Time, now time.Time) ([]domain.BackgroundJob, error) {
	rows, err := r.queries.ClaimBackgroundJobsByType(ctx, &dbgen.ClaimBackgroundJobsByTypeParams{JobType: jobType, LockedBy: workerID, BatchSize: batchSize, LeaseUntil: timestamptz(leaseUntil), Now: timestamptz(now)})
	if err != nil {
		return nil, err
	}
	result := make([]domain.BackgroundJob, 0, len(rows))
	for _, row := range rows {
		job, err := backgroundJob(row)
		if err != nil {
			return nil, err
		}
		result = append(result, job)
	}
	return result, nil
}

func (r *PostgreSQL) MarkBackgroundJobSuccess(ctx context.Context, job domain.BackgroundJob, workerID string, now time.Time) (bool, error) {
	rows, err := r.queries.MarkBackgroundJobSuccess(ctx, &dbgen.MarkBackgroundJobSuccessParams{BackgroundJobID: pgUUID(job.ID), LockedBy: workerID, RowVersion: job.RowVersion, CompletedAt: timestamptz(now)})
	return rows == 1, err
}

func (r *PostgreSQL) MarkBackgroundJobFailed(ctx context.Context, job domain.BackgroundJob, workerID string, code, summary string, nextRetryAt time.Time, now time.Time) (bool, error) {
	rows, err := r.queries.MarkBackgroundJobFailed(ctx, &dbgen.MarkBackgroundJobFailedParams{BackgroundJobID: pgUUID(job.ID), LockedBy: workerID, RowVersion: job.RowVersion, LastErrorCode: code, LastErrorSummary: summary, NextRetryAt: timestamptz(nextRetryAt), Now: timestamptz(now)})
	return rows == 1, err
}

func (r *PostgreSQL) MarkBackgroundJobDead(ctx context.Context, job domain.BackgroundJob, workerID string, code, summary string, now time.Time) (bool, error) {
	rows, err := r.queries.MarkBackgroundJobDead(ctx, &dbgen.MarkBackgroundJobDeadParams{BackgroundJobID: pgUUID(job.ID), LockedBy: workerID, RowVersion: job.RowVersion, LastErrorCode: code, LastErrorSummary: summary, Now: timestamptz(now)})
	return rows == 1, err
}

func (r *PostgreSQL) RenewBackgroundJobLease(ctx context.Context, job domain.BackgroundJob, workerID string, leaseUntil time.Time, now time.Time) (bool, error) {
	rows, err := r.queries.RenewBackgroundJobLease(ctx, &dbgen.RenewBackgroundJobLeaseParams{BackgroundJobID: pgUUID(job.ID), LockedBy: workerID, RowVersion: job.RowVersion, LeaseUntil: timestamptz(leaseUntil), Now: timestamptz(now)})
	return rows == 1, err
}

func (r *PostgreSQL) CountBackgroundJobsByStatus(ctx context.Context) ([]domain.OutboxStatusCount, error) {
	rows, err := r.queries.CountBackgroundJobsByStatus(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.OutboxStatusCount, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.OutboxStatusCount{Status: row.Status, Count: row.Count})
	}
	return result, nil
}

func (r *PostgreSQL) GetQueueLagSummary(ctx context.Context, now time.Time) (domain.QueueLagSummary, error) {
	row, err := r.queries.GetBackgroundQueueLagSummary(ctx, timestamptz(now))
	if err != nil {
		return domain.QueueLagSummary{}, err
	}
	return domain.QueueLagSummary{
		OutboxEvents: domain.QueueLagItem{
			DuePendingCount:        row.OutboxDuePendingCount,
			DueFailedCount:         row.OutboxDueFailedCount,
			ExpiredProcessingCount: row.OutboxExpiredProcessingCount,
			OldestDueAt:            optionalTime(row.OutboxOldestDueAt),
		},
		BackgroundJobs: domain.QueueLagItem{
			DuePendingCount:        row.BackgroundJobsDuePendingCount,
			DueFailedCount:         row.BackgroundJobsDueFailedCount,
			ExpiredProcessingCount: row.BackgroundJobsExpiredProcessingCount,
			OldestDueAt:            optionalTime(row.BackgroundJobsOldestDueAt),
		},
	}, nil
}

func (r *PostgreSQL) CountBackgroundJobFailuresByErrorCode(ctx context.Context) ([]domain.FailureSummaryItem, error) {
	rows, err := r.queries.CountBackgroundJobFailuresByErrorCode(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.FailureSummaryItem, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.FailureSummaryItem{ErrorCode: row.LastErrorCode.String, Count: row.Count, LatestAt: row.LatestAt.Time})
	}
	return result, nil
}

func (r *PostgreSQL) RecoverExpiredLeases(ctx context.Context, batchSize int, reason string, now time.Time) (domain.LeaseRecoveryResult, error) {
	row, err := r.queries.RecoverExpiredBackgroundLeases(ctx, &dbgen.RecoverExpiredBackgroundLeasesParams{
		Now:       timestamptz(now),
		BatchSize: int32(batchSize),
		Reason:    reason,
	})
	if err != nil {
		return domain.LeaseRecoveryResult{}, err
	}
	return domain.LeaseRecoveryResult{
		OutboxEvents: domain.LeaseRecoveryItem{
			Recovered: row.OutboxRecoveredCount,
			Retryable: row.OutboxRetryableCount,
			Dead:      row.OutboxDeadCount,
		},
		BackgroundJobs: domain.LeaseRecoveryItem{
			Recovered: row.BackgroundJobsRecoveredCount,
			Retryable: row.BackgroundJobsRetryableCount,
			Dead:      row.BackgroundJobsDeadCount,
		},
	}, nil
}

func (r *PostgreSQL) ListBackgroundJobs(ctx context.Context, filter domain.JobListFilter) (domain.JobListResult, error) {
	status, jobType := nullableText(filter.Status), nullableText(filter.JobType)
	total, err := r.queries.CountBackgroundJobs(ctx, &dbgen.CountBackgroundJobsParams{Status: status, JobType: jobType})
	if err != nil {
		return domain.JobListResult{}, err
	}
	rows, err := r.queries.ListBackgroundJobs(ctx, &dbgen.ListBackgroundJobsParams{Status: status, JobType: jobType, PageOffset: int64((filter.Page - 1) * filter.PageSize), PageSize: int32(filter.PageSize)})
	if err != nil {
		return domain.JobListResult{}, err
	}
	items := make([]domain.BackgroundJob, 0, len(rows))
	for _, row := range rows {
		item, err := backgroundJob(row)
		if err != nil {
			return domain.JobListResult{}, err
		}
		items = append(items, item)
	}
	return domain.JobListResult{Items: items, Page: filter.Page, PageSize: filter.PageSize, Total: total}, nil
}

func (r *PostgreSQL) RetryBackgroundJob(ctx context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error) {
	row, err := r.queries.RetryBackgroundJob(ctx, &dbgen.RetryBackgroundJobParams{BackgroundJobID: pgUUID(id), RowVersion: rowVersion, Reason: reason, AvailableAt: timestamptz(now)})
	if err != nil {
		return domain.BackgroundJob{}, r.classifyBackgroundJobRetryError(ctx, id, err)
	}
	return backgroundJob(row)
}

func (r *PostgreSQL) CancelBackgroundJob(ctx context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error) {
	row, err := r.queries.CancelBackgroundJob(ctx, &dbgen.CancelBackgroundJobParams{BackgroundJobID: pgUUID(id), RowVersion: rowVersion, Reason: reason, CompletedAt: timestamptz(now)})
	if err != nil {
		return domain.BackgroundJob{}, r.classifyBackgroundJobRetryError(ctx, id, err)
	}
	return backgroundJob(row)
}

func (r *PostgreSQL) DeadLetterBackgroundJob(ctx context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error) {
	row, err := r.queries.MarkBackgroundJobManuallyDead(ctx, &dbgen.MarkBackgroundJobManuallyDeadParams{BackgroundJobID: pgUUID(id), RowVersion: rowVersion, Reason: reason, CompletedAt: timestamptz(now)})
	if err != nil {
		return domain.BackgroundJob{}, r.classifyBackgroundJobRetryError(ctx, id, err)
	}
	return backgroundJob(row)
}

func (r *PostgreSQL) SkipBackgroundJob(ctx context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error) {
	row, err := r.queries.SkipBackgroundJob(ctx, &dbgen.SkipBackgroundJobParams{BackgroundJobID: pgUUID(id), RowVersion: rowVersion, Reason: reason, CompletedAt: timestamptz(now)})
	if err != nil {
		return domain.BackgroundJob{}, r.classifyBackgroundJobRetryError(ctx, id, err)
	}
	return backgroundJob(row)
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

func backgroundJob(row *dbgen.BackgroundJob) (domain.BackgroundJob, error) {
	if row == nil {
		return domain.BackgroundJob{}, fmt.Errorf("background job row is nil")
	}
	id, err := googleUUID(row.BackgroundJobID)
	if err != nil {
		return domain.BackgroundJob{}, fmt.Errorf("background_job_id: %w", err)
	}
	return domain.BackgroundJob{
		ID:                      id,
		JobType:                 row.JobType,
		TargetDocumentID:        optionalGoogleUUID(row.TargetDocumentID),
		TargetDocumentVersionID: optionalGoogleUUID(row.TargetDocumentVersionID),
		TargetStorageObjectID:   optionalGoogleUUID(row.TargetStorageObjectID),
		PayloadSchemaVersion:    row.PayloadSchemaVersion,
		PayloadJSON:             append([]byte(nil), row.PayloadJson...),
		DeduplicationKey:        row.DeduplicationKey,
		Priority:                row.Priority,
		Status:                  row.Status,
		AttemptCount:            row.AttemptCount,
		MaxAttempts:             row.MaxAttempts,
		AvailableAt:             row.AvailableAt.Time,
		LockedBy:                optionalString(row.LockedBy),
		LockedAt:                optionalTime(row.LockedAt),
		LeaseUntil:              optionalTime(row.LeaseUntil),
		HeartbeatAt:             optionalTime(row.HeartbeatAt),
		CreatedAt:               row.CreatedAt.Time,
		UpdatedAt:               row.UpdatedAt.Time,
		StartedAt:               optionalTime(row.StartedAt),
		CompletedAt:             optionalTime(row.CompletedAt),
		LastErrorCode:           optionalString(row.LastErrorCode),
		LastErrorSummary:        optionalString(row.LastErrorSummary),
		RowVersion:              row.RowVersion,
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

func optionalUUID(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return pgUUID(*value)
}

func nullableText(value *string) pgtype.Text {
	if value == nil || *value == "" {
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
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func (r *PostgreSQL) classifyOutboxRetryError(ctx context.Context, id uuid.UUID, err error) error {
	if err == nil {
		return nil
	}
	if _, getErr := r.queries.GetOutboxEvent(ctx, pgUUID(id)); errors.Is(getErr, pgx.ErrNoRows) {
		return domain.ErrNotFound
	} else if getErr != nil {
		return getErr
	}
	return domain.ErrConflict
}

func (r *PostgreSQL) classifyBackgroundJobRetryError(ctx context.Context, id uuid.UUID, err error) error {
	if err == nil {
		return nil
	}
	if _, getErr := r.queries.GetBackgroundJob(ctx, pgUUID(id)); errors.Is(getErr, pgx.ErrNoRows) {
		return domain.ErrNotFound
	} else if getErr != nil {
		return getErr
	}
	return domain.ErrConflict
}
