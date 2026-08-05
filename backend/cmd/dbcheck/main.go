package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"file-workshop/backend/internal/platform/config"
	"file-workshop/backend/internal/platform/database"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("database check failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := database.OpenPostgreSQL(ctx, cfg.App, cfg.PostgreSQL)
	if err != nil {
		return err
	}
	defer pool.Close()

	info, err := database.InspectPostgreSQL(ctx, pool, cfg.PostgreSQL.Schema)
	if err != nil {
		return err
	}
	stats := pool.Stat()
	logger.Info(
		"PostgreSQL connection pool is ready",
		"address", cfg.PostgreSQL.Address(),
		"database", info.Database,
		"user", info.User,
		"serverVersion", info.ServerVersion,
		"schema", info.CurrentSchema,
		"timezone", info.TimeZone,
		"maxConns", stats.MaxConns(),
		"totalConns", stats.TotalConns(),
		"environmentFile", cfg.EnvironmentFile,
	)
	return nil
}
