package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	backgroundapplication "file-workshop/backend/internal/modules/background/application"
	backgroundrepository "file-workshop/backend/internal/modules/background/repository"
	"file-workshop/backend/internal/platform/config"
	"file-workshop/backend/internal/platform/database"
)

func RunWorker(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	postgresPool, err := database.OpenPostgreSQL(ctx, cfg.App, cfg.PostgreSQL)
	if err != nil {
		return fmt.Errorf("open PostgreSQL: %w", err)
	}
	defer postgresPool.Close()

	repository := backgroundrepository.NewPostgreSQL(postgresPool)
	runner, err := backgroundapplication.NewRunner(repository, nil, backgroundapplication.RunnerConfig{
		WorkerID:            workerID(cfg),
		Concurrency:         cfg.Worker.Concurrency,
		BatchSize:           int32(cfg.Worker.BatchSize),
		PollInterval:        cfg.Worker.PollInterval,
		LeaseDuration:       cfg.Worker.LeaseDuration,
		HandlerTimeout:      cfg.Worker.HandlerTimeout,
		RetryInitialBackoff: cfg.Worker.RetryInitialBackoff,
		RetryMaxBackoff:     cfg.Worker.RetryMaxBackoff,
	}, logger, nil)
	if err != nil {
		return fmt.Errorf("configure outbox worker: %w", err)
	}
	logger.InfoContext(ctx, "background worker started", "workerId", workerID(cfg), "concurrency", cfg.Worker.Concurrency, "batchSize", cfg.Worker.BatchSize)
	if err := runner.Run(ctx); err != nil {
		return err
	}
	logger.InfoContext(ctx, "background worker stopped", "workerId", workerID(cfg))
	return nil
}

func workerID(cfg config.Config) string {
	if cfg.Worker.WorkerID != "" {
		return cfg.Worker.WorkerID
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown-host"
	}
	return cfg.App.ServiceName + ":" + hostname + ":" + strconv.Itoa(os.Getpid())
}
