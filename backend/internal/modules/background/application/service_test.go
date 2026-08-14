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

func TestServiceGetFailureSummaryRequiresAdmin(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)

	_, err := service.GetFailureSummary(context.Background(), domain.Actor{UserID: uuid.Must(uuid.NewV7()), Role: "USER"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error=%v, want ErrForbidden", err)
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
	outboxCounts     []domain.OutboxStatusCount
	jobCounts        []domain.OutboxStatusCount
	outboxFailures   []domain.FailureSummaryItem
	jobFailures      []domain.FailureSummaryItem
	conflictID       uuid.UUID
	missingID        uuid.UUID
	retryReason      string
	cancelID         uuid.UUID
	cancelRowVersion int64
	cancelReason     string
	cancelNow        time.Time
	deadReason       string
	skipReason       string
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

func (r *fakeOperationsRepository) RetryOutboxEvent(context.Context, uuid.UUID, int64, string, time.Time) (domain.OutboxEvent, error) {
	return domain.OutboxEvent{}, nil
}

func (r *fakeOperationsRepository) ListBackgroundJobs(context.Context, domain.JobListFilter) (domain.JobListResult, error) {
	return domain.JobListResult{}, nil
}

func (r *fakeOperationsRepository) CountBackgroundJobsByStatus(context.Context) ([]domain.OutboxStatusCount, error) {
	return r.jobCounts, nil
}

func (r *fakeOperationsRepository) CountBackgroundJobFailuresByErrorCode(context.Context) ([]domain.FailureSummaryItem, error) {
	return r.jobFailures, nil
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
