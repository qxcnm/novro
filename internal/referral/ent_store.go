package referral

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entsystemsetting "github.com/novro-gateway/novro/ent/systemsetting"
	enttopuporder "github.com/novro-gateway/novro/ent/topuporder"
	entuser "github.com/novro-gateway/novro/ent/user"
	entwallet "github.com/novro-gateway/novro/ent/wallet"
	entwalletentry "github.com/novro-gateway/novro/ent/walletentry"
)

type EntStore struct {
	client *ent.Client
}

const recentDetailLimit = 20

func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) RewardConfig(ctx context.Context) (StoredConfig, error) {
	entity, err := s.client.SystemSetting.Query().Where(entsystemsetting.IDEQ(RewardBPSSettingKey)).Only(ctx)
	if ent.IsNotFound(err) {
		return StoredConfig{}, nil
	}
	if err != nil {
		return StoredConfig{}, fmt.Errorf("read referral reward configuration: %w", err)
	}
	rewardBPS, err := strconv.ParseInt(strings.TrimSpace(entity.Value), 10, 64)
	if err != nil || !ValidRewardBPS(rewardBPS) {
		return StoredConfig{}, fmt.Errorf("read referral reward configuration: invalid stored rate")
	}
	return StoredConfig{RewardBPS: rewardBPS, UpdatedAt: entity.UpdatedAt, Found: true}, nil
}

func (s *EntStore) SaveRewardBPS(ctx context.Context, rewardBPS int64) (StoredConfig, error) {
	if !ValidRewardBPS(rewardBPS) {
		return StoredConfig{}, ErrInvalidInput
	}
	value := strconv.FormatInt(rewardBPS, 10)
	entity, err := s.client.SystemSetting.Query().Where(entsystemsetting.IDEQ(RewardBPSSettingKey)).Only(ctx)
	if ent.IsNotFound(err) {
		entity, err = s.client.SystemSetting.Create().SetID(RewardBPSSettingKey).SetValue(value).Save(ctx)
	} else if err == nil {
		entity, err = s.client.SystemSetting.UpdateOne(entity).SetValue(value).Save(ctx)
	}
	if err != nil {
		return StoredConfig{}, fmt.Errorf("save referral reward configuration: %w", err)
	}
	return StoredConfig{RewardBPS: rewardBPS, UpdatedAt: entity.UpdatedAt, Found: true}, nil
}

func (s *EntStore) Stats(ctx context.Context, userID uuid.UUID, rewardBPS int64) (Stats, error) {
	owner, err := s.client.User.Query().Where(entuser.IDEQ(userID)).Only(ctx)
	if ent.IsNotFound(err) {
		return Stats{}, ErrNotFound
	}
	if err != nil {
		return Stats{}, fmt.Errorf("read referral owner: %w", err)
	}

	invitedCount, err := s.client.User.Query().Where(entuser.ReferredByUserIDEQ(userID)).Count(ctx)
	if err != nil {
		return Stats{}, fmt.Errorf("count referred users: %w", err)
	}

	pendingBase, err := sumTopUpAmounts(ctx, s.client.TopUpOrder.Query().Where(
		enttopuporder.StatusEQ(enttopuporder.StatusPending),
		enttopuporder.HasUserWith(entuser.ReferredByUserIDEQ(userID)),
	))
	if err != nil {
		return Stats{}, fmt.Errorf("sum pending referral top-ups: %w", err)
	}
	pendingReward, ok := rewardAmount(pendingBase, rewardBPS)
	if !ok {
		return Stats{}, fmt.Errorf("calculate pending referral reward: amount overflow")
	}

	totalReward, err := sumWalletEntries(ctx, s.client.WalletEntry.Query().Where(
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeReferralReward),
		entwalletentry.HasWalletWith(entwallet.UserIDEQ(userID)),
	))
	if err != nil {
		return Stats{}, fmt.Errorf("sum credited referral rewards: %w", err)
	}

	invitations, err := s.recentInvitations(ctx, userID)
	if err != nil {
		return Stats{}, err
	}
	rewards, err := s.recentRewards(ctx, userID)
	if err != nil {
		return Stats{}, err
	}

	return Stats{
		InviteCode: owner.InviteCode, InvitedCount: invitedCount,
		PendingRewardMicros: pendingReward, TotalRewardMicros: totalReward,
		Invitations: invitations, Rewards: rewards,
	}, nil
}

func (s *EntStore) recentInvitations(ctx context.Context, userID uuid.UUID) ([]Invitation, error) {
	entities, err := s.client.User.Query().
		Where(entuser.ReferredByUserIDEQ(userID)).
		Order(ent.Desc(entuser.FieldCreatedAt)).
		Limit(recentDetailLimit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list recent referral invitations: %w", err)
	}
	invitations := make([]Invitation, 0, len(entities))
	for _, entity := range entities {
		invitations = append(invitations, Invitation{
			Username: entity.Username, DisplayName: referralDisplayName(entity.DisplayName, entity.Username), JoinedAt: entity.CreatedAt,
		})
	}
	return invitations, nil
}

func (s *EntStore) recentRewards(ctx context.Context, userID uuid.UUID) ([]Reward, error) {
	entries, err := s.client.WalletEntry.Query().Where(
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeReferralReward),
		entwalletentry.HasWalletWith(entwallet.UserIDEQ(userID)),
	).Order(ent.Desc(entwalletentry.FieldCreatedAt)).Limit(recentDetailLimit).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list recent referral rewards: %w", err)
	}
	if len(entries) == 0 {
		return []Reward{}, nil
	}

	orderIDs := make([]uuid.UUID, 0, len(entries))
	for _, entry := range entries {
		orderIDs = append(orderIDs, entry.ReferenceID)
	}
	orders, err := s.client.TopUpOrder.Query().
		Where(enttopuporder.IDIn(orderIDs...)).
		WithUser().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("read referral reward top-ups: %w", err)
	}
	orderByID := make(map[uuid.UUID]*ent.TopUpOrder, len(orders))
	for _, order := range orders {
		orderByID[order.ID] = order
	}

	rewards := make([]Reward, 0, len(entries))
	for _, entry := range entries {
		order := orderByID[entry.ReferenceID]
		if order == nil {
			continue
		}
		invitee, err := order.Edges.UserOrErr()
		if err != nil {
			return nil, fmt.Errorf("read referral reward invitee: %w", err)
		}
		rewards = append(rewards, Reward{
			Username: invitee.Username, DisplayName: referralDisplayName(invitee.DisplayName, invitee.Username),
			PaidAmountMicros: order.AmountMicros, RewardMicros: entry.AmountMicros, CreditedAt: entry.CreatedAt,
		})
	}
	return rewards, nil
}

func referralDisplayName(displayName, username string) string {
	if trimmed := strings.TrimSpace(displayName); trimmed != "" {
		return trimmed
	}
	return username
}

func sumTopUpAmounts(ctx context.Context, query *ent.TopUpOrderQuery) (int64, error) {
	var rows []struct {
		Total sql.NullInt64 `json:"total"`
	}
	if err := query.Aggregate(ent.As(ent.Sum(enttopuporder.FieldAmountMicros), "total")).Scan(ctx, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 || !rows[0].Total.Valid {
		return 0, nil
	}
	return rows[0].Total.Int64, nil
}

func sumWalletEntries(ctx context.Context, query *ent.WalletEntryQuery) (int64, error) {
	var rows []struct {
		Total sql.NullInt64 `json:"total"`
	}
	if err := query.Aggregate(ent.As(ent.Sum(entwalletentry.FieldAmountMicros), "total")).Scan(ctx, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 || !rows[0].Total.Valid {
		return 0, nil
	}
	return rows[0].Total.Int64, nil
}

func rewardAmount(amountMicros, rewardBPS int64) (int64, bool) {
	if amountMicros < 0 || rewardBPS < 0 || rewardBPS > 10_000 || (rewardBPS != 0 && amountMicros > math.MaxInt64/rewardBPS) {
		return 0, false
	}
	return amountMicros * rewardBPS / 10_000, true
}
