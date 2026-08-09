package modelroute

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/provider"
)

var publicNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{1,255}$`)

type Store interface {
	Create(context.Context, CreateInput) (Record, error)
	List(context.Context, ListFilter) ([]Record, error)
	Update(context.Context, uuid.UUID, UpdateParams) (Record, error)
	SetStatus(context.Context, uuid.UUID, Status) (Record, error)
	Delete(context.Context, uuid.UUID) error
	ResolveCandidates(context.Context, string, uuid.UUID) ([]Resolution, error)
	ListActive(context.Context, uuid.UUID) ([]Record, error)
}

type Service struct {
	store  Store
	cipher *provider.Cipher
}

func NewService(store Store, cipher *provider.Cipher) *Service {
	return &Service{store: store, cipher: cipher}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Record, error) {
	input.PublicName = strings.TrimSpace(input.PublicName)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.UpstreamName = strings.TrimSpace(input.UpstreamName)
	if input.UpstreamModelID == uuid.Nil || input.ProviderID == uuid.Nil || !publicNamePattern.MatchString(input.PublicName) || !validText(input.DisplayName, 128) {
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
	if id == uuid.Nil || (input.UpstreamModelID == nil && input.ProviderID == nil && input.DisplayName == nil && input.UpstreamName == nil && input.InputPriceMicros == nil && input.OutputPriceMicros == nil) {
		return Record{}, ErrInvalidInput
	}
	if input.ProviderID != nil && *input.ProviderID == uuid.Nil {
		return Record{}, ErrInvalidInput
	}
	if input.UpstreamModelID != nil && *input.UpstreamModelID == uuid.Nil {
		return Record{}, ErrInvalidInput
	}
	if input.DisplayName != nil {
		value := strings.TrimSpace(*input.DisplayName)
		if !validText(value, 128) {
			return Record{}, ErrInvalidInput
		}
		input.DisplayName = &value
	}
	if input.UpstreamName != nil {
		value := strings.TrimSpace(*input.UpstreamName)
		if !validText(value, 256) {
			return Record{}, ErrInvalidInput
		}
		input.UpstreamName = &value
	}
	if input.InputPriceMicros != nil && !validPrice(*input.InputPriceMicros) {
		return Record{}, ErrInvalidInput
	}
	if input.OutputPriceMicros != nil && !validPrice(*input.OutputPriceMicros) {
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

func (s *Service) ResolveCandidates(ctx context.Context, publicName string, billingGroupID uuid.UUID) ([]Resolved, error) {
	if billingGroupID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	resolutions, err := s.store.ResolveCandidates(ctx, strings.TrimSpace(publicName), billingGroupID)
	if err != nil {
		return nil, err
	}
	resolved := make([]Resolved, 0, len(resolutions))
	var decryptErr error
	for _, resolution := range resolutions {
		apiKey, err := s.cipher.Decrypt(resolution.EncryptedAPIKey)
		if err != nil {
			// A broken credential must not take healthy routes in the same pool offline.
			decryptErr = err
			continue
		}
		resolved = append(resolved, Resolved{Record: resolution.Record, BaseURL: resolution.BaseURL, APIKey: apiKey})
	}
	if len(resolved) == 0 && decryptErr != nil {
		return nil, decryptErr
	}
	return resolved, nil
}

func (s *Service) ListActive(ctx context.Context, billingGroupID uuid.UUID) ([]Record, error) {
	if billingGroupID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.store.ListActive(ctx, billingGroupID)
}

func validText(value string, max int) bool {
	return value != "" && utf8.RuneCountInString(value) <= max
}
func validPrice(value int64) bool          { return value >= 0 && value <= 1_000_000_000_000 }
func validPrices(input, output int64) bool { return validPrice(input) && validPrice(output) }
