package upstreammodel

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

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
	input.ProviderName = strings.TrimSpace(input.ProviderName)
	input.UpstreamName = strings.TrimSpace(input.UpstreamName)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !validText(input.ProviderName, 128) || !validText(input.UpstreamName, 256) || !validText(input.DisplayName, 128) || !validPrices(input.Prices) {
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
	if id == uuid.Nil || emptyUpdate(input) {
		return Record{}, ErrInvalidInput
	}
	for value, max := range map[**string]int{&input.ProviderName: 128, &input.UpstreamName: 256, &input.DisplayName: 128} {
		if *value != nil {
			trimmed := strings.TrimSpace(**value)
			if !validText(trimmed, max) {
				return Record{}, ErrInvalidInput
			}
			**value = trimmed
		}
	}
	for _, price := range []*int64{input.InputMicros, input.OutputMicros, input.CacheReadMicros, input.CacheWriteMicros, input.CacheWrite1hMicros, input.RequestMicros} {
		if price != nil && !validPrice(*price) {
			return Record{}, ErrInvalidInput
		}
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

func emptyUpdate(input UpdateInput) bool {
	return input.ProviderName == nil && input.UpstreamName == nil && input.DisplayName == nil && input.InputMicros == nil && input.OutputMicros == nil && input.CacheReadMicros == nil && input.CacheWriteMicros == nil && input.CacheWrite1hMicros == nil && input.RequestMicros == nil
}
func validText(value string, max int) bool {
	return value != "" && utf8.RuneCountInString(value) <= max
}
func validPrice(value int64) bool { return value >= 0 && value <= 1_000_000_000_000 }
func validPrices(p Prices) bool {
	return validPrice(p.InputMicros) && validPrice(p.OutputMicros) && validPrice(p.CacheReadMicros) && validPrice(p.CacheWriteMicros) && validPrice(p.CacheWrite1hMicros) && validPrice(p.RequestMicros)
}
