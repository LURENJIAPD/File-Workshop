package cache

import (
	"testing"
	"time"

	"file-workshop/backend/internal/platform/config"
)

func TestBuildRedisOptionsForRedis62(t *testing.T) {
	cfg := config.RedisConfig{
		Host:            "127.0.0.1",
		Port:            6379,
		Password:        "test-password",
		Database:        3,
		DialTimeout:     5 * time.Second,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
		PoolSize:        12,
		MinIdleConns:    2,
		MaxIdleConns:    6,
		PoolTimeout:     4 * time.Second,
		ConnMaxIdleTime: 30 * time.Minute,
		ConnMaxLifetime: time.Hour,
	}

	options := BuildRedisOptions(cfg)
	if options.Addr != "127.0.0.1:6379" || options.DB != 3 || options.Password != "test-password" {
		t.Fatalf("unexpected Redis endpoint options: %#v", options)
	}
	if options.Protocol != 2 || !options.DisableIdentity {
		t.Fatalf("Redis 6.2 compatibility options were not applied: protocol=%d disableIdentity=%t", options.Protocol, options.DisableIdentity)
	}
	if options.PoolSize != 12 || options.MinIdleConns != 2 || options.MaxIdleConns != 6 {
		t.Fatalf(
			"unexpected Redis pool options: size=%d minIdle=%d maxIdle=%d",
			options.PoolSize,
			options.MinIdleConns,
			options.MaxIdleConns,
		)
	}
}
