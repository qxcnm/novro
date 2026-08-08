package main

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/novro-gateway/novro/internal/auth"
)

func TestOptionalOIDCServicePreservesDisabledState(t *testing.T) {
	if service := optionalOIDCService(nil); service != nil {
		t.Fatal("disabled OIDC client must remain a nil service")
	}

	client := &auth.OIDCClient{}
	if service := optionalOIDCService(client); service == nil {
		t.Fatal("configured OIDC client must remain available")
	}
}

func TestApplyPendingMigrations(t *testing.T) {
	database := &sql.DB{}
	called := false
	err := applyPendingMigrations(context.Background(), database, func(ctx context.Context, db *sql.DB) error {
		called = true
		if ctx == nil || db != database {
			t.Fatal("migration applier received unexpected dependencies")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("apply pending migrations: %v", err)
	}
	if !called {
		t.Fatal("migration applier was not called")
	}
}

func TestApplyPendingMigrationsWrapsFailure(t *testing.T) {
	expected := errors.New("migration failed")
	err := applyPendingMigrations(context.Background(), nil, func(context.Context, *sql.DB) error {
		return expected
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected wrapped migration failure, got %v", err)
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("NOVRO_TEST_DEFAULT", "")
	if value := envOrDefault("NOVRO_TEST_DEFAULT", "fallback"); value != "fallback" {
		t.Fatalf("expected fallback value, got %q", value)
	}
	t.Setenv("NOVRO_TEST_DEFAULT", "configured")
	if value := envOrDefault("NOVRO_TEST_DEFAULT", "fallback"); value != "configured" {
		t.Fatalf("expected configured value, got %q", value)
	}
}
