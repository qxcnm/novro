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
	failure   FailureInput
	err       error
}

func (f *fakeStore) GetSummary(context.Context, uuid.UUID, EntryFilter) (Summary, error) {
	return Summary{}, f.err
}
func (f *fakeStore) ListUsage(context.Context, uuid.UUID, UsageFilter) (UsagePage, error) {
	return UsagePage{}, f.err
}
func (f *fakeStore) Adjust(_ context.Context, _, _ uuid.UUID, amount int64, note string) (Summary, error) {
	f.adjusted, f.note = amount, note
	return Summary{}, f.err
}
func (f *fakeStore) Reserve(_ context.Context, _, _ uuid.UUID, amount int64, _ string) error {
	f.reserved = amount
	return f.err
}
func (f *fakeStore) Refund(context.Context, uuid.UUID, uuid.UUID, int64, string) error { return f.err }
func (f *fakeStore) ReleaseReservation(context.Context, uuid.UUID, uuid.UUID, int64, string) error {
	return f.err
}
func (f *fakeStore) Finalize(_ context.Context, input UsageInput) error {
	f.finalized = input
	return f.err
}
func (f *fakeStore) RecordFailure(_ context.Context, input FailureInput) error {
	f.failure = input
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

func TestRecordFailureRequiresSafeAuditableFields(t *testing.T) {
	store := &fakeStore{}
	userID, keyID, routeID, upstreamID, groupID, requestID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	input := FailureInput{UserID: userID, APIKeyID: keyID, ModelRouteID: routeID, UpstreamModelID: &upstreamID, BillingGroupID: &groupID, RequestID: requestID, Endpoint: "chat_completions", StatusCode: 502, ErrorCode: "upstream_http_error", ErrorMessage: "上游返回 HTTP 502", MultiplierBPS: 4_000, ModelName: "demo", UpstreamModelName: "demo-upstream", BillingGroupCode: "default", BillingGroupName: "默认", DurationMS: 120}
	if err := NewService(store).RecordFailure(context.Background(), input); err != nil {
		t.Fatalf("record failure: %v", err)
	}
	if store.failure.RequestID != requestID || store.failure.StatusCode != 502 || store.failure.ErrorCode != "upstream_http_error" || store.failure.MultiplierBPS != 4_000 {
		t.Fatalf("unexpected failure input: %+v", store.failure)
	}
	input.ErrorMessage = ""
	if err := NewService(store).RecordFailure(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing error message err=%v", err)
	}
}
