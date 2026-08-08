package referral

import (
	"errors"
	"time"
)

const (
	DefaultRewardBPS    int64  = 1_000
	RewardBPSSettingKey string = "referral_reward_bps"
)

var (
	ErrInvalidInput = errors.New("invalid referral input")
	ErrNotFound     = errors.New("referral owner not found")
)

type Stats struct {
	InviteCode          string
	InvitedCount        int
	PendingRewardMicros int64
	TotalRewardMicros   int64
	Invitations         []Invitation
	Rewards             []Reward
}

type Summary struct {
	InviteCode          string       `json:"invite_code"`
	InviteURL           string       `json:"invite_url"`
	InvitedCount        int          `json:"invited_count"`
	PendingRewardMicros int64        `json:"pending_reward_micros"`
	TotalRewardMicros   int64        `json:"total_reward_micros"`
	RewardBPS           int64        `json:"reward_bps"`
	Invitations         []Invitation `json:"invitations"`
	Rewards             []Reward     `json:"rewards"`
}

type AdminConfig struct {
	RewardBPS int64      `json:"reward_bps"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type StoredConfig struct {
	RewardBPS int64
	UpdatedAt time.Time
	Found     bool
}

type Invitation struct {
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	JoinedAt    time.Time `json:"joined_at"`
}

type Reward struct {
	Username         string    `json:"username"`
	DisplayName      string    `json:"display_name"`
	PaidAmountMicros int64     `json:"paid_amount_micros"`
	RewardMicros     int64     `json:"reward_micros"`
	CreditedAt       time.Time `json:"credited_at"`
}

func ValidRewardBPS(value int64) bool {
	return value >= 0 && value <= 10_000
}
