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
	"github.com/novro-gateway/novro/internal/email"
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
	Payment       PaymentConfig
	Referral      ReferralConfig
	Email         email.Config
	AllowedOrigin []string
}

type ProviderConfig struct {
	EncryptionSecret string
}

type PaymentConfig struct {
	EPay EPayConfig
}

type ReferralConfig struct {
	RewardBPS int64
}

type EPayConfig struct {
	APIURL      string
	MerchantID  string
	MerchantKey string
	SiteName    string
	Channels    []string
}

func (c EPayConfig) Enabled() bool {
	return c.APIURL != "" && c.MerchantID != "" && c.MerchantKey != ""
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
	httpHost, _, err := net.SplitHostPort(httpAddr)
	if err != nil {
		return Config{}, fmt.Errorf("NOVRO_HTTP_ADDR must be a host:port address: %w", err)
	}
	if environment == "production" && !isLoopbackHost(httpHost) {
		return Config{}, errors.New("production requires NOVRO_HTTP_ADDR to use a loopback host")
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
	if err != nil || !isHTTPOrigin(parsedPublicURL) {
		return Config{}, errors.New("NOVRO_PUBLIC_URL must be an absolute http or https origin without a path, credentials, query, or fragment")
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
	emailConfig := email.Config{
		Host:     getString("NOVRO_EMAIL_SMTP_HOST", ""),
		Port:     587,
		Username: getString("NOVRO_EMAIL_SMTP_USERNAME", ""),
		Password: getString("NOVRO_EMAIL_SMTP_PASSWORD", ""),
		From:     getString("NOVRO_EMAIL_FROM", ""),
	}
	if rawPort := getString("NOVRO_EMAIL_SMTP_PORT", ""); rawPort != "" {
		parsedPort, parseErr := strconv.Atoi(rawPort)
		if parseErr != nil {
			return Config{}, errors.New("NOVRO_EMAIL_SMTP_PORT must be a valid port")
		}
		emailConfig.Port = parsedPort
	}
	emailTLS, err := parseBool("NOVRO_EMAIL_SMTP_TLS", true)
	if err != nil {
		return Config{}, err
	}
	emailConfig.TLS = emailTLS
	if err := email.ValidateConfig(emailConfig, environment == "production"); err != nil {
		return Config{}, err
	}

	epayAPIURL := strings.TrimRight(getString("NOVRO_EPAY_API_URL", ""), "/")
	epayMerchantID := getString("NOVRO_EPAY_MERCHANT_ID", "")
	epayMerchantKey := getString("NOVRO_EPAY_MERCHANT_KEY", "")
	configuredEPayValues := 0
	for _, value := range []string{epayAPIURL, epayMerchantID, epayMerchantKey} {
		if value != "" {
			configuredEPayValues++
		}
	}
	if configuredEPayValues != 0 && configuredEPayValues != 3 {
		return Config{}, errors.New("NOVRO_EPAY_API_URL, NOVRO_EPAY_MERCHANT_ID, and NOVRO_EPAY_MERCHANT_KEY must be configured together")
	}
	if epayAPIURL != "" {
		parsedEPayURL, parseErr := url.Parse(epayAPIURL)
		if parseErr != nil || parsedEPayURL.Host == "" || parsedEPayURL.User != nil || (parsedEPayURL.Scheme != "http" && parsedEPayURL.Scheme != "https") || parsedEPayURL.RawQuery != "" || parsedEPayURL.Fragment != "" {
			return Config{}, errors.New("NOVRO_EPAY_API_URL must be an absolute http or https URL without credentials, query, or fragment")
		}
		if environment == "production" && parsedEPayURL.Scheme != "https" {
			return Config{}, errors.New("production requires an https NOVRO_EPAY_API_URL")
		}
	}
	epayChannels := make([]string, 0, 3)
	for _, channel := range strings.Split(getString("NOVRO_EPAY_CHANNELS", "alipay,wxpay"), ",") {
		channel = strings.ToLower(strings.TrimSpace(channel))
		if channel == "" {
			continue
		}
		if !isPaymentChannel(channel) {
			return Config{}, errors.New("NOVRO_EPAY_CHANNELS must contain comma-separated lowercase letters, numbers, underscores, or hyphens")
		}
		if !containsString(epayChannels, channel) {
			epayChannels = append(epayChannels, channel)
		}
	}
	if len(epayChannels) == 0 {
		return Config{}, errors.New("NOVRO_EPAY_CHANNELS must contain at least one channel")
	}
	epaySiteName := getString("NOVRO_EPAY_SITE_NAME", "Novro")
	if len([]rune(epaySiteName)) > 64 {
		return Config{}, errors.New("NOVRO_EPAY_SITE_NAME must not exceed 64 characters")
	}
	referralRewardBPS, err := strconv.ParseInt(getString("NOVRO_REFERRAL_REWARD_BPS", "1000"), 10, 64)
	if err != nil || referralRewardBPS < 0 || referralRewardBPS > 10_000 {
		return Config{}, errors.New("NOVRO_REFERRAL_REWARD_BPS must be an integer between 0 and 10000")
	}

	origins := make([]string, 0)
	for _, origin := range strings.Split(getString("NOVRO_ALLOWED_ORIGINS", "http://localhost:3000"), ",") {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		if origin == "" {
			continue
		}
		parsedOrigin, parseErr := url.Parse(origin)
		if parseErr != nil || !isHTTPOrigin(parsedOrigin) {
			return Config{}, errors.New("NOVRO_ALLOWED_ORIGINS must contain only absolute http or https origins without paths, credentials, queries, or fragments")
		}
		if environment == "production" && parsedOrigin.Scheme != "https" {
			return Config{}, errors.New("production requires HTTPS NOVRO_ALLOWED_ORIGINS")
		}
		origins = append(origins, origin)
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
		Provider: ProviderConfig{EncryptionSecret: providerEncryptionSecret},
		Payment: PaymentConfig{EPay: EPayConfig{
			APIURL: epayAPIURL, MerchantID: epayMerchantID, MerchantKey: epayMerchantKey,
			SiteName: epaySiteName, Channels: epayChannels,
		}},
		Referral:      ReferralConfig{RewardBPS: referralRewardBPS},
		Email:         emailConfig,
		AllowedOrigin: origins,
	}, nil
}

func isPaymentChannel(value string) bool {
	if value == "" || len(value) > 32 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isHTTPOrigin(parsed *url.URL) bool {
	return parsed != nil && parsed.Host != "" && parsed.User == nil &&
		(parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.Path == "" && parsed.RawPath == "" && parsed.RawQuery == "" && parsed.Fragment == ""
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
