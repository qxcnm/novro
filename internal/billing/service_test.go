package billing

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakeStore struct {
	adjusted  int64
	note      string
	reserved  int64
	finalized UsageInput
	err       error
}

func (f *fakeStore) GetSummary(context.Context, uuid.UUID, int) (Summary, error) {
	return Summary{}, f.err
}
func (f *fakeStore) ListUsage(context.Context, uuid.UUID, int) ([]Usage, error) { return nil, f.err }
func (f *fakeStore) Adjust(_ context.Context, _, _ uuid.UUID, amount int64, note string) (Summary, error) {
	f.adjusted, f.note = amount, note
	return Summary{}, f.err
}
func (f *fakeStore) Reserve(_ context.Context, _, _ uuid.UUID, amount int64, _ string) error {
	f.reserved = amount
	return f.err
}
func (f *fakeStore) Refund(context.Context, uuid.UUID, uuid.UUID, int64, string) error { return f.err }
func (f *fakeStore) Finalize(_ context.Context, input UsageInput) error {
	f.finalized = input
	return f.err
}

func TestAdjustmentRequiresAuditableNonzeroAmount(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	userID, actorID := uuid.New(), uuid.New()
	if _, err := service.Adjust(context.Background(), userID, actorID, 5_000_000, "  初始额度  "); err != nil {
		t.Fatalf("adjust: %v", err)
	}
	if store.adjusted != 5_000_000 || store.note != "初始额度" {
		t.Fatalf("unexpected adjustment: %+v", store)
	}
	for _, input := range []struct {
		amount int64
		note   string
	}{{0, "note"}, {1, ""}, {1_000_000_000_000_001, "note"}} {
		if _, err := service.Adjust(context.Background(), userID, actorID, input.amount, input.note); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("input=%+v err=%v", input, err)
		}
	}
}

func TestFinalizeRequiresReservationToCoverCost(t *testing.T) {
	service := NewService(&fakeStore{})
	input := UsageInput{UserID: uuid.New(), APIKeyID: uuid.New(), ModelRouteID: uuid.New(), RequestID: uuid.New(), CostMicros: 11, ReservedMicros: 10}
	if err := service.Finalize(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}
}

func TestReservePreservesInsufficientBalance(t *testing.T) {
	store := &fakeStore{err: ErrInsufficientBalance}
	err := NewService(store).Reserve(context.Background(), uuid.New(), uuid.New(), 100, "request")
	if !errors.Is(err, ErrInsufficientBalance) || store.reserved != 100 {
		t.Fatalf("reserved=%d err=%v", store.reserved, err)
	}
}
