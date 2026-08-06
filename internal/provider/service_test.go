package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type fakeStore struct {
	createParams CreateParams
	updateParams UpdateParams
	status       Status
	err          error
}

func (f *fakeStore) Create(_ context.Context, params CreateParams) (Record, error) {
	f.createParams = params
	return Record{ID: uuid.New(), Code: params.Code, APIKeyHint: params.APIKeyHint}, f.err
}
func (f *fakeStore) List(context.Context, ListFilter) ([]Record, error) { return nil, f.err }
func (f *fakeStore) Update(_ context.Context, id uuid.UUID, params UpdateParams) (Record, error) {
	f.updateParams = params
	return Record{ID: id}, f.err
}
func (f *fakeStore) SetStatus(_ context.Context, id uuid.UUID, status Status) (Record, error) {
	f.status = status
	return Record{ID: id, Status: status}, f.err
}

func testService(t *testing.T, store *fakeStore) *Service {
	t.Helper()
	cipher, err := NewCipher("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	return NewService(store, cipher)
}

func TestCreateEncryptsCredentialAndNormalizesProvider(t *testing.T) {
	store := &fakeStore{}
	service := testService(t, store)
	record, err := service.Create(context.Background(), CreateInput{
		Code: " DeepSeek ", DisplayName: " DeepSeek ", Protocol: ProtocolOpenAI,
		BaseURL: "https://api.deepseek.com/", APIKey: "provider-secret-1234",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if record.Code != "deepseek" || store.createParams.BaseURL != "https://api.deepseek.com" || store.createParams.APIKeyHint != "1234" {
		t.Fatalf("unexpected provider: record=%+v params=%+v", record, store.createParams)
	}
	if store.createParams.EncryptedAPIKey == "provider-secret-1234" || strings.Contains(store.createParams.EncryptedAPIKey, "provider-secret") {
		t.Fatal("provider credential was not encrypted")
	}
}

func TestProviderValidationRejectsInsecureOrInvalidInput(t *testing.T) {
	service := testService(t, &fakeStore{})
	inputs := []CreateInput{
		{Code: "x", DisplayName: "X", Protocol: ProtocolOpenAI, BaseURL: "https://api.example.com", APIKey: "secret"},
		{Code: "valid-code", DisplayName: "X", Protocol: Protocol("other"), BaseURL: "https://api.example.com", APIKey: "secret"},
		{Code: "valid-code", DisplayName: "X", Protocol: ProtocolOpenAI, BaseURL: "http://api.example.com", APIKey: "secret"},
	}
	for _, input := range inputs {
		if _, err := service.Create(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for %+v, got %v", input, err)
		}
	}
}

func TestUpdateReencryptsOnlyWhenCredentialProvided(t *testing.T) {
	store := &fakeStore{}
	service := testService(t, store)
	name := "New Name"
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{DisplayName: &name}); err != nil {
		t.Fatalf("update name: %v", err)
	}
	if store.updateParams.EncryptedAPIKey != nil {
		t.Fatal("name update replaced provider credential")
	}
	secret := "replacement-5678"
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{APIKey: &secret}); err != nil {
		t.Fatalf("update secret: %v", err)
	}
	if store.updateParams.EncryptedAPIKey == nil || store.updateParams.APIKeyHint == nil || *store.updateParams.APIKeyHint != "5678" {
		t.Fatalf("credential was not replaced safely: %+v", store.updateParams)
	}
}

func TestCipherRoundTripAndRejectsWrongKey(t *testing.T) {
	cipher, _ := NewCipher("01234567890123456789012345678901")
	encrypted, err := cipher.Encrypt("secret-value")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil || decrypted != "secret-value" {
		t.Fatalf("decrypt=%q err=%v", decrypted, err)
	}
	other, _ := NewCipher("different-secret-012345678901234567")
	if _, err := other.Decrypt(encrypted); err == nil {
		t.Fatal("wrong key decrypted provider credential")
	}
}
