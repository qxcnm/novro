package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// VersionedSQL contains the migrations reviewed and deployed with Novro. The
// generated Ent schema remains available for model-level inspection; startup
// applies only these ordered SQL files and never uses automatic schema creation.
//
//go:embed migrations/*.sql
var VersionedSQL embed.FS

type migrationFile struct {
	Version  string
	SQL      string
	Checksum string
}

// Apply runs each migration once and records it in a small metadata table. It
// is shared by normal startup and the explicit deployment migration command.
func Apply(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("migration database is nil")
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve migration connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var locked int
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK('novro_schema_migrations', 30)`).Scan(&locked); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	if locked != 1 {
		return fmt.Errorf("acquire migration lock: timed out")
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT RELEASE_LOCK('novro_schema_migrations')`)
	}()

	_, err = conn.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS novro_schema_migrations (
    version VARCHAR(128) NOT NULL PRIMARY KEY,
	checksum CHAR(64) NOT NULL DEFAULT '',
    applied_at DATETIME(6) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`)
	if err != nil {
		return fmt.Errorf("create migration metadata table: %w", err)
	}
	var checksumColumns int
	if err := conn.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'novro_schema_migrations'
  AND column_name = 'checksum'`).Scan(&checksumColumns); err != nil {
		return fmt.Errorf("inspect migration metadata: %w", err)
	}
	if checksumColumns == 0 {
		if _, err := conn.ExecContext(ctx, `ALTER TABLE novro_schema_migrations ADD COLUMN checksum CHAR(64) NOT NULL DEFAULT '' AFTER version`); err != nil {
			return fmt.Errorf("upgrade migration metadata: %w", err)
		}
	}

	migrations, err := readMigrations(VersionedSQL)
	if err != nil {
		return err
	}

	applied := make(map[string]string)
	rows, err := conn.QueryContext(ctx, `SELECT version, checksum FROM novro_schema_migrations`)
	if err != nil {
		return fmt.Errorf("read migration metadata: %w", err)
	}
	for rows.Next() {
		var version, checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan migration metadata: %w", err)
		}
		applied[version] = checksum
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate migration metadata: %w", err)
	}
	_ = rows.Close()

	legacy, err := validateAppliedMigrations(migrations, applied)
	if err != nil {
		return err
	}
	for _, migration := range legacy {
		if _, err := conn.ExecContext(ctx, `UPDATE novro_schema_migrations SET checksum = ? WHERE version = ? AND checksum = ''`, migration.Checksum, migration.Version); err != nil {
			return fmt.Errorf("backfill migration checksum %s: %w", migration.Version, err)
		}
		applied[migration.Version] = migration.Checksum
	}

	for _, migration := range migrations {
		if _, ok := applied[migration.Version]; ok {
			continue
		}
		if _, err := conn.ExecContext(ctx, migration.SQL); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.Version, err)
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO novro_schema_migrations (version, checksum, applied_at) VALUES (?, ?, CURRENT_TIMESTAMP(6))`,
			migration.Version,
			migration.Checksum,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", migration.Version, err)
		}
	}

	return nil
}

func readMigrations(source fs.FS) ([]migrationFile, error) {
	files, err := fs.Glob(source, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files)
	migrations := make([]migrationFile, 0, len(files))
	for _, file := range files {
		contents, err := fs.ReadFile(source, file)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", path.Base(file), err)
		}
		digest := sha256.Sum256(contents)
		migrations = append(migrations, migrationFile{
			Version:  strings.TrimSuffix(path.Base(file), ".sql"),
			SQL:      string(contents),
			Checksum: fmt.Sprintf("%x", digest),
		})
	}
	if len(migrations) == 0 {
		return nil, fmt.Errorf("no versioned migrations found")
	}
	return migrations, nil
}

func validateAppliedMigrations(migrations []migrationFile, applied map[string]string) ([]migrationFile, error) {
	available := make(map[string]migrationFile, len(migrations))
	for _, migration := range migrations {
		available[migration.Version] = migration
	}
	legacy := make([]migrationFile, 0)
	for version, checksum := range applied {
		migration, ok := available[version]
		if !ok {
			return nil, fmt.Errorf("applied migration %s is missing from this release", version)
		}
		if checksum == "" {
			legacy = append(legacy, migration)
			continue
		}
		if !strings.EqualFold(checksum, migration.Checksum) {
			return nil, fmt.Errorf("applied migration %s checksum does not match this release", version)
		}
	}
	sort.Slice(legacy, func(i, j int) bool { return legacy[i].Version < legacy[j].Version })
	return legacy, nil
}
