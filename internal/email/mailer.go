package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLS      bool
	Security string
}

type Mailer interface {
	SendVerificationCode(context.Context, string, string) error
}

type SMTPMailer struct{ config Config }

func NewSMTPMailer(config Config) *SMTPMailer { return &SMTPMailer{config: config} }

func (m *SMTPMailer) SendVerificationCode(ctx context.Context, recipient, code string) error {
	body := fmt.Sprintf("您的 Novro 注册验证码是：%s\r\n验证码 10 分钟内有效，且只能使用一次。若不是您本人操作，请忽略此邮件。\r\n", code)
	return m.SendMessage(ctx, recipient, "Novro 注册验证码", body)
}

func (m *SMTPMailer) SendMessage(ctx context.Context, recipient, subject, body string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	address := net.JoinHostPort(m.config.Host, fmt.Sprintf("%d", m.config.Port))
	dialer := net.Dialer{Timeout: 15 * time.Second}
	security := strings.ToLower(strings.TrimSpace(m.config.Security))
	if security == "" {
		if m.config.TLS {
			security = SecuritySTARTTLS
		} else {
			security = SecurityNone
		}
	}
	var connection net.Conn
	var err error
	if security == SecuritySSL {
		tlsDialer := tls.Dialer{NetDialer: &dialer, Config: NewTLSClient(m.config)}
		connection, err = tlsDialer.DialContext(ctx, "tcp", address)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("connect SMTP server: %w", err)
	}
	defer func() { _ = connection.Close() }()
	deadline := time.Now().Add(30 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set SMTP deadline: %w", err)
	}
	client, err := smtp.NewClient(connection, m.config.Host)
	if err != nil {
		return fmt.Errorf("start SMTP client: %w", err)
	}
	defer func() { _ = client.Close() }()
	if security == SecuritySTARTTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(NewTLSClient(m.config)); err != nil {
			return fmt.Errorf("start SMTP TLS: %w", err)
		}
	}
	if m.config.Username != "" {
		var auth smtp.Auth = smtp.PlainAuth("", m.config.Username, m.config.Password, m.config.Host)
		if security == SecurityNone {
			auth = explicitPlainAuth{username: m.config.Username, password: m.config.Password}
		}
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authenticate SMTP client: %w", err)
		}
	}
	if err := client.Mail(m.config.From); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("start SMTP message: %w", err)
	}
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", m.config.From, recipient, mime.QEncoding.Encode("UTF-8", subject), body)
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish SMTP message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("finish SMTP session: %w", err)
	}
	return nil
}

// explicitPlainAuth is used only when an administrator deliberately selects
// an unencrypted development transport. Production configuration rejects this
// mode before it can be enabled.
type explicitPlainAuth struct {
	username string
	password string
}

func (a explicitPlainAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "PLAIN", []byte("\x00" + a.username + "\x00" + a.password), nil
}

func (explicitPlainAuth) Next([]byte, bool) ([]byte, error) {
	return nil, nil
}

type LogMailer struct{ logger *slog.Logger }

func NewLogMailer(logger *slog.Logger) *LogMailer {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogMailer{logger: logger}
}

func (m *LogMailer) SendVerificationCode(ctx context.Context, recipient, code string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.logger.Info("email verification code generated", "recipient", recipient, "code", code)
	return nil
}

func ValidateConfig(config Config, production bool) error {
	values := []string{config.Host, config.Username, config.Password, config.From}
	configured := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			configured++
		}
	}
	if configured == 0 {
		return nil
	}
	if configured != len(values) {
		return errors.New("NOVRO_EMAIL_SMTP_HOST, NOVRO_EMAIL_SMTP_USERNAME, NOVRO_EMAIL_SMTP_PASSWORD, and NOVRO_EMAIL_FROM must be configured together")
	}
	if config.Port <= 0 || config.Port > 65535 {
		return errors.New("NOVRO_EMAIL_SMTP_PORT must be a valid port")
	}
	parsed, err := mail.ParseAddress(config.From)
	if err != nil || parsed.Address != config.From {
		return errors.New("NOVRO_EMAIL_FROM must be a valid email address")
	}
	security := strings.ToLower(strings.TrimSpace(config.Security))
	if security == "" {
		if config.TLS {
			security = SecuritySTARTTLS
		} else {
			security = SecurityNone
		}
	}
	if security != SecurityNone && security != SecuritySTARTTLS && security != SecuritySSL {
		return errors.New("email SMTP security mode is invalid")
	}
	if production && security == SecurityNone {
		return errors.New("production requires NOVRO_EMAIL_SMTP_TLS=true")
	}
	return nil
}

// NewTLSClient is kept small so callers that need implicit TLS can wrap SMTP
// themselves without exposing credentials through this package.
func NewTLSClient(config Config) *tls.Config {
	return &tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12}
}
