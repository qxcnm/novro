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
	EntryManualAdjustment  EntryType = "manual_adjustment"
	EntryTopUp             EntryType = "top_up"
	EntryReferralReward    EntryType = "referral_reward"
	EntryUsageReservation  EntryType = "usage_reservation"
	EntryUsageRefund       EntryType = "usage_refund"
	EntryUsageSettlement   EntryType = "usage_settlement"
	EntryUsageCompensation EntryType = "usage_compensation"
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
	Wallet         Wallet  `json:"wallet"`
	Entries        []Entry `json:"entries"`
	EntriesTotal   int     `json:"entries_total"`
	EntriesOffset  int     `json:"entries_offset"`
	EntriesLimit   int     `json:"entries_limit"`
	ReservedMicros int64   `json:"reserved_micros"`
}

type EntryFilter struct {
	Offset int
	Limit  int
}

type UsageStatus string

const (
	UsageStatusAll     UsageStatus = ""
	UsageStatusSuccess UsageStatus = "success"
	UsageStatusFailed  UsageStatus = "failed"
)

type UsageFilter struct {
	Search   string
	APIKeyID uuid.UUID
	Model    string
	Status   UsageStatus
	From     *time.Time
	Offset   int
	Limit    int
}

type UsagePage struct {
	Usage           []Usage  `json:"usage"`
	Models          []string `json:"models"`
	Total           int      `json:"total"`
	Offset          int      `json:"offset"`
	Limit           int      `json:"limit"`
	TotalTokens     int64    `json:"total_tokens"`
	TotalCostMicros int64    `json:"total_cost_micros"`
}

type UsageRate struct {
	WindowSeconds int       `json:"window_seconds"`
	Requests      int       `json:"requests"`
	InputTokens   int64     `json:"input_tokens"`
	OutputTokens  int64     `json:"output_tokens"`
	TotalTokens   int64     `json:"total_tokens"`
	RPM           int       `json:"rpm"`
	TPM           int64     `json:"tpm"`
	CalculatedAt  time.Time `json:"calculated_at"`
}

type Usage struct {
	ID                      uuid.UUID `json:"id"`
	UserID                  uuid.UUID `json:"user_id"`
	Username                string    `json:"username"`
	UserDisplayName         string    `json:"user_display_name"`
	RequestID               uuid.UUID `json:"request_id"`
	StatusCode              int       `json:"status_code"`
	ErrorCode               string    `json:"error_code,omitempty"`
	ErrorMessage            string    `json:"error_message,omitempty"`
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
	DurationMS              int64     `json:"duration_ms"`
}

type UsageInput struct {
	UserID             uuid.UUID
	APIKeyID           uuid.UUID
	ModelRouteID       uuid.UUID
	UpstreamModelID    *uuid.UUID
	BillingGroupID     *uuid.UUID
	RequestID          uuid.UUID
	Endpoint           string
	StatusCode         int
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

type FailureInput struct {
	UserID            uuid.UUID
	APIKeyID          uuid.UUID
	ModelRouteID      uuid.UUID
	UpstreamModelID   *uuid.UUID
	BillingGroupID    *uuid.UUID
	RequestID         uuid.UUID
	Endpoint          string
	StatusCode        int
	ErrorCode         string
	ErrorMessage      string
	MultiplierBPS     int64
	ModelName         string
	UpstreamModelName string
	BillingGroupCode  string
	BillingGroupName  string
	CreatedAt         time.Time
	FinishedAt        time.Time
	DurationMS        int64
}

type OperationStatus string

const (
	OperationProcessing        OperationStatus = "processing"
	OperationPendingSettlement OperationStatus = "pending_settlement"
	OperationPendingUnknown    OperationStatus = "pending_unknown"
	OperationCompleted         OperationStatus = "completed"
	OperationFailed            OperationStatus = "failed"
)

type OperationStartInput struct {
	RequestID          uuid.UUID
	UserID             uuid.UUID
	APIKeyID           uuid.UUID
	IdempotencyKeyHash string
	RequestHash        string
	Endpoint           string
	ReservedMicros     int64
}

type Operation struct {
	RequestID          uuid.UUID
	UserID             uuid.UUID
	APIKeyID           uuid.UUID
	IdempotencyKeyHash string
	RequestHash        string
	Endpoint           string
	Status             OperationStatus
	ReservedMicros     int64
	FailureCode        string
	UpdatedAt          time.Time
}

type OperationStartResult struct {
	Operation Operation
	Created   bool
}

type PendingSettlement struct {
	Operation Operation
	Usage     UsageInput
}
