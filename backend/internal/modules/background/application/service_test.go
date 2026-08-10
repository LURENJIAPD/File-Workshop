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

	job, err := service.CancelBackgroundJob(context.Background(), adminActor(), jobID, 3, "任务不再需要执行")
	if err != nil {
		t.Fatalf("CancelBackgroundJob failed: %v", err)
	}
	if job.Status != domain.JobStatusCancelled {
		t.Fatalf("status=%s, want %s", job.Status, domain.JobStatusCancelled)
	}
	if repository.cancelReason != "manual cancel: 任务不再需要执行" {
		t.Fatalf("cancelReason=%q", repository.cancelReason)
	}
	if repository.cancelID != jobID || repository.cancelRowVersion != 3 || !repository.cancelNow.Equal(now) {
		t.Fatalf("unexpected cancel call: id=%s rowVersion=%d now=%s", repository.cancelID, repository.cancelRowVersion, repository.cancelNow)
	}
}

func TestServiceCancelBackgroundJobRequiresAdmin(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)

	_, err := service.CancelBackgroundJob(context.Background(), domain.Actor{UserID: uuid.Must(uuid.NewV7()), Role: "USER"}, uuid.Must(uuid.NewV7()), 1, "取消")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("error=%v, want ErrForbidden", err)
	}
}

func TestServiceCancelBackgroundJobValidatesInput(t *testing.T) {
	service := NewService(&fakeOperationsRepository{}, nil, time.Now)

	_, err := service.CancelBackgroundJob(context.Background(), adminActor(), uuid.Must(uuid.NewV7()), 0, "取消")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("rowVersion error=%v, want ErrInvalidInput", err)
	}
	_, err = service.CancelBackgroundJob(context.Background(), adminActor(), uuid.Must(uuid.NewV7()), 1, " ")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("reason error=%v, want ErrInvalidInput", err)
	}
}

func adminActor() domain.Actor {
	return domain.Actor{UserID: uuid.Must(uuid.NewV7()), SessionID: uuid.Must(uuid.NewV7()), Role: domain.SystemRoleAdmin}
}

type fakeOperationsRepository struct {
	cancelID         uuid.UUID
	cancelRowVersion int64
	cancelReason     string
	cancelNow        time.Time
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

func (r *fakeOperationsRepository) RetryBackgroundJob(context.Context, uuid.UUID, int64, string, time.Time) (domain.BackgroundJob, error) {
	return domain.BackgroundJob{}, nil
}

func (r *fakeOperationsRepository) CancelBackgroundJob(_ context.Context, id uuid.UUID, rowVersion int64, reason string, now time.Time) (domain.BackgroundJob, error) {
	r.cancelID = id
	r.cancelRowVersion = rowVersion
	r.cancelReason = reason
	r.cancelNow = now
	return domain.BackgroundJob{ID: id, Status: domain.JobStatusCancelled, RowVersion: rowVersion + 1}, nil
}
