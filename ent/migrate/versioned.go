package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// VersionedSQL contains the migrations reviewed and deployed with Novro. The
// generated Ent schema remains available for model-level inspection, while
// production startup never performs automatic schema creation.
//
//go:embed migrations/*.sql
var VersionedSQL embed.FS

// Apply runs each migration once and records it in a small metadata table.
// Migrations are executed explicitly by the deployment command.
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
    applied_at DATETIME(6) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`)
	if err != nil {
		return fmt.Errorf("create migration metadata table: %w", err)
	}

	applied := make(map[string]struct{})
	rows, err := conn.QueryContext(ctx, `SELECT version FROM novro_schema_migrations`)
	if err != nil {
		return fmt.Errorf("read migration metadata: %w", err)
	}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan migration metadata: %w", err)
		}
		applied[version] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate migration metadata: %w", err)
	}
	_ = rows.Close()

	files, err := fs.Glob(VersionedSQL, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(files)
	for _, file := range files {
		version := strings.TrimSuffix(path.Base(file), ".sql")
		if _, ok := applied[version]; ok {
			continue
		}
		contents, err := fs.ReadFile(VersionedSQL, file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}
		if _, err := conn.ExecContext(ctx, string(contents)); err != nil {
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO novro_schema_migrations (version, applied_at) VALUES (?, CURRENT_TIMESTAMP(6))`,
			version,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}
	}

	return nil
}
