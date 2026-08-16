package provider

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Protocol string
type Status string
type ValidationField string

const (
	ProtocolOpenAI    Protocol = "openai"
	ProtocolAnthropic Protocol = "anthropic"
	StatusActive      Status   = "active"
	StatusDisabled    Status   = "disabled"
	DefaultWeight              = 100
	MaxWeight                  = 1_000_000

	ValidationFieldCode          ValidationField = "code"
	ValidationFieldDisplayName   ValidationField = "display_name"
	ValidationFieldProtocols     ValidationField = "protocols"
	ValidationFieldBaseURL       ValidationField = "base_url"
	ValidationFieldModelListPath ValidationField = "model_list_path"
	ValidationFieldWeight        ValidationField = "weight"
	ValidationFieldAPIKey        ValidationField = "api_key"
)

var (
	ErrInvalidInput = errors.New("invalid provider input")
	ErrNotFound     = errors.New("provider not found")
	ErrCodeTaken    = errors.New("provider code already exists")
)

type ValidationError struct {
	Field ValidationField
}

func (e *ValidationError) Error() string {
	return ErrInvalidInput.Error() + ": " + string(e.Field)
}

func (e *ValidationError) Unwrap() error {
	return ErrInvalidInput
}

func invalidField(field ValidationField) error {
	return &ValidationError{Field: field}
}

type Record struct {
	ID            uuid.UUID  `json:"id"`
	Code          string     `json:"code"`
	DisplayName   string     `json:"display_name"`
	Protocols     []Protocol `json:"protocols"`
	BaseURL       string     `json:"base_url"`
	ModelListPath string     `json:"model_list_path"`
	Weight        int        `json:"weight"`
	APIKeyHint    string     `json:"api_key_hint"`
	HasAPIKey     bool       `json:"has_api_key"`
	Status        Status     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type CreateInput struct {
	Code          string     `json:"code"`
	DisplayName   string     `json:"display_name"`
	Protocols     []Protocol `json:"protocols"`
	BaseURL       string     `json:"base_url"`
	ModelListPath string     `json:"model_list_path"`
	Weight        int        `json:"weight"`
	APIKey        string     `json:"api_key"`
}

type UpdateInput struct {
	DisplayName   *string     `json:"display_name"`
	Protocols     *[]Protocol `json:"protocols"`
	BaseURL       *string     `json:"base_url"`
	ModelListPath *string     `json:"model_list_path"`
	Weight        *int        `json:"weight"`
	APIKey        *string     `json:"api_key"`
}

type UpdateParams struct {
	DisplayName     *string
	Protocols       *[]Protocol
	PrimaryProtocol *Protocol
	BaseURL         *string
	ModelListPath   *string
	Weight          *int
	EncryptedAPIKey *string
	APIKeyHint      *string
}

func NormalizeProtocols(values []Protocol) ([]Protocol, bool) {
	seen := make(map[Protocol]struct{}, len(values))
	for _, value := range values {
		if value != ProtocolOpenAI && value != ProtocolAnthropic {
			return nil, false
		}
		seen[value] = struct{}{}
	}
	if len(seen) == 0 {
		return nil, false
	}
	protocols := make([]Protocol, 0, len(seen))
	for _, value := range []Protocol{ProtocolOpenAI, ProtocolAnthropic} {
		if _, exists := seen[value]; exists {
			protocols = append(protocols, value)
		}
	}
	return protocols, true
}

func SupportsProtocol(values []Protocol, target Protocol) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func PrimaryProtocol(values []Protocol) Protocol {
	if SupportsProtocol(values, ProtocolOpenAI) {
		return ProtocolOpenAI
	}
	return ProtocolAnthropic
}

func ProtocolsFromStrings(values []string, fallback Protocol) []Protocol {
	protocols := make([]Protocol, 0, len(values))
	for _, value := range values {
		protocols = append(protocols, Protocol(value))
	}
	if normalized, ok := NormalizeProtocols(protocols); ok {
		return normalized
	}
	if normalized, ok := NormalizeProtocols([]Protocol{fallback}); ok {
		return normalized
	}
	return []Protocol{ProtocolOpenAI}
}

func protocolStrings(values []Protocol) []string {
	protocols := make([]string, len(values))
	for index, value := range values {
		protocols[index] = string(value)
	}
	return protocols
}

type ListFilter struct {
	Search string
	Status Status
}
