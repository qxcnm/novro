package billinggroup

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

var codePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

type Store interface {
	Create(context.Context, CreateInput) (Record, error)
	List(context.Context, ListFilter) ([]Record, error)
	Update(context.Context, uuid.UUID, UpdateInput) (Record, error)
	SetStatus(context.Context, uuid.UUID, Status) (Record, error)
	Delete(context.Context, uuid.UUID) error
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Create(ctx context.Context, input CreateInput) (Record, error) {
	input.Code = strings.ToLower(strings.TrimSpace(input.Code))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !codePattern.MatchString(input.Code) || !validName(input.DisplayName) || !validMultiplier(input.MultiplierBPS) {
		return Record{}, ErrInvalidInput
	}
	return s.store.Create(ctx, input)
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]Record, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if filter.Status != "" && filter.Status != StatusActive && filter.Status != StatusDisabled {
		return nil, ErrInvalidInput
	}
	return s.store.List(ctx, filter)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (Record, error) {
	if id == uuid.Nil || (input.DisplayName == nil && input.MultiplierBPS == nil) {
		return Record{}, ErrInvalidInput
	}
	if input.DisplayName != nil {
		value := strings.TrimSpace(*input.DisplayName)
		if !validName(value) {
			return Record{}, ErrInvalidInput
		}
		input.DisplayName = &value
	}
	if input.MultiplierBPS != nil && !validMultiplier(*input.MultiplierBPS) {
		return Record{}, ErrInvalidInput
	}
	return s.store.Update(ctx, id, input)
}

func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, status Status) (Record, error) {
	if id == uuid.Nil || (status != StatusActive && status != StatusDisabled) {
		return Record{}, ErrInvalidInput
	}
	return s.store.SetStatus(ctx, id, status)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidInput
	}
	return s.store.Delete(ctx, id)
}

func validName(value string) bool      { return value != "" && utf8.RuneCountInString(value) <= 128 }
func validMultiplier(value int64) bool { return value >= 1 && value <= 1_000_000 }
