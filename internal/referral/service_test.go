package referral

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeStore struct {
	stats       Stats
	config      StoredConfig
	err         error
	rate        int64
	savedRate   int64
	configError error
}

/**
 * Stats 封装该名称对应的业务处理逻辑。
 * @param rewardBPS 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Stats(_ context.Context, _ uuid.UUID, rewardBPS int64) (Stats, error) {
	f.rate = rewardBPS
	return f.stats, f.err
}

/**
 * RewardConfig 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) RewardConfig(context.Context) (StoredConfig, error) {
	return f.config, f.configError
}

/**
 * SaveRewardBPS 封装该名称对应的业务处理逻辑。
 * @param rewardBPS 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) SaveRewardBPS(_ context.Context, rewardBPS int64) (StoredConfig, error) {
	f.savedRate = rewardBPS
	return StoredConfig{RewardBPS: rewardBPS, UpdatedAt: time.Unix(1_700_000_000, 0), Found: true}, f.configError
}

/**
 * TestSummaryBuildsPublicInviteLink 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestSummaryBuildsPublicInviteLink(t *testing.T) {
	store := &fakeStore{stats: Stats{
		InviteCode: "ABCD1234EF56", InvitedCount: 3,
		PendingRewardMicros: 1_000_000, TotalRewardMicros: 2_000_000,
		Invitations: []Invitation{{Username: "member.one", DisplayName: "Member One"}},
		Rewards:     []Reward{{Username: "member.one", RewardMicros: 750_000}},
	}}
	service := NewService(store, 750, "https://app.example.invalid/")
	summary, err := service.Summary(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.InviteURL != "https://app.example.invalid/register?ref=ABCD1234EF56" || summary.RewardBPS != 750 || store.rate != 750 {
		t.Fatalf("unexpected referral summary: %+v rate=%d", summary, store.rate)
	}
	if len(summary.Invitations) != 1 || summary.Invitations[0].Username != "member.one" || len(summary.Rewards) != 1 || summary.Rewards[0].RewardMicros != 750_000 {
		t.Fatalf("unexpected referral details: %+v", summary)
	}
}

/**
 * TestSummaryUsesDatabaseRewardRate 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestSummaryUsesDatabaseRewardRate(t *testing.T) {
	store := &fakeStore{stats: Stats{InviteCode: "ABCD1234EF56"}, config: StoredConfig{RewardBPS: 500, Found: true}}
	service := NewService(store, 750, "https://app.example.invalid")
	summary, err := service.Summary(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.RewardBPS != 500 || store.rate != 500 {
		t.Fatalf("database reward rate was not applied: summary=%+v rate=%d", summary, store.rate)
	}
}

/**
 * TestAdminConfigDefaultsAndUpdates 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestAdminConfigDefaultsAndUpdates(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, 750, "https://app.example.invalid")
	config, err := service.AdminConfig(context.Background())
	if err != nil || config.RewardBPS != 750 || config.UpdatedAt != nil {
		t.Fatalf("unexpected default config: config=%+v err=%v", config, err)
	}
	config, err = service.UpdateRewardBPS(context.Background(), 625)
	if err != nil || config.RewardBPS != 625 || config.UpdatedAt == nil || store.savedRate != 625 {
		t.Fatalf("unexpected saved config: config=%+v rate=%d err=%v", config, store.savedRate, err)
	}
	for _, invalid := range []int64{-1, 10_001} {
		if _, err := service.UpdateRewardBPS(context.Background(), invalid); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("rate %d was not rejected: %v", invalid, err)
		}
	}
}

/**
 * TestSummaryValidatesDependenciesAndRate 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestSummaryValidatesDependenciesAndRate(t *testing.T) {
	for _, service := range []*Service{
		NewService(nil, 1_000, "https://app.example.invalid"),
		NewService(&fakeStore{}, -1, "https://app.example.invalid"),
		NewService(&fakeStore{}, 10_001, "https://app.example.invalid"),
		NewService(&fakeStore{}, 1_000, ""),
	} {
		if _, err := service.Summary(context.Background(), uuid.New()); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input, got %v", err)
		}
	}
}

/**
 * TestRewardAmountRejectsOverflow 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestRewardAmountRejectsOverflow(t *testing.T) {
	if reward, ok := rewardAmount(10_000_000, 1_000); !ok || reward != 1_000_000 {
		t.Fatalf("reward=%d ok=%v", reward, ok)
	}
	if _, ok := rewardAmount(-1, 1_000); ok {
		t.Fatal("negative amount was accepted")
	}
}
