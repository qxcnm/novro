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
	ErrInvalidInput       = errors.New("invalid user input")
	ErrNotFound           = errors.New("user not found")
	ErrUsernameTaken      = errors.New("username already exists")
	ErrLastActiveAdmin    = errors.New("cannot disable the last active administrator")
	ErrAlreadyInitialized = errors.New("administrator already initialized")
)

type Record struct {
	ID          uuid.UUID  `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Role        Role       `json:"role"`
	Status      Status     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type CreateInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Role        Role   `json:"role"`
}

type UpdateInput struct {
	DisplayName *string `json:"display_name"`
	Role        *Role   `json:"role"`
}

type RegisterInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
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
