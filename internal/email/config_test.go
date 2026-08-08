package email

import (
	"context"
	"errors"
	"testing"
)

type fakeConfigStore struct {
	record StoredConfig
	exists bool
	input  StoredConfigInput
}

func (f *fakeConfigStore) Get(context.Context) (StoredConfig, error) {
	if !f.exists {
		return StoredConfig{}, ErrConfigNotFound
	}
	return f.record, nil
}

func (f *fakeConfigStore) Upsert(_ context.Context, input StoredConfigInput) (StoredConfig, error) {
	f.input = input
	f.exists = true
	f.record = StoredConfig{
		ID: input.ID, Enabled: input.Enabled, Host: input.Host, Port: input.Port,
		Username: input.Username, EncryptedPassword: input.EncryptedPassword,
		FromAddress: input.FromAddress, Security: input.Security,
	}
	return f.record, nil
}

type fakeCipher struct{}

func (fakeCipher) Encrypt(value string) (string, error) { return "encrypted:" + value, nil }
func (fakeCipher) Decrypt(value string) (string, error) {
	if len(value) < len("encrypted:") || value[:len("encrypted:")] != "encrypted:" {
		return "", errors.New("invalid ciphertext")
	}
	return value[len("encrypted:"):], nil
}

type countingMailer struct{ calls int }

func (m *countingMailer) SendVerificationCode(context.Context, string, string) error {
	m.calls++
	return nil
}

func TestServiceEncryptsRuntimePasswordAndPreservesSavedPassword(t *testing.T) {
	store := &fakeConfigStore{}
	service := NewService(store, fakeCipher{}, Config{
		Host: "smtp.env.example", Port: 587, Username: "env@example.com", Password: "environment-secret",
		From: "env@example.com", TLS: true,
	}, nil, false)

	updated, err := service.UpdateConfig(context.Background(), ConfigInput{
		Enabled: true, Host: "smtp.db.example", Port: 465, Username: "verify@example.com",
		FromAddress: "verify@example.com", Security: SecuritySSL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.input.EncryptedPassword != "encrypted:environment-secret" || !updated.HasPassword || updated.Security != SecuritySSL {
		t.Fatalf("unexpected saved configuration: input=%+v admin=%+v", store.input, updated)
	}

	empty := ""
	_, err = service.UpdateConfig(context.Background(), ConfigInput{
		Enabled: true, Host: "smtp.db.example", Port: 587, Username: "verify@example.com",
		Password: &empty, FromAddress: "verify@example.com", Security: SecuritySTARTTLS,
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.input.EncryptedPassword != "encrypted:environment-secret" {
		t.Fatalf("empty password replaced saved credential: %q", store.input.EncryptedPassword)
	}
}

func TestServiceHonorsExplicitDisableBeforeDevelopmentFallback(t *testing.T) {
	fallback := &countingMailer{}
	store := &fakeConfigStore{exists: true, record: StoredConfig{
		ID: configID, Enabled: false, Host: "smtp.example.com", Port: 587, Username: "verify@example.com",
		EncryptedPassword: "encrypted:secret", FromAddress: "verify@example.com", Security: SecuritySTARTTLS,
	}}
	service := NewService(store, fakeCipher{}, Config{}, fallback, false)
	if err := service.SendVerificationCode(context.Background(), "user@example.com", "123456"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected disabled error, got %v", err)
	}
	if fallback.calls != 0 {
		t.Fatalf("disabled database configuration used development fallback %d times", fallback.calls)
	}

	store.exists = false
	if err := service.SendVerificationCode(context.Background(), "user@example.com", "123456"); err != nil {
		t.Fatalf("development fallback failed: %v", err)
	}
	if fallback.calls != 1 {
		t.Fatalf("development fallback calls=%d", fallback.calls)
	}
}

func TestServiceReadsLatestStoredConfigForEveryMessage(t *testing.T) {
	store := &fakeConfigStore{exists: true, record: StoredConfig{
		ID: configID, Enabled: true, Host: "smtp.one.example", Port: 587, Username: "verify@example.com",
		EncryptedPassword: "encrypted:secret", FromAddress: "verify@example.com", Security: SecuritySTARTTLS,
	}}
	service := NewService(store, fakeCipher{}, Config{}, nil, false)
	var hosts []string
	service.deliver = func(_ context.Context, record StoredConfig, _, _, _ string) error {
		hosts = append(hosts, record.Host)
		return nil
	}
	if err := service.SendVerificationCode(context.Background(), "user@example.com", "123456"); err != nil {
		t.Fatal(err)
	}
	store.record.Host = "smtp.two.example"
	if err := service.SendVerificationCode(context.Background(), "user@example.com", "654321"); err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 || hosts[0] != "smtp.one.example" || hosts[1] != "smtp.two.example" {
		t.Fatalf("messages did not use latest configuration: %#v", hosts)
	}
}

func TestServiceRejectsInvalidConfigurationAndTestRecipient(t *testing.T) {
	store := &fakeConfigStore{}
	service := NewService(store, fakeCipher{}, Config{}, nil, false)
	password := "secret"
	for _, input := range []ConfigInput{
		{Enabled: true, Host: "smtp.example.com", Port: 70000, Username: "verify@example.com", Password: &password, FromAddress: "verify@example.com", Security: SecuritySTARTTLS},
		{Enabled: true, Host: "smtp.example.com", Port: 587, Username: "verify@example.com", Password: &password, FromAddress: "not-an-email", Security: SecuritySTARTTLS},
		{Enabled: true, Host: "smtp.example.com", Port: 587, Username: "verify@example.com", Password: &password, FromAddress: "verify@example.com", Security: "insecure-tls"},
	} {
		if _, err := service.UpdateConfig(context.Background(), input); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("expected invalid configuration for %+v, got %v", input, err)
		}
	}
	if err := service.Test(context.Background(), "not-an-email"); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected invalid recipient, got %v", err)
	}
}

func TestServiceRejectsUnencryptedSMTPWhenProduction(t *testing.T) {
	store := &fakeConfigStore{}
	service := NewService(store, fakeCipher{}, Config{}, nil, true)
	password := "secret"
	_, err := service.UpdateConfig(context.Background(), ConfigInput{
		Enabled: true, Host: "smtp.example.com", Port: 25, Username: "verify@example.com",
		Password: &password, FromAddress: "verify@example.com", Security: SecurityNone,
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected production transport rejection, got %v", err)
	}
}

func TestValidateConfigSupportsSMTPTransportModes(t *testing.T) {
	base := Config{Host: "smtp.example.com", Port: 587, Username: "verify@example.com", Password: "secret", From: "verify@example.com"}
	for _, security := range []string{SecurityNone, SecuritySTARTTLS, SecuritySSL} {
		config := base
		config.Security = security
		if err := ValidateConfig(config, false); err != nil {
			t.Fatalf("security %q rejected: %v", security, err)
		}
	}
	base.Security = "invalid"
	if err := ValidateConfig(base, false); err == nil {
		t.Fatal("invalid SMTP transport mode was accepted")
	}
	base.Security = SecurityNone
	if err := ValidateConfig(base, true); err == nil {
		t.Fatal("unencrypted SMTP was accepted for environment fallback in production")
	}
}
