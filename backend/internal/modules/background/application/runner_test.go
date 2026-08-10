package application

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"file-workshop/backend/internal/modules/background/domain"

	"github.com/google/uuid"
)

func TestRunnerPublishesSuccessfulEvent(t *testing.T) {
	now := time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC)
	event := testEvent("USER_CREATED", 1, 3)
	repository := newFakeRepository(event)
	handled := 0
	runner := newTestRunner(t, repository, []OutboxHandler{OutboxHandlerFunc{Types: []string{"USER_CREATED"}, Fn: func(context.Context, domain.OutboxEvent) error {
		handled++
		return nil
	}}}, now)

	processed, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}
	if processed != 1 || handled != 1 {
		t.Fatalf("processed=%d handled=%d, want 1/1", processed, handled)
	}
	if repository.published != 1 || repository.failed != 0 || repository.dead != 0 {
		t.Fatalf("unexpected marks: published=%d failed=%d dead=%d", repository.published, repository.failed, repository.dead)
	}
}

func TestRunnerRetriesRetryableError(t *testing.T) {
	now := time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC)
	event := testEvent("USER_CREATED", 1, 3)
	repository := newFakeRepository(event)
	runner := newTestRunner(t, repository, []OutboxHandler{OutboxHandlerFunc{Types: []string{"USER_CREATED"}, Fn: func(context.Context, domain.OutboxEvent) error {
		return domain.RetryableError("TEMPORARY_FAILURE", "temporary dependency failure")
	}}}, now)

	processed, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}
	if processed != 1 || repository.failed != 1 || repository.dead != 0 {
		t.Fatalf("processed=%d failed=%d dead=%d, want 1/1/0", processed, repository.failed, repository.dead)
	}
	if repository.nextRetryAt == nil || !repository.nextRetryAt.Equal(now.Add(5*time.Second)) {
		t.Fatalf("nextRetryAt=%v, want %v", repository.nextRetryAt, now.Add(5*time.Second))
	}
}

func TestRunnerMarksPermanentErrorDead(t *testing.T) {
	now := time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC)
	event := testEvent("USER_CREATED", 1, 3)
	repository := newFakeRepository(event)
	runner := newTestRunner(t, repository, []OutboxHandler{OutboxHandlerFunc{Types: []string{"USER_CREATED"}, Fn: func(context.Context, domain.OutboxEvent) error {
		return domain.PermanentError("UNSUPPORTED_EVENT", "unsupported event")
	}}}, now)

	processed, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}
	if processed != 1 || repository.failed != 0 || repository.dead != 1 {
		t.Fatalf("processed=%d failed=%d dead=%d, want 1/0/1", processed, repository.failed, repository.dead)
	}
}

func TestRunnerOnlyClaimsRegisteredEventTypes(t *testing.T) {
	now := time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC)
	repository := newFakeRepository(testEvent("USER_CREATED", 1, 3), testEvent("DOCUMENT_CREATED", 1, 3))
	runner := newTestRunner(t, repository, []OutboxHandler{OutboxHandlerFunc{Types: []string{"DOCUMENT_CREATED"}, Fn: func(context.Context, domain.OutboxEvent) error {
		return nil
	}}}, now)

	processed, err := runner.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed=%d, want 1", processed)
	}
	if repository.claims["USER_CREATED"] != 0 || repository.claims["DOCUMENT_CREATED"] != 1 {
		t.Fatalf("unexpected claims: %#v", repository.claims)
	}
}

func TestRunnerReturnsErrorWithoutHandlers(t *testing.T) {
	runner := newTestRunner(t, newFakeRepository(), nil, time.Now())
	_, err := runner.RunOnce(context.Background())
	if !errors.Is(err, domain.ErrNoHandlers) {
		t.Fatalf("RunOnce error=%v, want ErrNoHandlers", err)
	}
}

func newTestRunner(t *testing.T, repository *fakeRepository, handlers []OutboxHandler, now time.Time) *Runner {
	t.Helper()
	runner, err := NewRunner(repository, handlers, RunnerConfig{
		WorkerID:            "test-worker",
		Concurrency:         1,
		BatchSize:           10,
		PollInterval:        time.Millisecond,
		LeaseDuration:       time.Minute,
		HandlerTimeout:      time.Second,
		RetryInitialBackoff: 5 * time.Second,
		RetryMaxBackoff:     time.Minute,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return now })
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}
	return runner
}

func testEvent(eventType string, attempt, maxAttempts int32) domain.OutboxEvent {
	return domain.OutboxEvent{ID: uuid.Must(uuid.NewV7()), AggregateType: "TEST", AggregateID: uuid.Must(uuid.NewV7()), AggregateVersion: 1, EventType: eventType, EventSchemaVersion: 1, PayloadJSON: []byte(`{}`), DeduplicationKey: eventType + "-" + uuid.NewString(), Status: domain.OutboxStatusPending, AttemptCount: attempt, MaxAttempts: maxAttempts, RowVersion: 1}
}

type fakeRepository struct {
	events      []domain.OutboxEvent
	claims      map[string]int
	published   int
	failed      int
	dead        int
	nextRetryAt *time.Time
}

func newFakeRepository(events ...domain.OutboxEvent) *fakeRepository {
	return &fakeRepository{events: events, claims: map[string]int{}}
}

func (r *fakeRepository) ClaimOutboxEventsByType(_ context.Context, eventType, workerID string, batchSize int32, leaseUntil time.Time, now time.Time) ([]domain.OutboxEvent, error) {
	r.claims[eventType]++
	result := make([]domain.OutboxEvent, 0, batchSize)
	for _, event := range r.events {
		if event.EventType != eventType || int32(len(result)) >= batchSize {
			continue
		}
		event.Status = domain.OutboxStatusProcessing
		event.LockedBy = &workerID
		event.LockedAt = &now
		event.LeaseUntil = &leaseUntil
		result = append(result, event)
	}
	return result, nil
}

func (r *fakeRepository) MarkOutboxEventPublished(context.Context, domain.OutboxEvent, string, time.Time) (bool, error) {
	r.published++
	return true, nil
}

func (r *fakeRepository) MarkOutboxEventFailed(_ context.Context, _ domain.OutboxEvent, _ string, _, _ string, nextRetryAt time.Time, _ time.Time) (bool, error) {
	r.failed++
	r.nextRetryAt = &nextRetryAt
	return true, nil
}

func (r *fakeRepository) MarkOutboxEventDead(context.Context, domain.OutboxEvent, string, string, string, time.Time) (bool, error) {
	r.dead++
	return true, nil
}

func (r *fakeRepository) RenewOutboxEventLease(context.Context, domain.OutboxEvent, string, time.Time, time.Time) (bool, error) {
	return true, nil
}

func (r *fakeRepository) CountOutboxEventsByStatus(context.Context) ([]domain.OutboxStatusCount, error) {
	return nil, nil
}
