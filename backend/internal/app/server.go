package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"file-workshop/backend/internal/modules/identity/application"
	"file-workshop/backend/internal/modules/identity/domain"
	identityrepository "file-workshop/backend/internal/modules/identity/repository"
	"file-workshop/backend/internal/modules/identity/security"
	identitytransport "file-workshop/backend/internal/modules/identity/transport"
	organizationsapplication "file-workshop/backend/internal/modules/organizations/application"
	organizationsrepository "file-workshop/backend/internal/modules/organizations/repository"
	organizationstransport "file-workshop/backend/internal/modules/organizations/transport"
	usersapplication "file-workshop/backend/internal/modules/users/application"
	usersrepository "file-workshop/backend/internal/modules/users/repository"
	userstransport "file-workshop/backend/internal/modules/users/transport"
	"file-workshop/backend/internal/platform/cache"
	"file-workshop/backend/internal/platform/config"
	"file-workshop/backend/internal/platform/database"
	"file-workshop/backend/internal/platform/health"
	"file-workshop/backend/internal/platform/httpserver"
)

func RunServer(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	postgresPool, err := database.OpenPostgreSQL(ctx, cfg.App, cfg.PostgreSQL)
	if err != nil {
		return fmt.Errorf("open PostgreSQL: %w", err)
	}
	defer postgresPool.Close()

	redisClient := cache.NewRedisClient(cfg.Redis)
	defer func() {
		if err := redisClient.Close(); err != nil {
			logger.Warn("close Redis client failed", "error", err)
		}
	}()

	if err := cache.PingRedis(ctx, redisClient, cfg.Redis.Address(), cfg.Redis.PingTimeout); err != nil {
		logger.Warn("Redis is unavailable; server will start in degraded mode", "error", err)
	} else {
		logger.Info("Redis connection pool is ready", "address", cfg.Redis.Address(), "poolSize", cfg.Redis.PoolSize)
	}

	healthService, err := health.NewService(cfg.App.ServiceName, []health.Dependency{
		{
			Name:     "postgresql",
			Required: true,
			Enabled:  true,
			Timeout:  cfg.PostgreSQL.PingTimeout,
			Check:    postgresPool.Ping,
		},
		{
			Name:     "redis",
			Required: false,
			Enabled:  true,
			Timeout:  cfg.Redis.PingTimeout,
			Check: func(checkContext context.Context) error {
				return redisClient.Ping(checkContext).Err()
			},
		},
		{
			Name:     "minio",
			Required: false,
			Enabled:  false,
		},
	}, time.Now)
	if err != nil {
		return fmt.Errorf("configure health service: %w", err)
	}

	healthHandler := httpserver.NewHealthHandler(healthService, logger)
	identityRepository := identityrepository.NewPostgreSQL(postgresPool)
	identityService, err := application.NewService(
		identityRepository,
		identityRepository,
		domain.NewArgon2IDHasher(),
		security.NewAccessTokenManager(cfg.Auth),
		security.NewIPLimiter(),
		cfg.Auth,
		time.Now,
	)
	if err != nil {
		return fmt.Errorf("configure identity service: %w", err)
	}
	identityHandler := identitytransport.NewHandler(identityService, cfg.Auth)
	usersRepository := usersrepository.NewPostgreSQL(postgresPool)
	usersService := usersapplication.NewService(usersRepository, usersRepository, domain.NewArgon2IDHasher(), time.Now)
	usersHandler := userstransport.NewHandler(usersService, identityService, cfg.Auth)
	organizationsRepository := organizationsrepository.NewPostgreSQL(postgresPool)
	organizationsService := organizationsapplication.NewService(organizationsRepository, organizationsRepository, time.Now)
	organizationsHandler := organizationstransport.NewHandler(organizationsService, identityService, cfg.Auth)
	router := httpserver.NewRouter(httpserver.NewAPIHandler(healthHandler, identityHandler, usersHandler, organizationsHandler), logger, cfg.Auth.AllowedOrigins)
	server := httpserver.NewServer(cfg.HTTP, router)

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	logger.Info(
		"HTTP server started",
		"address", cfg.HTTP.Address(),
		"environment", cfg.App.Environment,
		"service", cfg.App.ServiceName,
	)

	select {
	case serverError := <-serverErrors:
		if errors.Is(serverError, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP at %s: %w", cfg.HTTP.Address(), serverError)
	case <-ctx.Done():
		logger.Info("HTTP server shutdown started")
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if shutdownErr := server.Shutdown(shutdownContext); shutdownErr != nil {
		closeErr := server.Close()
		serverError := <-serverErrors
		if errors.Is(serverError, http.ErrServerClosed) {
			serverError = nil
		}
		return errors.Join(
			fmt.Errorf("shutdown HTTP server: %w", shutdownErr),
			closeErr,
			serverError,
		)
	}

	serverError := <-serverErrors
	if serverError != nil && !errors.Is(serverError, http.ErrServerClosed) {
		return fmt.Errorf("HTTP server stopped unexpectedly: %w", serverError)
	}
	logger.Info("HTTP server stopped")
	return nil
}
