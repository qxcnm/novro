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
	ID                     uuid.UUID        `json:"id"`
	Code                   string           `json:"code"`
	DisplayName            string           `json:"display_name"`
	MultiplierBPS          int64            `json:"multiplier_bps"`
	DiscountName           string           `json:"discount_name"`
	DiscountMultiplierBPS  int64            `json:"discount_multiplier_bps"`
	DiscountStartsAt       *time.Time       `json:"discount_starts_at"`
	DiscountEndsAt         *time.Time       `json:"discount_ends_at"`
	EffectiveMultiplierBPS int64            `json:"effective_multiplier_bps"`
	DiscountActive         bool             `json:"discount_active"`
	IsDefault              bool             `json:"is_default"`
	IsHidden               bool             `json:"is_hidden"`
	Status                 Status           `json:"status"`
	APIKeyCount            int              `json:"api_key_count"`
	ModelRouteCount        int              `json:"model_route_count"`
	AuthorizedUsers        []AuthorizedUser `json:"authorized_users"`
	CreatedAt              time.Time        `json:"created_at"`
	UpdatedAt              time.Time        `json:"updated_at"`
}

type AuthorizedUser struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
}

type Summary struct {
	ID                     uuid.UUID  `json:"id"`
	Code                   string     `json:"code"`
	DisplayName            string     `json:"display_name"`
	MultiplierBPS          int64      `json:"multiplier_bps"`
	DiscountName           string     `json:"discount_name"`
	DiscountMultiplierBPS  int64      `json:"discount_multiplier_bps"`
	DiscountStartsAt       *time.Time `json:"discount_starts_at"`
	DiscountEndsAt         *time.Time `json:"discount_ends_at"`
	EffectiveMultiplierBPS int64      `json:"effective_multiplier_bps"`
	DiscountActive         bool       `json:"discount_active"`
}

type DiscountInput struct {
	Name          string    `json:"name"`
	MultiplierBPS int64     `json:"multiplier_bps"`
	StartsAt      time.Time `json:"starts_at"`
	EndsAt        time.Time `json:"ends_at"`
}

type CreateInput struct {
	Code              string         `json:"code"`
	DisplayName       string         `json:"display_name"`
	MultiplierBPS     int64          `json:"multiplier_bps"`
	IsHidden          bool           `json:"is_hidden"`
	AuthorizedUserIDs []uuid.UUID    `json:"authorized_user_ids"`
	Discount          *DiscountInput `json:"discount"`
}

type UpdateInput struct {
	DisplayName       *string        `json:"display_name"`
	MultiplierBPS     *int64         `json:"multiplier_bps"`
	IsHidden          *bool          `json:"is_hidden"`
	AuthorizedUserIDs *[]uuid.UUID   `json:"authorized_user_ids"`
	Discount          *DiscountInput `json:"discount"`
	ClearDiscount     bool           `json:"clear_discount"`
}

func (s Summary) MultiplierAt(at time.Time) int64 {
	return multiplierAt(s.MultiplierBPS, s.DiscountMultiplierBPS, s.DiscountStartsAt, s.DiscountEndsAt, at)
}

func (r Record) MultiplierAt(at time.Time) int64 {
	return multiplierAt(r.MultiplierBPS, r.DiscountMultiplierBPS, r.DiscountStartsAt, r.DiscountEndsAt, at)
}

func multiplierAt(base, discount int64, startsAt, endsAt *time.Time, at time.Time) int64 {
	if base < 1 || discount < 1 || discount >= DefaultMultiplierBPS || startsAt == nil || endsAt == nil || at.Before(*startsAt) || !at.Before(*endsAt) {
		return base
	}
	return (base*discount + DefaultMultiplierBPS - 1) / DefaultMultiplierBPS
}

func NewSummary(id uuid.UUID, code, displayName string, multiplierBPS int64, discountName string, discountMultiplierBPS int64, discountStartsAt, discountEndsAt *time.Time) Summary {
	summary := Summary{ID: id, Code: code, DisplayName: displayName, MultiplierBPS: multiplierBPS,
		DiscountName: discountName, DiscountMultiplierBPS: discountMultiplierBPS, DiscountStartsAt: discountStartsAt, DiscountEndsAt: discountEndsAt}
	summary.EffectiveMultiplierBPS = summary.MultiplierAt(time.Now().UTC())
	summary.DiscountActive = summary.EffectiveMultiplierBPS != summary.MultiplierBPS
	return summary
}

type ListFilter struct {
	Search           string
	Status           Status
	IncludeHidden    bool
	AuthorizedUserID uuid.UUID
}
