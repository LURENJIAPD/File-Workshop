package database

import (
	"testing"
	"time"

	"file-workshop/backend/internal/platform/config"
)

func TestBuildPostgreSQLPoolConfig(t *testing.T) {
	databaseConfig := config.PostgreSQLConfig{
		Host:                      "127.0.0.1",
		Port:                      5432,
		User:                      "postgres",
		Password:                  "test-password",
		Database:                  "file_workshop",
		Schema:                    "file_workshop",
		SSLMode:                   "disable",
		PoolMaxConns:              10,
		PoolMinConns:              1,
		PoolMaxConnLifetime:       time.Hour,
		PoolMaxConnLifetimeJitter: 10 * time.Minute,
		PoolMaxConnIdleTime:       30 * time.Minute,
		PoolHealthCheckPeriod:     time.Minute,
		ConnectTimeout:            5 * time.Second,
		PingTimeout:               5 * time.Second,
		StatementTimeout:          30 * time.Second,
		LockTimeout:               5 * time.Second,
		IdleInTransactionTimeout:  time.Minute,
	}

	poolConfig, err := BuildPostgreSQLPoolConfig(config.AppConfig{ServiceName: "file-workshop-server"}, databaseConfig)
	if err != nil {
		t.Fatalf("BuildPostgreSQLPoolConfig() error = %v", err)
	}
	if poolConfig.MaxConns != 10 || poolConfig.MinConns != 1 {
		t.Fatalf("unexpected pool bounds: min=%d max=%d", poolConfig.MinConns, poolConfig.MaxConns)
	}
	if poolConfig.ConnConfig.RuntimeParams["search_path"] != "file_workshop,public" {
		t.Fatalf("unexpected search_path: %q", poolConfig.ConnConfig.RuntimeParams["search_path"])
	}
	if poolConfig.ConnConfig.RuntimeParams["timezone"] != "UTC" {
		t.Fatalf("unexpected timezone: %q", poolConfig.ConnConfig.RuntimeParams["timezone"])
	}
	if poolConfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] != "60000" {
		t.Fatalf(
			"unexpected idle transaction timeout: %q",
			poolConfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"],
		)
	}
	if poolConfig.ConnConfig.ConnectTimeout != 5*time.Second || poolConfig.PingTimeout != 5*time.Second {
		t.Fatal("connection timeouts were not applied")
	}
}
