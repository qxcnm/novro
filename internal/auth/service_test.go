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
	identifier   string
}

/**
 * FindUserByUsername 执行该名称对应的业务处理逻辑。
 * @param identifier 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeAuthStore) FindUserByUsername(_ context.Context, identifier string) (LoginUser, error) {
	f.identifier = identifier
	return f.loginUser, f.findErr
}

/**
 * FindOrCreateOIDCUser 执行该名称对应的业务处理逻辑。
 * @param identity 本次操作需要使用的输入参数。
 * @param autoRegister 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeAuthStore) FindOrCreateOIDCUser(_ context.Context, identity OIDCUser, autoRegister bool) (user.Record, error) {
	f.oidcIdentity = identity
	f.autoRegister = autoRegister
	return f.oidcUser, f.findErr
}

/**
 * CreateSession 执行该名称对应的业务处理逻辑。
 * @param hash 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeAuthStore) CreateSession(_ context.Context, _ uuid.UUID, hash string, _, _ time.Time) error {
	f.sessionHash = hash
	return nil
}

/**
 * FindUserBySession 执行该名称对应的业务处理逻辑。
 * @param hash 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeAuthStore) FindUserBySession(_ context.Context, hash string, _ time.Time) (user.Record, error) {
	f.sessionHash = hash
	if f.findErr != nil {
		return user.Record{}, f.findErr
	}
	return f.sessionUser, nil
}

/**
 * RevokeSession 执行该名称对应的业务处理逻辑。
 * @param hash 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeAuthStore) RevokeSession(_ context.Context, hash string, _ time.Time) error {
	f.revokedHash = hash
	return nil
}

type fakePasswords struct{}

/**
 * Hash 执行该名称对应的业务处理逻辑。
 * @param value 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (fakePasswords) Hash(value string) (string, error) { return "hash:" + value, nil }
/**
 * Verify 执行该名称对应的业务处理逻辑。
 * @param hash 本次操作需要使用的输入参数。
 * @param value 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (fakePasswords) Verify(hash, value string) bool    { return hash == "hash:"+value }

/**
 * newTestService 执行该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @param store 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestLoginCreatesHashedSession 执行该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestLoginCreatesHashedSession(t *testing.T) {
	hash := "hash:correct-password"
	store := &fakeAuthStore{loginUser: LoginUser{
		User:         user.Record{ID: uuid.New(), Username: "admin", Status: user.StatusActive, IsSystemAdmin: true},
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
	if !result.User.IsSystemAdmin {
		t.Fatal("system administrator marker was lost during login")
	}
}

/**
 * TestLoginNormalizesUsernameOrEmailIdentifier 执行该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestLoginNormalizesUsernameOrEmailIdentifier(t *testing.T) {
	hash := "hash:correct-password"
	store := &fakeAuthStore{loginUser: LoginUser{
		User:         user.Record{ID: uuid.New(), Username: "alice", Email: "alice@example.com", Status: user.StatusActive},
		PasswordHash: &hash,
	}}
	service := newTestService(t, store)
	if _, err := service.Login(context.Background(), " Alice@Example.COM ", "correct-password"); err != nil {
		t.Fatalf("login by email: %v", err)
	}
	if store.identifier != "alice@example.com" {
		t.Fatalf("identifier was not normalized: %q", store.identifier)
	}
}

/**
 * TestLoginHidesMissingDisabledAndWrongPassword 执行该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestOIDCLoginCreatesLocalSession 执行该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestAuthenticateAndLogoutHashTokens 执行该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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
