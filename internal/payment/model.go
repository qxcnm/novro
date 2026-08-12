package payment

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrDisabled       = errors.New("payments are disabled")
	ErrInvalidInput   = errors.New("invalid payment input")
	ErrInvalidNotice  = errors.New("invalid payment notification")
	ErrOrderNotFound  = errors.New("top-up order not found")
	ErrOrderConflict  = errors.New("top-up order conflicts with payment notification")
	ErrOrderUnpaid    = errors.New("top-up order is not paid at provider")
	ErrWalletNotFound = errors.New("wallet not found")
	ErrGatewayQuery   = errors.New("payment gateway order query failed")
	ErrConfigNotFound = errors.New("payment configuration not found")
)

const (
	MinTopUpMicros int64 = 1_000_000
	MaxTopUpMicros int64 = 50_000_000_000
)

type Status string

const (
	StatusPending Status = "pending"
	StatusPaid    Status = "paid"
)

type Order struct {
	ID              uuid.UUID  `json:"id"`
	UserID          uuid.UUID  `json:"user_id"`
	OutTradeNo      string     `json:"out_trade_no"`
	Provider        string     `json:"provider"`
	Channel         string     `json:"channel"`
	AmountMicros    int64      `json:"amount_micros"`
	CreditedMicros  int64      `json:"credited_micros"`
	Status          Status     `json:"status"`
	ProviderTradeNo *string    `json:"provider_trade_no,omitempty"`
	PaidAt          *time.Time `json:"paid_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Page struct {
	Orders []Order `json:"orders"`
	Total  int     `json:"total"`
	Offset int     `json:"offset"`
	Limit  int     `json:"limit"`
}

type ListFilter struct {
	Offset int
	Limit  int
}

type PaymentMethod struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	MinMicros int64  `json:"min_micros"`
	Enabled   bool   `json:"enabled"`
}

type BonusTier struct {
	ThresholdMicros int64 `json:"threshold_micros"`
	BonusBPS        int   `json:"bonus_bps"`
}

type Checkout struct {
	Action string            `json:"action"`
	Method string            `json:"method"`
	Fields map[string]string `json:"fields"`
}

type CreateResult struct {
	Order    Order    `json:"order"`
	Checkout Checkout `json:"checkout"`
}

type PublicConfig struct {
	Enabled            bool            `json:"enabled"`
	Provider           string          `json:"provider,omitempty"`
	Channels           []string        `json:"channels"`
	Methods            []PaymentMethod `json:"methods"`
	MinMicros          int64           `json:"min_micros"`
	MaxMicros          int64           `json:"max_micros"`
	PresetAmountMicros []int64         `json:"preset_amounts_micros"`
	BonusTiers         []BonusTier     `json:"bonus_tiers"`
}

// AdminConfig is the safe, non-secret representation used by the admin
// console. The merchant key is intentionally represented only by a boolean.
type AdminConfig struct {
	Provider           string          `json:"provider"`
	Enabled            bool            `json:"enabled"`
	Configured         bool            `json:"configured"`
	APIURL             string          `json:"api_url"`
	MerchantID         string          `json:"merchant_id"`
	SiteName           string          `json:"site_name"`
	Channels           []string        `json:"channels"`
	Methods            []PaymentMethod `json:"methods"`
	MinMicros          int64           `json:"min_micros"`
	MaxMicros          int64           `json:"max_micros"`
	PresetAmountMicros []int64         `json:"preset_amounts_micros"`
	BonusTiers         []BonusTier     `json:"bonus_tiers"`
	NotifyURL          string          `json:"notify_url"`
	ReturnURL          string          `json:"return_url"`
	HasMerchantKey     bool            `json:"has_merchant_key"`
	UpdatedAt          time.Time       `json:"updated_at,omitempty"`
}

// ConfigInput contains administrator-editable payment settings. A nil key
// preserves the encrypted key already stored in the database.
type ConfigInput struct {
	Enabled            bool
	APIURL             string
	MerchantID         string
	MerchantKey        *string
	SiteName           string
	Channels           []string
	Methods            []PaymentMethod
	MinMicros          int64
	MaxMicros          int64
	PresetAmountMicros []int64
	BonusTiers         []BonusTier
}

type Notification struct {
	OutTradeNo      string
	ProviderTradeNo string
	Channel         string
	AmountMicros    int64
}

type CreateParams struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	OutTradeNo     string
	Channel        string
	AmountMicros   int64
	CreditedMicros int64
}

type TopUpOwner struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
}

type AdminOrder struct {
	Order
	Owner TopUpOwner `json:"owner"`
}

type AdminPage struct {
	Orders []AdminOrder `json:"orders"`
	Total  int          `json:"total"`
	Offset int          `json:"offset"`
	Limit  int          `json:"limit"`
}

type AdminListFilter struct {
	Search  string
	Status  Status
	Channel string
	Offset  int
	Limit   int
}

type CompleteParams struct {
	OutTradeNo      string
	ProviderTradeNo string
	Channel         string
	AmountMicros    int64
	PaidAt          time.Time
}
