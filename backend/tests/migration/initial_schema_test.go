package migration_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"file-workshop/backend/internal/platform/config"
	"file-workshop/backend/internal/platform/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const integrationEnvironment = "FILE_WORKSHOP_RUN_INTEGRATION"

var temporaryDatabaseName = regexp.MustCompile(`^file_workshop_migration_check_[a-f0-9]{32}$`)

func TestInitialMigrationOnEmptyDatabase(t *testing.T) {
	if os.Getenv(integrationEnvironment) != "1" {
		t.Skip("set FILE_WORKSHOP_RUN_INTEGRATION=1 to run PostgreSQL migration integration tests")
	}

	backendRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve backend root: %v", err)
	}
	t.Setenv("FILE_WORKSHOP_ENV_FILE", filepath.Join(backendRoot, ".env"))
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	adminConnection, err := pgx.Connect(ctx, cfg.PostgreSQL.ConnectionString())
	if err != nil {
		t.Fatalf("connect PostgreSQL admin database: %v", err)
	}
	defer adminConnection.Close(context.Background())

	databaseName := "file_workshop_migration_check_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if !temporaryDatabaseName.MatchString(databaseName) {
		t.Fatalf("refuse unsafe temporary database name %q", databaseName)
	}
	identifier := pgx.Identifier{databaseName}.Sanitize()
	if _, err := adminConnection.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatalf("create temporary database %q: %v", databaseName, err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if !temporaryDatabaseName.MatchString(databaseName) {
			t.Errorf("refuse unsafe temporary database cleanup target %q", databaseName)
			return
		}
		cleanupConnection, err := pgx.Connect(cleanupContext, cfg.PostgreSQL.ConnectionString())
		if err != nil {
			t.Errorf("connect for temporary database cleanup: %v", err)
			return
		}
		defer cleanupConnection.Close(context.Background())
		if _, err := cleanupConnection.Exec(cleanupContext, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Errorf("drop temporary database %q: %v", databaseName, err)
		}
	})

	temporaryConfig := cfg.PostgreSQL
	temporaryConfig.Database = databaseName
	runGoose(t, ctx, backendRoot, temporaryConfig.ConnectionString(), "up")

	pool, err := database.OpenPostgreSQL(ctx, cfg.App, temporaryConfig)
	if err != nil {
		t.Fatalf("open migrated PostgreSQL database: %v", err)
	}
	info, err := database.InspectPostgreSQL(ctx, pool, temporaryConfig.Schema)
	pool.Close()
	if err != nil {
		t.Fatalf("inspect migrated PostgreSQL database: %v", err)
	}
	if !info.SchemaAvailable || info.CurrentSchema != temporaryConfig.Schema {
		t.Fatalf("migration did not establish expected schema: %#v", info)
	}

	runGoose(t, ctx, backendRoot, temporaryConfig.ConnectionString(), "down")
	verificationConnection, err := pgx.Connect(ctx, temporaryConfig.ConnectionString())
	if err != nil {
		t.Fatalf("connect rolled back PostgreSQL database: %v", err)
	}
	defer verificationConnection.Close(context.Background())
	var schemaExists bool
	if err := verificationConnection.QueryRow(
		ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)",
		temporaryConfig.Schema,
	).Scan(&schemaExists); err != nil {
		t.Fatalf("verify migration rollback: %v", err)
	}
	if schemaExists {
		t.Fatalf("schema %q still exists after migration rollback", temporaryConfig.Schema)
	}
}

func runGoose(t *testing.T, ctx context.Context, backendRoot, connectionString, command string) {
	t.Helper()
	gooseCommand := exec.CommandContext(ctx, "go", "tool", "goose", "-dir", filepath.Join(backendRoot, "migrations"), command)
	gooseCommand.Dir = backendRoot
	gooseCommand.Env = append(
		os.Environ(),
		"GOOSE_DRIVER=postgres",
		"GOOSE_DBSTRING="+connectionString,
	)
	output, err := gooseCommand.CombinedOutput()
	if err != nil {
		t.Fatalf("goose %s failed: %v\n%s", command, err, sanitizeGooseOutput(string(output), connectionString))
	}
}

func sanitizeGooseOutput(output, connectionString string) string {
	output = strings.ReplaceAll(output, connectionString, "<redacted-connection-string>")
	if len(output) > 4000 {
		return fmt.Sprintf("%s\n... output truncated", output[:4000])
	}
	return output
}
