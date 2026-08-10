package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"file-workshop/backend/internal/app"
	"file-workshop/backend/internal/platform/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("worker stopped with an error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	workerContext, cancel := context.WithCancel(context.Background())
	go func() {
		<-ctx.Done()
		cancel()
	}()
	done := make(chan error, 1)
	go func() { done <- app.RunWorker(workerContext, cfg, logger) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdownTimer := time.NewTimer(cfg.Worker.ShutdownTimeout)
		defer shutdownTimer.Stop()
		select {
		case err := <-done:
			return err
		case <-shutdownTimer.C:
			return fmt.Errorf("worker did not stop within %s", cfg.Worker.ShutdownTimeout)
		}
	}
}
