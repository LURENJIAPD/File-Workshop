package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	auditapplication "file-workshop/backend/internal/modules/audit/application"
	auditrepository "file-workshop/backend/internal/modules/audit/repository"
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
	auditRepository := auditrepository.NewPostgreSQL(postgresPool)
	auditService := auditapplication.NewService(auditRepository, nil)
	runnerConfig := backgroundapplication.RunnerConfig{
		WorkerID:            workerID(cfg),
		Concurrency:         cfg.Worker.Concurrency,
		BatchSize:           int32(cfg.Worker.BatchSize),
		PollInterval:        cfg.Worker.PollInterval,
		LeaseDuration:       cfg.Worker.LeaseDuration,
		HandlerTimeout:      cfg.Worker.HandlerTimeout,
		RetryInitialBackoff: cfg.Worker.RetryInitialBackoff,
		RetryMaxBackoff:     cfg.Worker.RetryMaxBackoff,
	}
	outboxRunner, err := backgroundapplication.NewRunner(repository, []backgroundapplication.OutboxHandler{auditService}, runnerConfig, logger, nil)
	if err != nil {
		return fmt.Errorf("configure outbox worker: %w", err)
	}
	jobRunner, err := backgroundapplication.NewJobRunner(repository, nil, runnerConfig, logger, nil)
	if err != nil {
		return fmt.Errorf("configure background job worker: %w", err)
	}
	logger.InfoContext(ctx, "background worker started", "workerId", workerID(cfg), "concurrency", cfg.Worker.Concurrency, "batchSize", cfg.Worker.BatchSize)
	errs := make(chan error, 2)
	go func() { errs <- outboxRunner.Run(ctx) }()
	go func() { errs <- jobRunner.Run(ctx) }()
	var firstErr error
	for range 2 {
		if err := <-errs; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	logger.InfoContext(ctx, "background worker stopped", "workerId", workerID(cfg))
	return firstErr
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
