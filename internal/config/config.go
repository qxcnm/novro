package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

const (
	defaultHTTPAddr        = ":8080"
	defaultDatabaseDriver  = "mysql"
	defaultDatabasePort    = 3306
	defaultMaxOpenConns    = 10
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 30 * time.Minute
	defaultSessionTTL      = 24 * time.Hour
	defaultCookieName      = "novro_session"
)

// Config contains only validated runtime configuration. Secrets remain in
// memory and are never included in its string representation or logs.
type Config struct {
	Environment   string
	HTTPAddr      string
	Database      DatabaseConfig
	Session       SessionConfig
	Auth          AuthConfig
	Provider      ProviderConfig
	AllowedOrigin []string
}

type ProviderConfig struct {
	EncryptionSecret string
}

type AuthConfig struct {
	PublicURL           string
	SetupToken          string
	RegistrationEnabled bool
	OIDC                OIDCConfig
}

type OIDCConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	DisplayName  string
	AutoRegister bool
}

func (c OIDCConfig) Enabled() bool {
	return c.Issuer != "" && c.ClientID != ""
}

type DatabaseConfig struct {
	Driver   string
	Host     string
	Port     int
	Name     string
	User     string
	Password string
	TLS      bool

	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type SessionConfig struct {
	Secret       string
	TTL          time.Duration
	CookieName   string
	CookieSecure bool
}

// Load reads and validates process environment variables.
func Load() (Config, error) {
	return loadEnv(func(key string) (string, bool) {
		return lookupEnv(key)
	})
}

var lookupEnv = func(key string) (string, bool) {
	return os.LookupEnv(key)
}

// loadEnv is separated from os.LookupEnv so validation can be tested without
// mutating process-global environment state.
func loadEnv(get func(string) (string, bool)) (Config, error) {
	getString := func(key, fallback string) string {
		if value, ok := get(key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		return fallback
	}
	required := func(key string) (string, error) {
		value, ok := get(key)
		if !ok || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("%s is required", key)
		}
		return strings.TrimSpace(value), nil
	}
	parseInt := func(key string, fallback int) (int, error) {
		value := getString(key, strconv.Itoa(fallback))
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", key)
		}
		return parsed, nil
	}
	parseDuration := func(key string, fallback time.Duration) (time.Duration, error) {
		value := getString(key, fallback.String())
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return 0, fmt.Errorf("%s must be a positive duration", key)
		}
		return parsed, nil
	}
	parseBool := func(key string, fallback bool) (bool, error) {
		value := getString(key, strconv.FormatBool(fallback))
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return false, fmt.Errorf("%s must be true or false", key)
		}
		return parsed, nil
	}

	environment := strings.ToLower(getString("NOVRO_ENVIRONMENT", "development"))
	if environment != "development" && environment != "test" && environment != "production" {
		return Config{}, errors.New("NOVRO_ENVIRONMENT must be development, test, or production")
	}
	httpAddr := getString("NOVRO_HTTP_ADDR", defaultHTTPAddr)
	if _, _, err := net.SplitHostPort(httpAddr); err != nil {
		return Config{}, fmt.Errorf("NOVRO_HTTP_ADDR must be a host:port address: %w", err)
	}

	driver := strings.ToLower(getString("NOVRO_DATABASE_DRIVER", defaultDatabaseDriver))
	if driver != defaultDatabaseDriver {
		return Config{}, fmt.Errorf("unsupported database driver %q", driver)
	}
	databaseHost, err := required("NOVRO_DATABASE_HOST")
	if err != nil {
		return Config{}, err
	}
	databaseName, err := required("NOVRO_DATABASE_NAME")
	if err != nil {
		return Config{}, err
	}
	databaseUser, err := required("NOVRO_DATABASE_USER")
	if err != nil {
		return Config{}, err
	}
	databasePassword, err := required("NOVRO_DATABASE_PASSWORD")
	if err != nil {
		return Config{}, err
	}
	databasePort, err := parseInt("NOVRO_DATABASE_PORT", defaultDatabasePort)
	if err != nil {
		return Config{}, err
	}
	databaseTLS, err := parseBool("NOVRO_DATABASE_TLS", true)
	if err != nil {
		return Config{}, err
	}
	databaseMaxOpen, err := parseInt("NOVRO_DATABASE_MAX_OPEN_CONNS", defaultMaxOpenConns)
	if err != nil {
		return Config{}, err
	}
	databaseMaxIdle, err := parseInt("NOVRO_DATABASE_MAX_IDLE_CONNS", defaultMaxIdleConns)
	if err != nil {
		return Config{}, err
	}
	if databaseMaxIdle > databaseMaxOpen {
		return Config{}, errors.New("NOVRO_DATABASE_MAX_IDLE_CONNS cannot exceed NOVRO_DATABASE_MAX_OPEN_CONNS")
	}
	databaseLifetime, err := parseDuration("NOVRO_DATABASE_CONN_MAX_LIFETIME", defaultConnMaxLifetime)
	if err != nil {
		return Config{}, err
	}

	sessionSecret, err := required("NOVRO_SESSION_SECRET")
	if err != nil {
		return Config{}, err
	}
	if len([]byte(sessionSecret)) < 32 {
		return Config{}, errors.New("NOVRO_SESSION_SECRET must be at least 32 bytes")
	}
	providerEncryptionSecret := getString("NOVRO_PROVIDER_ENCRYPTION_SECRET", "")
	if providerEncryptionSecret == "" {
		if environment == "production" {
			return Config{}, errors.New("NOVRO_PROVIDER_ENCRYPTION_SECRET is required in production")
		}
		providerEncryptionSecret = sessionSecret
	}
	if len([]byte(providerEncryptionSecret)) < 32 {
		return Config{}, errors.New("NOVRO_PROVIDER_ENCRYPTION_SECRET must be at least 32 bytes")
	}
	sessionTTL, err := parseDuration("NOVRO_SESSION_TTL", defaultSessionTTL)
	if err != nil {
		return Config{}, err
	}
	cookieSecure, err := parseBool("NOVRO_SESSION_COOKIE_SECURE", environment == "production")
	if err != nil {
		return Config{}, err
	}
	if environment == "production" && (!databaseTLS || !cookieSecure) {
		return Config{}, errors.New("production requires database TLS and secure session cookies")
	}

	publicURL := strings.TrimRight(getString("NOVRO_PUBLIC_URL", "http://localhost:3000"), "/")
	parsedPublicURL, err := url.Parse(publicURL)
	if err != nil || parsedPublicURL.Host == "" || (parsedPublicURL.Scheme != "http" && parsedPublicURL.Scheme != "https") || parsedPublicURL.RawQuery != "" || parsedPublicURL.Fragment != "" {
		return Config{}, errors.New("NOVRO_PUBLIC_URL must be an absolute http or https URL without query or fragment")
	}
	if environment == "production" && parsedPublicURL.Scheme != "https" {
		return Config{}, errors.New("production requires an https NOVRO_PUBLIC_URL")
	}
	registrationEnabled, err := parseBool("NOVRO_REGISTRATION_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	oidcAutoRegister, err := parseBool("NOVRO_OIDC_AUTO_REGISTER", true)
	if err != nil {
		return Config{}, err
	}
	oidcIssuer := strings.TrimRight(getString("NOVRO_OIDC_ISSUER", ""), "/")
	oidcClientID := getString("NOVRO_OIDC_CLIENT_ID", "")
	oidcClientSecret := getString("NOVRO_OIDC_CLIENT_SECRET", "")
	if oidcIssuer == "" || oidcClientID == "" || oidcClientSecret == "" {
		if oidcIssuer != "" || oidcClientID != "" || oidcClientSecret != "" {
			return Config{}, errors.New("NOVRO_OIDC_ISSUER, NOVRO_OIDC_CLIENT_ID, and NOVRO_OIDC_CLIENT_SECRET must be configured together")
		}
	}
	if oidcIssuer != "" {
		issuerURL, parseErr := url.Parse(oidcIssuer)
		if parseErr != nil || issuerURL.Scheme != "https" || issuerURL.Host == "" || issuerURL.RawQuery != "" || issuerURL.Fragment != "" {
			return Config{}, errors.New("NOVRO_OIDC_ISSUER must be an absolute https URL without query or fragment")
		}
	}
	setupToken := getString("NOVRO_SETUP_TOKEN", "")
	if setupToken != "" && len([]byte(setupToken)) < 24 {
		return Config{}, errors.New("NOVRO_SETUP_TOKEN must be at least 24 bytes when enabled")
	}

	origins := make([]string, 0)
	for _, origin := range strings.Split(getString("NOVRO_ALLOWED_ORIGINS", "http://localhost:3000"), ",") {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		if origin != "" {
			origins = append(origins, origin)
		}
	}
	if len(origins) == 0 {
		return Config{}, errors.New("NOVRO_ALLOWED_ORIGINS must contain at least one origin")
	}

	return Config{
		Environment: environment,
		HTTPAddr:    httpAddr,
		Database: DatabaseConfig{
			Driver:   driver,
			Host:     databaseHost,
			Port:     databasePort,
			Name:     databaseName,
			User:     databaseUser,
			Password: databasePassword,
			TLS:      databaseTLS,

			MaxOpenConns:    databaseMaxOpen,
			MaxIdleConns:    databaseMaxIdle,
			ConnMaxLifetime: databaseLifetime,
		},
		Session: SessionConfig{
			Secret:       sessionSecret,
			TTL:          sessionTTL,
			CookieName:   getString("NOVRO_SESSION_COOKIE_NAME", defaultCookieName),
			CookieSecure: cookieSecure,
		},
		Auth: AuthConfig{
			PublicURL:           publicURL,
			SetupToken:          setupToken,
			RegistrationEnabled: registrationEnabled,
			OIDC: OIDCConfig{
				Issuer:       oidcIssuer,
				ClientID:     oidcClientID,
				ClientSecret: oidcClientSecret,
				DisplayName:  getString("NOVRO_OIDC_DISPLAY_NAME", "企业账号"),
				AutoRegister: oidcAutoRegister,
			},
		},
		Provider:      ProviderConfig{EncryptionSecret: providerEncryptionSecret},
		AllowedOrigin: origins,
	}, nil
}

func (c DatabaseConfig) DSN() string {
	return c.MySQLConfig().FormatDSN()
}

func (c DatabaseConfig) MySQLConfig() *mysql.Config {
	tls := "false"
	if c.TLS {
		tls = "skip-verify"
	}
	return &mysql.Config{
		User:                 c.User,
		Passwd:               c.Password,
		Net:                  "tcp",
		Addr:                 net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		DBName:               c.Name,
		TLSConfig:            tls,
		AllowNativePasswords: true,
		MultiStatements:      true,
		ParseTime:            true,
		Loc:                  time.UTC,
	}
}
