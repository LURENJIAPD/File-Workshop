package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const envPrefix = "FILE_WORKSHOP_"

var postgresIdentifierPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

type Config struct {
	EnvironmentFile string
	App             AppConfig
	HTTP            HTTPConfig
	Auth            AuthConfig
	Worker          WorkerConfig
	PostgreSQL      PostgreSQLConfig
	Redis           RedisConfig
	ObjectStorage   ObjectStorageConfig
}

type AppConfig struct {
	Environment string
	ServiceName string
}

type HTTPConfig struct {
	Host              string
	Port              int
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxHeaderBytes    int
}

type AuthConfig struct {
	JWTIssuer          string
	JWTAudience        string
	JWTSecret          []byte
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	LoginFailureWindow time.Duration
	LoginFailureLimit  int
	LoginLockDuration  time.Duration
	AccessCookieName   string
	RefreshCookieName  string
	CookieSecure       bool
	CookieSameSite     string
	CookieDomain       string
	AllowedOrigins     []string
}

type WorkerConfig struct {
	WorkerID            string
	Concurrency         int
	BatchSize           int
	PollInterval        time.Duration
	LeaseDuration       time.Duration
	HandlerTimeout      time.Duration
	RetryInitialBackoff time.Duration
	RetryMaxBackoff     time.Duration
	ShutdownTimeout     time.Duration
}

func (c HTTPConfig) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

type PostgreSQLConfig struct {
	Host                      string
	Port                      int
	User                      string
	Password                  string
	Database                  string
	Schema                    string
	SSLMode                   string
	PoolMaxConns              int32
	PoolMinConns              int32
	PoolMaxConnLifetime       time.Duration
	PoolMaxConnLifetimeJitter time.Duration
	PoolMaxConnIdleTime       time.Duration
	PoolHealthCheckPeriod     time.Duration
	ConnectTimeout            time.Duration
	PingTimeout               time.Duration
	StatementTimeout          time.Duration
	LockTimeout               time.Duration
	IdleInTransactionTimeout  time.Duration
}

func (c PostgreSQLConfig) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

func (c PostgreSQLConfig) ConnectionString() string {
	connectionURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   c.Address(),
		Path:   "/" + c.Database,
	}
	query := connectionURL.Query()
	query.Set("sslmode", c.SSLMode)
	connectionURL.RawQuery = query.Encode()
	return connectionURL.String()
}

type RedisConfig struct {
	Host            string
	Port            int
	Password        string
	Database        int
	DialTimeout     time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	PingTimeout     time.Duration
	PoolSize        int
	MinIdleConns    int
	MaxIdleConns    int
	PoolTimeout     time.Duration
	ConnMaxIdleTime time.Duration
	ConnMaxLifetime time.Duration
}

func (c RedisConfig) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

type ObjectStorageConfig struct {
	Enabled         bool
	Provider        string
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	ForcePathStyle  bool
	PresignTTL      time.Duration
	HealthTimeout   time.Duration
}

func Load() (Config, error) {
	environmentFile, err := loadEnvironmentFile()
	if err != nil {
		return Config{}, err
	}

	cfg, err := loadFromEnvironment()
	if err != nil {
		return Config{}, err
	}
	cfg.EnvironmentFile = environmentFile
	return cfg, nil
}

func loadEnvironmentFile() (string, error) {
	if explicitPath := strings.TrimSpace(os.Getenv(envPrefix + "ENV_FILE")); explicitPath != "" {
		return loadDotEnvPath(explicitPath, true)
	}

	for _, candidate := range []string{".env", filepath.Join("backend", ".env")} {
		loadedPath, err := loadDotEnvPath(candidate, false)
		if err != nil {
			return "", err
		}
		if loadedPath != "" {
			return loadedPath, nil
		}
	}
	return "", nil
}

func loadDotEnvPath(path string, required bool) (string, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !required {
			return "", nil
		}
		return "", fmt.Errorf("inspect environment file %q: %w", path, err)
	}
	if !fileInfo.Mode().IsRegular() {
		return "", fmt.Errorf("environment file %q is not a regular file", path)
	}
	if err := godotenv.Load(path); err != nil {
		return "", fmt.Errorf("load environment file %q: %w", path, err)
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve environment file %q: %w", path, err)
	}
	return absolutePath, nil
}

func loadFromEnvironment() (Config, error) {
	var validationErrors []error

	postgresPort := intValue(envPrefix+"POSTGRES_PORT", 5432, 1, 65535, &validationErrors)
	postgresMaxConns := intValue(envPrefix+"POSTGRES_POOL_MAX_CONNS", 10, 1, 1000, &validationErrors)
	postgresMinConns := intValue(envPrefix+"POSTGRES_POOL_MIN_CONNS", 1, 0, 1000, &validationErrors)
	redisPoolSize := intValue(envPrefix+"REDIS_POOL_SIZE", 10, 1, 1000, &validationErrors)
	redisMinIdleConns := intValue(envPrefix+"REDIS_MIN_IDLE_CONNS", 1, 0, 1000, &validationErrors)
	redisMaxIdleConns := intValue(envPrefix+"REDIS_MAX_IDLE_CONNS", 5, 0, 1000, &validationErrors)

	cfg := Config{
		App: AppConfig{
			Environment: stringValue(envPrefix+"ENV", "development"),
			ServiceName: stringValue(envPrefix+"SERVICE_NAME", "file-workshop-server"),
		},
		HTTP: HTTPConfig{
			Host:              stringValue(envPrefix+"HTTP_HOST", "127.0.0.1"),
			Port:              intValue(envPrefix+"HTTP_PORT", 8080, 1, 65535, &validationErrors),
			ReadHeaderTimeout: durationValue(envPrefix+"HTTP_READ_HEADER_TIMEOUT", 5*time.Second, &validationErrors),
			ReadTimeout:       durationValue(envPrefix+"HTTP_READ_TIMEOUT", 15*time.Second, &validationErrors),
			WriteTimeout:      durationValue(envPrefix+"HTTP_WRITE_TIMEOUT", 30*time.Second, &validationErrors),
			IdleTimeout:       durationValue(envPrefix+"HTTP_IDLE_TIMEOUT", 60*time.Second, &validationErrors),
			ShutdownTimeout:   durationValue(envPrefix+"HTTP_SHUTDOWN_TIMEOUT", 15*time.Second, &validationErrors),
			MaxHeaderBytes:    intValue(envPrefix+"HTTP_MAX_HEADER_BYTES", 1<<20, 1024, 16<<20, &validationErrors),
		},
		Auth: AuthConfig{
			JWTIssuer:          stringValue(envPrefix+"AUTH_JWT_ISSUER", "file-workshop"),
			JWTAudience:        stringValue(envPrefix+"AUTH_JWT_AUDIENCE", "file-workshop-api"),
			JWTSecret:          base64Value(envPrefix+"AUTH_JWT_SECRET_BASE64", 32, &validationErrors),
			AccessTokenTTL:     durationValue(envPrefix+"AUTH_ACCESS_TOKEN_TTL", 15*time.Minute, &validationErrors),
			RefreshTokenTTL:    durationValue(envPrefix+"AUTH_REFRESH_TOKEN_TTL", 7*24*time.Hour, &validationErrors),
			LoginFailureWindow: durationValue(envPrefix+"AUTH_LOGIN_FAILURE_WINDOW", 15*time.Minute, &validationErrors),
			LoginFailureLimit:  intValue(envPrefix+"AUTH_LOGIN_FAILURE_LIMIT", 5, 1, 100, &validationErrors),
			LoginLockDuration:  durationValue(envPrefix+"AUTH_LOGIN_LOCK_DURATION", 15*time.Minute, &validationErrors),
			AccessCookieName:   stringValue(envPrefix+"AUTH_ACCESS_COOKIE_NAME", "file_workshop_access"),
			RefreshCookieName:  stringValue(envPrefix+"AUTH_REFRESH_COOKIE_NAME", "file_workshop_refresh"),
			CookieSecure:       boolValue(envPrefix+"AUTH_COOKIE_SECURE", false, &validationErrors),
			CookieSameSite:     strings.ToLower(stringValue(envPrefix+"AUTH_COOKIE_SAME_SITE", "lax")),
			CookieDomain:       strings.TrimSpace(os.Getenv(envPrefix + "AUTH_COOKIE_DOMAIN")),
			AllowedOrigins:     csvValue(envPrefix+"AUTH_ALLOWED_ORIGINS", "http://127.0.0.1:5173,http://localhost:5173"),
		},
		Worker: WorkerConfig{
			WorkerID:            stringValue(envPrefix+"WORKER_ID", ""),
			Concurrency:         intValue(envPrefix+"WORKER_CONCURRENCY", 2, 1, 64, &validationErrors),
			BatchSize:           intValue(envPrefix+"WORKER_BATCH_SIZE", 10, 1, 500, &validationErrors),
			PollInterval:        durationValue(envPrefix+"WORKER_POLL_INTERVAL", time.Second, &validationErrors),
			LeaseDuration:       durationValue(envPrefix+"WORKER_LEASE_DURATION", 30*time.Second, &validationErrors),
			HandlerTimeout:      durationValue(envPrefix+"WORKER_HANDLER_TIMEOUT", 20*time.Second, &validationErrors),
			RetryInitialBackoff: durationValue(envPrefix+"WORKER_RETRY_INITIAL_BACKOFF", 5*time.Second, &validationErrors),
			RetryMaxBackoff:     durationValue(envPrefix+"WORKER_RETRY_MAX_BACKOFF", 5*time.Minute, &validationErrors),
			ShutdownTimeout:     durationValue(envPrefix+"WORKER_SHUTDOWN_TIMEOUT", 15*time.Second, &validationErrors),
		},
		PostgreSQL: PostgreSQLConfig{
			Host:                      requiredString(envPrefix+"POSTGRES_HOST", &validationErrors),
			Port:                      postgresPort,
			User:                      requiredString(envPrefix+"POSTGRES_USER", &validationErrors),
			Password:                  requiredString(envPrefix+"POSTGRES_PASSWORD", &validationErrors),
			Database:                  requiredString(envPrefix+"POSTGRES_DATABASE", &validationErrors),
			Schema:                    stringValue(envPrefix+"POSTGRES_SCHEMA", "file_workshop"),
			SSLMode:                   stringValue(envPrefix+"POSTGRES_SSL_MODE", "disable"),
			PoolMaxConns:              int32(postgresMaxConns),
			PoolMinConns:              int32(postgresMinConns),
			PoolMaxConnLifetime:       durationValue(envPrefix+"POSTGRES_POOL_MAX_CONN_LIFETIME", time.Hour, &validationErrors),
			PoolMaxConnLifetimeJitter: durationValue(envPrefix+"POSTGRES_POOL_MAX_CONN_LIFETIME_JITTER", 10*time.Minute, &validationErrors),
			PoolMaxConnIdleTime:       durationValue(envPrefix+"POSTGRES_POOL_MAX_CONN_IDLE_TIME", 30*time.Minute, &validationErrors),
			PoolHealthCheckPeriod:     durationValue(envPrefix+"POSTGRES_POOL_HEALTH_CHECK_PERIOD", time.Minute, &validationErrors),
			ConnectTimeout:            durationValue(envPrefix+"POSTGRES_CONNECT_TIMEOUT", 5*time.Second, &validationErrors),
			PingTimeout:               durationValue(envPrefix+"POSTGRES_PING_TIMEOUT", 5*time.Second, &validationErrors),
			StatementTimeout:          durationValue(envPrefix+"POSTGRES_STATEMENT_TIMEOUT", 30*time.Second, &validationErrors),
			LockTimeout:               durationValue(envPrefix+"POSTGRES_LOCK_TIMEOUT", 5*time.Second, &validationErrors),
			IdleInTransactionTimeout:  durationValue(envPrefix+"POSTGRES_IDLE_IN_TRANSACTION_TIMEOUT", time.Minute, &validationErrors),
		},
		Redis: RedisConfig{
			Host:            requiredString(envPrefix+"REDIS_HOST", &validationErrors),
			Port:            intValue(envPrefix+"REDIS_PORT", 6379, 1, 65535, &validationErrors),
			Password:        os.Getenv(envPrefix + "REDIS_PASSWORD"),
			Database:        intValue(envPrefix+"REDIS_DATABASE", 0, 0, 15, &validationErrors),
			DialTimeout:     durationValue(envPrefix+"REDIS_DIAL_TIMEOUT", 5*time.Second, &validationErrors),
			ReadTimeout:     durationValue(envPrefix+"REDIS_READ_TIMEOUT", 3*time.Second, &validationErrors),
			WriteTimeout:    durationValue(envPrefix+"REDIS_WRITE_TIMEOUT", 3*time.Second, &validationErrors),
			PingTimeout:     durationValue(envPrefix+"REDIS_PING_TIMEOUT", 3*time.Second, &validationErrors),
			PoolSize:        redisPoolSize,
			MinIdleConns:    redisMinIdleConns,
			MaxIdleConns:    redisMaxIdleConns,
			PoolTimeout:     durationValue(envPrefix+"REDIS_POOL_TIMEOUT", 4*time.Second, &validationErrors),
			ConnMaxIdleTime: durationValue(envPrefix+"REDIS_CONN_MAX_IDLE_TIME", 30*time.Minute, &validationErrors),
			ConnMaxLifetime: durationValue(envPrefix+"REDIS_CONN_MAX_LIFETIME", time.Hour, &validationErrors),
		},
		ObjectStorage: ObjectStorageConfig{
			Enabled:         boolValue(envPrefix+"OBJECT_STORAGE_ENABLED", false, &validationErrors),
			Provider:        stringValue(envPrefix+"OBJECT_STORAGE_PROVIDER", "seaweedfs-s3"),
			Endpoint:        strings.TrimSpace(os.Getenv(envPrefix + "OBJECT_STORAGE_ENDPOINT")),
			Region:          stringValue(envPrefix+"OBJECT_STORAGE_REGION", "us-east-1"),
			Bucket:          strings.TrimSpace(os.Getenv(envPrefix + "OBJECT_STORAGE_BUCKET")),
			AccessKeyID:     strings.TrimSpace(os.Getenv(envPrefix + "OBJECT_STORAGE_ACCESS_KEY_ID")),
			SecretAccessKey: strings.TrimSpace(os.Getenv(envPrefix + "OBJECT_STORAGE_SECRET_ACCESS_KEY")),
			ForcePathStyle:  boolValue(envPrefix+"OBJECT_STORAGE_FORCE_PATH_STYLE", true, &validationErrors),
			PresignTTL:      durationValue(envPrefix+"OBJECT_STORAGE_PRESIGN_TTL", 15*time.Minute, &validationErrors),
			HealthTimeout:   durationValue(envPrefix+"OBJECT_STORAGE_HEALTH_TIMEOUT", 3*time.Second, &validationErrors),
		},
	}

	validateConfig(cfg, &validationErrors)
	if len(validationErrors) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(validationErrors...))
	}
	return cfg, nil
}

func validateConfig(cfg Config, validationErrors *[]error) {
	if cfg.Auth.RefreshTokenTTL <= cfg.Auth.AccessTokenTTL {
		*validationErrors = append(*validationErrors, fmt.Errorf(
			"%sAUTH_REFRESH_TOKEN_TTL must exceed %sAUTH_ACCESS_TOKEN_TTL",
			envPrefix,
			envPrefix,
		))
	}
	if cfg.Auth.AccessCookieName == cfg.Auth.RefreshCookieName {
		*validationErrors = append(*validationErrors, fmt.Errorf("authentication cookie names must be different"))
	}
	switch cfg.Auth.CookieSameSite {
	case "strict", "lax":
	case "none":
		if !cfg.Auth.CookieSecure {
			*validationErrors = append(*validationErrors, fmt.Errorf("%sAUTH_COOKIE_SECURE must be true when SameSite is none", envPrefix))
		}
	default:
		*validationErrors = append(*validationErrors, fmt.Errorf("%sAUTH_COOKIE_SAME_SITE must be strict, lax, or none", envPrefix))
	}
	for _, origin := range cfg.Auth.AllowedOrigins {
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			*validationErrors = append(*validationErrors, fmt.Errorf("%sAUTH_ALLOWED_ORIGINS contains invalid origin %q", envPrefix, origin))
		}
	}
	if cfg.PostgreSQL.PoolMinConns > cfg.PostgreSQL.PoolMaxConns {
		*validationErrors = append(*validationErrors, fmt.Errorf(
			"%sPOSTGRES_POOL_MIN_CONNS must not exceed %sPOSTGRES_POOL_MAX_CONNS",
			envPrefix,
			envPrefix,
		))
	}
	if !postgresIdentifierPattern.MatchString(cfg.PostgreSQL.Schema) {
		*validationErrors = append(*validationErrors, fmt.Errorf(
			"%sPOSTGRES_SCHEMA must be a lowercase PostgreSQL identifier",
			envPrefix,
		))
	}
	allowedSSLModes := map[string]struct{}{
		"disable": {}, "allow": {}, "prefer": {}, "require": {}, "verify-ca": {}, "verify-full": {},
	}
	if _, allowed := allowedSSLModes[cfg.PostgreSQL.SSLMode]; !allowed {
		*validationErrors = append(*validationErrors, fmt.Errorf("%sPOSTGRES_SSL_MODE is invalid", envPrefix))
	}
	if cfg.Redis.MinIdleConns > cfg.Redis.PoolSize {
		*validationErrors = append(*validationErrors, fmt.Errorf(
			"%sREDIS_MIN_IDLE_CONNS must not exceed %sREDIS_POOL_SIZE",
			envPrefix,
			envPrefix,
		))
	}
	if cfg.Redis.MaxIdleConns > cfg.Redis.PoolSize {
		*validationErrors = append(*validationErrors, fmt.Errorf(
			"%sREDIS_MAX_IDLE_CONNS must not exceed %sREDIS_POOL_SIZE",
			envPrefix,
			envPrefix,
		))
	}
	if cfg.Worker.HandlerTimeout >= cfg.Worker.LeaseDuration {
		*validationErrors = append(*validationErrors, fmt.Errorf("%sWORKER_HANDLER_TIMEOUT must be shorter than %sWORKER_LEASE_DURATION", envPrefix, envPrefix))
	}
	if cfg.Worker.RetryMaxBackoff < cfg.Worker.RetryInitialBackoff {
		*validationErrors = append(*validationErrors, fmt.Errorf("%sWORKER_RETRY_MAX_BACKOFF must not be shorter than %sWORKER_RETRY_INITIAL_BACKOFF", envPrefix, envPrefix))
	}
	if cfg.ObjectStorage.Enabled {
		if strings.TrimSpace(cfg.ObjectStorage.Provider) == "" {
			*validationErrors = append(*validationErrors, fmt.Errorf("%sOBJECT_STORAGE_PROVIDER is required when object storage is enabled", envPrefix))
		}
		if strings.TrimSpace(cfg.ObjectStorage.Endpoint) == "" {
			*validationErrors = append(*validationErrors, fmt.Errorf("%sOBJECT_STORAGE_ENDPOINT is required when object storage is enabled", envPrefix))
		}
		if strings.TrimSpace(cfg.ObjectStorage.Bucket) == "" {
			*validationErrors = append(*validationErrors, fmt.Errorf("%sOBJECT_STORAGE_BUCKET is required when object storage is enabled", envPrefix))
		}
		if strings.TrimSpace(cfg.ObjectStorage.AccessKeyID) == "" || strings.TrimSpace(cfg.ObjectStorage.SecretAccessKey) == "" {
			*validationErrors = append(*validationErrors, fmt.Errorf("%sOBJECT_STORAGE_ACCESS_KEY_ID and %sOBJECT_STORAGE_SECRET_ACCESS_KEY are required when object storage is enabled", envPrefix, envPrefix))
		}
	}
}

func stringValue(key, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return defaultValue
}

func requiredString(key string, validationErrors *[]error) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		*validationErrors = append(*validationErrors, fmt.Errorf("%s is required", key))
	}
	return value
}

func intValue(key string, defaultValue, minimum, maximum int, validationErrors *[]error) int {
	rawValue := strings.TrimSpace(os.Getenv(key))
	if rawValue == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(rawValue)
	if err != nil || value < minimum || value > maximum {
		*validationErrors = append(*validationErrors, fmt.Errorf("%s must be an integer between %d and %d", key, minimum, maximum))
		return defaultValue
	}
	return value
}

func durationValue(key string, defaultValue time.Duration, validationErrors *[]error) time.Duration {
	rawValue := strings.TrimSpace(os.Getenv(key))
	if rawValue == "" {
		return defaultValue
	}
	value, err := time.ParseDuration(rawValue)
	if err != nil || value <= 0 {
		*validationErrors = append(*validationErrors, fmt.Errorf("%s must be a positive Go duration", key))
		return defaultValue
	}
	return value
}

func boolValue(key string, defaultValue bool, validationErrors *[]error) bool {
	rawValue := strings.TrimSpace(os.Getenv(key))
	if rawValue == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(rawValue)
	if err != nil {
		*validationErrors = append(*validationErrors, fmt.Errorf("%s must be a boolean", key))
		return defaultValue
	}
	return value
}

func base64Value(key string, minimumBytes int, validationErrors *[]error) []byte {
	rawValue := strings.TrimSpace(os.Getenv(key))
	if rawValue == "" {
		*validationErrors = append(*validationErrors, fmt.Errorf("%s is required", key))
		return nil
	}
	value, err := base64.StdEncoding.DecodeString(rawValue)
	if err != nil || len(value) < minimumBytes {
		*validationErrors = append(*validationErrors, fmt.Errorf("%s must be standard Base64 encoding at least %d random bytes", key, minimumBytes))
		return nil
	}
	return value
}

func csvValue(key, defaultValue string) []string {
	rawValue := strings.TrimSpace(os.Getenv(key))
	if rawValue == "" {
		rawValue = defaultValue
	}
	parts := strings.Split(rawValue, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}
