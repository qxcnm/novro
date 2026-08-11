package migrate

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestReadMigrationsSortsAndHashesFiles(t *testing.T) {
	source := fstest.MapFS{
		"migrations/0002_second.sql": {Data: []byte("SELECT 2;")},
		"migrations/0001_first.sql":  {Data: []byte("SELECT 1;")},
	}
	migrations, err := readMigrations(source)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	expectedHash := sha256.Sum256([]byte("SELECT 1;"))
	if len(migrations) != 2 || migrations[0].Version != "0001_first" || migrations[0].SQL != "SELECT 1;" || migrations[0].Checksum != fmt.Sprintf("%x", expectedHash) {
		t.Fatalf("unexpected migrations: %+v", migrations)
	}
}

func TestVersionedSQLContainsInitialAndProviderWeightMigrations(t *testing.T) {
	entries, err := fs.ReadDir(VersionedSQL, "migrations")
	if err != nil {
		t.Fatalf("read migration directory: %v", err)
	}
	if len(entries) != 2 || entries[0].IsDir() || entries[0].Name() != "0001_initial_schema.sql" || entries[1].IsDir() || entries[1].Name() != "0002_provider_weight.sql" {
		t.Fatalf("expected initial and provider weight migrations, got %+v", entries)
	}
}

func TestInitialMigrationContainsCurrentSchemaInvariants(t *testing.T) {
	contents, err := fs.ReadFile(VersionedSQL, "migrations/0001_initial_schema.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(contents)
	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS users",
		"is_system_admin BOOLEAN NOT NULL DEFAULT FALSE",
		"CREATE TABLE IF NOT EXISTS api_keys",
		"api_keys_billing_groups_api_keys",
		"CREATE TABLE IF NOT EXISTS providers",
		"providers_billing_groups_providers",
		"UNIQUE KEY upstreammodel_upstream_name (upstream_name)",
		"UNIQUE KEY modelroute_public_name_provider_id_upstream_model_id",
		"ON DELETE SET NULL ON UPDATE NO ACTION",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("initial migration is missing %q", expected)
		}
	}
	users, ok := migrationTableDefinition(sql, "users")
	if !ok {
		t.Fatal("initial migration is missing users table definition")
	}
	if strings.Contains(users, "billing_group_id") {
		t.Fatal("users table must not retain account-level billing group")
	}
	for _, table := range []string{"api_keys", "providers"} {
		definition, found := migrationTableDefinition(sql, table)
		if !found {
			t.Fatalf("initial migration is missing %s table definition", table)
		}
		if !strings.Contains(definition, "billing_group_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL") {
			t.Fatalf("%s table must require billing_group_id", table)
		}
	}
}

func TestInitialMigrationSeedsOnlyRequiredDefaults(t *testing.T) {
	contents, err := fs.ReadFile(VersionedSQL, "migrations/0001_initial_schema.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(contents)
	for _, expected := range []string{
		"'00000000-0000-0000-0000-000000000001', 'default', '默认分组', 10000, TRUE, 'active'",
		"'referral_reward_bps', '1000'",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("initial migration is missing required seed %q", expected)
		}
	}
	for _, removedSeed := range []string{"deepseek-v4-flash", "glm-5.2", "kimi-k3"} {
		if strings.Contains(sql, removedSeed) {
			t.Fatalf("initial migration should not seed model catalog row %q", removedSeed)
		}
	}
}

func TestValidateAppliedMigrationsRejectsDriftAndMissingFiles(t *testing.T) {
	migrations := []migrationFile{{Version: "0001_first", Checksum: strings.Repeat("a", 64)}}
	if _, err := validateAppliedMigrations(migrations, map[string]string{"0001_first": strings.Repeat("b", 64)}); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
	if _, err := validateAppliedMigrations(migrations, map[string]string{"0000_missing": strings.Repeat("a", 64)}); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing migration error, got %v", err)
	}
}

func TestValidateAppliedMigrationsReturnsLegacyBackfill(t *testing.T) {
	migrations := []migrationFile{
		{Version: "0002_second", Checksum: strings.Repeat("b", 64)},
		{Version: "0001_first", Checksum: strings.Repeat("a", 64)},
	}
	legacy, err := validateAppliedMigrations(migrations, map[string]string{"0002_second": "", "0001_first": ""})
	if err != nil {
		t.Fatalf("validate legacy migrations: %v", err)
	}
	if len(legacy) != 2 || legacy[0].Version != "0001_first" || legacy[1].Version != "0002_second" {
		t.Fatalf("unexpected legacy backfill order: %+v", legacy)
	}
}

func migrationTableDefinition(sql, table string) (string, bool) {
	marker := "CREATE TABLE IF NOT EXISTS " + table + " ("
	start := strings.Index(sql, marker)
	if start < 0 {
		return "", false
	}
	remainder := sql[start:]
	end := strings.Index(remainder, "\n) ENGINE=")
	if end < 0 {
		return "", false
	}
	return remainder[:end], true
}
