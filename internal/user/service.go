package user

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var (
	usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)
	emailPattern    = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
)

type Store interface {
	Create(context.Context, CreateParams) (Record, error)
	CreateInitialAdmin(context.Context, CreateParams) (Record, error)
	IsInitialized(context.Context) (bool, error)
	List(context.Context, ListFilter) (Page, error)
	Update(context.Context, uuid.UUID, UpdateParams) (Record, error)
	SetStatus(context.Context, uuid.UUID, Status) (Record, error)
	ResetPassword(context.Context, uuid.UUID, string) error
}

type PasswordHasher interface {
	Hash(string) (string, error)
}

type CreateParams struct {
	Username       string
	Email          string
	DisplayName    string
	PasswordHash   string
	Role           Role
	BillingGroupID *uuid.UUID
}

type UpdateParams struct {
	DisplayName    *string
	Role           *Role
	BillingGroupID *uuid.UUID
}

type Service struct {
	store  Store
	hasher PasswordHasher
}

func NewService(store Store, hasher PasswordHasher) *Service {
	return &Service{store: store, hasher: hasher}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Record, error) {
	return s.create(ctx, input, s.store.Create)
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (Record, error) {
	return s.create(ctx, CreateInput{
		Username: input.Username, Email: input.Email, DisplayName: input.DisplayName, Password: input.Password, Role: RoleMember,
	}, s.store.Create)
}

func (s *Service) InitializeAdmin(ctx context.Context, input RegisterInput) (Record, error) {
	return s.create(ctx, CreateInput{
		Username: input.Username, Email: input.Email, DisplayName: input.DisplayName, Password: input.Password, Role: RoleAdmin,
	}, s.store.CreateInitialAdmin)
}

func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	initialized, err := s.store.IsInitialized(ctx)
	if err != nil {
		return false, fmt.Errorf("check installation state: %w", err)
	}
	return !initialized, nil
}

func (s *Service) create(ctx context.Context, input CreateInput, save func(context.Context, CreateParams) (Record, error)) (Record, error) {
	username := strings.ToLower(strings.TrimSpace(input.Username))
	email, emailOK := NormalizeEmail(input.Email)
	displayName := strings.TrimSpace(input.DisplayName)
	if !usernamePattern.MatchString(username) || !emailOK || len([]rune(displayName)) > 128 {
		return Record{}, ErrInvalidInput
	}
	role := input.Role
	if role == "" {
		role = RoleMember
	}
	if role != RoleAdmin && role != RoleMember {
		return Record{}, ErrInvalidInput
	}
	passwordHash, err := s.hasher.Hash(input.Password)
	if err != nil {
		return Record{}, fmt.Errorf("%w: password: %v", ErrInvalidInput, err)
	}
	return save(ctx, CreateParams{
		Username:       username,
		Email:          email,
		DisplayName:    displayName,
		PasswordHash:   passwordHash,
		Role:           role,
		BillingGroupID: input.BillingGroupID,
	})
}

func NormalizeEmail(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	return value, value != "" && len(value) <= 320 && emailPattern.MatchString(value)
}

func (s *Service) List(ctx context.Context, filter ListFilter) (Page, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if filter.Status != "" && filter.Status != StatusActive && filter.Status != StatusDisabled {
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
	return s.store.List(ctx, filter)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (Record, error) {
	if id == uuid.Nil || (input.DisplayName == nil && input.Role == nil && input.BillingGroupID == nil) {
		return Record{}, ErrInvalidInput
	}
	params := UpdateParams{Role: input.Role, BillingGroupID: input.BillingGroupID}
	if input.DisplayName != nil {
		displayName := strings.TrimSpace(*input.DisplayName)
		if len([]rune(displayName)) > 128 {
			return Record{}, ErrInvalidInput
		}
		params.DisplayName = &displayName
	}
	if input.Role != nil && *input.Role != RoleAdmin && *input.Role != RoleMember {
		return Record{}, ErrInvalidInput
	}
	if input.BillingGroupID != nil && *input.BillingGroupID == uuid.Nil {
		return Record{}, ErrInvalidInput
	}
	return s.store.Update(ctx, id, params)
}

func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, status Status) (Record, error) {
	if id == uuid.Nil || (status != StatusActive && status != StatusDisabled) {
		return Record{}, ErrInvalidInput
	}
	return s.store.SetStatus(ctx, id, status)
}

func (s *Service) ResetPassword(ctx context.Context, id uuid.UUID, plainText string) error {
	if id == uuid.Nil {
		return ErrInvalidInput
	}
	passwordHash, err := s.hasher.Hash(plainText)
	if err != nil {
		return fmt.Errorf("%w: password: %v", ErrInvalidInput, err)
	}
	return s.store.ResetPassword(ctx, id, passwordHash)
}
