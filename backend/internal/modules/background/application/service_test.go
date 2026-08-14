package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"file-workshop/backend/internal/modules/background/domain"

	"github.com/google/uuid"
)

func TestServiceCancelsBackgroundJob(t *testing.T) {
	now := time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC)
	repository := &fakeOperationsRepository{}
	service := NewService(repository, nil, func() time.Time { return now })
	jobID := uuid.Must(uuid.NewV7())

	job, err := service.CancelBackgroundJob(context.Background(), adminActor(), jobID, 3, "job no longer needed")
	if err != nil {
		t.Fatalf("CancelBackgroundJob failed: %v", err)
	}
	if job.Status != domain.JobStatusCancelled {
		t.Fatalf("status=%s, want %s", job.Status, domain.JobStatusCancelled)
	}
	if repository.cancelReason != "manual cancel: job no longer needed" {
		t.Fatalf("cancelReason=%q", repository.cancelReason)
	}
	if repository.cancelID != jobID || repository.cancelRowVersion != 3 || !repository.cancelNow.Equal(now) {
		t.Fatalf("unexpected cancel call: id=%s rowVersion=%d now=%s", repository.cancelID, repository.cancelRowVersion, repository.cancelNow)
	}
}

func TestServiceCancelBackgroundJobRequiresAdmin(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)

	_, err := service.CancelBackgroundJob(context.Background(), domain.Actor{UserID: uuid.Must(uuid.NewV7()), Role: "USER"}, uuid.Must(uuid.NewV7()), 1, "cancel")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error=%v, want ErrForbidden", err)
	}
}

func TestServiceCancelBackgroundJobValidatesInput(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)

	_, err := service.CancelBackgroundJob(context.Background(), adminActor(), uuid.Must(uuid.NewV7()), 0, "cancel")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("rowVersion error=%v, want ErrInvalidInput", err)
	}
	_, err = service.CancelBackgroundJob(context.Background(), adminActor(), uuid.Must(uuid.NewV7()), 1, " ")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("reason error=%v, want ErrInvalidInput", err)
	}
}

func TestServiceGetsSummary(t *testing.T) {
	repository := &fakeOperationsRepository{
		outboxCounts: []domain.OutboxStatusCount{{Status: domain.OutboxStatusPending, Count: 2}},
		jobCounts:    []domain.OutboxStatusCount{{Status: domain.JobStatusFailed, Count: 3}},
	}
	service := NewService(repository, nil, time.Now)

	summary, err := service.GetSummary(context.Background(), adminActor())
	if err != nil {
		t.Fatalf("GetSummary failed: %v", err)
	}
	if summary.OutboxEvents[0].Count != 2 || summary.BackgroundJobs[0].Count != 3 {
		t.Fatalf("summary=%#v", summary)
	}
}

func TestServiceGetsFailureSummary(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	repository := &fakeOperationsRepository{
		outboxFailures: []domain.FailureSummaryItem{{ErrorCode: "AUDIT_WRITE_FAILED", Count: 2, LatestAt: now}},
		jobFailures:    []domain.FailureSummaryItem{{ErrorCode: "INDEX_FAILED", Count: 3, LatestAt: now}},
	}
	service := NewService(repository, nil, time.Now)

	summary, err := service.GetFailureSummary(context.Background(), adminActor())
	if err != nil {
		t.Fatalf("GetFailureSummary failed: %v", err)
	}
	if summary.OutboxEvents[0].ErrorCode != "AUDIT_WRITE_FAILED" || summary.BackgroundJobs[0].Count != 3 {
		t.Fatalf("summary=%#v", summary)
	}
}

func TestServiceGetsQueueLagSummary(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	oldest := now.Add(-5 * time.Minute)
	repository := &fakeOperationsRepository{
		queueLag: domain.QueueLagSummary{
			OutboxEvents:   domain.QueueLagItem{DuePendingCount: 2, DueFailedCount: 1, ExpiredProcessingCount: 1, OldestDueAt: &oldest},
			BackgroundJobs: domain.QueueLagItem{DuePendingCount: 3, DueFailedCount: 0, ExpiredProcessingCount: 2, OldestDueAt: &oldest},
		},
	}
	service := NewService(repository, nil, func() time.Time { return now })

	summary, err := service.GetQueueLagSummary(context.Background(), adminActor())
	if err != nil {
		t.Fatalf("GetQueueLagSummary failed: %v", err)
	}
	if !repository.queueLagNow.Equal(now) {
		t.Fatalf("queueLagNow=%s, want %s", repository.queueLagNow, now)
	}
	if summary.OutboxEvents.DuePendingCount != 2 || summary.BackgroundJobs.ExpiredProcessingCount != 2 {
		t.Fatalf("summary=%#v", summary)
	}
}

func TestServiceGetQueueLagSummaryRequiresAdmin(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)

	_, err := service.GetQueueLagSummary(context.Background(), domain.Actor{UserID: uuid.Must(uuid.NewV7()), Role: "USER"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error=%v, want ErrForbidden", err)
	}
}

func TestServiceGetsHealthSummary(t *testing.T) {
	now := time.Date(2026, 8, 14, 13, 0, 0, 0, time.UTC)
	oldest := now.Add(-10 * time.Minute)
	repository := &fakeOperationsRepository{
		outboxCounts: []domain.OutboxStatusCount{{Status: domain.OutboxStatusDead, Count: 1}},
		jobCounts:    []domain.OutboxStatusCount{{Status: domain.JobStatusDead, Count: 2}},
		queueLag: domain.QueueLagSummary{
			OutboxEvents:   domain.QueueLagItem{DuePendingCount: 4, DueFailedCount: 1, ExpiredProcessingCount: 0, OldestDueAt: &oldest},
			BackgroundJobs: domain.QueueLagItem{DuePendingCount: 0, DueFailedCount: 0, ExpiredProcessingCount: 3, OldestDueAt: &oldest},
		},
	}
	service := NewService(repository, nil, func() time.Time { return now })

	summary, err := service.GetHealthSummary(context.Background(), adminActor())
	if err != nil {
		t.Fatalf("GetHealthSummary failed: %v", err)
	}
	if summary.Status != domain.HealthStatusAttentionRequired {
		t.Fatalf("status=%s, want %s", summary.Status, domain.HealthStatusAttentionRequired)
	}
	if !repository.queueLagNow.Equal(now) {
		t.Fatalf("queueLagNow=%s, want %s", repository.queueLagNow, now)
	}
	if len(summary.Signals) != 5 {
		t.Fatalf("signals=%#v", summary.Signals)
	}
	if summary.Signals[0].Code != "OUTBOX_DEAD_EVENTS" || summary.Signals[0].Severity != domain.HealthSignalSeverityCritical {
		t.Fatalf("first signal=%#v", summary.Signals[0])
	}
}

func TestServiceGetsHealthyHealthSummary(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)

	summary, err := service.GetHealthSummary(context.Background(), adminActor())
	if err != nil {
		t.Fatalf("GetHealthSummary failed: %v", err)
	}
	if summary.Status != domain.HealthStatusOK || len(summary.Signals) != 0 {
		t.Fatalf("summary=%#v", summary)
	}
}

func TestServiceGetHealthSummaryRequiresAdmin(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)

	_, err := service.GetHealthSummary(context.Background(), domain.Actor{UserID: uuid.Must(uuid.NewV7()), Role: "USER"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error=%v, want ErrForbidden", err)
	}
}

func TestServiceGetFailureSummaryRequiresAdmin(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)

	_, err := service.GetFailureSummary(context.Background(), domain.Actor{UserID: uuid.Must(uuid.NewV7()), Role: "USER"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error=%v, want ErrForbidden", err)
	}
}

func TestServiceRecoversExpiredLeases(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	repository := &fakeOperationsRepository{
		leaseRecovery: domain.LeaseRecoveryResult{
			OutboxEvents:   domain.LeaseRecoveryItem{Recovered: 2, Retryable: 1, Dead: 1},
			BackgroundJobs: domain.LeaseRecoveryItem{Recovered: 3, Retryable: 2, Dead: 1},
		},
	}
	service := NewService(repository, nil, func() time.Time { return now })

	result, err := service.RecoverExpiredLeases(context.Background(), adminActor(), 0, "worker crash confirmed")
	if err != nil {
		t.Fatalf("RecoverExpiredLeases failed: %v", err)
	}
	if repository.leaseRecoveryBatchSize != domain.DefaultLeaseRecoveryBatchSize {
		t.Fatalf("batchSize=%d, want default %d", repository.leaseRecoveryBatchSize, domain.DefaultLeaseRecoveryBatchSize)
	}
	if repository.leaseRecoveryReason != "lease expired: worker crash confirmed" || !repository.leaseRecoveryNow.Equal(now) {
		t.Fatalf("unexpected recovery call: reason=%q now=%s", repository.leaseRecoveryReason, repository.leaseRecoveryNow)
	}
	if result.OutboxEvents.Dead != 1 || result.BackgroundJobs.Retryable != 2 {
		t.Fatalf("result=%#v", result)
	}
}

func TestServiceRecoverExpiredLeasesValidatesInput(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)

	_, err := service.RecoverExpiredLeases(context.Background(), adminActor(), domain.MaxLeaseRecoveryBatchSize+1, "too many")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("batchSize error=%v, want ErrInvalidInput", err)
	}
	_, err = service.RecoverExpiredLeases(context.Background(), adminActor(), 10, " ")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("reason error=%v, want ErrInvalidInput", err)
	}
}

func TestServiceRecoverExpiredLeasesRequiresAdmin(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)

	_, err := service.RecoverExpiredLeases(context.Background(), domain.Actor{UserID: uuid.Must(uuid.NewV7()), Role: "USER"}, 10, "recover")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error=%v, want ErrForbidden", err)
	}
}

func TestServiceBatchRetriesOutboxEventsWithPartialFailures(t *testing.T) {
	now := time.Date(2026, 8, 14, 11, 0, 0, 0, time.UTC)
	successID := uuid.Must(uuid.NewV7())
	conflictID := uuid.Must(uuid.NewV7())
	missingID := uuid.Must(uuid.NewV7())
	repository := &fakeOperationsRepository{conflictID: conflictID, missingID: missingID}
	service := NewService(repository, nil, func() time.Time { return now })

	result, err := service.BatchRetryOutboxEvents(context.Background(), adminActor(), []domain.BatchOutboxEventItem{
		{ID: successID, RowVersion: 1},
		{ID: conflictID, RowVersion: 2},
		{ID: missingID, RowVersion: 3},
	}, "retry selected outbox events")
	if err != nil {
		t.Fatalf("BatchRetryOutboxEvents failed: %v", err)
	}
	if result.Succeeded != 1 || result.Failed != 2 || len(result.Items) != 3 {
		t.Fatalf("result=%#v", result)
	}
	if repository.outboxRetryReason != "manual batch retry: retry selected outbox events" {
		t.Fatalf("outboxRetryReason=%q", repository.outboxRetryReason)
	}
	if result.Items[0].Event == nil || result.Items[0].Event.Status != domain.OutboxStatusPending {
		t.Fatalf("successful item=%#v", result.Items[0])
	}
}

func TestServiceBatchRetryOutboxEventsValidatesDuplicates(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)
	id := uuid.Must(uuid.NewV7())

	_, err := service.BatchRetryOutboxEvents(context.Background(), adminActor(), []domain.BatchOutboxEventItem{
		{ID: id, RowVersion: 1},
		{ID: id, RowVersion: 1},
	}, "retry duplicates")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("error=%v, want ErrInvalidInput", err)
	}
}

func TestServiceBatchRetriesBackgroundJobsWithPartialFailures(t *testing.T) {
	now := time.Date(2026, 8, 10, 5, 0, 0, 0, time.UTC)
	successID := uuid.Must(uuid.NewV7())
	conflictID := uuid.Must(uuid.NewV7())
	missingID := uuid.Must(uuid.NewV7())
	repository := &fakeOperationsRepository{conflictID: conflictID, missingID: missingID}
	service := NewService(repository, nil, func() time.Time { return now })

	result, err := service.BatchRetryBackgroundJobs(context.Background(), adminActor(), []domain.BatchJobItem{
		{ID: successID, RowVersion: 1},
		{ID: conflictID, RowVersion: 2},
		{ID: missingID, RowVersion: 3},
	}, "retry selected jobs")
	if err != nil {
		t.Fatalf("BatchRetryBackgroundJobs failed: %v", err)
	}
	if result.Succeeded != 1 || result.Failed != 2 || len(result.Items) != 3 {
		t.Fatalf("result=%#v", result)
	}
	if repository.retryReason != "manual batch retry: retry selected jobs" {
		t.Fatalf("retryReason=%q", repository.retryReason)
	}
}

func TestServiceBatchCancelBackgroundJobsValidatesDuplicates(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)
	id := uuid.Must(uuid.NewV7())

	_, err := service.BatchCancelBackgroundJobs(context.Background(), adminActor(), []domain.BatchJobItem{
		{ID: id, RowVersion: 1},
		{ID: id, RowVersion: 1},
	}, "cancel duplicates")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("error=%v, want ErrInvalidInput", err)
	}
}

func TestServiceBatchMarksBackgroundJobsDeadWithPartialFailures(t *testing.T) {
	now := time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC)
	successID := uuid.Must(uuid.NewV7())
	conflictID := uuid.Must(uuid.NewV7())
	repository := &fakeOperationsRepository{conflictID: conflictID}
	service := NewService(repository, nil, func() time.Time { return now })

	result, err := service.BatchMarkBackgroundJobsDead(context.Background(), adminActor(), []domain.BatchJobItem{
		{ID: successID, RowVersion: 1},
		{ID: conflictID, RowVersion: 2},
	}, "operator accepted failure")
	if err != nil {
		t.Fatalf("BatchMarkBackgroundJobsDead failed: %v", err)
	}
	if result.Succeeded != 1 || result.Failed != 1 || len(result.Items) != 2 {
		t.Fatalf("result=%#v", result)
	}
	if repository.deadReason != "manual batch dead letter: operator accepted failure" {
		t.Fatalf("deadReason=%q", repository.deadReason)
	}
	if result.Items[0].Job == nil || result.Items[0].Job.Status != domain.JobStatusDead {
		t.Fatalf("successful item=%#v", result.Items[0])
	}
}

func TestServiceBatchSkipsBackgroundJobs(t *testing.T) {
	now := time.Date(2026, 8, 10, 7, 0, 0, 0, time.UTC)
	jobID := uuid.Must(uuid.NewV7())
	repository := &fakeOperationsRepository{}
	service := NewService(repository, nil, func() time.Time { return now })

	result, err := service.BatchSkipBackgroundJobs(context.Background(), adminActor(), []domain.BatchJobItem{{ID: jobID, RowVersion: 8}}, "task superseded")
	if err != nil {
		t.Fatalf("BatchSkipBackgroundJobs failed: %v", err)
	}
	if result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("result=%#v", result)
	}
	if repository.skipReason != "manual batch skip: task superseded" {
		t.Fatalf("skipReason=%q", repository.skipReason)
	}
	if result.Items[0].Job == nil || result.Items[0].Job.Status != domain.JobStatusSkipped {
		t.Fatalf("successful item=%#v", result.Items[0])
	}
}

func TestServiceBatchSkipBackgroundJobsRequiresAdmin(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)

	_, err := service.BatchSkipBackgroundJobs(context.Background(), domain.Actor{UserID: uuid.Must(uuid.NewV7()), Role: "USER"}, []domain.BatchJobItem{{ID: uuid.Must(uuid.NewV7()), RowVersion: 1}}, "skip")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error=%v, want ErrForbidden", err)
	}
}

func adminActor() domain.Actor {
	return domain.Actor{UserID: uuid.Must(uuid.NewV7()), SessionID: uuid.Must(uuid.NewV7()), Role: domain.SystemRoleAdmin}
}

type fakeOperationsRepository struct {
	outboxCounts           []domain.OutboxStatusCount
	jobCounts              []domain.OutboxStatusCount
	outboxFailures         []domain.FailureSummaryItem
	jobFailures            []domain.FailureSummaryItem
	queueLag               domain.QueueLagSummary
	queueLagNow            time.Time
	leaseRecovery          domain.LeaseRecoveryResult
	leaseRecoveryBatchSize int
	leaseRecoveryReason    string
	leaseRecoveryNow       time.Time
	conflictID             uuid.UUID
	missingID              uuid.UUID
	outboxRetryReason      string
	retryReason            string
	cancelID               uuid.UUID
	cancelRowVersion       int64
	cancelReason           string
	cancelNow              time.Time
	deadReason             string
	skipReason             string
}

func (r *fakeOperationsRepository) CountOutboxEventsByStatus(context.Context) ([]domain.OutboxStatusCount, error) {
	return r.outboxCounts, nil
}

func (r *fakeOperationsRepository) CountOutboxFailuresByErrorCode(context.Context) ([]domain.FailureSummaryItem, error) {
	return r.outboxFailures, nil
}

func (r *fakeOperationsRepository) ListOutboxEvents(context.Context, domain.OutboxListFilter) (domain.OutboxListResult, error) {
	return domain.OutboxListResult{}, nil
}

func (r *fakeOperationsRepository) RetryOutboxEvent(_ context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.OutboxEvent, error) {
	r.outboxRetryReason = reason
	if id == r.conflictID {
		return domain.OutboxEvent{}, domain.ErrConflict
	}
	if id == r.missingID {
		return domain.OutboxEvent{}, domain.ErrNotFound
	}
	return domain.OutboxEvent{ID: id, Status: domain.OutboxStatusPending, RowVersion: rowVersion + 1, AvailableAt: now}, nil
}

func (r *fakeOperationsRepository) ListBackgroundJobs(context.Context, domain.JobListFilter) (domain.JobListResult, error) {
	return domain.JobListResult{}, nil
}

func (r *fakeOperationsRepository) CountBackgroundJobsByStatus(context.Context) ([]domain.OutboxStatusCount, error) {
	return r.jobCounts, nil
}

func (r *fakeOperationsRepository) GetQueueLagSummary(_ context.Context, now time.Time) (domain.QueueLagSummary, error) {
	r.queueLagNow = now
	return r.queueLag, nil
}

func (r *fakeOperationsRepository) CountBackgroundJobFailuresByErrorCode(context.Context) ([]domain.FailureSummaryItem, error) {
	return r.jobFailures, nil
}

func (r *fakeOperationsRepository) RecoverExpiredLeases(_ context.Context, batchSize int, reason string, now time.Time) (domain.LeaseRecoveryResult, error) {
	r.leaseRecoveryBatchSize = batchSize
	r.leaseRecoveryReason = reason
	r.leaseRecoveryNow = now
	return r.leaseRecovery, nil
}

func (r *fakeOperationsRepository) RetryBackgroundJob(_ context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error) {
	r.retryReason = reason
	if id == r.conflictID {
		return domain.BackgroundJob{}, domain.ErrConflict
	}
	if id == r.missingID {
		return domain.BackgroundJob{}, domain.ErrNotFound
	}
	return domain.BackgroundJob{ID: id, Status: domain.JobStatusPending, RowVersion: rowVersion + 1, AvailableAt: now}, nil
}

func (r *fakeOperationsRepository) CancelBackgroundJob(_ context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error) {
	r.cancelID = id
	r.cancelRowVersion = rowVersion
	r.cancelReason = reason
	r.cancelNow = now
	return domain.BackgroundJob{ID: id, Status: domain.JobStatusCancelled, RowVersion: rowVersion + 1}, nil
}

func (r *fakeOperationsRepository) DeadLetterBackgroundJob(_ context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error) {
	r.deadReason = reason
	if id == r.conflictID {
		return domain.BackgroundJob{}, domain.ErrConflict
	}
	if id == r.missingID {
		return domain.BackgroundJob{}, domain.ErrNotFound
	}
	return domain.BackgroundJob{ID: id, Status: domain.JobStatusDead, RowVersion: rowVersion + 1, CompletedAt: &now}, nil
}

func (r *fakeOperationsRepository) SkipBackgroundJob(_ context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error) {
	r.skipReason = reason
	if id == r.conflictID {
		return domain.BackgroundJob{}, domain.ErrConflict
	}
	if id == r.missingID {
		return domain.BackgroundJob{}, domain.ErrNotFound
	}
	return domain.BackgroundJob{ID: id, Status: domain.JobStatusSkipped, RowVersion: rowVersion + 1, CompletedAt: &now}, nil
}
