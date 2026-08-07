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

func TestPopularModelSeedContainsAuditedOfficialRates(t *testing.T) {
	contents, err := fs.ReadFile(VersionedSQL, "migrations/0009_seed_popular_model_catalog.sql")
	if err != nil {
		t.Fatalf("read model seed migration: %v", err)
	}
	sql := string(contents)
	expectedRows := []string{
		"'DeepSeek', 'deepseek-v4-flash', 'DeepSeek V4 Flash', 1000000, 2000000, 20000",
		"'DeepSeek', 'deepseek-v4-pro', 'DeepSeek V4 Pro', 3000000, 6000000, 25000",
		"'智谱 GLM', 'glm-5.2', 'GLM-5.2', 8000000, 28000000, 2000000",
		"'智谱 GLM', 'glm-4.7-flashx', 'GLM-4.7 FlashX', 500000, 3000000, 100000",
		"'Kimi', 'kimi-k3', 'Kimi K3', 20000000, 100000000, 2000000",
		"'Kimi', 'kimi-k2.7-code', 'Kimi K2.7 Code', 6500000, 27000000, 1300000",
		"'Kimi', 'kimi-k2.7-code-highspeed', 'Kimi K2.7 Code HighSpeed', 13000000, 54000000, 2600000",
		"'Kimi', 'kimi-k2.6', 'Kimi K2.6', 6500000, 27000000, 1100000",
	}
	for _, expected := range expectedRows {
		if !strings.Contains(sql, expected) {
			t.Fatalf("model seed is missing audited rate row %q", expected)
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
