package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeStore struct {
	adjusted  int64
	reference uuid.UUID
	note      string
	reserved  int64
	finalized UsageInput
	pendingIn UsageInput
	failure   FailureInput
	rate      UsageRate
	rateSince time.Time
	pending   []PendingSettlement
	completed int
	err       error
}

func (f *fakeStore) GetSummary(context.Context, uuid.UUID, EntryFilter) (Summary, error) {
	return Summary{}, f.err
}
func (f *fakeStore) ListUsage(context.Context, uuid.UUID, UsageFilter) (UsagePage, error) {
	return UsagePage{}, f.err
}
func (f *fakeStore) GetUsageRate(_ context.Context, _ uuid.UUID, since time.Time) (UsageRate, error) {
	f.rateSince = since
	return f.rate, f.err
}
func (f *fakeStore) Adjust(_ context.Context, _, _, reference uuid.UUID, amount int64, note string) (Summary, error) {
	f.adjusted, f.note, f.reference = amount, note, reference
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
func (f *fakeStore) StartOperation(_ context.Context, input OperationStartInput) (OperationStartResult, error) {
	return OperationStartResult{Created: true, Operation: Operation{RequestID: input.RequestID}}, f.err
}
func (f *fakeStore) MarkOperationPendingSettlement(_ context.Context, _ uuid.UUID, input UsageInput) error {
	f.pendingIn = input
	return f.err
}
func (f *fakeStore) MarkOperationPendingUnknown(context.Context, uuid.UUID, string) error {
	return f.err
}
func (f *fakeStore) CompleteOperation(context.Context, uuid.UUID) error {
	f.completed++
	return f.err
}
func (f *fakeStore) FailOperation(context.Context, uuid.UUID, string) error { return f.err }
func (f *fakeStore) ListPendingSettlements(context.Context, int) ([]PendingSettlement, error) {
	return f.pending, f.err
}
func (f *fakeStore) CompensateLegacyUsage(context.Context, uuid.UUID, uuid.UUID) (Summary, int64, error) {
	return Summary{}, 0, f.err
}

func TestAdjustmentRequiresAuditableNonzeroAmount(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	userID, actorID := uuid.New(), uuid.New()
	referenceID := uuid.New()
	if _, err := service.Adjust(context.Background(), userID, actorID, referenceID, 5_000_000, "  初始额度  "); err != nil {
		t.Fatalf("adjust: %v", err)
	}
	if store.adjusted != 5_000_000 || store.note != "初始额度" || store.reference != referenceID {
		t.Fatalf("unexpected adjustment: %+v", store)
	}
	for _, input := range []struct {
		amount int64
		note   string
	}{{0, "note"}, {1, ""}, {1_000_000_000_000_001, "note"}} {
		if _, err := service.Adjust(context.Background(), userID, actorID, referenceID, input.amount, input.note); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("input=%+v err=%v", input, err)
		}
	}
}

func TestFinalizeAllowsActualCostAboveReservation(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	userID, keyID, routeID, upstreamID, groupID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	input := UsageInput{
		UserID: userID, APIKeyID: keyID, ModelRouteID: routeID, UpstreamModelID: &upstreamID, BillingGroupID: &groupID, RequestID: uuid.New(),
		Endpoint: "chat_completions", InputTokens: 1, Tokens: TokenBreakdown{UncachedInput: 1}, Rates: RateCard{InputMicros: 11 * PriceUnitTokens},
		BaseCostMicros: 11, MultiplierBPS: 10_000, CostMicros: 11, ReservedMicros: 10, CalculationVersion: CalculationVersion,
	}
	if err := service.Finalize(context.Background(), input); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if store.finalized.CostMicros != 11 || store.finalized.ReservedMicros != 10 || store.finalized.StatusCode != 200 {
		t.Fatalf("unexpected finalized input: %+v", store.finalized)
	}
}

func TestMarkPendingSettlementPersistsNormalizedStatusCode(t *testing.T) {
	store := &fakeStore{}
	userID, keyID, routeID, upstreamID, groupID, requestID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	input := UsageInput{UserID: userID, APIKeyID: keyID, ModelRouteID: routeID, UpstreamModelID: &upstreamID, BillingGroupID: &groupID, RequestID: requestID, Endpoint: "responses", InputTokens: 1, Tokens: TokenBreakdown{UncachedInput: 1}, Rates: RateCard{InputMicros: PriceUnitTokens}, BaseCostMicros: 1, MultiplierBPS: 10_000, CostMicros: 1, ReservedMicros: 1, CalculationVersion: CalculationVersion}
	if err := NewService(store).MarkOperationPendingSettlement(context.Background(), requestID, input); err != nil {
		t.Fatalf("mark pending settlement: %v", err)
	}
	if store.pendingIn.StatusCode != 200 {
		t.Fatalf("pending status=%d want=200", store.pendingIn.StatusCode)
	}
}

func TestRecoverPendingSettlementFinalizesAndCompletes(t *testing.T) {
	userID, keyID, routeID, upstreamID, groupID, requestID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	usage := UsageInput{UserID: userID, APIKeyID: keyID, ModelRouteID: routeID, UpstreamModelID: &upstreamID, BillingGroupID: &groupID, RequestID: requestID, Endpoint: "chat_completions", InputTokens: 1, Tokens: TokenBreakdown{UncachedInput: 1}, Rates: RateCard{InputMicros: PriceUnitTokens}, BaseCostMicros: 1, MultiplierBPS: 10_000, CostMicros: 1, ReservedMicros: 1, CalculationVersion: CalculationVersion}
	store := &fakeStore{pending: []PendingSettlement{{Operation: Operation{RequestID: requestID}, Usage: usage}}}
	recovered, err := NewService(store).RecoverPendingSettlements(context.Background(), 100)
	if err != nil || recovered != 1 || store.finalized.RequestID != requestID || store.finalized.StatusCode != 200 || store.completed != 1 {
		t.Fatalf("recovered=%d err=%v store=%+v", recovered, err, store)
	}
}

func TestFinalizeRejectsPaidUsageWithoutReservation(t *testing.T) {
	userID, keyID, routeID, upstreamID, groupID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	input := UsageInput{UserID: userID, APIKeyID: keyID, ModelRouteID: routeID, UpstreamModelID: &upstreamID, BillingGroupID: &groupID, RequestID: uuid.New(), Endpoint: "responses", StatusCode: 200, InputTokens: 1, Tokens: TokenBreakdown{UncachedInput: 1}, Rates: RateCard{InputMicros: PriceUnitTokens}, BaseCostMicros: 1, MultiplierBPS: 10_000, CostMicros: 1, CalculationVersion: CalculationVersion}
	if err := NewService(&fakeStore{}).Finalize(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}
}

func TestFinalizeRejectsFailureStatus(t *testing.T) {
	userID, keyID, routeID, upstreamID, groupID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	input := UsageInput{UserID: userID, APIKeyID: keyID, ModelRouteID: routeID, UpstreamModelID: &upstreamID, BillingGroupID: &groupID, RequestID: uuid.New(), Endpoint: "responses", StatusCode: 500, MultiplierBPS: 10_000, ReservedMicros: 1, CalculationVersion: CalculationVersion}
	if err := NewService(&fakeStore{}).Finalize(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
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

func TestUsageRateUsesRollingMinuteAndCalculatesTotals(t *testing.T) {
	calculatedAt := time.Date(2026, time.August, 12, 15, 2, 3, 0, time.FixedZone("CST", 8*60*60))
	store := &fakeStore{rate: UsageRate{Requests: 7, InputTokens: 1200, OutputTokens: 345}}
	service := NewService(store)
	service.now = func() time.Time { return calculatedAt }

	rate, err := service.UsageRate(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("usage rate: %v", err)
	}
	if !store.rateSince.Equal(calculatedAt.UTC().Add(-time.Minute)) {
		t.Fatalf("since=%s", store.rateSince)
	}
	if rate.WindowSeconds != 60 || rate.Requests != 7 || rate.RPM != 7 || rate.TotalTokens != 1545 || rate.TPM != 1545 || !rate.CalculatedAt.Equal(calculatedAt.UTC()) {
		t.Fatalf("unexpected rate: %+v", rate)
	}
}

func TestUsageRateRejectsMissingUser(t *testing.T) {
	if _, err := NewService(&fakeStore{}).UsageRate(context.Background(), uuid.Nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
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
