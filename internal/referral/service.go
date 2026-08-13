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
	/**
	 * Stats 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 int64 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Stats(context.Context, uuid.UUID, int64) (Stats, error)
	/**
	 * RewardConfig 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	RewardConfig(context.Context) (StoredConfig, error)
	/**
	 * SaveRewardBPS 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 int64 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	SaveRewardBPS(context.Context, int64) (StoredConfig, error)
}

type Service struct {
	store     Store
	rewardBPS int64
	publicURL string
}

/**
 * NewService 用于创建并返回所需的对象或记录。
 * @param store 用于持久化和查询数据的存储实现。
 * @param rewardBPS 本次操作需要使用的输入参数。
 * @param publicURL 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewService(store Store, rewardBPS int64, publicURL string) *Service {
	return &Service{store: store, rewardBPS: rewardBPS, publicURL: strings.TrimRight(publicURL, "/")}
}

/**
 * Summary 用于计算并返回对应结果。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * AdminConfig 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * UpdateRewardBPS 用于更新指定的数据或状态。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param rewardBPS 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * validate 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) validate() error {
	if s == nil || s.store == nil || !ValidRewardBPS(s.rewardBPS) {
		return ErrInvalidInput
	}
	return nil
}

/**
 * effectiveRewardBPS 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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
