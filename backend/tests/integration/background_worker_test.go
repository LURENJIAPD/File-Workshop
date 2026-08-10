package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	backgroundapplication "file-workshop/backend/internal/modules/background/application"
	backgrounddomain "file-workshop/backend/internal/modules/background/domain"
	backgroundrepository "file-workshop/backend/internal/modules/background/repository"
	"file-workshop/backend/internal/platform/config"
	"file-workshop/backend/internal/platform/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestBackgroundWorkerOutboxLifecycle(t *testing.T) {
	if value := getenv(integrationEnvironment); value != "1" {
		t.Skip("set FILE_WORKSHOP_RUN_INTEGRATION=1 to run local dependency integration tests")
	}
	backendRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve backend root: %v", err)
	}
	t.Setenv("FILE_WORKSHOP_ENV_FILE", filepath.Join(backendRoot, ".env"))
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	connection, err := pgx.Connect(ctx, cfg.PostgreSQL.ConnectionString())
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	if _, err := connection.Exec(ctx, "SET search_path TO "+pgx.Identifier{cfg.PostgreSQL.Schema}.Sanitize()+",public"); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	prefix := "background-worker-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = connection.Exec(cleanupContext, "DELETE FROM outbox_events WHERE deduplication_key LIKE $1", prefix+"%")
		_ = connection.Close(cleanupContext)
	})

	successID := insertOutboxFixture(t, ctx, connection, prefix+"-success", "TEST_OUTBOX_SUCCESS", 3)
	unsupportedID := insertOutboxFixture(t, ctx, connection, prefix+"-unsupported", "TEST_OUTBOX_UNSUPPORTED", 3)
	retryID := insertOutboxFixture(t, ctx, connection, prefix+"-retry", "TEST_OUTBOX_RETRY", 3)
	deadID := insertOutboxFixture(t, ctx, connection, prefix+"-dead", "TEST_OUTBOX_DEAD", 1)

	pool, err := database.OpenPostgreSQL(ctx, cfg.App, cfg.PostgreSQL)
	if err != nil {
		t.Fatalf("open PostgreSQL pool: %v", err)
	}
	defer pool.Close()
	repository := backgroundrepository.NewPostgreSQL(pool)
	now := time.Date(2026, 8, 10, 2, 0, 0, 0, time.UTC)
	runner, err := backgroundapplication.NewRunner(repository, []backgroundapplication.OutboxHandler{
		backgroundapplication.OutboxHandlerFunc{Types: []string{"TEST_OUTBOX_SUCCESS"}, Fn: func(context.Context, backgrounddomain.OutboxEvent) error { return nil }},
		backgroundapplication.OutboxHandlerFunc{Types: []string{"TEST_OUTBOX_RETRY"}, Fn: func(context.Context, backgrounddomain.OutboxEvent) error {
			return backgrounddomain.RetryableError("TEMPORARY_FAILURE", "temporary test failure")
		}},
		backgroundapplication.OutboxHandlerFunc{Types: []string{"TEST_OUTBOX_DEAD"}, Fn: func(context.Context, backgrounddomain.OutboxEvent) error {
			return backgrounddomain.RetryableError("TEMPORARY_FAILURE", "temporary test failure")
		}},
	}, backgroundapplication.RunnerConfig{
		WorkerID:            "integration-worker",
		Concurrency:         1,
		BatchSize:           5,
		PollInterval:        time.Millisecond,
		LeaseDuration:       time.Minute,
		HandlerTimeout:      time.Second,
		RetryInitialBackoff: time.Second,
		RetryMaxBackoff:     time.Minute,
	}, nil, func() time.Time { return now })
	if err != nil {
		t.Fatalf("create runner: %v", err)
	}
	processed, err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if processed != 3 {
		t.Fatalf("processed=%d, want 3", processed)
	}
	assertOutboxStatus(t, ctx, connection, successID, "PUBLISHED")
	assertOutboxStatus(t, ctx, connection, unsupportedID, "PENDING")
	assertOutboxStatus(t, ctx, connection, retryID, "FAILED")
	assertOutboxStatus(t, ctx, connection, deadID, "DEAD")
}

func insertOutboxFixture(t *testing.T, ctx context.Context, connection *pgx.Conn, deduplicationKey, eventType string, maxAttempts int) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	aggregateID := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	if _, err := connection.Exec(ctx, `
		INSERT INTO outbox_events (
			outbox_event_id, aggregate_type, aggregate_id, aggregate_version, event_type,
			event_schema_version, payload_json, deduplication_key, correlation_id,
			max_attempts, available_at, created_at, updated_at
		) VALUES ($1, 'TEST', $2, 1, $3, 1, '{}'::jsonb, $4, $5, $6, $7, $7, $7)`,
		id, aggregateID, eventType, deduplicationKey, uuid.Must(uuid.NewV7()), maxAttempts, now,
	); err != nil {
		t.Fatalf("insert outbox fixture: %v", err)
	}
	return id
}

func assertOutboxStatus(t *testing.T, ctx context.Context, connection *pgx.Conn, id uuid.UUID, expected string) {
	t.Helper()
	var status string
	if err := connection.QueryRow(ctx, "SELECT status FROM outbox_events WHERE outbox_event_id = $1", id).Scan(&status); err != nil {
		t.Fatalf("query outbox status: %v", err)
	}
	if status != expected {
		t.Fatalf("outbox status=%s, want %s", status, expected)
	}
}

func getenv(key string) string {
	return strings.TrimSpace(strings.TrimSpace(strings.Trim(os.Getenv(key), "\x00")))
}
