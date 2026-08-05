package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/app"
	"file-workshop/backend/internal/modules/identity/domain"
	"file-workshop/backend/internal/platform/config"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestIdentityHTTPLoginRotationReuseAndLogout(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	connection, err := pgx.Connect(ctx, cfg.PostgreSQL.ConnectionString())
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	if _, err := connection.Exec(ctx, "SET search_path TO "+pgx.Identifier{cfg.PostgreSQL.Schema}.Sanitize()+",public"); err != nil {
		t.Fatalf("set search path: %v", err)
	}

	userID := uuid.Must(uuid.NewV7())
	credentialID := uuid.Must(uuid.NewV7())
	username := "identity_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = connection.Exec(cleanupContext, "DELETE FROM login_attempts WHERE username_normalized = $1", username)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM session_refresh_tokens WHERE user_session_id IN (SELECT user_session_id FROM user_sessions WHERE user_id = $1)", userID)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM user_sessions WHERE user_id = $1", userID)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM user_credentials WHERE user_id = $1", userID)
		_, _ = connection.Exec(cleanupContext, "DELETE FROM users WHERE user_id = $1", userID)
		_ = connection.Close(cleanupContext)
	})
	password := "correct horse battery staple"
	hash, err := domain.NewArgon2IDHasher().Hash(password)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}
	now := time.Now().UTC()
	if _, err := connection.Exec(ctx, `
		INSERT INTO users (
			user_id, username, username_normalized, display_name, system_role, status,
			locale, timezone, created_at, updated_at
		) VALUES ($1, $2, $2, $3, 'SYSTEM_ADMIN', 'ACTIVE', 'zh-CN', 'Asia/Shanghai', $4, $4)`,
		userID, username, "Identity Integration", now,
	); err != nil {
		t.Fatalf("insert identity user fixture: %v", err)
	}
	if _, err := connection.Exec(ctx, `
		INSERT INTO user_credentials (
			user_credential_id, user_id, credential_type, identifier, identifier_normalized,
			secret_hash, status, created_at, updated_at
		) VALUES ($1, $2, 'PASSWORD', $3, $3, $4, 'ACTIVE', $5, $5)`,
		credentialID, userID, username, hash, now,
	); err != nil {
		t.Fatalf("insert password credential fixture: %v", err)
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
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	client := &http.Client{Timeout: 5 * time.Second, Jar: jar}

	login := postLogin(t, client, baseURL, username, password)
	if login.User.UserId != userID || login.AccessToken == "" {
		t.Fatalf("unexpected login response: %#v", login)
	}
	parsedBaseURL, _ := url.Parse(baseURL + "/api/v1/auth/refresh")
	initialRefreshToken := cookieValue(jar.Cookies(parsedBaseURL), cfg.Auth.RefreshCookieName)
	if initialRefreshToken == "" {
		t.Fatal("login did not set refresh HttpOnly cookie")
	}
	assertCurrentSession(t, client, baseURL, login.AccessToken, http.StatusOK)

	refreshResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/auth/refresh", nil, nil)
	if refreshResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(refreshResponse.Body)
		_ = refreshResponse.Body.Close()
		t.Fatalf("refresh status = %d; body=%s", refreshResponse.StatusCode, body)
	}
	var refreshed api.AuthTokenResponse
	decodeResponse(t, refreshResponse, &refreshed)
	if refreshed.AccessToken == login.AccessToken {
		t.Fatal("refresh did not rotate access token")
	}

	replayHeaders := map[string]string{"Cookie": cfg.Auth.RefreshCookieName + "=" + initialRefreshToken}
	replayResponse := doRequest(t, &http.Client{Timeout: 5 * time.Second}, http.MethodPost, baseURL+"/api/v1/auth/refresh", nil, replayHeaders)
	if replayResponse.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(replayResponse.Body)
		_ = replayResponse.Body.Close()
		t.Fatalf("replayed refresh status = %d; body=%s", replayResponse.StatusCode, body)
	}
	_ = replayResponse.Body.Close()
	assertCurrentSession(t, client, baseURL, refreshed.AccessToken, http.StatusUnauthorized)

	postLogin(t, client, baseURL, username, password)
	concurrentRefreshToken := cookieValue(jar.Cookies(parsedBaseURL), cfg.Auth.RefreshCookieName)
	start := make(chan struct{})
	results := make(chan concurrentRefreshResult, 2)
	for index := 0; index < 2; index++ {
		go func() {
			<-start
			results <- executeConcurrentRefresh(baseURL, cfg.Auth.RefreshCookieName, concurrentRefreshToken)
		}()
	}
	close(start)
	statuses := make([]int, 0, 2)
	concurrentAccessToken := ""
	for index := 0; index < 2; index++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent refresh failed: %v", result.err)
		}
		statuses = append(statuses, result.status)
		if result.status == http.StatusOK {
			concurrentAccessToken = result.accessToken
		}
	}
	sort.Ints(statuses)
	if statuses[0] != http.StatusOK || statuses[1] != http.StatusUnauthorized || concurrentAccessToken == "" {
		t.Fatalf("concurrent refresh statuses = %v, want [200 401]", statuses)
	}
	assertCurrentSession(t, client, baseURL, concurrentAccessToken, http.StatusUnauthorized)

	thirdLogin := postLogin(t, client, baseURL, username, password)
	logoutResponse := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/auth/logout", nil, nil)
	if logoutResponse.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(logoutResponse.Body)
		_ = logoutResponse.Body.Close()
		t.Fatalf("logout status = %d; body=%s", logoutResponse.StatusCode, body)
	}
	_ = logoutResponse.Body.Close()
	assertCurrentSession(t, client, baseURL, thirdLogin.AccessToken, http.StatusUnauthorized)

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

type concurrentRefreshResult struct {
	status      int
	accessToken string
	err         error
}

func executeConcurrentRefresh(baseURL, cookieName, refreshToken string) concurrentRefreshResult {
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/auth/refresh", nil)
	if err != nil {
		return concurrentRefreshResult{err: err}
	}
	request.Header.Set("Cookie", cookieName+"="+refreshToken)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return concurrentRefreshResult{err: err}
	}
	defer response.Body.Close()
	result := concurrentRefreshResult{status: response.StatusCode}
	if response.StatusCode == http.StatusOK {
		var body api.AuthTokenResponse
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			result.err = err
			return result
		}
		result.accessToken = body.AccessToken
	}
	return result
}

func postLogin(t *testing.T, client *http.Client, baseURL, username, password string) api.AuthTokenResponse {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"username": username, "password": password, "deviceId": "integration-test"})
	if err != nil {
		t.Fatalf("encode login request: %v", err)
	}
	response := doRequest(t, client, http.MethodPost, baseURL+"/api/v1/auth/login", bytes.NewReader(payload), map[string]string{"Content-Type": "application/json"})
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		t.Fatalf("login status = %d; body=%s", response.StatusCode, body)
	}
	var result api.AuthTokenResponse
	decodeResponse(t, response, &result)
	return result
}

func assertCurrentSession(t *testing.T, client *http.Client, baseURL, accessToken string, expectedStatus int) {
	t.Helper()
	response := doRequest(t, client, http.MethodGet, baseURL+"/api/v1/auth/session", nil, map[string]string{"Authorization": "Bearer " + accessToken})
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("current session status = %d, want %d; body=%s", response.StatusCode, expectedStatus, body)
	}
}

func doRequest(t *testing.T, client *http.Client, method, requestURL string, body io.Reader, headers map[string]string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, requestURL, body)
	if err != nil {
		t.Fatalf("create %s request: %v", method, err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("execute %s %s: %v", method, requestURL, err)
	}
	return response
}

func decodeResponse(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode HTTP response: %v", err)
	}
}

func cookieValue(cookies []*http.Cookie, name string) string {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}
