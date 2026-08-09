package provider

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/billinggroup"
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
	ErrInvalidInput     = errors.New("invalid provider input")
	ErrNotFound         = errors.New("provider not found")
	ErrCodeTaken        = errors.New("provider code already exists")
	ErrGroupUnavailable = errors.New("billing group is unavailable")
)

type Record struct {
	ID             uuid.UUID            `json:"id"`
	BillingGroupID uuid.UUID            `json:"billing_group_id"`
	BillingGroup   billinggroup.Summary `json:"billing_group"`
	Code           string               `json:"code"`
	DisplayName    string               `json:"display_name"`
	Protocol       Protocol             `json:"protocol"`
	BaseURL        string               `json:"base_url"`
	ModelListPath  string               `json:"model_list_path"`
	APIKeyHint     string               `json:"api_key_hint"`
	HasAPIKey      bool                 `json:"has_api_key"`
	Status         Status               `json:"status"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

type CreateInput struct {
	Code           string    `json:"code"`
	DisplayName    string    `json:"display_name"`
	Protocol       Protocol  `json:"protocol"`
	BaseURL        string    `json:"base_url"`
	ModelListPath  string    `json:"model_list_path"`
	APIKey         string    `json:"api_key"`
	BillingGroupID uuid.UUID `json:"billing_group_id"`
}

type UpdateInput struct {
	DisplayName    *string    `json:"display_name"`
	Protocol       *Protocol  `json:"protocol"`
	BaseURL        *string    `json:"base_url"`
	ModelListPath  *string    `json:"model_list_path"`
	APIKey         *string    `json:"api_key"`
	BillingGroupID *uuid.UUID `json:"billing_group_id"`
}

type UpdateParams struct {
	DisplayName     *string
	Protocol        *Protocol
	BaseURL         *string
	ModelListPath   *string
	EncryptedAPIKey *string
	APIKeyHint      *string
	BillingGroupID  *uuid.UUID
}

type ListFilter struct {
	Search string
	Status Status
}
