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
	ID              uuid.UUID        `json:"id"`
	Code            string           `json:"code"`
	DisplayName     string           `json:"display_name"`
	MultiplierBPS   int64            `json:"multiplier_bps"`
	IsDefault       bool             `json:"is_default"`
	IsHidden        bool             `json:"is_hidden"`
	Status          Status           `json:"status"`
	APIKeyCount     int              `json:"api_key_count"`
	ProviderCount   int              `json:"provider_count"`
	AuthorizedUsers []AuthorizedUser `json:"authorized_users"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

type AuthorizedUser struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
}

type Summary struct {
	ID            uuid.UUID `json:"id"`
	Code          string    `json:"code"`
	DisplayName   string    `json:"display_name"`
	MultiplierBPS int64     `json:"multiplier_bps"`
}

type CreateInput struct {
	Code              string      `json:"code"`
	DisplayName       string      `json:"display_name"`
	MultiplierBPS     int64       `json:"multiplier_bps"`
	IsHidden          bool        `json:"is_hidden"`
	AuthorizedUserIDs []uuid.UUID `json:"authorized_user_ids"`
}

type UpdateInput struct {
	DisplayName       *string      `json:"display_name"`
	MultiplierBPS     *int64       `json:"multiplier_bps"`
	IsHidden          *bool        `json:"is_hidden"`
	AuthorizedUserIDs *[]uuid.UUID `json:"authorized_user_ids"`
}

type ListFilter struct {
	Search           string
	Status           Status
	IncludeHidden    bool
	AuthorizedUserID uuid.UUID
}
