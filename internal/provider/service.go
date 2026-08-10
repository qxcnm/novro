package provider

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

var providerCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,62}[a-z0-9]$`)

type Store interface {
	Create(context.Context, CreateParams) (Record, error)
	List(context.Context, ListFilter) ([]Record, error)
	Update(context.Context, uuid.UUID, UpdateParams) (Record, error)
	SetStatus(context.Context, uuid.UUID, Status) (Record, error)
	Delete(context.Context, uuid.UUID) error
}

type CreateParams struct {
	Code            string
	BillingGroupID  uuid.UUID
	DisplayName     string
	Protocol        Protocol
	BaseURL         string
	ModelListPath   string
	EncryptedAPIKey string
	APIKeyHint      string
}

type Service struct {
	store  Store
	cipher *Cipher
}

func NewService(store Store, cipher *Cipher) *Service {
	return &Service{store: store, cipher: cipher}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Record, error) {
	code := strings.ToLower(strings.TrimSpace(input.Code))
	displayName := strings.TrimSpace(input.DisplayName)
	baseURL, ok := normalizeBaseURL(input.BaseURL)
	modelListPath, pathOK := normalizeModelListPath(input.ModelListPath)
	apiKey := strings.TrimSpace(input.APIKey)
	if !providerCodePattern.MatchString(code) || input.BillingGroupID == uuid.Nil || displayName == "" || utf8.RuneCountInString(displayName) > 128 || !validProtocol(input.Protocol) || !ok || !pathOK || apiKey == "" || len(apiKey) > 1024 {
		return Record{}, ErrInvalidInput
	}
	encrypted, err := s.cipher.Encrypt(apiKey)
	if err != nil {
		return Record{}, err
	}
	return s.store.Create(ctx, CreateParams{
		Code: code, BillingGroupID: input.BillingGroupID, DisplayName: displayName, Protocol: input.Protocol, BaseURL: baseURL, ModelListPath: modelListPath,
		EncryptedAPIKey: encrypted, APIKeyHint: secretHint(apiKey),
	})
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]Record, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if filter.Status != "" && filter.Status != StatusActive && filter.Status != StatusDisabled {
		return nil, ErrInvalidInput
	}
	return s.store.List(ctx, filter)
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (Record, error) {
	if id == uuid.Nil || (input.DisplayName == nil && input.Protocol == nil && input.BaseURL == nil && input.ModelListPath == nil && input.APIKey == nil && input.BillingGroupID == nil) {
		return Record{}, ErrInvalidInput
	}
	params := UpdateParams{Protocol: input.Protocol}
	if input.BillingGroupID != nil {
		if *input.BillingGroupID == uuid.Nil {
			return Record{}, ErrInvalidInput
		}
		params.BillingGroupID = input.BillingGroupID
	}
	if input.DisplayName != nil {
		value := strings.TrimSpace(*input.DisplayName)
		if value == "" || utf8.RuneCountInString(value) > 128 {
			return Record{}, ErrInvalidInput
		}
		params.DisplayName = &value
	}
	if input.Protocol != nil && !validProtocol(*input.Protocol) {
		return Record{}, ErrInvalidInput
	}
	if input.BaseURL != nil {
		value, ok := normalizeBaseURL(*input.BaseURL)
		if !ok {
			return Record{}, ErrInvalidInput
		}
		params.BaseURL = &value
	}
	if input.ModelListPath != nil {
		value, ok := normalizeModelListPath(*input.ModelListPath)
		if !ok {
			return Record{}, ErrInvalidInput
		}
		params.ModelListPath = &value
	}
	if input.APIKey != nil {
		value := strings.TrimSpace(*input.APIKey)
		if value == "" || len(value) > 1024 {
			return Record{}, ErrInvalidInput
		}
		encrypted, err := s.cipher.Encrypt(value)
		if err != nil {
			return Record{}, err
		}
		hint := secretHint(value)
		params.EncryptedAPIKey = &encrypted
		params.APIKeyHint = &hint
	}
	return s.store.Update(ctx, id, params)
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

func validProtocol(protocol Protocol) bool {
	return protocol == ProtocolOpenAI || protocol == ProtocolAnthropic
}

func normalizeBaseURL(value string) (string, bool) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return parsed.String(), true
}

func normalizeModelListPath(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", true
	}
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "?#") || len(value) > 512 {
		return "", false
	}
	return "/" + strings.Trim(value, "/"), true
}

func secretHint(value string) string {
	runes := []rune(value)
	if len(runes) <= 4 {
		return string(runes)
	}
	return string(runes[len(runes)-4:])
}
