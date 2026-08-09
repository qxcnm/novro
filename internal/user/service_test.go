package user

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakeStore struct {
	createParams  CreateParams
	initialParams CreateParams
	emailExists   bool
	emailCheckErr error
	initialized   bool
	listFilter    ListFilter
	updateParams  UpdateParams
	status        Status
	resetHash     string
}

func (f *fakeStore) Create(_ context.Context, params CreateParams) (Record, error) {
	f.createParams = params
	return Record{ID: uuid.New(), Username: params.Username, Role: params.Role, Status: StatusActive}, nil
}

func (f *fakeStore) CreateInitialAdmin(_ context.Context, params CreateParams) (Record, error) {
	f.initialParams = params
	return Record{ID: uuid.New(), Username: params.Username, Role: RoleAdmin, Status: StatusActive}, nil
}

func (f *fakeStore) EmailExists(context.Context, string) (bool, error) {
	return f.emailExists, f.emailCheckErr
}

func (f *fakeStore) IsInitialized(context.Context) (bool, error) { return f.initialized, nil }

func (f *fakeStore) List(_ context.Context, filter ListFilter) (Page, error) {
	f.listFilter = filter
	return Page{Limit: filter.Limit}, nil
}

func (f *fakeStore) SetStatus(_ context.Context, id uuid.UUID, status Status) (Record, error) {
	f.status = status
	return Record{ID: id, Status: status}, nil
}

func (f *fakeStore) Update(_ context.Context, id uuid.UUID, params UpdateParams) (Record, error) {
	f.updateParams = params
	record := Record{ID: id}
	if params.DisplayName != nil {
		record.DisplayName = *params.DisplayName
	}
	if params.Role != nil {
		record.Role = *params.Role
	}
	return record, nil
}

func (f *fakeStore) ResetPassword(_ context.Context, _ uuid.UUID, hash string) error {
	f.resetHash = hash
	return nil
}
func (f *fakeStore) FindByUsername(context.Context, string) (Record, error) {
	return Record{}, ErrNotFound
}

type fakeHasher struct{}

func (fakeHasher) Hash(value string) (string, error) {
	if value == "invalid" {
		return "", errors.New("invalid password")
	}
	return "hashed:" + value, nil
}

func TestCreateNormalizesUserAndHashesPassword(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, fakeHasher{})
	created, err := service.Create(context.Background(), CreateInput{
		Username:    "  Alice.Admin ",
		Email:       " Alice.Admin@Example.COM ",
		DisplayName: " Alice ",
		Password:    "test-password",
		Role:        RoleAdmin,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Username != "alice.admin" || store.createParams.Email != "alice.admin@example.com" || store.createParams.PasswordHash != "hashed:test-password" {
		t.Fatalf("unexpected create params: %+v", store.createParams)
	}
}

func TestRegisterCannotCreateAdministrator(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, fakeHasher{})
	created, err := service.Register(context.Background(), RegisterInput{
		Username: "member.one", Email: "member@example.com", DisplayName: "Member", Password: "test-password", ReferralCode: "  abcd1234ef56 ",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if created.Role != RoleMember || store.createParams.Role != RoleMember {
		t.Fatalf("registration elevated role: created=%+v params=%+v", created, store.createParams)
	}
	if store.createParams.ReferralCode != "ABCD1234EF56" {
		t.Fatalf("referral code was not normalized: %+v", store.createParams)
	}
}

func TestRegisterRejectsMalformedReferralCode(t *testing.T) {
	service := NewService(&fakeStore{}, fakeHasher{})
	_, err := service.Register(context.Background(), RegisterInput{
		Username: "member.one", Email: "member@example.com", Password: "test-password", ReferralCode: "bad-code",
	})
	if !errors.Is(err, ErrInvalidReferralCode) {
		t.Fatalf("expected invalid referral code, got %v", err)
	}
}

func TestEmailAvailableNormalizesAndChecksStore(t *testing.T) {
	store := &fakeStore{emailExists: true}
	service := NewService(store, fakeHasher{})
	available, err := service.EmailAvailable(context.Background(), " User@Example.COM ")
	if err != nil {
		t.Fatalf("check email availability: %v", err)
	}
	if available {
		t.Fatal("registered email was reported as available")
	}
	if _, err := service.EmailAvailable(context.Background(), "not-an-email"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid email error, got %v", err)
	}
}

func TestInitializeAdminUsesDedicatedStoreOperation(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, fakeHasher{})
	created, err := service.InitializeAdmin(context.Background(), RegisterInput{
		Username: "owner.admin", Email: "owner@example.com", Password: "test-password",
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if created.Role != RoleAdmin || store.initialParams.Role != RoleAdmin || store.initialParams.PasswordHash == "" {
		t.Fatalf("unexpected administrator initialization: created=%+v params=%+v", created, store.initialParams)
	}
}

func TestProtectedAdminErrorIsDistinct(t *testing.T) {
	if ErrProtectedAdmin == ErrLastActiveAdmin {
		t.Fatal("protected administrator error must be distinct from last-admin invariant")
	}
}

func TestSetupRequiredUsesPermanentInstallationState(t *testing.T) {
	service := NewService(&fakeStore{initialized: true}, fakeHasher{})
	required, err := service.SetupRequired(context.Background())
	if err != nil || required {
		t.Fatalf("initialized installation requires setup: required=%v err=%v", required, err)
	}
}

func TestServiceValidatesListAndStatus(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, fakeHasher{})
	page, err := service.List(context.Background(), ListFilter{})
	if err != nil || page.Limit != 50 {
		t.Fatalf("default list limit: page=%+v err=%v", page, err)
	}
	if _, err := service.List(context.Background(), ListFilter{Limit: 101}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid list limit, got %v", err)
	}
	if _, err := service.SetStatus(context.Background(), uuid.Nil, StatusActive); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid ID, got %v", err)
	}
}

func TestResetPasswordHashesBeforeStore(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, fakeHasher{})
	if err := service.ResetPassword(context.Background(), uuid.New(), "new-password"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if store.resetHash != "hashed:new-password" {
		t.Fatalf("unexpected stored hash %q", store.resetHash)
	}
}

func TestUpdateNormalizesEditableFields(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, fakeHasher{})
	displayName := "  Alice Chen  "
	role := RoleAdmin
	updated, err := service.Update(context.Background(), uuid.New(), UpdateInput{DisplayName: &displayName, Role: &role})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.DisplayName != "Alice Chen" || updated.Role != RoleAdmin {
		t.Fatalf("unexpected update: record=%+v params=%+v", updated, store.updateParams)
	}
}

func TestUpdateRejectsEmptyAndInvalidRole(t *testing.T) {
	service := NewService(&fakeStore{}, fakeHasher{})
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected empty update to fail, got %v", err)
	}
	role := Role("owner")
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{Role: &role}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid role to fail, got %v", err)
	}
}

func TestPasswordValidationMapsToInvalidInput(t *testing.T) {
	service := NewService(&fakeStore{}, fakeHasher{})
	if _, err := service.Create(context.Background(), CreateInput{
		Username: "valid.user",
		Email:    "valid@example.com",
		Password: "invalid",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid create input, got %v", err)
	}
	if err := service.ResetPassword(context.Background(), uuid.New(), "invalid"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid reset input, got %v", err)
	}
}

func TestCreateRejectsMissingOrInvalidEmail(t *testing.T) {
	service := NewService(&fakeStore{}, fakeHasher{})
	for _, email := range []string{"", "not-an-email", "user@example"} {
		_, err := service.Create(context.Background(), CreateInput{Username: "valid.user", Email: email, Password: "test-password"})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("email=%q err=%v", email, err)
		}
	}
}
