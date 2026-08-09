package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const maxActiveKeysPerUser = 10

type Store interface {
	Create(context.Context, uuid.UUID, uuid.UUID, string, string, string, int) (Record, error)
	ListByUser(context.Context, uuid.UUID) ([]Record, error)
	RevokeByUser(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	ListAll(context.Context, ListFilter) (Page, error)
	Revoke(context.Context, uuid.UUID, time.Time) error
	AuthenticateHash(context.Context, string, time.Time) (Actor, error)
}

func (s *Service) Authenticate(ctx context.Context, token string) (Actor, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "nvr_") || len(token) < 40 || len(token) > 128 {
		return Actor{}, ErrUnauthenticated
	}
	digest := sha256.Sum256([]byte(token))
	return s.store.AuthenticateHash(ctx, hex.EncodeToString(digest[:]), s.now())
}

type Service struct {
	store         Store
	now           func() time.Time
	generateToken func() (string, error)
}

type CreateResult struct {
	APIKey Record `json:"api_key"`
	Key    string `json:"key"`
}

func NewService(store Store) *Service {
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }, generateToken: newToken}
}

func (s *Service) Create(ctx context.Context, userID, billingGroupID uuid.UUID, name string) (CreateResult, error) {
	name = strings.TrimSpace(name)
	if userID == uuid.Nil || billingGroupID == uuid.Nil || name == "" || utf8.RuneCountInString(name) > 64 {
		return CreateResult{}, ErrInvalidInput
	}
	token, err := s.generateToken()
	if err != nil {
		return CreateResult{}, fmt.Errorf("generate API key: %w", err)
	}
	digest := sha256.Sum256([]byte(token))
	prefix := token[:12]
	record, err := s.store.Create(ctx, userID, billingGroupID, name, prefix, hex.EncodeToString(digest[:]), maxActiveKeysPerUser)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{APIKey: record, Key: token}, nil
}

func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID) ([]Record, error) {
	if userID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.store.ListByUser(ctx, userID)
}

func (s *Service) RevokeForUser(ctx context.Context, userID, id uuid.UUID) error {
	if userID == uuid.Nil || id == uuid.Nil {
		return ErrInvalidInput
	}
	return s.store.RevokeByUser(ctx, userID, id, s.now())
}

func (s *Service) ListAll(ctx context.Context, filter ListFilter) (Page, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if filter.Status != "" && filter.Status != StatusActive && filter.Status != StatusRevoked {
		return Page{}, ErrInvalidInput
	}
	if filter.Offset < 0 {
		return Page{}, ErrInvalidInput
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		return Page{}, ErrInvalidInput
	}
	return s.store.ListAll(ctx, filter)
}

func (s *Service) Revoke(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidInput
	}
	return s.store.Revoke(ctx, id, s.now())
}

func newToken() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "nvr_" + base64.RawURLEncoding.EncodeToString(random), nil
}
