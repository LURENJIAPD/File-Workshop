package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	auditapplication "file-workshop/backend/internal/modules/audit/application"
	auditrepository "file-workshop/backend/internal/modules/audit/repository"
	audittransport "file-workshop/backend/internal/modules/audit/transport"
	backgroundapplication "file-workshop/backend/internal/modules/background/application"
	backgroundrepository "file-workshop/backend/internal/modules/background/repository"
	backgroundtransport "file-workshop/backend/internal/modules/background/transport"
	filesapplication "file-workshop/backend/internal/modules/files/application"
	filesrepository "file-workshop/backend/internal/modules/files/repository"
	filestransport "file-workshop/backend/internal/modules/files/transport"
	"file-workshop/backend/internal/modules/identity/application"
	"file-workshop/backend/internal/modules/identity/domain"
	identityrepository "file-workshop/backend/internal/modules/identity/repository"
	"file-workshop/backend/internal/modules/identity/security"
	identitytransport "file-workshop/backend/internal/modules/identity/transport"
	lifecycleapplication "file-workshop/backend/internal/modules/lifecycle/application"
	lifecyclerepository "file-workshop/backend/internal/modules/lifecycle/repository"
	lifecycletransport "file-workshop/backend/internal/modules/lifecycle/transport"
	organizationsapplication "file-workshop/backend/internal/modules/organizations/application"
	organizationsrepository "file-workshop/backend/internal/modules/organizations/repository"
	organizationstransport "file-workshop/backend/internal/modules/organizations/transport"
	permissionsapplication "file-workshop/backend/internal/modules/permissions/application"
	permissionscache "file-workshop/backend/internal/modules/permissions/cache"
	permissionsrepository "file-workshop/backend/internal/modules/permissions/repository"
	permissionstransport "file-workshop/backend/internal/modules/permissions/transport"
	searchapplication "file-workshop/backend/internal/modules/search/application"
	searchrepository "file-workshop/backend/internal/modules/search/repository"
	searchtransport "file-workshop/backend/internal/modules/search/transport"
	sharesapplication "file-workshop/backend/internal/modules/shares/application"
	sharesrepository "file-workshop/backend/internal/modules/shares/repository"
	sharestransport "file-workshop/backend/internal/modules/shares/transport"
	uploadsapplication "file-workshop/backend/internal/modules/uploads/application"
	uploadsrepository "file-workshop/backend/internal/modules/uploads/repository"
	uploadstransport "file-workshop/backend/internal/modules/uploads/transport"
	usersapplication "file-workshop/backend/internal/modules/users/application"
	usersrepository "file-workshop/backend/internal/modules/users/repository"
	userstransport "file-workshop/backend/internal/modules/users/transport"
	versionsapplication "file-workshop/backend/internal/modules/versions/application"
	versionsrepository "file-workshop/backend/internal/modules/versions/repository"
	versionstransport "file-workshop/backend/internal/modules/versions/transport"
	"file-workshop/backend/internal/platform/cache"
	"file-workshop/backend/internal/platform/config"
	"file-workshop/backend/internal/platform/database"
	"file-workshop/backend/internal/platform/health"
	"file-workshop/backend/internal/platform/httpserver"
	"file-workshop/backend/internal/platform/objectstorage"
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

	objectStorageClient, objectStorageDependency := configureObjectStorage(ctx, cfg, logger)

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
		objectStorageDependency,
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
	permissionsRepository := permissionsrepository.NewPostgreSQL(postgresPool)
	permissionsService := permissionsapplication.NewService(permissionsRepository, permissionsRepository, permissionscache.NewRedisDecisionCache(redisClient), time.Now)
	permissionsHandler := permissionstransport.NewHandler(permissionsService, identityService, cfg.Auth)
	filesRepository := filesrepository.NewPostgreSQL(postgresPool)
	filesService := filesapplication.NewService(filesRepository, filesRepository, permissionsService, time.Now)
	filesHandler := filestransport.NewHandler(filesService, identityService, cfg.Auth)
	uploadsRepository := uploadsrepository.NewPostgreSQL(postgresPool)
	uploadsService := uploadsapplication.NewService(uploadsRepository, uploadsRepository, permissionsService, objectStorageClient, uploadsapplication.Config{Bucket: cfg.ObjectStorage.Bucket, PresignTTL: cfg.ObjectStorage.PresignTTL}, time.Now)
	uploadsHandler := uploadstransport.NewHandler(uploadsService, identityService, cfg.Auth)
	versionsRepository := versionsrepository.NewPostgreSQL(postgresPool)
	versionsService := versionsapplication.NewService(versionsRepository, versionsRepository, permissionsService, time.Now)
	versionsHandler := versionstransport.NewHandler(versionsService, identityService, cfg.Auth)
	sharesRepository := sharesrepository.NewPostgreSQL(postgresPool)
	sharesService := sharesapplication.NewService(sharesRepository, sharesRepository, permissionsService, time.Now)
	sharesHandler := sharestransport.NewHandler(sharesService, identityService, cfg.Auth)
	lifecycleRepository := lifecyclerepository.NewPostgreSQL(postgresPool)
	lifecycleService := lifecycleapplication.NewService(lifecycleRepository, lifecycleRepository, permissionsService, time.Now)
	lifecycleHandler := lifecycletransport.NewHandler(lifecycleService, identityService, cfg.Auth)
	searchRepository := searchrepository.NewPostgreSQL(postgresPool)
	searchService := searchapplication.NewService(searchRepository, permissionsService)
	searchHandler := searchtransport.NewHandler(searchService, identityService, cfg.Auth)
	backgroundRepository := backgroundrepository.NewPostgreSQL(postgresPool)
	backgroundService := backgroundapplication.NewService(backgroundRepository, backgroundRepository, time.Now)
	backgroundHandler := backgroundtransport.NewHandler(backgroundService, identityService, cfg.Auth)
	auditRepository := auditrepository.NewPostgreSQL(postgresPool)
	auditService := auditapplication.NewService(auditRepository, time.Now)
	auditHandler := audittransport.NewHandler(auditService, identityService, cfg.Auth)
	router := httpserver.NewRouter(httpserver.NewAPIHandler(healthHandler, identityHandler, usersHandler, organizationsHandler, permissionsHandler, filesHandler, uploadsHandler, versionsHandler, sharesHandler, lifecycleHandler, searchHandler, backgroundHandler, auditHandler), logger, cfg.Auth.AllowedOrigins)
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

func configureObjectStorage(ctx context.Context, cfg config.Config, logger *slog.Logger) (objectstorage.Client, health.Dependency) {
	if !cfg.ObjectStorage.Enabled {
		return objectstorage.NewDisabledClient(), health.Dependency{Name: "objectStorage", Required: false, Enabled: false}
	}
	client, err := objectstorage.NewS3Client(ctx, objectstorage.S3Config{
		Region:          cfg.ObjectStorage.Region,
		Endpoint:        cfg.ObjectStorage.Endpoint,
		AccessKeyID:     cfg.ObjectStorage.AccessKeyID,
		SecretAccessKey: cfg.ObjectStorage.SecretAccessKey,
		ForcePathStyle:  cfg.ObjectStorage.ForcePathStyle,
		DefaultBucket:   cfg.ObjectStorage.Bucket,
	}, time.Now)
	if err != nil {
		logger.Error("configure object storage failed", "error", err)
		return objectstorage.NewDisabledClient(), health.Dependency{Name: "objectStorage", Required: false, Enabled: true, Check: func(context.Context) error { return err }}
	}
	return client, health.Dependency{Name: "objectStorage", Required: false, Enabled: true, Timeout: cfg.ObjectStorage.HealthTimeout, Check: client.Check}
}
