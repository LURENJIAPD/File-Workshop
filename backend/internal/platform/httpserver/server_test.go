package httpserver

import (
	"net/http"
	"testing"
	"time"

	"file-workshop/backend/internal/platform/config"
)

func TestNewServerAppliesHTTPBoundaries(t *testing.T) {
	cfg := config.HTTPConfig{
		Host:              "127.0.0.1",
		Port:              8080,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
	server := NewServer(cfg, http.NewServeMux())

	if server.Addr != "127.0.0.1:8080" || server.ReadHeaderTimeout != 5*time.Second || server.MaxHeaderBytes != 1<<20 {
		t.Fatalf("HTTP server boundaries were not applied: %#v", server)
	}
}
