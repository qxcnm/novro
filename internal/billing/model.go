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
	EntryUsageReservation EntryType = "usage_reservation"
	EntryUsageRefund      EntryType = "usage_refund"
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
	ID                uuid.UUID `json:"id"`
	RequestID         uuid.UUID `json:"request_id"`
	APIKeyID          uuid.UUID `json:"api_key_id"`
	APIKeyName        string    `json:"api_key_name"`
	ModelName         string    `json:"model"`
	Endpoint          string    `json:"endpoint"`
	InputTokens       int       `json:"input_tokens"`
	OutputTokens      int       `json:"output_tokens"`
	CostMicros        int64     `json:"cost_micros"`
	Estimated         bool      `json:"estimated"`
	UpstreamRequestID string    `json:"upstream_request_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	FinishedAt        time.Time `json:"finished_at"`
}

type UsageInput struct {
	UserID            uuid.UUID
	APIKeyID          uuid.UUID
	ModelRouteID      uuid.UUID
	RequestID         uuid.UUID
	Endpoint          string
	InputTokens       int
	OutputTokens      int
	CostMicros        int64
	ReservedMicros    int64
	Estimated         bool
	UpstreamRequestID string
	CreatedAt         time.Time
	FinishedAt        time.Time
}
