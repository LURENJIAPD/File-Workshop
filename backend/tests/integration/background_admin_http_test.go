package integration_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/app"
	identitydomain "file-workshop/backend/internal/modules/identity/domain"
	"file-workshop/backend/internal/platform/config"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestBackgroundAdministrationHTTPWorkflow(t *testing.T) {
	if os.Getenv(integrationEnvironment) != "1" {
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	connection, err := pgx.Connect(ctx, cfg.PostgreSQL.ConnectionString())
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	if _, err := connection.Exec(ctx, "SET search_path TO "+pgx.Identifier{cfg.PostgreSQL.Schema}.Sanitize()+",public"); err != nil {
		t.Fatalf("set search path: %v", err)
	}

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	prefix := "background-admin-" + suffix
	adminID := uuid.Must(uuid.NewV7())
	userID := uuid.Must(uuid.NewV7())
	adminUsername := "background_admin_" + suffix
	userUsername := "background_user_" + suffix
	adminPassword := "Admin-" + suffix + "!Aa1"
	userPassword := "User-" + suffix + "!Aa1"
	outboxID := uuid.Nil
	jobID := uuid.Nil

	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		ids := []uuid.UUID{adminID, userID}
		_, _ = connection.Exec(cleanupContext, "DELETE FROM outbox_events WHERE deduplication_key LIKE $1", prefix+"%")
		_, _ = connection.Exec(cleanupContext, "DELETE FROM background_jobs WHERE deduplication_key LIKE $1", prefix+"%")
		_, _ = connection.Exec(cleanupContext, "DELETE FROM login_attempts WHERE username_normalized = ANY($1::text[])", []string{adminUsername, userUsername})
		_, _ = connection.Exec(cleanupContext, "DELETE FROM session_refresh_tokens WHERE user_session_id IN (SELECT user_session_id FROM user_sessions WHERE user_id = ANY($1::uuid[]))", ids)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM user_sessions WHERE user_id = ANY($1::uuid[])", ids)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM user_credentials WHERE user_id = ANY($1::uuid[])", ids)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM principal_security_versions WHERE user_id = ANY($1::uuid[])", ids)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM users WHERE user_id = ANY($1::uuid[])", ids)
		_ = connection.Close(cleanupContext)
	})

	insertBackgroundHTTPUserFixture(t, ctx, connection, adminID, adminUsername, adminPassword, "SYSTEM_ADMIN")
	insertBackgroundHTTPUserFixture(t, ctx, connection, userID, userUsername, userPassword, "USER")
	outboxID = insertFailedOutboxFixture(t, ctx, connection, prefix+"-outbox")
	jobID = insertFailedBackgroundJobFixture(t, ctx, connection, prefix+"-job")

	cfg.HTTP.Port = availablePort(t)
	cfg.HTTP.ShutdownTimeout = 5 * time.Second
	serverContext, stopServer := context.WithCancel(context.Background())
	t.Cleanup(stopServer)
	serverErrors := make(chan error, 1)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	go func() { serverErrors <- app.RunServer(serverContext, cfg, logger) }()
	waitForReadiness(t, cfg.HTTP.Address(), serverErrors)
	baseURL := "http://" + cfg.HTTP.Address()
	client := &http.Client{Timeout: 10 * time.Second}

	adminLogin := postLogin(t, client, baseURL, adminUsername, adminPassword)
	userLogin := postLogin(t, client, baseURL, userUsername, userPassword)
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminLogin.AccessToken, "Content-Type": "application/json"}
	userHeaders := map[string]string{"Authorization": "Bearer " + userLogin.AccessToken}

	forbiddenResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/background/jobs?page=1&pageSize=50", nil, userHeaders)
	assertErrorCode(t, forbiddenResponse, http.StatusForbidden, "AUTH_FORBIDDEN")

	outboxListResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/background/outbox-events?page=1&pageSize=50&status=FAILED&eventType=BACKGROUND_HTTP_TEST", nil, adminHeaders)
	assertStatus(t, outboxListResponse, http.StatusOK)
	var outboxList api.BackgroundOutboxEventListResponse
	decodeResponse(t, outboxListResponse, &outboxList)
	if outboxList.Page != 1 || outboxList.PageSize != 50 || len(outboxList.Items) != 1 || uuid.UUID(outboxList.Items[0].OutboxEventId) != outboxID {
		t.Fatalf("unexpected outbox list: %#v", outboxList)
	}
	retryOutboxResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/background/outbox-events/"+outboxID.String()+"/retry", bytes.NewReader(mustJSON(t, map[string]any{"rowVersion": outboxList.Items[0].RowVersion, "reason": "integration retry outbox"})), adminHeaders)
	assertStatus(t, retryOutboxResponse, http.StatusOK)
	var retriedOutbox api.BackgroundOutboxEventResponse
	decodeResponse(t, retryOutboxResponse, &retriedOutbox)
	if retriedOutbox.Event.Status != api.BackgroundOutboxStatusPENDING || retriedOutbox.Event.AttemptCount != 0 || retriedOutbox.Event.RowVersion <= outboxList.Items[0].RowVersion {
		t.Fatalf("unexpected retried outbox: %#v", retriedOutbox)
	}

	jobListResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/background/jobs?page=1&pageSize=50&status=FAILED&jobType=BACKGROUND_HTTP_TEST", nil, adminHeaders)
	assertStatus(t, jobListResponse, http.StatusOK)
	var jobList api.BackgroundJobListResponse
	decodeResponse(t, jobListResponse, &jobList)
	if jobList.Page != 1 || jobList.PageSize != 50 || len(jobList.Items) != 1 || uuid.UUID(jobList.Items[0].BackgroundJobId) != jobID {
		t.Fatalf("unexpected background job list: %#v", jobList)
	}
	retryJobResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/background/jobs/"+jobID.String()+"/retry", bytes.NewReader(mustJSON(t, map[string]any{"rowVersion": jobList.Items[0].RowVersion, "reason": "integration retry job"})), adminHeaders)
	assertStatus(t, retryJobResponse, http.StatusOK)
	var retriedJob api.BackgroundJobResponse
	decodeResponse(t, retryJobResponse, &retriedJob)
	if retriedJob.Job.Status != api.BackgroundJobStatusPENDING || retriedJob.Job.AttemptCount != 0 || retriedJob.Job.RowVersion <= jobList.Items[0].RowVersion {
		t.Fatalf("unexpected retried background job: %#v", retriedJob)
	}

	stopServer()
	select {
	case err := <-serverErrors:
		if err != nil {
			t.Fatalf("server shutdown failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not stop within shutdown deadline")
	}
}

func insertBackgroundHTTPUserFixture(t *testing.T, ctx context.Context, connection *pgx.Conn, userID uuid.UUID, username, password, systemRole string) {
	t.Helper()
	hash, err := identitydomain.NewArgon2IDHasher().Hash(password)
	if err != nil {
		t.Fatalf("hash password fixture: %v", err)
	}
	now := time.Now().UTC()
	if _, err := connection.Exec(ctx, `
		INSERT INTO users (
			user_id, username, username_normalized, display_name, system_role, status,
			locale, timezone, created_at, updated_at
		) VALUES ($1, $2, $2, $3, $4, 'ACTIVE', 'zh-CN', 'Asia/Shanghai', $5, $5)`,
		userID, username, "Background HTTP "+systemRole, systemRole, now,
	); err != nil {
		t.Fatalf("insert background HTTP user fixture: %v", err)
	}
	if _, err := connection.Exec(ctx, `
		INSERT INTO user_credentials (
			user_credential_id, user_id, credential_type, identifier, identifier_normalized,
			secret_hash, status, created_at, updated_at
		) VALUES ($1, $2, 'PASSWORD', $3, $3, $4, 'ACTIVE', $5, $5)`,
		uuid.Must(uuid.NewV7()), userID, username, hash, now,
	); err != nil {
		t.Fatalf("insert background HTTP credential fixture: %v", err)
	}
	if _, err := connection.Exec(ctx, "INSERT INTO principal_security_versions (user_id, updated_at) VALUES ($1, $2)", userID, now); err != nil {
		t.Fatalf("insert background HTTP security versions: %v", err)
	}
}

func insertFailedOutboxFixture(t *testing.T, ctx context.Context, connection *pgx.Conn, deduplicationKey string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	aggregateID := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	if _, err := connection.Exec(ctx, `
		INSERT INTO outbox_events (
			outbox_event_id, aggregate_type, aggregate_id, aggregate_version, event_type,
			event_schema_version, payload_json, deduplication_key, correlation_id,
			status, attempt_count, max_attempts, available_at, last_error_code, last_error_summary,
			created_at, updated_at
		) VALUES ($1, 'BACKGROUND_HTTP', $2, 1, 'BACKGROUND_HTTP_TEST', 1, '{}'::jsonb, $3, $4,
			'FAILED', 1, 3, $5, 'INTEGRATION_FAILED', 'integration fixture failed', $5, $5)`,
		id, aggregateID, deduplicationKey, uuid.Must(uuid.NewV7()), now,
	); err != nil {
		t.Fatalf("insert failed outbox fixture: %v", err)
	}
	return id
}

func insertFailedBackgroundJobFixture(t *testing.T, ctx context.Context, connection *pgx.Conn, deduplicationKey string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	if _, err := connection.Exec(ctx, `
		INSERT INTO background_jobs (
			background_job_id, job_type, payload_schema_version, payload_json, deduplication_key,
			status, attempt_count, max_attempts, available_at, last_error_code, last_error_summary,
			created_at, updated_at
		) VALUES ($1, 'BACKGROUND_HTTP_TEST', 1, '{}'::jsonb, $2, 'FAILED', 1, 3, $3,
			'INTEGRATION_FAILED', 'integration fixture failed', $3, $3)`,
		id, deduplicationKey, now,
	); err != nil {
		t.Fatalf("insert failed background job fixture: %v", err)
	}
	return id
}
