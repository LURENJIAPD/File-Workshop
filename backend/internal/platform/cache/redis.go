package cache

import (
	"context"
	"fmt"
	"time"

	"file-workshop/backend/internal/platform/config"

	"github.com/redis/go-redis/v9"
)

// BuildRedisOptions keeps the Redis 6.2 development baseline explicit.
// RESP2 avoids relying on newer protocol features, and DisableIdentity prevents
// the client from sending CLIENT SETINFO, which older Redis servers do not know.
func BuildRedisOptions(cfg config.RedisConfig) *redis.Options {
	return &redis.Options{
		Addr:            cfg.Address(),
		Password:        cfg.Password,
		DB:              cfg.Database,
		Protocol:        2,
		DisableIdentity: true,
		DialTimeout:     cfg.DialTimeout,
		ReadTimeout:     cfg.ReadTimeout,
		WriteTimeout:    cfg.WriteTimeout,
		PoolSize:        cfg.PoolSize,
		MinIdleConns:    cfg.MinIdleConns,
		MaxIdleConns:    cfg.MaxIdleConns,
		PoolTimeout:     cfg.PoolTimeout,
		ConnMaxIdleTime: cfg.ConnMaxIdleTime,
		ConnMaxLifetime: cfg.ConnMaxLifetime,
	}
}

func NewRedisClient(cfg config.RedisConfig) *redis.Client {
	return redis.NewClient(BuildRedisOptions(cfg))
}

func PingRedis(ctx context.Context, client *redis.Client, address string, timeout time.Duration) error {
	pingContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := client.Ping(pingContext).Err(); err != nil {
		return fmt.Errorf("ping Redis at %s: %w", address, err)
	}
	return nil
}
