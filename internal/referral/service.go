package referral

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Store interface {
	Stats(context.Context, uuid.UUID, int64) (Stats, error)
	RewardConfig(context.Context) (StoredConfig, error)
	SaveRewardBPS(context.Context, int64) (StoredConfig, error)
}

type Service struct {
	store     Store
	rewardBPS int64
	publicURL string
}

func NewService(store Store, rewardBPS int64, publicURL string) *Service {
	return &Service{store: store, rewardBPS: rewardBPS, publicURL: strings.TrimRight(publicURL, "/")}
}

func (s *Service) Summary(ctx context.Context, userID uuid.UUID) (Summary, error) {
	if err := s.validate(); err != nil || userID == uuid.Nil || s.publicURL == "" {
		return Summary{}, ErrInvalidInput
	}
	rewardBPS, _, err := s.effectiveRewardBPS(ctx)
	if err != nil {
		return Summary{}, err
	}
	stats, err := s.store.Stats(ctx, userID, rewardBPS)
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		InviteCode:          stats.InviteCode,
		InviteURL:           fmt.Sprintf("%s/register?ref=%s", s.publicURL, url.QueryEscape(stats.InviteCode)),
		InvitedCount:        stats.InvitedCount,
		PendingRewardMicros: stats.PendingRewardMicros,
		TotalRewardMicros:   stats.TotalRewardMicros,
		RewardBPS:           rewardBPS,
		Invitations:         stats.Invitations,
		Rewards:             stats.Rewards,
	}, nil
}

func (s *Service) AdminConfig(ctx context.Context) (AdminConfig, error) {
	if err := s.validate(); err != nil {
		return AdminConfig{}, err
	}
	rewardBPS, updatedAt, err := s.effectiveRewardBPS(ctx)
	if err != nil {
		return AdminConfig{}, err
	}
	return AdminConfig{RewardBPS: rewardBPS, UpdatedAt: updatedAt}, nil
}

func (s *Service) UpdateRewardBPS(ctx context.Context, rewardBPS int64) (AdminConfig, error) {
	if err := s.validate(); err != nil || !ValidRewardBPS(rewardBPS) {
		return AdminConfig{}, ErrInvalidInput
	}
	stored, err := s.store.SaveRewardBPS(ctx, rewardBPS)
	if err != nil {
		return AdminConfig{}, err
	}
	updatedAt := stored.UpdatedAt
	return AdminConfig{RewardBPS: stored.RewardBPS, UpdatedAt: &updatedAt}, nil
}

func (s *Service) validate() error {
	if s == nil || s.store == nil || !ValidRewardBPS(s.rewardBPS) {
		return ErrInvalidInput
	}
	return nil
}

func (s *Service) effectiveRewardBPS(ctx context.Context) (int64, *time.Time, error) {
	stored, err := s.store.RewardConfig(ctx)
	if err != nil {
		return 0, nil, err
	}
	if !stored.Found {
		return s.rewardBPS, nil, nil
	}
	updatedAt := stored.UpdatedAt
	return stored.RewardBPS, &updatedAt, nil
}
