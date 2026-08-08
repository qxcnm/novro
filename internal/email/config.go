package email

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

const (
	SecurityNone     = "none"
	SecuritySTARTTLS = "starttls"
	SecuritySSL      = "ssl"
	configID         = "smtp"
)

var (
	ErrConfigNotFound = errors.New("email SMTP configuration not found")
	ErrInvalidConfig  = errors.New("invalid email SMTP configuration")
	ErrNotConfigured  = errors.New("email SMTP is not configured")
)

type SecretCipher interface {
	Encrypt(string) (string, error)
	Decrypt(string) (string, error)
}

type ConfigStore interface {
	Get(context.Context) (StoredConfig, error)
	Upsert(context.Context, StoredConfigInput) (StoredConfig, error)
}

type StoredConfig struct {
	ID                string
	Enabled           bool
	Host              string
	Port              int
	Username          string
	Password          string
	EncryptedPassword string
	FromAddress       string
	Security          string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	useFallback       bool
}

type StoredConfigInput struct {
	ID                string
	Enabled           bool
	Host              string
	Port              int
	Username          string
	EncryptedPassword string
	FromAddress       string
	Security          string
}

type ConfigInput struct {
	Enabled     bool
	Host        string
	Port        int
	Username    string
	Password    *string
	FromAddress string
	Security    string
}

type AdminConfig struct {
	Enabled     bool       `json:"enabled"`
	Configured  bool       `json:"configured"`
	Host        string     `json:"host"`
	Port        int        `json:"port"`
	Username    string     `json:"username"`
	FromAddress string     `json:"from_address"`
	Security    string     `json:"security"`
	HasPassword bool       `json:"has_password"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

type Service struct {
	store         ConfigStore
	cipher        SecretCipher
	defaultConfig Config
	fallback      Mailer
	production    bool
	deliver       func(context.Context, StoredConfig, string, string, string) error
}

func NewService(store ConfigStore, cipher SecretCipher, defaultConfig Config, fallback Mailer, production bool) *Service {
	return &Service{store: store, cipher: cipher, defaultConfig: defaultConfig, fallback: fallback, production: production}
}

func (s *Service) AdminConfig(ctx context.Context) (AdminConfig, error) {
	record, err := s.current(ctx)
	if err != nil {
		return AdminConfig{}, err
	}
	return adminConfig(record), nil
}

func (s *Service) UpdateConfig(ctx context.Context, input ConfigInput) (AdminConfig, error) {
	if s == nil || s.store == nil || s.cipher == nil {
		return AdminConfig{}, ErrInvalidConfig
	}
	current, err := s.current(ctx)
	if err != nil {
		return AdminConfig{}, err
	}
	normalized, err := normalizeInput(input, current, s.production)
	if err != nil {
		return AdminConfig{}, err
	}
	encrypted := current.EncryptedPassword
	if normalized.Password != "" {
		encrypted, err = s.cipher.Encrypt(normalized.Password)
		if err != nil {
			return AdminConfig{}, fmt.Errorf("encrypt SMTP password: %w", err)
		}
	} else if encrypted == "" && current.Password != "" {
		encrypted, err = s.cipher.Encrypt(current.Password)
		if err != nil {
			return AdminConfig{}, fmt.Errorf("encrypt SMTP bootstrap password: %w", err)
		}
	}
	updated, err := s.store.Upsert(ctx, StoredConfigInput{
		ID: configID, Enabled: normalized.Enabled, Host: normalized.Host, Port: normalized.Port,
		Username: normalized.Username, EncryptedPassword: encrypted, FromAddress: normalized.FromAddress,
		Security: normalized.Security,
	})
	if err != nil {
		return AdminConfig{}, err
	}
	return adminConfig(updated), nil
}

func (s *Service) Test(ctx context.Context, recipient string) error {
	recipient = strings.TrimSpace(recipient)
	parsed, err := mail.ParseAddress(recipient)
	if err != nil || parsed.Address != recipient {
		return ErrInvalidConfig
	}
	record, err := s.current(ctx)
	if err != nil {
		return err
	}
	if !record.Enabled || !configured(record) {
		return ErrNotConfigured
	}
	return s.sendMessage(ctx, record, recipient, "Novro SMTP 测试邮件", "这是一封来自 Novro 的 SMTP 测试邮件。\r\n如果你能看到这封邮件，注册验证码邮件配置已生效。\r\n")
}

func (s *Service) SendVerificationCode(ctx context.Context, recipient, code string) error {
	record, err := s.current(ctx)
	if err != nil {
		return err
	}
	if !record.Enabled || !configured(record) {
		if record.useFallback && s.fallback != nil {
			return s.fallback.SendVerificationCode(ctx, recipient, code)
		}
		return ErrNotConfigured
	}
	return s.sendMessage(ctx, record, recipient, "Novro 注册验证码", fmt.Sprintf("您的 Novro 注册验证码是：%s\r\n验证码 10 分钟内有效，且只能使用一次。若不是您本人操作，请忽略此邮件。\r\n", code))
}

func (s *Service) sendMessage(ctx context.Context, record StoredConfig, recipient, subject, body string) error {
	if s.deliver != nil {
		return s.deliver(ctx, record, recipient, subject, body)
	}
	password := record.Password
	if password == "" && record.EncryptedPassword != "" {
		if s.cipher == nil {
			return ErrNotConfigured
		}
		decrypted, err := s.cipher.Decrypt(record.EncryptedPassword)
		if err != nil {
			return fmt.Errorf("decrypt SMTP password: %w", err)
		}
		password = decrypted
	}
	mailer := NewSMTPMailer(Config{Host: record.Host, Port: record.Port, Username: record.Username, Password: password, From: record.FromAddress, Security: record.Security})
	return mailer.SendMessage(ctx, recipient, subject, body)
}

func (s *Service) current(ctx context.Context) (StoredConfig, error) {
	if s == nil {
		return StoredConfig{}, ErrNotConfigured
	}
	if s.store != nil {
		record, err := s.store.Get(ctx)
		if err == nil {
			return normalizeStored(record), nil
		}
		if !errors.Is(err, ErrConfigNotFound) {
			return StoredConfig{}, err
		}
	}
	return fromRuntime(s.defaultConfig), nil
}

type normalizedInput struct {
	Enabled     bool
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	Security    string
}

func normalizeInput(input ConfigInput, current StoredConfig, production bool) (normalizedInput, error) {
	result := normalizedInput{
		Enabled: input.Enabled, Host: strings.TrimSpace(input.Host), Port: input.Port,
		Username: strings.TrimSpace(input.Username), FromAddress: strings.TrimSpace(input.FromAddress),
		Security: strings.ToLower(strings.TrimSpace(input.Security)),
	}
	if result.Port == 0 {
		result.Port = current.Port
	}
	if result.Port == 0 {
		result.Port = 587
	}
	if result.Security == "" {
		result.Security = current.Security
	}
	if result.Security == "" {
		result.Security = SecuritySTARTTLS
	}
	if input.Password != nil && strings.TrimSpace(*input.Password) != "" {
		result.Password = strings.TrimSpace(*input.Password)
	}
	if result.Port < 1 || result.Port > 65535 || len(result.Host) > 255 || len(result.Username) > 320 || len(result.FromAddress) > 320 {
		return normalizedInput{}, ErrInvalidConfig
	}
	if result.Security != SecurityNone && result.Security != SecuritySTARTTLS && result.Security != SecuritySSL {
		return normalizedInput{}, ErrInvalidConfig
	}
	if production && result.Enabled && result.Security == SecurityNone {
		return normalizedInput{}, ErrInvalidConfig
	}
	if result.FromAddress != "" {
		parsed, err := mail.ParseAddress(result.FromAddress)
		if err != nil || parsed.Address != result.FromAddress {
			return normalizedInput{}, ErrInvalidConfig
		}
	}
	if result.Enabled && (result.Host == "" || result.FromAddress == "" || result.Username == "" || (result.Password == "" && current.EncryptedPassword == "" && current.Password == "")) {
		return normalizedInput{}, ErrInvalidConfig
	}
	return result, nil
}

func normalizeStored(record StoredConfig) StoredConfig {
	if record.ID == "" {
		record.ID = configID
	}
	if record.Port == 0 {
		record.Port = 587
	}
	if record.Security == "" {
		record.Security = SecuritySTARTTLS
	}
	return record
}

func fromRuntime(config Config) StoredConfig {
	security := strings.ToLower(strings.TrimSpace(config.Security))
	if security == "" {
		security = SecurityNone
		if config.TLS {
			security = SecuritySTARTTLS
		}
	}
	port := config.Port
	if port == 0 {
		port = 587
	}
	host := strings.TrimSpace(config.Host)
	return StoredConfig{ID: configID, Enabled: host != "", Host: host, Port: port, Username: strings.TrimSpace(config.Username), Password: config.Password, FromAddress: strings.TrimSpace(config.From), Security: security, useFallback: host == ""}
}

func configured(record StoredConfig) bool {
	return strings.TrimSpace(record.Host) != "" && record.Port > 0 && strings.TrimSpace(record.FromAddress) != "" && strings.TrimSpace(record.Username) != "" && (record.Password != "" || record.EncryptedPassword != "")
}

func adminConfig(record StoredConfig) AdminConfig {
	var updatedAt *time.Time
	if !record.UpdatedAt.IsZero() {
		value := record.UpdatedAt
		updatedAt = &value
	}
	return AdminConfig{Enabled: record.Enabled, Configured: configured(record), Host: record.Host, Port: record.Port, Username: record.Username, FromAddress: record.FromAddress, Security: record.Security, HasPassword: record.Password != "" || record.EncryptedPassword != "", UpdatedAt: updatedAt}
}
