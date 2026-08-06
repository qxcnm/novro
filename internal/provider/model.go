package provider

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Protocol string
type Status string

const (
	ProtocolOpenAI    Protocol = "openai"
	ProtocolAnthropic Protocol = "anthropic"
	StatusActive      Status   = "active"
	StatusDisabled    Status   = "disabled"
)

var (
	ErrInvalidInput = errors.New("invalid provider input")
	ErrNotFound     = errors.New("provider not found")
	ErrCodeTaken    = errors.New("provider code already exists")
)

type Record struct {
	ID          uuid.UUID `json:"id"`
	Code        string    `json:"code"`
	DisplayName string    `json:"display_name"`
	Protocol    Protocol  `json:"protocol"`
	BaseURL     string    `json:"base_url"`
	APIKeyHint  string    `json:"api_key_hint"`
	HasAPIKey   bool      `json:"has_api_key"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateInput struct {
	Code        string   `json:"code"`
	DisplayName string   `json:"display_name"`
	Protocol    Protocol `json:"protocol"`
	BaseURL     string   `json:"base_url"`
	APIKey      string   `json:"api_key"`
}

type UpdateInput struct {
	DisplayName *string   `json:"display_name"`
	Protocol    *Protocol `json:"protocol"`
	BaseURL     *string   `json:"base_url"`
	APIKey      *string   `json:"api_key"`
}

type UpdateParams struct {
	DisplayName     *string
	Protocol        *Protocol
	BaseURL         *string
	EncryptedAPIKey *string
	APIKeyHint      *string
}

type ListFilter struct {
	Search string
	Status Status
}
