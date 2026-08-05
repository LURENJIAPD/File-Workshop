package config

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoadFromEnvironment(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv(envPrefix+"POSTGRES_POOL_MAX_CONNS", "12")
	t.Setenv(envPrefix+"POSTGRES_POOL_MIN_CONNS", "2")
	t.Setenv(envPrefix+"POSTGRES_STATEMENT_TIMEOUT", "45s")

	cfg, err := loadFromEnvironment()
	if err != nil {
		t.Fatalf("loadFromEnvironment() error = %v", err)
	}
	if cfg.PostgreSQL.PoolMaxConns != 12 || cfg.PostgreSQL.PoolMinConns != 2 {
		t.Fatalf("unexpected pool bounds: min=%d max=%d", cfg.PostgreSQL.PoolMinConns, cfg.PostgreSQL.PoolMaxConns)
	}
	if cfg.PostgreSQL.StatementTimeout != 45*time.Second {
		t.Fatalf("unexpected statement timeout: %v", cfg.PostgreSQL.StatementTimeout)
	}
	if cfg.Redis.Address() != "127.0.0.1:6379" {
		t.Fatalf("unexpected Redis address: %s", cfg.Redis.Address())
	}
	if cfg.Redis.PoolSize != 10 || cfg.Redis.MinIdleConns != 1 || cfg.Redis.MaxIdleConns != 5 {
		t.Fatalf(
			"unexpected Redis pool settings: size=%d minIdle=%d maxIdle=%d",
			cfg.Redis.PoolSize,
			cfg.Redis.MinIdleConns,
			cfg.Redis.MaxIdleConns,
		)
	}
	if cfg.Auth.AccessTokenTTL != 15*time.Minute || cfg.Auth.RefreshTokenTTL != 7*24*time.Hour {
		t.Fatalf("unexpected authentication token TTLs: access=%v refresh=%v", cfg.Auth.AccessTokenTTL, cfg.Auth.RefreshTokenTTL)
	}
	if len(cfg.Auth.JWTSecret) != 32 || cfg.Auth.LoginFailureLimit != 5 {
		t.Fatalf("unexpected authentication security configuration")
	}
}

func TestLoadFromEnvironmentRejectsWeakJWTSecret(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv(envPrefix+"AUTH_JWT_SECRET_BASE64", base64.StdEncoding.EncodeToString([]byte("too-short")))

	_, err := loadFromEnvironment()
	if err == nil || !strings.Contains(err.Error(), "AUTH_JWT_SECRET_BASE64") {
		t.Fatalf("expected JWT secret validation error, got %v", err)
	}
}

func TestLoadFromEnvironmentRejectsInvalidPoolBounds(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv(envPrefix+"POSTGRES_POOL_MAX_CONNS", "2")
	t.Setenv(envPrefix+"POSTGRES_POOL_MIN_CONNS", "3")

	_, err := loadFromEnvironment()
	if err == nil || !strings.Contains(err.Error(), "POSTGRES_POOL_MIN_CONNS") {
		t.Fatalf("expected pool bounds error, got %v", err)
	}
}

func TestPostgreSQLConnectionStringEscapesCredentials(t *testing.T) {
	cfg := PostgreSQLConfig{
		Host:     "127.0.0.1",
		Port:     5432,
		User:     "test-user",
		Password: "p@ss:/word",
		Database: "file_workshop",
		SSLMode:  "disable",
	}

	parsed, err := url.Parse(cfg.ConnectionString())
	if err != nil {
		t.Fatalf("parse connection string: %v", err)
	}
	password, hasPassword := parsed.User.Password()
	if !hasPassword || password != cfg.Password {
		t.Fatal("connection string did not preserve the password")
	}
	if parsed.User.Username() != cfg.User || parsed.Host != "127.0.0.1:5432" {
		t.Fatalf("unexpected connection URL: user=%s host=%s", parsed.User.Username(), parsed.Host)
	}
}

func TestLoadFromEnvironmentRejectsInvalidRedisPoolBounds(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv(envPrefix+"REDIS_POOL_SIZE", "2")
	t.Setenv(envPrefix+"REDIS_MIN_IDLE_CONNS", "3")
	t.Setenv(envPrefix+"REDIS_MAX_IDLE_CONNS", "4")

	_, err := loadFromEnvironment()
	if err == nil || !strings.Contains(err.Error(), "REDIS_MIN_IDLE_CONNS") || !strings.Contains(err.Error(), "REDIS_MAX_IDLE_CONNS") {
		t.Fatalf("expected Redis pool bounds errors, got %v", err)
	}
}

func setRequiredEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv(envPrefix+"POSTGRES_HOST", "127.0.0.1")
	t.Setenv(envPrefix+"POSTGRES_USER", "postgres")
	t.Setenv(envPrefix+"POSTGRES_PASSWORD", "test-password")
	t.Setenv(envPrefix+"POSTGRES_DATABASE", "file_workshop")
	t.Setenv(envPrefix+"POSTGRES_SCHEMA", "file_workshop")
	t.Setenv(envPrefix+"REDIS_HOST", "127.0.0.1")
	t.Setenv(envPrefix+"AUTH_JWT_SECRET_BASE64", base64.StdEncoding.EncodeToString(make([]byte, 32)))
}
