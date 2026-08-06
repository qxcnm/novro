package migrate

import (
	"crypto/sha256"
	"fmt"
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
