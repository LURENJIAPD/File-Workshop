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

func adminActor() domain.Actor {
	return domain.Actor{UserID: uuid.Must(uuid.NewV7()), SessionID: uuid.Must(uuid.NewV7()), Role: domain.SystemRoleAdmin}
}

type fakeOperationsRepository struct {
	outboxCounts     []domain.OutboxStatusCount
	jobCounts        []domain.OutboxStatusCount
	conflictID       uuid.UUID
	missingID        uuid.UUID
	retryReason      string
	cancelID         uuid.UUID
	cancelRowVersion int64
	cancelReason     string
	cancelNow        time.Time
}

func (r *fakeOperationsRepository) CountOutboxEventsByStatus(context.Context) ([]domain.OutboxStatusCount, error) {
	return r.outboxCounts, nil
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
