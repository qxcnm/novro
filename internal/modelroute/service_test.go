package modelroute

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/provider"
)

type fakeStore struct {
	created        CreateInput
	updated        UpdateParams
	resolutions    []Resolution
	deletedID      uuid.UUID
	billingGroupID uuid.UUID
	err            error
}

func (f *fakeStore) Create(_ context.Context, input CreateInput) (Record, error) {
	f.created = input
	return Record{PublicName: input.PublicName}, f.err
}
func (f *fakeStore) List(context.Context, ListFilter) ([]Record, error) { return nil, f.err }
func (f *fakeStore) Update(_ context.Context, _ uuid.UUID, input UpdateParams) (Record, error) {
	f.updated = input
	return Record{}, f.err
}
func (f *fakeStore) SetStatus(context.Context, uuid.UUID, Status) (Record, error) {
	return Record{}, f.err
}
func (f *fakeStore) Delete(_ context.Context, id uuid.UUID) error {
	f.deletedID = id
	return f.err
}
func (f *fakeStore) ResolveCandidates(_ context.Context, _ string, billingGroupID uuid.UUID) ([]Resolution, error) {
	f.billingGroupID = billingGroupID
	return f.resolutions, f.err
}
func (f *fakeStore) ListActive(_ context.Context, billingGroupID uuid.UUID) ([]Record, error) {
	f.billingGroupID = billingGroupID
	return nil, f.err
}

func TestCreateNormalizesAndValidatesModelRoute(t *testing.T) {
	cipher, _ := provider.NewCipher("01234567890123456789012345678901")
	store := &fakeStore{}
	service := NewService(store, cipher)
	upstreamModelID := uuid.New()
	providerID := uuid.New()
	if _, err := service.Create(context.Background(), CreateInput{ProviderID: providerID, UpstreamModelID: upstreamModelID, PublicName: "  deepseek-chat  ", DisplayName: " DeepSeek Chat "}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if store.created.PublicName != "deepseek-chat" || store.created.DisplayName != "DeepSeek Chat" || store.created.ProviderID != providerID || store.created.UpstreamModelID != upstreamModelID {
		t.Fatalf("not normalized: %+v", store.created)
	}
	for _, name := range []string{"", "bad model", "/starts-wrong"} {
		_, err := service.Create(context.Background(), CreateInput{ProviderID: providerID, UpstreamModelID: upstreamModelID, PublicName: name, DisplayName: "name"})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("name=%q err=%v", name, err)
		}
	}
	longName := "m" + strings.Repeat("-segment", 30)
	if len(longName) <= 128 || len(longName) > 256 {
		t.Fatalf("invalid long-name test fixture length %d", len(longName))
	}
	if _, err := service.Create(context.Background(), CreateInput{ProviderID: providerID, UpstreamModelID: upstreamModelID, PublicName: longName, DisplayName: "Long Model"}); err != nil {
		t.Fatalf("256-character model IDs should be accepted: %v", err)
	}
}

func TestResolveCandidatesDecryptsEveryProviderCredential(t *testing.T) {
	cipher, _ := provider.NewCipher("01234567890123456789012345678901")
	firstEncrypted, _ := cipher.Encrypt("first-secret")
	secondEncrypted, _ := cipher.Encrypt("second-secret")
	store := &fakeStore{resolutions: []Resolution{
		{Record: Record{PublicName: "public"}, BaseURL: "https://first.example.com/v1", EncryptedAPIKey: firstEncrypted},
		{Record: Record{PublicName: "public"}, BaseURL: "https://second.example.com/v1", EncryptedAPIKey: secondEncrypted},
	}}
	groupID := uuid.New()
	resolved, err := NewService(store, cipher).ResolveCandidates(context.Background(), "public", groupID)
	if err != nil || len(resolved) != 2 || resolved[0].APIKey != "first-secret" || resolved[1].APIKey != "second-secret" || resolved[1].BaseURL != "https://second.example.com/v1" {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	if store.billingGroupID != groupID {
		t.Fatalf("billing group filter=%s want=%s", store.billingGroupID, groupID)
	}
}

func TestResolveCandidatesSkipsOneInvalidProviderCredential(t *testing.T) {
	cipher, _ := provider.NewCipher("01234567890123456789012345678901")
	encrypted, _ := cipher.Encrypt("healthy-secret")
	store := &fakeStore{resolutions: []Resolution{
		{Record: Record{PublicName: "public"}, BaseURL: "https://broken.example.com/v1", EncryptedAPIKey: "not-encrypted"},
		{Record: Record{PublicName: "public"}, BaseURL: "https://healthy.example.com/v1", EncryptedAPIKey: encrypted},
	}}
	resolved, err := NewService(store, cipher).ResolveCandidates(context.Background(), "public", uuid.New())
	if err != nil || len(resolved) != 1 || resolved[0].APIKey != "healthy-secret" || resolved[0].BaseURL != "https://healthy.example.com/v1" {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
}

func TestUpdateRejectsNegativePrice(t *testing.T) {
	cipher, _ := provider.NewCipher("01234567890123456789012345678901")
	value := int64(-1)
	_, err := NewService(&fakeStore{}, cipher).Update(context.Background(), uuid.New(), UpdateInput{InputPriceMicros: &value})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}
}

func TestDeleteValidatesIDAndDelegates(t *testing.T) {
	cipher, _ := provider.NewCipher("01234567890123456789012345678901")
	store := &fakeStore{}
	service := NewService(store, cipher)
	if err := service.Delete(context.Background(), uuid.Nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid ID, got %v", err)
	}
	id := uuid.New()
	if err := service.Delete(context.Background(), id); err != nil || store.deletedID != id {
		t.Fatalf("delete id=%s stored=%s err=%v", id, store.deletedID, err)
	}
}
