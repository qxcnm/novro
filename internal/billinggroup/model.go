package billinggroup

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive         Status = "active"
	StatusDisabled       Status = "disabled"
	DefaultCode                 = "default"
	DefaultMultiplierBPS        = int64(10_000)
)

var (
	ErrInvalidInput = errors.New("invalid billing group input")
	ErrNotFound     = errors.New("billing group not found")
	ErrCodeTaken    = errors.New("billing group code already exists")
	ErrProtected    = errors.New("default billing group cannot be disabled")
	ErrInUse        = errors.New("billing group is in use")
)

type Record struct {
	ID            uuid.UUID `json:"id"`
	Code          string    `json:"code"`
	DisplayName   string    `json:"display_name"`
	MultiplierBPS int64     `json:"multiplier_bps"`
	IsDefault     bool      `json:"is_default"`
	Status        Status    `json:"status"`
	UserCount     int       `json:"user_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateInput struct {
	Code          string `json:"code"`
	DisplayName   string `json:"display_name"`
	MultiplierBPS int64  `json:"multiplier_bps"`
}

type UpdateInput struct {
	DisplayName   *string `json:"display_name"`
	MultiplierBPS *int64  `json:"multiplier_bps"`
}

type ListFilter struct {
	Search string
	Status Status
}
