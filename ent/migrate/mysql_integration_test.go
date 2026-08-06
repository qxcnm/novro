package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

func TestMySQLMigrationChecksumsUpgradeAndRejectDrift(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("NOVRO_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("set NOVRO_TEST_MYSQL_DSN to run the MySQL migration integration test")
	}
	serverConfig, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse NOVRO_TEST_MYSQL_DSN: %v", err)
	}
	serverConfig.DBName = ""
	serverConfig.MultiStatements = true
	serverConfig.ParseTime = true
	serverConfig.Loc = time.UTC
	connector, err := mysql.NewConnector(serverConfig)
	if err != nil {
		t.Fatalf("create MySQL integration connector: %v", err)
	}
	adminDB := sql.OpenDB(connector)
	t.Cleanup(func() { _ = adminDB.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("connect to MySQL integration server: %v", err)
	}
	databaseName := "novro_test_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci", databaseName)); err != nil {
		t.Fatalf("create isolated migration database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = adminDB.ExecContext(cleanupCtx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", databaseName))
	})

	databaseConfig := *serverConfig
	databaseConfig.DBName = databaseName
	databaseConnector, err := mysql.NewConnector(&databaseConfig)
	if err != nil {
		t.Fatalf("create isolated database connector: %v", err)
	}
	database := sql.OpenDB(databaseConnector)
	t.Cleanup(func() { _ = database.Close() })
	if err := database.PingContext(ctx); err != nil {
		t.Fatalf("connect to isolated migration database: %v", err)
	}

	if err := Apply(ctx, database); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := Apply(ctx, database); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
	assertMigrationChecksums(t, ctx, database)

	if _, err := database.ExecContext(ctx, `ALTER TABLE novro_schema_migrations DROP COLUMN checksum`); err != nil {
		t.Fatalf("simulate legacy migration metadata: %v", err)
	}
	if err := Apply(ctx, database); err != nil {
		t.Fatalf("upgrade legacy migration metadata: %v", err)
	}
	assertMigrationChecksums(t, ctx, database)

	migrations, err := readMigrations(VersionedSQL)
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE novro_schema_migrations SET checksum = ? WHERE version = ?`, strings.Repeat("0", 64), migrations[0].Version); err != nil {
		t.Fatalf("alter recorded checksum: %v", err)
	}
	if err := Apply(ctx, database); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected checksum drift rejection, got %v", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE novro_schema_migrations SET checksum = ? WHERE version = ?`, migrations[0].Checksum, migrations[0].Version); err != nil {
		t.Fatalf("restore recorded checksum: %v", err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO novro_schema_migrations (version, checksum, applied_at) VALUES ('9999_missing', ?, CURRENT_TIMESTAMP(6))`, strings.Repeat("1", 64)); err != nil {
		t.Fatalf("insert missing migration metadata: %v", err)
	}
	if err := Apply(ctx, database); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing migration rejection, got %v", err)
	}
}

func assertMigrationChecksums(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	var total, valid int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*), SUM(CHAR_LENGTH(checksum) = 64) FROM novro_schema_migrations`).Scan(&total, &valid); err != nil {
		t.Fatalf("read migration checksums: %v", err)
	}
	if total == 0 || valid != total {
		t.Fatalf("migration checksums are incomplete: total=%d valid=%d", total, valid)
	}
}
