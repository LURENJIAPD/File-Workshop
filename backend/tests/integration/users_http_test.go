package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

func TestUserManagementHTTPWorkflow(t *testing.T) {
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
	adminID := uuid.Must(uuid.NewV7())
	adminCredentialID := uuid.Must(uuid.NewV7())
	adminUsername := "users_admin_" + suffix
	adminPassword := "Admin-" + suffix + "!Aa1"
	createdUsername := "worker_" + suffix
	createdPassword := "Worker-" + suffix + "!Aa1"
	resetPassword := "Reset-" + suffix + "!Bb2"
	createdUserID := uuid.Nil

	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		ids := []uuid.UUID{adminID}
		if createdUserID != uuid.Nil {
			ids = append(ids, createdUserID)
		}
		_, _ = connection.Exec(cleanupContext, "DELETE FROM outbox_events WHERE aggregate_id = ANY($1::uuid[])", ids)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM idempotency_records WHERE user_id = $1", adminID)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM login_attempts WHERE username_normalized = ANY($1::text[])", []string{adminUsername, createdUsername})
		_, _ = connection.Exec(cleanupContext, "DELETE FROM session_refresh_tokens WHERE user_session_id IN (SELECT user_session_id FROM user_sessions WHERE user_id = ANY($1::uuid[]))", ids)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM user_sessions WHERE user_id = ANY($1::uuid[])", ids)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM user_credentials WHERE user_id = ANY($1::uuid[])", ids)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM principal_security_versions WHERE user_id = ANY($1::uuid[])", ids)
		if createdUserID != uuid.Nil {
			_, _ = connection.Exec(cleanupContext, "DELETE FROM users WHERE user_id = $1", createdUserID)
		}
		_, _ = connection.Exec(cleanupContext, "DELETE FROM users WHERE user_id = $1", adminID)
		_ = connection.Close(cleanupContext)
	})

	hash, err := identitydomain.NewArgon2IDHasher().Hash(adminPassword)
	if err != nil {
		t.Fatalf("hash administrator password: %v", err)
	}
	now := time.Now().UTC()
	if _, err := connection.Exec(ctx, `
		INSERT INTO users (
			user_id, username, username_normalized, display_name, system_role, status,
			locale, timezone, created_at, updated_at
		) VALUES ($1, $2, $2, 'Users Integration Admin', 'SYSTEM_ADMIN', 'ACTIVE', 'zh-CN', 'Asia/Shanghai', $3, $3)`, adminID, adminUsername, now); err != nil {
		t.Fatalf("insert administrator fixture: %v", err)
	}
	if _, err := connection.Exec(ctx, `
		INSERT INTO user_credentials (
			user_credential_id, user_id, credential_type, identifier, identifier_normalized,
			secret_hash, status, created_at, updated_at
		) VALUES ($1, $2, 'PASSWORD', $3, $3, $4, 'ACTIVE', $5, $5)`, adminCredentialID, adminID, adminUsername, hash, now); err != nil {
		t.Fatalf("insert administrator credential: %v", err)
	}
	if _, err := connection.Exec(ctx, "INSERT INTO principal_security_versions (user_id, updated_at) VALUES ($1, $2)", adminID, now); err != nil {
		t.Fatalf("insert administrator security versions: %v", err)
	}

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
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminLogin.AccessToken, "Content-Type": "application/json"}

	currentResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/users/me", nil, adminHeaders)
	assertStatus(t, currentResponse, http.StatusOK)
	_ = currentResponse.Body.Close()

	createBody := map[string]any{"username": createdUsername, "password": createdPassword, "employeeNo": "FW-" + suffix[:8], "displayName": "Integration Worker", "email": createdUsername + "@example.com", "phone": "13800000000", "systemRole": "USER", "locale": "zh-CN", "timezone": "Asia/Shanghai"}
	createPayload := mustJSON(t, createBody)
	createHeaders := cloneHeaders(adminHeaders)
	createHeaders["Idempotency-Key"] = "create-" + suffix
	createResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/users", bytes.NewReader(createPayload), createHeaders)
	if createResponse.StatusCode != http.StatusCreated {
		assertStatus(t, createResponse, http.StatusCreated)
	}
	var created api.UserResponse
	decodeResponse(t, createResponse, &created)
	createdUserID = uuid.UUID(created.User.UserId)
	if created.User.Username != createdUsername || created.User.RowVersion != 1 {
		t.Fatalf("unexpected created user: %#v", created.User)
	}

	replayResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/users", bytes.NewReader(createPayload), createHeaders)
	assertStatus(t, replayResponse, http.StatusCreated)
	var replayed api.UserResponse
	decodeResponse(t, replayResponse, &replayed)
	if replayed.User.UserId != created.User.UserId {
		t.Fatalf("idempotent replay userId = %s, want %s", replayed.User.UserId, created.User.UserId)
	}

	conflictingBody := cloneMap(createBody)
	conflictingBody["displayName"] = "Different Request"
	conflictResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/users", bytes.NewReader(mustJSON(t, conflictingBody)), createHeaders)
	assertErrorCode(t, conflictResponse, http.StatusConflict, "IDEMPOTENCY_CONFLICT")

	duplicateHeaders := cloneHeaders(adminHeaders)
	duplicateHeaders["Idempotency-Key"] = "duplicate-" + suffix
	duplicateResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/users", bytes.NewReader(createPayload), duplicateHeaders)
	assertErrorCode(t, duplicateResponse, http.StatusConflict, "USER_CONFLICT")

	listResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/users?page=1&pageSize=1&status=ACTIVE&systemRole=USER", nil, adminHeaders)
	assertStatus(t, listResponse, http.StatusOK)
	var list api.UserListResponse
	decodeResponse(t, listResponse, &list)
	if list.Page != 1 || list.PageSize != 1 || list.Total < 1 || len(list.Items) != 1 {
		t.Fatalf("unexpected paginated users: %#v", list)
	}

	updatePayload := mustJSON(t, map[string]any{"displayName": "Updated Worker", "email": "updated-" + createdUsername + "@example.com", "rowVersion": created.User.RowVersion})
	updateResponse := doRequest(t, client, http.MethodPatch, baseURL+"/api/v1/admin/users/"+createdUserID.String(), bytes.NewReader(updatePayload), adminHeaders)
	assertStatus(t, updateResponse, http.StatusOK)
	var updated api.UserResponse
	decodeResponse(t, updateResponse, &updated)
	if updated.User.RowVersion != created.User.RowVersion+1 || updated.User.DisplayName != "Updated Worker" {
		t.Fatalf("unexpected updated user: %#v", updated.User)
	}

	staleResponse := doRequest(t, client, http.MethodPatch, baseURL+"/api/v1/admin/users/"+createdUserID.String(), bytes.NewReader(updatePayload), adminHeaders)
	assertErrorCode(t, staleResponse, http.StatusConflict, "USER_VERSION_CONFLICT")

	userLogin := postLogin(t, client, baseURL, createdUsername, createdPassword)
	nonAdminResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/users", nil, map[string]string{"Authorization": "Bearer " + userLogin.AccessToken})
	assertErrorCode(t, nonAdminResponse, http.StatusForbidden, "AUTH_FORBIDDEN")
	freshResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/users/"+createdUserID.String(), nil, adminHeaders)
	assertStatus(t, freshResponse, http.StatusOK)
	var fresh api.UserResponse
	decodeResponse(t, freshResponse, &fresh)
	type concurrentUpdateResult struct {
		status int
		err    error
	}
	concurrentResults := make(chan concurrentUpdateResult, 2)
	startConcurrentUpdate := make(chan struct{})
	concurrentPayloads := [][]byte{
		mustJSON(t, map[string]any{"displayName": "Concurrent Worker 0", "rowVersion": fresh.User.RowVersion}),
		mustJSON(t, map[string]any{"displayName": "Concurrent Worker 1", "rowVersion": fresh.User.RowVersion}),
	}
	for index := 0; index < 2; index++ {
		go func() {
			<-startConcurrentUpdate
			request, err := http.NewRequest(http.MethodPatch, baseURL+"/api/v1/admin/users/"+createdUserID.String(), bytes.NewReader(concurrentPayloads[index]))
			if err != nil {
				concurrentResults <- concurrentUpdateResult{err: err}
				return
			}
			for name, value := range adminHeaders {
				request.Header.Set(name, value)
			}
			response, err := client.Do(request)
			if err != nil {
				concurrentResults <- concurrentUpdateResult{err: err}
				return
			}
			defer response.Body.Close()
			concurrentResults <- concurrentUpdateResult{status: response.StatusCode}
		}()
	}
	close(startConcurrentUpdate)
	statuses := make([]int, 0, 2)
	for index := 0; index < 2; index++ {
		result := <-concurrentResults
		if result.err != nil {
			t.Fatalf("concurrent user update failed: %v", result.err)
		}
		statuses = append(statuses, result.status)
	}
	sort.Ints(statuses)
	if statuses[0] != http.StatusOK || statuses[1] != http.StatusConflict {
		t.Fatalf("concurrent update statuses = %v, want [200 409]", statuses)
	}
	freshResponse = doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/users/"+createdUserID.String(), nil, adminHeaders)
	assertStatus(t, freshResponse, http.StatusOK)
	decodeResponse(t, freshResponse, &fresh)

	lockPayload := mustJSON(t, map[string]any{"rowVersion": fresh.User.RowVersion, "reason": "integration lock test"})
	lockResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/users/"+createdUserID.String()+"/lock", bytes.NewReader(lockPayload), adminHeaders)
	assertStatus(t, lockResponse, http.StatusOK)
	var locked api.UserResponse
	decodeResponse(t, lockResponse, &locked)
	if locked.User.Status != api.UserStatusLOCKED {
		t.Fatalf("locked status = %s", locked.User.Status)
	}
	assertCurrentSession(t, client, baseURL, userLogin.AccessToken, http.StatusUnauthorized)

	enablePayload := mustJSON(t, map[string]any{"rowVersion": locked.User.RowVersion, "reason": "integration enable test"})
	enableResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/admin/users/"+createdUserID.String()+"/enable", bytes.NewReader(enablePayload), adminHeaders)
	assertStatus(t, enableResponse, http.StatusOK)
	var enabled api.UserResponse
	decodeResponse(t, enableResponse, &enabled)

	resetPayload := mustJSON(t, map[string]any{"password": resetPassword, "rowVersion": enabled.User.RowVersion})
	resetResponse := doRequest(t, client, http.MethodPut, baseURL+"/api/v1/admin/users/"+createdUserID.String()+"/password", bytes.NewReader(resetPayload), adminHeaders)
	assertStatus(t, resetResponse, http.StatusNoContent)
	_ = resetResponse.Body.Close()

	oldPasswordResponse := loginRequest(t, client, baseURL, createdUsername, createdPassword)
	assertErrorCode(t, oldPasswordResponse, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS")
	firstLogin := postLogin(t, client, baseURL, createdUsername, resetPassword)
	secondLogin := postLogin(t, client, baseURL, createdUsername, resetPassword)
	userHeaders := map[string]string{"Authorization": "Bearer " + secondLogin.AccessToken}
	sessionsResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/users/me/sessions?page=1&pageSize=50", nil, userHeaders)
	assertStatus(t, sessionsResponse, http.StatusOK)
	var sessions api.UserSessionListResponse
	decodeResponse(t, sessionsResponse, &sessions)
	var currentSessionID, otherSessionID string
	for _, session := range sessions.Items {
		if session.IsCurrent {
			currentSessionID = uuid.UUID(session.SessionId).String()
		} else if session.Status == api.UserSessionStatusACTIVE && otherSessionID == "" {
			otherSessionID = uuid.UUID(session.SessionId).String()
		}
	}
	if currentSessionID == "" || otherSessionID == "" {
		t.Fatalf("expected current and other active sessions: %#v", sessions)
	}
	for range 2 {
		revokeOtherResponse := doRequest(t, client, http.MethodDelete, baseURL+"/api/v1/users/me/sessions/"+otherSessionID, nil, userHeaders)
		assertStatus(t, revokeOtherResponse, http.StatusNoContent)
		_ = revokeOtherResponse.Body.Close()
	}
	assertCurrentSession(t, client, baseURL, firstLogin.AccessToken, http.StatusUnauthorized)
	revokeResponse := doRequest(t, client, http.MethodDelete, baseURL+"/api/v1/users/me/sessions/"+currentSessionID, nil, userHeaders)
	assertStatus(t, revokeResponse, http.StatusNoContent)
	_ = revokeResponse.Body.Close()
	assertCurrentSession(t, client, baseURL, secondLogin.AccessToken, http.StatusUnauthorized)

	getResponse := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/admin/users/"+createdUserID.String(), nil, adminHeaders)
	assertStatus(t, getResponse, http.StatusOK)
	var beforeDelete api.UserResponse
	decodeResponse(t, getResponse, &beforeDelete)
	deletePayload := mustJSON(t, map[string]any{"rowVersion": beforeDelete.User.RowVersion, "reason": "integration delete test"})
	deleteResponse := doRequest(t, client, http.MethodDelete, baseURL+"/api/v1/admin/users/"+createdUserID.String(), bytes.NewReader(deletePayload), adminHeaders)
	assertStatus(t, deleteResponse, http.StatusNoContent)
	_ = deleteResponse.Body.Close()

	var status string
	var deletedAt *time.Time
	if err := connection.QueryRow(ctx, "SELECT status, deleted_at FROM users WHERE user_id = $1", createdUserID).Scan(&status, &deletedAt); err != nil {
		t.Fatalf("read logically deleted user: %v", err)
	}
	if status != "DELETED" || deletedAt == nil {
		t.Fatalf("logical delete state = (%q, %v)", status, deletedAt)
	}
	deletedLoginResponse := loginRequest(t, client, baseURL, createdUsername, resetPassword)
	assertErrorCode(t, deletedLoginResponse, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS")

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

func loginRequest(t *testing.T, client *http.Client, baseURL, username, password string) *http.Response {
	t.Helper()
	payload := mustJSON(t, map[string]string{"username": username, "password": password, "deviceId": "users-integration-test"})
	return doRequest(t, client, http.MethodPost, baseURL+"/api/v1/auth/login", bytes.NewReader(payload), map[string]string{"Content-Type": "application/json"})
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
	return payload
}

func cloneHeaders(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func assertStatus(t *testing.T, response *http.Response, expected int) {
	t.Helper()
	if response.StatusCode != expected {
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("HTTP status = %d, want %d; body=%s", response.StatusCode, expected, body)
	}
}

func assertErrorCode(t *testing.T, response *http.Response, expectedStatus int, expectedCode string) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("HTTP status = %d, want %d; body=%s", response.StatusCode, expectedStatus, body)
	}
	var body api.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Code != expectedCode {
		t.Fatalf("error code = %q, want %q; response=%s", body.Code, expectedCode, fmt.Sprintf("%#v", body))
	}
}
