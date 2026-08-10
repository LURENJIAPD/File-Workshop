package application

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"file-workshop/backend/internal/modules/background/domain"

	"github.com/google/uuid"
)

func TestJobRunnerMarksSuccessAndDead(t *testing.T) {
	now := time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC)
	repository := newFakeJobRepository(testJob("HASH", 1, 1), testJob("SCAN", 1, 1))
	runner, err := NewJobRunner(repository, []JobHandler{
		JobHandlerFunc{Types: []string{"HASH"}, Fn: func(context.Context, domain.BackgroundJob) error { return nil }},
		JobHandlerFunc{Types: []string{"SCAN"}, Fn: func(context.Context, domain.BackgroundJob) error {
			return domain.RetryableError("SCAN_FAILED", "scan failed")
		}},
	}, RunnerConfig{WorkerID: "test-worker", Concurrency: 1, BatchSize: 10, PollInterval: time.Millisecond, LeaseDuration: time.Minute, HandlerTimeout: time.Second, RetryInitialBackoff: time.Second, RetryMaxBackoff: time.Minute}, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return now })
	if err != nil {
		t.Fatalf("create job runner: %v", err)
	}
	processed, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}
	if processed != 2 || repository.success != 1 || repository.dead != 1 || repository.failed != 0 {
		t.Fatalf("processed=%d success=%d failed=%d dead=%d, want 2/1/0/1", processed, repository.success, repository.failed, repository.dead)
	}
}

func testJob(jobType string, attempt, maxAttempts int32) domain.BackgroundJob {
	return domain.BackgroundJob{ID: uuid.Must(uuid.NewV7()), JobType: jobType, PayloadSchemaVersion: 1, PayloadJSON: []byte(`{}`), DeduplicationKey: jobType + "-" + uuid.NewString(), Status: domain.JobStatusPending, AttemptCount: attempt, MaxAttempts: maxAttempts, RowVersion: 1}
}

type fakeJobRepository struct {
	jobs    []domain.BackgroundJob
	success int
	failed  int
	dead    int
}

func newFakeJobRepository(jobs ...domain.BackgroundJob) *fakeJobRepository {
	return &fakeJobRepository{jobs: jobs}
}

func (r *fakeJobRepository) EnqueueJob(context.Context, domain.EnqueueJobInput, time.Time) (domain.BackgroundJob, error) {
	return domain.BackgroundJob{}, nil
}

func (r *fakeJobRepository) ClaimBackgroundJobsByType(_ context.Context, jobType, workerID string, batchSize int32, leaseUntil time.Time, now time.Time) ([]domain.BackgroundJob, error) {
	result := make([]domain.BackgroundJob, 0, batchSize)
	for _, job := range r.jobs {
		if job.JobType != jobType || int32(len(result)) >= batchSize {
			continue
		}
		job.Status = domain.JobStatusProcessing
		job.LockedBy = &workerID
		job.LockedAt = &now
		job.LeaseUntil = &leaseUntil
		result = append(result, job)
	}
	return result, nil
}

func (r *fakeJobRepository) MarkBackgroundJobSuccess(context.Context, domain.BackgroundJob, string, time.Time) (bool, error) {
	r.success++
	return true, nil
}

func (r *fakeJobRepository) MarkBackgroundJobFailed(context.Context, domain.BackgroundJob, string, string, string, time.Time, time.Time) (bool, error) {
	r.failed++
	return true, nil
}

func (r *fakeJobRepository) MarkBackgroundJobDead(context.Context, domain.BackgroundJob, string, string, string, time.Time) (bool, error) {
	r.dead++
	return true, nil
}

func (r *fakeJobRepository) RenewBackgroundJobLease(context.Context, domain.BackgroundJob, string, time.Time, time.Time) (bool, error) {
	return true, nil
}

func (r *fakeJobRepository) CountBackgroundJobsByStatus(context.Context) ([]domain.OutboxStatusCount, error) {
	return nil, nil
}
