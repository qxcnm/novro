package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/novro-gateway/novro/internal/user"
)

type fakeAuthStore struct {
	loginUser    LoginUser
	findErr      error
	sessionHash  string
	sessionUser  user.Record
	revokedHash  string
	oidcUser     user.Record
	oidcIdentity OIDCUser
	autoRegister bool
}

func (f *fakeAuthStore) FindUserByUsername(context.Context, string) (LoginUser, error) {
	return f.loginUser, f.findErr
}

func (f *fakeAuthStore) FindOrCreateOIDCUser(_ context.Context, identity OIDCUser, autoRegister bool) (user.Record, error) {
	f.oidcIdentity = identity
	f.autoRegister = autoRegister
	return f.oidcUser, f.findErr
}

func (f *fakeAuthStore) CreateSession(_ context.Context, _ uuid.UUID, hash string, _, _ time.Time) error {
	f.sessionHash = hash
	return nil
}

func (f *fakeAuthStore) FindUserBySession(_ context.Context, hash string, _ time.Time) (user.Record, error) {
	f.sessionHash = hash
	if f.findErr != nil {
		return user.Record{}, f.findErr
	}
	return f.sessionUser, nil
}

func (f *fakeAuthStore) RevokeSession(_ context.Context, hash string, _ time.Time) error {
	f.revokedHash = hash
	return nil
}

type fakePasswords struct{}

func (fakePasswords) Hash(value string) (string, error) { return "hash:" + value, nil }
func (fakePasswords) Verify(hash, value string) bool    { return hash == "hash:"+value }

func newTestService(t *testing.T, store *fakeAuthStore) *Service {
	t.Helper()
	service, err := NewService(store, fakePasswords{}, "01234567890123456789012345678901", time.Hour)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	service.generateToken = func() (string, error) { return "nvs_abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG", nil }
	service.now = func() time.Time { return time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC) }
	return service
}

func TestLoginCreatesHashedSession(t *testing.T) {
	hash := "hash:correct-password"
	store := &fakeAuthStore{loginUser: LoginUser{
		User:         user.Record{ID: uuid.New(), Username: "admin", Status: user.StatusActive},
		PasswordHash: &hash,
	}}
	service := newTestService(t, store)
	result, err := service.Login(context.Background(), " ADMIN ", "correct-password")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if store.sessionHash == "" || store.sessionHash == result.Token || result.ExpiresAt.Sub(service.now()) != time.Hour {
		t.Fatalf("unexpected login result: %+v hash=%q", result, store.sessionHash)
	}
}

func TestLoginHidesMissingDisabledAndWrongPassword(t *testing.T) {
	hash := "hash:correct"
	for _, store := range []*fakeAuthStore{
		{findErr: user.ErrNotFound},
		{loginUser: LoginUser{User: user.Record{Status: user.StatusDisabled}, PasswordHash: &hash}},
		{loginUser: LoginUser{User: user.Record{Status: user.StatusActive}, PasswordHash: &hash}},
		{loginUser: LoginUser{User: user.Record{Status: user.StatusActive}, PasswordHash: nil}},
	} {
		service := newTestService(t, store)
		if _, err := service.Login(context.Background(), "admin", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("expected generic login error, got %v", err)
		}
	}
}

func TestOIDCLoginCreatesLocalSession(t *testing.T) {
	store := &fakeAuthStore{oidcUser: user.Record{ID: uuid.New(), Username: "oidc-user", Status: user.StatusActive}}
	service := newTestService(t, store)
	identity := OIDCUser{Issuer: "https://id.example.com", Subject: "subject-1", Email: "user@example.com"}
	result, err := service.LoginOIDC(context.Background(), identity, true)
	if err != nil {
		t.Fatalf("OIDC login: %v", err)
	}
	if result.User.Username != "oidc-user" || store.oidcIdentity.Subject != identity.Subject || !store.autoRegister || store.sessionHash == "" {
		t.Fatalf("unexpected OIDC login: result=%+v store=%+v", result, store)
	}
}

func TestAuthenticateAndLogoutHashTokens(t *testing.T) {
	store := &fakeAuthStore{sessionUser: user.Record{ID: uuid.New(), Status: user.StatusActive}}
	service := newTestService(t, store)
	token := "nvs_abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	if _, err := service.Authenticate(context.Background(), token); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	authHash := store.sessionHash
	if err := service.Logout(context.Background(), token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if store.revokedHash != authHash || authHash == token {
		t.Fatal("session token was not consistently hashed")
	}
	if _, err := service.Authenticate(context.Background(), "invalid"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected unauthenticated, got %v", err)
	}
}
