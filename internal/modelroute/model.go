package modelroute

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/billinggroup"
	"github.com/novro-gateway/novro/internal/provider"
	"github.com/novro-gateway/novro/internal/upstreammodel"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

var (
	ErrInvalidInput     = errors.New("invalid model route input")
	ErrNotFound         = errors.New("model route not found")
	ErrNameTaken        = errors.New("model route already exists for provider and model")
	ErrPricingRequired  = errors.New("upstream model pricing is required")
	ErrGroupUnavailable = errors.New("billing group is unavailable")
)

type ProviderSummary struct {
	ID          uuid.UUID           `json:"id"`
	Code        string              `json:"code"`
	DisplayName string              `json:"display_name"`
	Weight      int                 `json:"weight"`
	Protocols   []provider.Protocol `json:"protocols"`
	Status      provider.Status     `json:"status"`
}

type Record struct {
	ID                uuid.UUID             `json:"id"`
	ProviderID        uuid.UUID             `json:"provider_id"`
	UpstreamModelID   *uuid.UUID            `json:"upstream_model_id"`
	BillingGroupID    uuid.UUID             `json:"billing_group_id"`
	BillingGroup      billinggroup.Summary  `json:"billing_group"`
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
	BillingGroupID    uuid.UUID `json:"billing_group_id"`
	PublicName        string    `json:"public_name"`
	DisplayName       string    `json:"display_name"`
	UpstreamName      string    `json:"upstream_name"`
	InputPriceMicros  int64     `json:"input_price_micros"`
	OutputPriceMicros int64     `json:"output_price_micros"`
}

type UpdateInput struct {
	UpstreamModelID   *uuid.UUID `json:"upstream_model_id"`
	ProviderID        *uuid.UUID `json:"provider_id"`
	BillingGroupID    *uuid.UUID `json:"billing_group_id"`
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

/**
 * Resolved 表示已完成提供商解析并附带本次请求价格快照的路由。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type Resolved struct {
	Record
	BaseURL             string
	APIKey              string
	ResolvedPrices      *upstreammodel.Prices
	PricingPlanID       *uuid.UUID
	PricingWindowID     *uuid.UUID
	PricingWindowLabel  string
	PinnedMultiplierBPS int64
}

type Resolution struct {
	Record          Record
	BaseURL         string
	EncryptedAPIKey string
}
