package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"file-workshop/backend/internal/platform/cache"
	"file-workshop/backend/internal/platform/config"

	"github.com/google/uuid"
)

func TestRedisRoundTripUsesUniqueKey(t *testing.T) {
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

	client := cache.NewRedisClient(cfg.Redis)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close Redis client: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := cache.PingRedis(ctx, client, cfg.Redis.Address(), cfg.Redis.PingTimeout); err != nil {
		t.Fatalf("connect Redis test dependency: %v", err)
	}

	key := "file-workshop:test:pre-012:" + uuid.NewString()
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if err := client.Del(cleanupContext, key).Err(); err != nil {
			t.Errorf("remove isolated Redis test key: %v", err)
		}
	})

	want := uuid.NewString()
	if err := client.Set(ctx, key, want, time.Minute).Err(); err != nil {
		t.Fatalf("write isolated Redis test key: %v", err)
	}
	got, err := client.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("read isolated Redis test key: %v", err)
	}
	if got != want {
		t.Fatalf("Redis value = %q, want %q", got, want)
	}
}
