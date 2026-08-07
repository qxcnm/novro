package modelroute

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/provider"
)

type fakeStore struct {
	created           CreateInput
	updated           UpdateParams
	resolvedEncrypted string
	deletedID         uuid.UUID
	err               error
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
func (f *fakeStore) Resolve(context.Context, string) (Record, string, string, error) {
	return Record{PublicName: "public"}, "https://api.example.com/v1", f.resolvedEncrypted, f.err
}
func (f *fakeStore) ListActive(context.Context) ([]Record, error) { return nil, f.err }

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
}

func TestResolveDecryptsProviderCredential(t *testing.T) {
	cipher, _ := provider.NewCipher("01234567890123456789012345678901")
	encrypted, _ := cipher.Encrypt("secret-upstream-key")
	store := &fakeStore{resolvedEncrypted: encrypted}
	resolved, err := NewService(store, cipher).Resolve(context.Background(), "public")
	if err != nil || resolved.APIKey != "secret-upstream-key" || resolved.BaseURL != "https://api.example.com/v1" {
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
