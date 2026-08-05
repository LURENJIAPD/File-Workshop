package database

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"file-workshop/backend/internal/platform/config"
	"file-workshop/backend/internal/platform/database/dbgen"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgreSQLInfo struct {
	ServerVersion   string
	Database        string
	User            string
	CurrentSchema   string
	TimeZone        string
	SchemaAvailable bool
}

func BuildPostgreSQLPoolConfig(app config.AppConfig, database config.PostgreSQLConfig) (*pgxpool.Config, error) {
	poolConfig, err := pgxpool.ParseConfig(database.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL pool configuration: %w", err)
	}

	poolConfig.MaxConns = database.PoolMaxConns
	poolConfig.MinConns = database.PoolMinConns
	poolConfig.MaxConnLifetime = database.PoolMaxConnLifetime
	poolConfig.MaxConnLifetimeJitter = database.PoolMaxConnLifetimeJitter
	poolConfig.MaxConnIdleTime = database.PoolMaxConnIdleTime
	poolConfig.HealthCheckPeriod = database.PoolHealthCheckPeriod
	poolConfig.PingTimeout = database.PingTimeout
	poolConfig.ConnConfig.ConnectTimeout = database.ConnectTimeout
	poolConfig.ConnConfig.RuntimeParams["application_name"] = app.ServiceName
	poolConfig.ConnConfig.RuntimeParams["search_path"] = database.Schema + ",public"
	poolConfig.ConnConfig.RuntimeParams["timezone"] = "UTC"
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = postgresMilliseconds(database.StatementTimeout)
	poolConfig.ConnConfig.RuntimeParams["lock_timeout"] = postgresMilliseconds(database.LockTimeout)
	poolConfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = postgresMilliseconds(database.IdleInTransactionTimeout)

	return poolConfig, nil
}

func postgresMilliseconds(duration time.Duration) string {
	return strconv.FormatInt(duration.Milliseconds(), 10)
}

func OpenPostgreSQL(ctx context.Context, app config.AppConfig, database config.PostgreSQLConfig) (*pgxpool.Pool, error) {
	poolConfig, err := BuildPostgreSQLPoolConfig(app, database)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool for %s: %w", database.Address(), err)
	}

	pingContext, cancel := context.WithTimeout(ctx, database.PingTimeout)
	defer cancel()
	if err := pool.Ping(pingContext); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL at %s: %w", database.Address(), err)
	}

	var schemaAvailable bool
	if err := pool.QueryRow(
		pingContext,
		`SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`,
		database.Schema,
	).Scan(&schemaAvailable); err != nil {
		pool.Close()
		return nil, fmt.Errorf("verify PostgreSQL schema %q: %w", database.Schema, err)
	}
	if !schemaAvailable {
		pool.Close()
		return nil, fmt.Errorf("required PostgreSQL schema %q does not exist in database %q", database.Schema, database.Database)
	}

	return pool, nil
}

func InspectPostgreSQL(ctx context.Context, pool *pgxpool.Pool, schema string) (PostgreSQLInfo, error) {
	row, err := dbgen.New(pool).GetDatabaseHealth(ctx)
	if err != nil {
		return PostgreSQLInfo{}, fmt.Errorf("inspect PostgreSQL connection: %w", err)
	}

	var schemaAvailable bool
	if err := pool.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`,
		schema,
	).Scan(&schemaAvailable); err != nil {
		return PostgreSQLInfo{}, fmt.Errorf("inspect PostgreSQL schema %q: %w", schema, err)
	}

	return PostgreSQLInfo{
		ServerVersion:   row.ServerVersion,
		Database:        row.DatabaseName,
		User:            row.DatabaseUser,
		CurrentSchema:   row.CurrentSchema,
		TimeZone:        row.Timezone,
		SchemaAvailable: schemaAvailable,
	}, nil
}
