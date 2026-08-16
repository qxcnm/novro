package apikey

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/billinggroup"
	"github.com/novro-gateway/novro/internal/user"
)

type Status string

const (
	StatusActive  Status = "active"
	StatusRevoked Status = "revoked"
)

var (
	ErrInvalidInput      = errors.New("invalid API key input")
	ErrNotFound          = errors.New("API key not found")
	ErrLimitReached      = errors.New("active API key limit reached")
	ErrSecretUnavailable = errors.New("API key secret unavailable")
	ErrUnauthenticated   = errors.New("invalid API key")
	ErrGroupUnavailable  = errors.New("billing group is unavailable")
)

type Record struct {
	ID                  uuid.UUID            `json:"id"`
	UserID              uuid.UUID            `json:"user_id"`
	BillingGroupID      uuid.UUID            `json:"billing_group_id"`
	BillingGroup        billinggroup.Summary `json:"billing_group"`
	Name                string               `json:"name"`
	KeyPrefix           string               `json:"key_prefix"`
	CanCopySecret       bool                 `json:"can_copy_secret"`
	Status              Status               `json:"status"`
	LastUsedAt          *time.Time           `json:"last_used_at"`
	CreatedAt           time.Time            `json:"created_at"`
	RevokedAt           *time.Time           `json:"revoked_at"`
	KeySecretCiphertext string               `json:"-"`
}

type Actor struct {
	APIKey              Record
	User                user.Record
	PinnedMultiplierBPS int64
}

type Owner struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
}

type AdminRecord struct {
	Record
	Owner Owner `json:"owner"`
}

type Page struct {
	APIKeys []AdminRecord `json:"api_keys"`
	Total   int           `json:"total"`
	Offset  int           `json:"offset"`
	Limit   int           `json:"limit"`
}

type ListFilter struct {
	Search string
	Status Status
	Offset int
	Limit  int
}
