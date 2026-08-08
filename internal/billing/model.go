package billing

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput        = errors.New("invalid billing input")
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrRequestConflict     = errors.New("billing request conflicts with an existing operation")
)

type EntryType string

const (
	EntryManualAdjustment EntryType = "manual_adjustment"
	EntryTopUp            EntryType = "top_up"
	EntryReferralReward   EntryType = "referral_reward"
	EntryUsageReservation EntryType = "usage_reservation"
	EntryUsageRefund      EntryType = "usage_refund"
	EntryUsageSettlement  EntryType = "usage_settlement"
)

type Wallet struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	BalanceMicros int64     `json:"balance_micros"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Entry struct {
	ID                 uuid.UUID `json:"id"`
	ReferenceID        uuid.UUID `json:"reference_id"`
	Type               EntryType `json:"entry_type"`
	AmountMicros       int64     `json:"amount_micros"`
	BalanceAfterMicros int64     `json:"balance_after_micros"`
	Description        string    `json:"description"`
	CreatedAt          time.Time `json:"created_at"`
}

type Summary struct {
	Wallet  Wallet  `json:"wallet"`
	Entries []Entry `json:"entries"`
}

type Usage struct {
	ID                      uuid.UUID `json:"id"`
	RequestID               uuid.UUID `json:"request_id"`
	APIKeyID                uuid.UUID `json:"api_key_id"`
	APIKeyName              string    `json:"api_key_name"`
	ModelName               string    `json:"model"`
	Endpoint                string    `json:"endpoint"`
	InputTokens             int       `json:"input_tokens"`
	UncachedInputTokens     int       `json:"uncached_input_tokens"`
	CacheReadInputTokens    int       `json:"cache_read_input_tokens"`
	CacheWriteInputTokens   int       `json:"cache_write_input_tokens"`
	CacheWrite1hInputTokens int       `json:"cache_write_1h_input_tokens"`
	OutputTokens            int       `json:"output_tokens"`
	Rates                   RateCard  `json:"rates"`
	BaseCostMicros          int64     `json:"base_cost_micros"`
	MultiplierBPS           int64     `json:"multiplier_bps"`
	CostMicros              int64     `json:"cost_micros"`
	ReservedMicros          int64     `json:"reserved_micros"`
	BillingGroupCode        string    `json:"billing_group_code"`
	BillingGroupName        string    `json:"billing_group_name"`
	UpstreamModelName       string    `json:"upstream_model_name"`
	CalculationVersion      string    `json:"calculation_version"`
	Estimated               bool      `json:"estimated"`
	UpstreamRequestID       string    `json:"upstream_request_id,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
	FinishedAt              time.Time `json:"finished_at"`
}

type UsageInput struct {
	UserID             uuid.UUID
	APIKeyID           uuid.UUID
	ModelRouteID       uuid.UUID
	UpstreamModelID    *uuid.UUID
	BillingGroupID     *uuid.UUID
	RequestID          uuid.UUID
	Endpoint           string
	InputTokens        int
	Tokens             TokenBreakdown
	OutputTokens       int
	Rates              RateCard
	BaseCostMicros     int64
	MultiplierBPS      int64
	CostMicros         int64
	ReservedMicros     int64
	Estimated          bool
	UpstreamRequestID  string
	ModelName          string
	UpstreamModelName  string
	BillingGroupCode   string
	BillingGroupName   string
	CalculationVersion string
	CreatedAt          time.Time
	FinishedAt         time.Time
}
