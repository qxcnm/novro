package apikey

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/user"
)

type Status string

const (
	StatusActive  Status = "active"
	StatusRevoked Status = "revoked"
)

var (
	ErrInvalidInput    = errors.New("invalid API key input")
	ErrNotFound        = errors.New("API key not found")
	ErrLimitReached    = errors.New("active API key limit reached")
	ErrUnauthenticated = errors.New("invalid API key")
)

type Record struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	Status     Status     `json:"status"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

type Actor struct {
	APIKey Record
	User   user.Record
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
