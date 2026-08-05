package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/platform/health"
	"file-workshop/backend/internal/platform/requestid"

	"github.com/gin-gonic/gin"
)

func TestLivenessDoesNotDependOnExternalServices(t *testing.T) {
	router := testRouter(t, errors.New("postgres down"), errors.New("redis down"))
	request := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	request.Header.Set(requestid.Header, "upstream-request-123")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response api.HealthResponse
	decodeJSON(t, recorder, &response)
	if response.Status != api.HealthStatusOk || len(response.Checks) != 0 {
		t.Fatalf("unexpected liveness response: %#v", response)
	}
	if response.RequestId != "upstream-request-123" || recorder.Header().Get(requestid.Header) != response.RequestId {
		t.Fatalf("request ID was not propagated: body=%q header=%q", response.RequestId, recorder.Header().Get(requestid.Header))
	}
}

func TestReadinessDegradesWhenRedisIsUnavailable(t *testing.T) {
	router := testRouter(t, nil, errors.New("redis down"))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response api.HealthResponse
	decodeJSON(t, recorder, &response)
	if response.Status != api.HealthStatusDegraded {
		t.Fatalf("status = %q, want degraded", response.Status)
	}
	if response.Checks["redis"].Status != api.ComponentStatusUnavailable || response.Checks["minio"].Status != api.ComponentStatusDisabled {
		t.Fatalf("unexpected component checks: %#v", response.Checks)
	}
}

func TestReadinessFailsWhenPostgreSQLIsUnavailable(t *testing.T) {
	router := testRouter(t, errors.New("postgres down"), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	var response api.HealthResponse
	decodeJSON(t, recorder, &response)
	if response.Status != api.HealthStatusUnavailable {
		t.Fatalf("status = %q, want unavailable", response.Status)
	}
}

func TestRouterUsesUniformErrorsAndRecoversPanics(t *testing.T) {
	router := testRouter(t, nil, nil)
	router.GET("/panic", func(*gin.Context) { panic("test panic") })

	tests := []struct {
		name      string
		method    string
		path      string
		status    int
		errorCode string
	}{
		{name: "not found", method: http.MethodGet, path: "/missing", status: http.StatusNotFound, errorCode: errorCodeNotFound},
		{name: "method not allowed", method: http.MethodPost, path: "/health/live", status: http.StatusMethodNotAllowed, errorCode: errorCodeMethodNotAllowed},
		{name: "panic", method: http.MethodGet, path: "/panic", status: http.StatusInternalServerError, errorCode: errorCodeInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
			var response api.ErrorResponse
			decodeJSON(t, recorder, &response)
			if response.Code != test.errorCode || response.RequestId == "" {
				t.Fatalf("unexpected error response: %#v", response)
			}
		})
	}
}

func TestRouterHandlesCredentialedCORSPreflightFromAllowedOrigin(t *testing.T) {
	router := testRouter(t, nil, nil)
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:5173" || recorder.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("unexpected CORS headers: %#v", recorder.Header())
	}
}

func TestRouterRejectsCORSPreflightFromUnknownOrigin(t *testing.T) {
	router := testRouter(t, nil, nil)
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	request.Header.Set("Origin", "https://attacker.example")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	var response api.ErrorResponse
	decodeJSON(t, recorder, &response)
	if response.Code != errorCodeCORSOriginRejected {
		t.Fatalf("error code = %q", response.Code)
	}
}

func testRouter(t *testing.T, postgresErr, redisErr error) *gin.Engine {
	t.Helper()
	service, err := health.NewService("file-workshop-server", []health.Dependency{
		{Name: "postgresql", Required: true, Enabled: true, Timeout: time.Second, Check: func(context.Context) error { return postgresErr }},
		{Name: "redis", Required: false, Enabled: true, Timeout: time.Second, Check: func(context.Context) error { return redisErr }},
		{Name: "minio", Required: false, Enabled: false},
	}, func() time.Time { return time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatalf("health.NewService() error = %v", err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return NewRouter(NewAPIHandler(NewHealthHandler(service, logger), nil, nil), logger, []string{"http://127.0.0.1:5173"})
}

func decodeJSON(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
}
