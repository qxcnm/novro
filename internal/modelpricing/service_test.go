package modelpricing

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/billing"
)

type fakeStore struct {
	created       PlanInput
	modelID       uuid.UUID
	plans         []Plan
	resolution    Resolution
	published     Plan
	listCalls     int
	resolveCalls  int
	listStarted   chan struct{}
	listRelease   chan struct{}
	listStartOnce sync.Once
	mu            sync.Mutex
}

func (f *fakeStore) List(context.Context, uuid.UUID) ([]Plan, error) {
	f.mu.Lock()
	f.listCalls++
	if f.listStarted != nil {
		f.listStartOnce.Do(func() { close(f.listStarted) })
	}
	plans := f.plans
	release := f.listRelease
	f.mu.Unlock()
	if release != nil {
		<-release
	}
	return plans, nil
}
func (f *fakeStore) CreateDraft(_ context.Context, modelID uuid.UUID, input PlanInput) (Plan, error) {
	f.modelID = modelID
	f.created = input
	return Plan{UpstreamModelID: modelID, Mode: input.Mode, Timezone: input.Timezone}, nil
}
func (f *fakeStore) UpdateDraft(context.Context, uuid.UUID, PlanInput) (Plan, error) {
	return Plan{}, nil
}
func (f *fakeStore) Publish(context.Context, uuid.UUID) (Plan, error) { return f.published, nil }
func (f *fakeStore) Republish(context.Context, uuid.UUID, time.Time) (RepublishResult, error) {
	return RepublishResult{Plan: f.published, Created: false}, nil
}
func (f *fakeStore) DeleteDraft(context.Context, uuid.UUID) error { return nil }
func (f *fakeStore) Resolve(context.Context, uuid.UUID, time.Time) (Resolution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolveCalls++
	return f.resolution, nil
}

/**
 * TestResolveSharesPublishedPlanSnapshotAndReplacesItAfterPublish 验证共享快照和发布替换行为。
 * @param t 本次测试需要使用的测试上下文。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func TestResolveSharesPublishedPlanSnapshotAndReplacesItAfterPublish(t *testing.T) {
	modelID := uuid.New()
	planID := uuid.New()
	windowID := uuid.New()
	store := &fakeStore{plans: []Plan{{
		ID: planID, UpstreamModelID: modelID, Version: 1, Status: StatusPublished,
		Mode: ModeScheduled, Timezone: "Asia/Shanghai", EffectiveFrom: time.Unix(0, 0).UTC(),
		DefaultRates: billing.RateCard{InputMicros: 1_000_000, OutputMicros: 2_000_000},
		Windows:      []Window{{ID: windowID, Label: "morning peak", WeekdayMask: 2, StartMinute: 480, EndMinute: 600, Rates: billing.RateCard{InputMicros: 3_000_000, OutputMicros: 6_000_000}}},
	}}}
	service := NewService(store)
	beforePeak := time.Date(2026, time.August, 16, 23, 59, 0, 0, time.UTC)
	atPeak := beforePeak.Add(time.Minute)
	before, err := service.Resolve(context.Background(), modelID, beforePeak)
	if err != nil {
		t.Fatalf("resolve before peak: %v", err)
	}
	peak, err := service.Resolve(context.Background(), modelID, atPeak)
	if err != nil {
		t.Fatalf("resolve at peak: %v", err)
	}
	if before.Rates.InputMicros != 1_000_000 || peak.Rates.InputMicros != 3_000_000 || peak.WindowID == nil || *peak.WindowID != windowID {
		t.Fatalf("unexpected scheduled resolutions before=%+v peak=%+v", before, peak)
	}
	store.mu.Lock()
	listCalls := store.listCalls
	store.mu.Unlock()
	if listCalls != 1 {
		t.Fatalf("published plans were loaded more than once: calls=%d", listCalls)
	}

	store.published = Plan{UpstreamModelID: modelID}
	if _, err := service.Publish(context.Background(), uuid.New()); err != nil {
		t.Fatalf("publish replacement plan: %v", err)
	}
	if _, err := service.Resolve(context.Background(), modelID, atPeak); err != nil {
		t.Fatalf("resolve after replacement: %v", err)
	}
	store.mu.Lock()
	listCalls = store.listCalls
	store.mu.Unlock()
	if listCalls != 2 {
		t.Fatalf("publish did not replace cached plan snapshot: calls=%d", listCalls)
	}
}

/**
 * TestResolveLoadsOneSharedSnapshotForConcurrentRequests 验证并发请求只加载一次价格快照。
 * @param t 本次测试需要使用的测试上下文。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func TestResolveLoadsOneSharedSnapshotForConcurrentRequests(t *testing.T) {
	modelID := uuid.New()
	store := &fakeStore{
		plans: []Plan{{
			ID: uuid.New(), UpstreamModelID: modelID, Version: 1, Status: StatusPublished,
			Mode: ModeFixed, Timezone: "UTC", EffectiveFrom: time.Unix(0, 0).UTC(),
			DefaultRates: billing.RateCard{InputMicros: 1_000_000, OutputMicros: 2_000_000},
		}},
		listStarted: make(chan struct{}),
		listRelease: make(chan struct{}),
	}
	service := NewService(store)
	results := make(chan error, 64)
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		_, err := service.Resolve(context.Background(), modelID, time.Now().UTC())
		results <- err
	}()
	<-store.listStarted
	for index := 0; index < 63; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := service.Resolve(context.Background(), modelID, time.Now().UTC())
			results <- err
		}()
	}
	close(store.listRelease)
	group.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent resolve: %v", err)
		}
	}
	store.mu.Lock()
	listCalls := store.listCalls
	store.mu.Unlock()
	if listCalls != 1 {
		t.Fatalf("concurrent requests loaded pricing more than once: calls=%d", listCalls)
	}
}

func TestCreateDraftAcceptsValidPeakValleySchedule(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	modelID := uuid.New()
	from := time.Date(2026, time.August, 17, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	input := PlanInput{
		Mode: ModeScheduled, Timezone: "Asia/Shanghai", EffectiveFrom: from,
		DefaultRates: billing.RateCard{InputMicros: 1_500_000, CacheReadMicros: 50_000, OutputMicros: 4_500_000},
		Windows: []WindowInput{
			{Label: "上午高峰", WeekdayMask: 127, StartMinute: 9 * 60, EndMinute: 12 * 60, Rates: billing.RateCard{InputMicros: 3_000_000, CacheReadMicros: 100_000, OutputMicros: 9_000_000}},
			{Label: "下午高峰", WeekdayMask: 127, StartMinute: 14 * 60, EndMinute: 18 * 60, Rates: billing.RateCard{InputMicros: 3_000_000, CacheReadMicros: 100_000, OutputMicros: 9_000_000}},
		},
	}
	if _, err := service.CreateDraft(context.Background(), modelID, input); err != nil {
		t.Fatalf("create peak-valley draft: %v", err)
	}
	if store.modelID != modelID || !store.created.EffectiveFrom.Equal(from.UTC()) || len(store.created.Windows) != 2 {
		t.Fatalf("unexpected normalized input: %+v", store.created)
	}
}

func TestCreateDraftRejectsOverlappingWindowsOnSharedWeekday(t *testing.T) {
	service := NewService(&fakeStore{})
	input := PlanInput{
		Mode: ModeScheduled, Timezone: "Asia/Shanghai", EffectiveFrom: time.Now(),
		Windows: []WindowInput{
			{Label: "first", WeekdayMask: 2, StartMinute: 540, EndMinute: 720},
			{Label: "second", WeekdayMask: 2, StartMinute: 660, EndMinute: 780},
		},
	}
	if _, err := service.CreateDraft(context.Background(), uuid.New(), input); err != ErrInvalidInput {
		t.Fatalf("expected overlapping window rejection, got %v", err)
	}
	input.Windows[1].WeekdayMask = 4
	if _, err := service.CreateDraft(context.Background(), uuid.New(), input); err != nil {
		t.Fatalf("different weekdays should be valid: %v", err)
	}
}

func TestCreateDraftRejectsInvalidModeAndTimezone(t *testing.T) {
	service := NewService(&fakeStore{})
	base := PlanInput{Mode: ModeFixed, Timezone: "not/a-zone", EffectiveFrom: time.Now()}
	if _, err := service.CreateDraft(context.Background(), uuid.New(), base); err != ErrInvalidInput {
		t.Fatalf("expected invalid timezone rejection, got %v", err)
	}
	base.Timezone = "UTC"
	base.Mode = Mode("rule-script")
	if _, err := service.CreateDraft(context.Background(), uuid.New(), base); err != ErrInvalidInput {
		t.Fatalf("expected invalid mode rejection, got %v", err)
	}
}
