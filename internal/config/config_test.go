package config

import (
	"strings"
	"testing"
	"time"
)

func testEnv() map[string]string {
	return map[string]string{
		"NOVRO_ENVIRONMENT":                "test",
		"NOVRO_DATABASE_HOST":              "127.0.0.1",
		"NOVRO_DATABASE_NAME":              "novro-db",
		"NOVRO_DATABASE_USER":              "novro_app",
		"NOVRO_DATABASE_PASSWORD":          "temporary-password",
		"NOVRO_DATABASE_TLS":               "true",
		"NOVRO_SESSION_SECRET":             "01234567890123456789012345678901",
		"NOVRO_PROVIDER_ENCRYPTION_SECRET": "provider-secret-0123456789012345",
		"NOVRO_SESSION_TTL":                "2h",
		"NOVRO_PUBLIC_URL":                 "http://localhost:3000",
		"NOVRO_ALLOWED_ORIGINS":            "http://localhost:3000, http://localhost:3001/",
		"NOVRO_DATABASE_CONN_MAX_LIFETIME": "15m",
		"NOVRO_DATABASE_MAX_OPEN_CONNS":    "12",
		"NOVRO_DATABASE_MAX_IDLE_CONNS":    "4",
	}
}

func fromMap(values map[string]string) (string, bool) {
	value, ok := values["_"]
	return value, ok
}

func loadMap(values map[string]string) (Config, error) {
	return loadEnv(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func TestLoadValidatesAndAppliesDefaults(t *testing.T) {
	cfg, err := loadMap(testEnv())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Database.Name != "novro-db" || cfg.Database.Port != 3306 {
		t.Fatalf("unexpected database config: %+v", cfg.Database)
	}
	if cfg.Session.TTL != 2*time.Hour || cfg.Session.CookieName != defaultCookieName {
		t.Fatalf("unexpected session config: %+v", cfg.Session)
	}
	if len(cfg.AllowedOrigin) != 2 || cfg.AllowedOrigin[1] != "http://localhost:3001" {
		t.Fatalf("unexpected origins: %#v", cfg.AllowedOrigin)
	}
	if !cfg.Auth.RegistrationEnabled || cfg.Auth.OIDC.Enabled() || cfg.Auth.PublicURL != "http://localhost:3000" {
		t.Fatalf("unexpected authentication config: %+v", cfg.Auth)
	}
	if !strings.Contains(cfg.Database.DSN(), "tls=skip-verify") || !strings.Contains(cfg.Database.DSN(), "multiStatements=true") {
		t.Fatalf("DSN does not contain required connection options: %s", redactDSN(cfg.Database.DSN()))
	}
}

func TestLoadValidatesOIDCAndSetupConfiguration(t *testing.T) {
	values := testEnv()
	values["NOVRO_OIDC_ISSUER"] = "https://id.example.com"
	if _, err := loadMap(values); err == nil || !strings.Contains(err.Error(), "OIDC") {
		t.Fatalf("expected incomplete OIDC error, got %v", err)
	}

	values = testEnv()
	values["NOVRO_OIDC_ISSUER"] = "https://id.example.com"
	values["NOVRO_OIDC_CLIENT_ID"] = "novro"
	if _, err := loadMap(values); err == nil || !strings.Contains(err.Error(), "CLIENT_SECRET") {
		t.Fatalf("expected missing OIDC client secret error, got %v", err)
	}

	values = testEnv()
	values["NOVRO_OIDC_ISSUER"] = "http://id.example.com"
	values["NOVRO_OIDC_CLIENT_ID"] = "novro"
	values["NOVRO_OIDC_CLIENT_SECRET"] = "client-secret"
	if _, err := loadMap(values); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("expected insecure OIDC issuer error, got %v", err)
	}

	values = testEnv()
	values["NOVRO_OIDC_ISSUER"] = "https://id.example.com"
	values["NOVRO_OIDC_CLIENT_ID"] = "novro"
	values["NOVRO_OIDC_CLIENT_SECRET"] = "client-secret"
	cfg, err := loadMap(values)
	if err != nil || !cfg.Auth.OIDC.Enabled() || cfg.Auth.OIDC.ClientSecret != "client-secret" {
		t.Fatalf("valid OIDC configuration rejected: cfg=%+v err=%v", cfg.Auth.OIDC, err)
	}

	values = testEnv()
	values["NOVRO_SETUP_TOKEN"] = "short"
	if _, err := loadMap(values); err == nil || !strings.Contains(err.Error(), "SETUP_TOKEN") {
		t.Fatalf("expected short setup token error, got %v", err)
	}
}

func TestLoadRejectsMissingSecretAndInsecureProduction(t *testing.T) {
	values := testEnv()
	delete(values, "NOVRO_SESSION_SECRET")
	if _, err := loadMap(values); err == nil || !strings.Contains(err.Error(), "NOVRO_SESSION_SECRET") {
		t.Fatalf("expected missing secret error, got %v", err)
	}

	values = testEnv()
	values["NOVRO_ENVIRONMENT"] = "production"
	values["NOVRO_HTTP_ADDR"] = "127.0.0.1:8080"
	values["NOVRO_PUBLIC_URL"] = "https://novro.example.com"
	values["NOVRO_ALLOWED_ORIGINS"] = "https://novro.example.com"
	values["NOVRO_DATABASE_TLS"] = "false"
	if _, err := loadMap(values); err == nil || !strings.Contains(err.Error(), "production") {
		t.Fatalf("expected production transport error, got %v", err)
	}

	values = testEnv()
	values["NOVRO_ENVIRONMENT"] = "production"
	values["NOVRO_HTTP_ADDR"] = "127.0.0.1:8080"
	values["NOVRO_PUBLIC_URL"] = "https://novro.example.com"
	values["NOVRO_ALLOWED_ORIGINS"] = "https://novro.example.com"
	delete(values, "NOVRO_PROVIDER_ENCRYPTION_SECRET")
	if _, err := loadMap(values); err == nil || !strings.Contains(err.Error(), "PROVIDER_ENCRYPTION_SECRET") {
		t.Fatalf("expected missing provider encryption secret error, got %v", err)
	}

}

func TestLoadRejectsInvalidPoolAndAddress(t *testing.T) {
	values := testEnv()
	values["NOVRO_HTTP_ADDR"] = "localhost"
	if _, err := loadMap(values); err == nil || !strings.Contains(err.Error(), "NOVRO_HTTP_ADDR") {
		t.Fatalf("expected address error, got %v", err)
	}

	values = testEnv()
	values["NOVRO_DATABASE_MAX_IDLE_CONNS"] = "20"
	if _, err := loadMap(values); err == nil || !strings.Contains(err.Error(), "MAX_IDLE") {
		t.Fatalf("expected pool error, got %v", err)
	}
}

func TestLoadRestrictsProductionListenerAndAllowedOrigins(t *testing.T) {
	production := func() map[string]string {
		values := testEnv()
		values["NOVRO_ENVIRONMENT"] = "production"
		values["NOVRO_HTTP_ADDR"] = "127.0.0.1:8080"
		values["NOVRO_PUBLIC_URL"] = "https://novro.example.com"
		values["NOVRO_ALLOWED_ORIGINS"] = "https://novro.example.com"
		values["NOVRO_SESSION_COOKIE_SECURE"] = "true"
		return values
	}

	values := production()
	values["NOVRO_HTTP_ADDR"] = "0.0.0.0:8080"
	if _, err := loadMap(values); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected public production listener error, got %v", err)
	}

	values = production()
	values["NOVRO_ALLOWED_ORIGINS"] = "http://novro.example.com"
	if _, err := loadMap(values); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("expected insecure production origin error, got %v", err)
	}

	for _, origin := range []string{
		"https://user@novro.example.com",
		"https://novro.example.com/console",
		"https://novro.example.com?source=test",
		"ftp://novro.example.com",
	} {
		values = testEnv()
		values["NOVRO_ALLOWED_ORIGINS"] = origin
		if _, err := loadMap(values); err == nil || !strings.Contains(err.Error(), "NOVRO_ALLOWED_ORIGINS") {
			t.Fatalf("origin %q was not rejected: %v", origin, err)
		}
	}

	values = production()
	if _, err := loadMap(values); err != nil {
		t.Fatalf("valid production network configuration rejected: %v", err)
	}
}

func redactDSN(dsn string) string {
	if index := strings.Index(dsn, "@tcp("); index >= 0 {
		return "<redacted>" + dsn[index:]
	}
	return "<redacted>"
}
