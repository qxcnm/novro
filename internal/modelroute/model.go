package modelroute

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/provider"
	"github.com/novro-gateway/novro/internal/upstreammodel"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

var (
	ErrInvalidInput    = errors.New("invalid model route input")
	ErrNotFound        = errors.New("model route not found")
	ErrNameTaken       = errors.New("model route already exists for provider and model")
	ErrPricingRequired = errors.New("upstream model pricing is required")
)

type ProviderSummary struct {
	ID          uuid.UUID         `json:"id"`
	Code        string            `json:"code"`
	DisplayName string            `json:"display_name"`
	Protocol    provider.Protocol `json:"protocol"`
	Status      provider.Status   `json:"status"`
}

type Record struct {
	ID                uuid.UUID             `json:"id"`
	ProviderID        uuid.UUID             `json:"provider_id"`
	UpstreamModelID   *uuid.UUID            `json:"upstream_model_id"`
	PublicName        string                `json:"public_name"`
	DisplayName       string                `json:"display_name"`
	UpstreamName      string                `json:"upstream_name"`
	InputPriceMicros  int64                 `json:"input_price_micros"`
	OutputPriceMicros int64                 `json:"output_price_micros"`
	Status            Status                `json:"status"`
	Provider          ProviderSummary       `json:"provider"`
	UpstreamModel     *upstreammodel.Record `json:"upstream_model,omitempty"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
}

type CreateInput struct {
	UpstreamModelID   uuid.UUID `json:"upstream_model_id"`
	ProviderID        uuid.UUID `json:"provider_id"`
	PublicName        string    `json:"public_name"`
	DisplayName       string    `json:"display_name"`
	UpstreamName      string    `json:"upstream_name"`
	InputPriceMicros  int64     `json:"input_price_micros"`
	OutputPriceMicros int64     `json:"output_price_micros"`
}

type UpdateInput struct {
	UpstreamModelID   *uuid.UUID `json:"upstream_model_id"`
	ProviderID        *uuid.UUID `json:"provider_id"`
	DisplayName       *string    `json:"display_name"`
	UpstreamName      *string    `json:"upstream_name"`
	InputPriceMicros  *int64     `json:"input_price_micros"`
	OutputPriceMicros *int64     `json:"output_price_micros"`
}

type UpdateParams = UpdateInput

type ListFilter struct {
	Search string
	Status Status
}

type Resolved struct {
	Record
	BaseURL string
	APIKey  string
}

type Resolution struct {
	Record          Record
	BaseURL         string
	EncryptedAPIKey string
}
