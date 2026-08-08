package user

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

var (
	ErrInvalidInput        = errors.New("invalid user input")
	ErrNotFound            = errors.New("user not found")
	ErrUsernameTaken       = errors.New("username already exists")
	ErrEmailTaken          = errors.New("email already exists")
	ErrInvalidReferralCode = errors.New("invalid referral code")
	ErrLastActiveAdmin     = errors.New("cannot disable the last active administrator")
	ErrAlreadyInitialized  = errors.New("administrator already initialized")
)

type Record struct {
	ID             uuid.UUID            `json:"id"`
	BillingGroupID *uuid.UUID           `json:"billing_group_id"`
	BillingGroup   *BillingGroupSummary `json:"billing_group,omitempty"`
	Username       string               `json:"username"`
	Email          string               `json:"email"`
	DisplayName    string               `json:"display_name"`
	Role           Role                 `json:"role"`
	Status         Status               `json:"status"`
	LastLoginAt    *time.Time           `json:"last_login_at"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

type CreateInput struct {
	Username       string     `json:"username"`
	Email          string     `json:"email"`
	DisplayName    string     `json:"display_name"`
	Password       string     `json:"password"`
	Role           Role       `json:"role"`
	BillingGroupID *uuid.UUID `json:"billing_group_id"`
}

type UpdateInput struct {
	DisplayName    *string    `json:"display_name"`
	Role           *Role      `json:"role"`
	BillingGroupID *uuid.UUID `json:"billing_group_id"`
}

type BillingGroupSummary struct {
	ID            uuid.UUID `json:"id"`
	Code          string    `json:"code"`
	DisplayName   string    `json:"display_name"`
	MultiplierBPS int64     `json:"multiplier_bps"`
}

type RegisterInput struct {
	Username     string `json:"username"`
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	Password     string `json:"password"`
	ReferralCode string `json:"referral_code"`
}

type ListFilter struct {
	Search string
	Status Status
	Offset int
	Limit  int
}

type Page struct {
	Users  []Record `json:"users"`
	Total  int      `json:"total"`
	Offset int      `json:"offset"`
	Limit  int      `json:"limit"`
}
