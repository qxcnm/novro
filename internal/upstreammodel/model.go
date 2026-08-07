package upstreammodel

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

var (
	ErrInvalidInput    = errors.New("invalid upstream model input")
	ErrNotFound        = errors.New("upstream model not found")
	ErrNameTaken       = errors.New("upstream model already exists for provider")
	ErrPricingRequired = errors.New("upstream model pricing is required")
)

type Prices struct {
	InputMicros        int64 `json:"input_price_micros"`
	OutputMicros       int64 `json:"output_price_micros"`
	CacheReadMicros    int64 `json:"cache_read_price_micros"`
	CacheWriteMicros   int64 `json:"cache_write_price_micros"`
	CacheWrite1hMicros int64 `json:"cache_write_1h_price_micros"`
	RequestMicros      int64 `json:"request_price_micros"`
}

type Record struct {
	ID                uuid.UUID `json:"id"`
	ProviderName      string    `json:"provider_name"`
	UpstreamName      string    `json:"upstream_name"`
	DisplayName       string    `json:"display_name"`
	Prices            Prices    `json:"prices"`
	PricingConfigured bool      `json:"pricing_configured"`
	Status            Status    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type CreateInput struct {
	ProviderName string `json:"provider_name"`
	UpstreamName string `json:"upstream_name"`
	DisplayName  string `json:"display_name"`
	Prices
}

type UpdateInput struct {
	ProviderName       *string `json:"provider_name"`
	UpstreamName       *string `json:"upstream_name"`
	DisplayName        *string `json:"display_name"`
	InputMicros        *int64  `json:"input_price_micros"`
	OutputMicros       *int64  `json:"output_price_micros"`
	CacheReadMicros    *int64  `json:"cache_read_price_micros"`
	CacheWriteMicros   *int64  `json:"cache_write_price_micros"`
	CacheWrite1hMicros *int64  `json:"cache_write_1h_price_micros"`
	RequestMicros      *int64  `json:"request_price_micros"`
}

type ListFilter struct {
	Search string
	Status Status
}
