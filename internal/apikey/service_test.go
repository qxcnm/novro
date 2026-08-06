package apikey

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeStore struct {
	createdName   string
	createdPrefix string
	createdHash   string
	createdLimit  int
	listFilter    ListFilter
	revokedUserID uuid.UUID
	revokedID     uuid.UUID
	err           error
	authHash      string
}

func (f *fakeStore) Create(_ context.Context, userID uuid.UUID, name, prefix, hash string, limit int) (Record, error) {
	f.createdName, f.createdPrefix, f.createdHash, f.createdLimit = name, prefix, hash, limit
	if f.err != nil {
		return Record{}, f.err
	}
	return Record{ID: uuid.New(), UserID: userID, Name: name, KeyPrefix: prefix, Status: StatusActive}, nil
}

func (f *fakeStore) ListByUser(context.Context, uuid.UUID) ([]Record, error) { return nil, f.err }

func (f *fakeStore) RevokeByUser(_ context.Context, userID, id uuid.UUID, _ time.Time) error {
	f.revokedUserID, f.revokedID = userID, id
	return f.err
}

func (f *fakeStore) ListAll(_ context.Context, filter ListFilter) (Page, error) {
	f.listFilter = filter
	return Page{Limit: filter.Limit}, f.err
}

func (f *fakeStore) Revoke(_ context.Context, id uuid.UUID, _ time.Time) error {
	f.revokedID = id
	return f.err
}

func (f *fakeStore) AuthenticateHash(_ context.Context, hash string, _ time.Time) (Actor, error) {
	f.authHash = hash
	return Actor{}, f.err
}

func TestCreateReturnsSecretOnceAndStoresOnlyHash(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	service.generateToken = func() (string, error) { return "nvr_abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG", nil }
	result, err := service.Create(context.Background(), uuid.New(), "  Production  ")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if result.Key == "" || store.createdName != "Production" || store.createdPrefix != result.Key[:12] || store.createdLimit != maxActiveKeysPerUser {
		t.Fatalf("unexpected create result=%+v store=%+v", result, store)
	}
	if store.createdHash == result.Key || len(store.createdHash) != 64 || strings.Contains(store.createdHash, "nvr_") {
		t.Fatalf("plaintext API key reached persistence: %q", store.createdHash)
	}
}

func TestCreateValidatesNameAndPreservesLimitError(t *testing.T) {
	service := NewService(&fakeStore{})
	if _, err := service.Create(context.Background(), uuid.New(), " "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid name, got %v", err)
	}
	service = NewService(&fakeStore{err: ErrLimitReached})
	if _, err := service.Create(context.Background(), uuid.New(), "Test"); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("expected limit error, got %v", err)
	}
}

func TestAuthenticateHashesTokenBeforePersistence(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	if _, err := service.Authenticate(context.Background(), "nvr_test-token-value-12345678901234567890"); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if len(store.authHash) != 64 || strings.Contains(store.authHash, "nvr_") {
		t.Fatalf("plaintext token reached persistence: %q", store.authHash)
	}
	if _, err := service.Authenticate(context.Background(), "invalid"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected invalid token rejection, got %v", err)
	}
}

func TestListAndRevokeValidateScope(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	page, err := service.ListAll(context.Background(), ListFilter{})
	if err != nil || page.Limit != 50 || store.listFilter.Limit != 50 {
		t.Fatalf("default list: page=%+v filter=%+v err=%v", page, store.listFilter, err)
	}
	if _, err := service.ListAll(context.Background(), ListFilter{Status: "unknown"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid status, got %v", err)
	}
	userID, id := uuid.New(), uuid.New()
	if err := service.RevokeForUser(context.Background(), userID, id); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if store.revokedUserID != userID || store.revokedID != id {
		t.Fatalf("revoke escaped user scope: %+v", store)
	}
}
