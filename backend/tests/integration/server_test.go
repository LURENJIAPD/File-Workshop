package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"file-workshop/backend/api"
	"file-workshop/backend/internal/app"
	"file-workshop/backend/internal/platform/config"
)

const integrationEnvironment = "FILE_WORKSHOP_RUN_INTEGRATION"

func TestServerWithLocalPostgreSQLAndRedis(t *testing.T) {
	if value := os.Getenv(integrationEnvironment); value != "1" {
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
	cfg.HTTP.Port = availablePort(t)
	cfg.HTTP.ShutdownTimeout = 5 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverErrors := make(chan error, 1)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	go func() {
		serverErrors <- app.RunServer(ctx, cfg, logger)
	}()

	response := waitForReadiness(t, cfg.HTTP.Address(), serverErrors)
	if response.Status != api.HealthStatusOk {
		t.Fatalf("readiness status = %q, want ok; checks=%#v", response.Status, response.Checks)
	}
	if response.Checks["postgresql"].Status != api.ComponentStatusOk {
		t.Fatalf("PostgreSQL status = %q, want ok", response.Checks["postgresql"].Status)
	}
	if response.Checks["redis"].Status != api.ComponentStatusOk {
		t.Fatalf("Redis status = %q, want ok", response.Checks["redis"].Status)
	}
	if response.Checks["objectStorage"].Status != api.ComponentStatusDisabled {
		t.Fatalf("object storage status = %q, want disabled", response.Checks["objectStorage"].Status)
	}

	cancel()
	select {
	case err := <-serverErrors:
		if err != nil {
			t.Fatalf("server graceful shutdown failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not stop within the graceful shutdown deadline")
	}
}

func availablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local HTTP port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release local HTTP port: %v", err)
	}
	return port
}

func waitForReadiness(t *testing.T, address string, serverErrors <-chan error) api.HealthResponse {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(15 * time.Second)
	url := "http://" + address + "/health/ready"
	for time.Now().Before(deadline) {
		select {
		case err := <-serverErrors:
			t.Fatalf("server stopped before readiness check: %v", err)
		default:
		}

		response, err := client.Get(url)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var healthResponse api.HealthResponse
		decodeErr := json.NewDecoder(response.Body).Decode(&healthResponse)
		closeErr := response.Body.Close()
		if decodeErr != nil {
			t.Fatalf("decode readiness response: %v", decodeErr)
		}
		if closeErr != nil {
			t.Fatalf("close readiness response: %v", closeErr)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("readiness HTTP status = %d; response=%#v", response.StatusCode, healthResponse)
		}
		return healthResponse
	}
	t.Fatalf("server did not become reachable at %s", url)
	return api.HealthResponse{}
}
