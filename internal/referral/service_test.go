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

func (f *fakeStore) Stats(_ context.Context, _ uuid.UUID, rewardBPS int64) (Stats, error) {
	f.rate = rewardBPS
	return f.stats, f.err
}

func (f *fakeStore) RewardConfig(context.Context) (StoredConfig, error) {
	return f.config, f.configError
}

func (f *fakeStore) SaveRewardBPS(_ context.Context, rewardBPS int64) (StoredConfig, error) {
	f.savedRate = rewardBPS
	return StoredConfig{RewardBPS: rewardBPS, UpdatedAt: time.Unix(1_700_000_000, 0), Found: true}, f.configError
}

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

func TestRewardAmountRejectsOverflow(t *testing.T) {
	if reward, ok := rewardAmount(10_000_000, 1_000); !ok || reward != 1_000_000 {
		t.Fatalf("reward=%d ok=%v", reward, ok)
	}
	if _, ok := rewardAmount(-1, 1_000); ok {
		t.Fatal("negative amount was accepted")
	}
}
