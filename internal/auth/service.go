package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/novro-gateway/novro/internal/user"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUnauthenticated    = errors.New("authentication required")
	ErrOIDCNotProvisioned = errors.New("OIDC user is not provisioned")
)

type PasswordManager interface {
	Hash(string) (string, error)
	Verify(string, string) bool
}

type LoginUser struct {
	User         user.Record
	PasswordHash *string
}

type Store interface {
	FindUserByUsername(context.Context, string) (LoginUser, error)
	FindOrCreateOIDCUser(context.Context, OIDCUser, bool) (user.Record, error)
	CreateSession(context.Context, uuid.UUID, string, time.Time, time.Time) error
	FindUserBySession(context.Context, string, time.Time) (user.Record, error)
	RevokeSession(context.Context, string, time.Time) error
}

type OIDCUser struct {
	Issuer            string
	Subject           string
	Email             string
	PreferredUsername string
	DisplayName       string
}

type Service struct {
	store         Store
	passwords     PasswordManager
	secret        string
	ttl           time.Duration
	dummyHash     string
	now           func() time.Time
	generateToken func() (string, error)
}

type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      user.Record
}

func NewService(store Store, passwords PasswordManager, secret string, ttl time.Duration) (*Service, error) {
	dummyHash, err := passwords.Hash("novro-dummy-password-value")
	if err != nil {
		return nil, fmt.Errorf("create authentication comparison hash: %w", err)
	}
	return &Service{
		store:         store,
		passwords:     passwords,
		secret:        secret,
		ttl:           ttl,
		dummyHash:     dummyHash,
		now:           func() time.Time { return time.Now().UTC() },
		generateToken: newSessionToken,
	}, nil
}

func (s *Service) Login(ctx context.Context, username, plainTextPassword string) (LoginResult, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	loginUser, err := s.store.FindUserByUsername(ctx, username)
	if err != nil {
		_ = s.passwords.Verify(s.dummyHash, plainTextPassword)
		if errors.Is(err, user.ErrNotFound) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, fmt.Errorf("find login user: %w", err)
	}
	passwordValid := false
	if loginUser.PasswordHash != nil {
		passwordValid = s.passwords.Verify(*loginUser.PasswordHash, plainTextPassword)
	} else {
		_ = s.passwords.Verify(s.dummyHash, plainTextPassword)
	}
	if loginUser.User.Status != user.StatusActive || !passwordValid {
		return LoginResult{}, ErrInvalidCredentials
	}

	return s.createLoginSession(ctx, loginUser.User)
}

func (s *Service) LoginOIDC(ctx context.Context, identity OIDCUser, autoRegister bool) (LoginResult, error) {
	record, err := s.store.FindOrCreateOIDCUser(ctx, identity, autoRegister)
	if err != nil {
		return LoginResult{}, err
	}
	if record.Status != user.StatusActive {
		return LoginResult{}, ErrUnauthenticated
	}
	return s.createLoginSession(ctx, record)
}

func (s *Service) createLoginSession(ctx context.Context, record user.Record) (LoginResult, error) {
	token, err := s.generateToken()
	if err != nil {
		return LoginResult{}, err
	}
	now := s.now()
	expiresAt := now.Add(s.ttl)
	if err := s.store.CreateSession(ctx, record.ID, hashSessionToken(s.secret, token), expiresAt, now); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, fmt.Errorf("create login session: %w", err)
	}
	record.LastLoginAt = &now
	return LoginResult{Token: token, ExpiresAt: expiresAt, User: record}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (user.Record, error) {
	if !strings.HasPrefix(token, "nvs_") || len(token) < 40 || len(token) > 128 {
		return user.Record{}, ErrUnauthenticated
	}
	record, err := s.store.FindUserBySession(ctx, hashSessionToken(s.secret, token), s.now())
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return user.Record{}, ErrUnauthenticated
		}
		return user.Record{}, fmt.Errorf("authenticate session: %w", err)
	}
	return record, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := s.store.RevokeSession(ctx, hashSessionToken(s.secret, token), s.now()); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}
