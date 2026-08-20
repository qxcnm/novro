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
type ValidationField string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"

	ValidationFieldUsername     ValidationField = "username"
	ValidationFieldEmail        ValidationField = "email"
	ValidationFieldDisplayName  ValidationField = "display_name"
	ValidationFieldPassword     ValidationField = "password"
	ValidationFieldRole         ValidationField = "role"
	ValidationFieldReferralCode ValidationField = "referral_code"
)

var (
	ErrInvalidInput        = errors.New("invalid user input")
	ErrNotFound            = errors.New("user not found")
	ErrUsernameTaken       = errors.New("username already exists")
	ErrEmailTaken          = errors.New("email already exists")
	ErrInvalidReferralCode = errors.New("invalid referral code")
	ErrLastActiveAdmin     = errors.New("cannot disable the last active administrator")
	ErrProtectedAdmin      = errors.New("cannot modify the system administrator role or status")
	ErrAlreadyInitialized  = errors.New("administrator already initialized")
)

type ValidationError struct {
	Field ValidationField
}

func (e *ValidationError) Error() string {
	return ErrInvalidInput.Error() + ": " + string(e.Field)
}

func (e *ValidationError) Unwrap() error {
	return ErrInvalidInput
}

func invalidField(field ValidationField) error {
	return &ValidationError{Field: field}
}

type Record struct {
	ID            uuid.UUID  `json:"id"`
	Username      string     `json:"username"`
	Email         string     `json:"email"`
	DisplayName   string     `json:"display_name"`
	Role          Role       `json:"role"`
	Status        Status     `json:"status"`
	IsSystemAdmin bool       `json:"is_system_admin"`
	LastLoginAt   *time.Time `json:"last_login_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type AdminRecord struct {
	Record
	BalanceMicros int64 `json:"balance_micros"`
}

type CreateInput struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Role        Role   `json:"role"`
}

type UpdateInput struct {
	DisplayName *string `json:"display_name"`
	Role        *Role   `json:"role"`
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
	Users  []AdminRecord `json:"users"`
	Total  int           `json:"total"`
	Offset int           `json:"offset"`
	Limit  int           `json:"limit"`
}
