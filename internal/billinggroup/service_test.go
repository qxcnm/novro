package billinggroup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeStore struct {
	deletedID   uuid.UUID
	createInput CreateInput
	updateInput UpdateInput
	created     Record
	updated     Record
	status      Record
}

/**
 * Create 用于创建并返回所需的对象或记录。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Create(_ context.Context, input CreateInput) (Record, error) {
	f.createInput = input
	return f.created, nil
}

/**
 * List 用于筛选并返回数据列表。
 * @param ListFilter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (*fakeStore) List(context.Context, ListFilter) ([]Record, error) { return nil, nil }

/**
 * Update 用于更新指定的数据或状态。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Update(_ context.Context, _ uuid.UUID, input UpdateInput) (Record, error) {
	f.updateInput = input
	return f.updated, nil
}

/**
 * SetStatus 用于更新指定的数据或状态。
 * @param Status 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) SetStatus(context.Context, uuid.UUID, Status) (Record, error) {
	return f.status, nil
}

/**
 * TestCreateValidatesPerGroupAuthorizations 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestCreateValidatesPerGroupAuthorizations(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	userID := uuid.New()
	valid := CreateInput{Code: "partner", DisplayName: "代理端", MultiplierBPS: 5_000, IsHidden: true, AuthorizedUserIDs: []uuid.UUID{userID}}
	if _, err := service.Create(context.Background(), valid); err != nil {
		t.Fatalf("create hidden group: %v", err)
	}
	if len(store.createInput.AuthorizedUserIDs) != 1 || store.createInput.AuthorizedUserIDs[0] != userID {
		t.Fatalf("unexpected authorized users: %+v", store.createInput.AuthorizedUserIDs)
	}
	public := valid
	public.Code = "public"
	public.IsHidden = false
	if _, err := service.Create(context.Background(), public); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected public group authorization rejection, got %v", err)
	}
	duplicate := valid
	duplicate.Code = "duplicate"
	duplicate.AuthorizedUserIDs = []uuid.UUID{userID, userID}
	if _, err := service.Create(context.Background(), duplicate); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected duplicate authorization rejection, got %v", err)
	}
}

/**
 * TestUpdateRejectsInvalidAuthorizedUserIDs 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestUpdateRejectsInvalidAuthorizedUserIDs(t *testing.T) {
	service := NewService(&fakeStore{})
	ids := []uuid.UUID{uuid.Nil}
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{AuthorizedUserIDs: &ids}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected nil authorization ID rejection, got %v", err)
	}
}

func TestDiscountValidationAndScheduledMultiplier(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	start := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	end := start.Add(7 * 24 * time.Hour)
	discount := &DiscountInput{Name: " 国庆优惠 ", MultiplierBPS: 8_800, StartsAt: start, EndsAt: end}
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{Discount: discount}); err != nil {
		t.Fatalf("schedule discount: %v", err)
	}
	if store.updateInput.Discount == nil || store.updateInput.Discount.Name != "国庆优惠" || store.updateInput.Discount.StartsAt.Location() != time.UTC {
		t.Fatalf("discount was not normalized: %+v", store.updateInput.Discount)
	}
	summary := Summary{MultiplierBPS: 5_000, DiscountMultiplierBPS: 8_800, DiscountStartsAt: &store.updateInput.Discount.StartsAt, DiscountEndsAt: &store.updateInput.Discount.EndsAt}
	if got := summary.MultiplierAt(store.updateInput.Discount.StartsAt); got != 4_400 {
		t.Fatalf("active multiplier=%d", got)
	}
	if got := summary.MultiplierAt(store.updateInput.Discount.EndsAt); got != 5_000 {
		t.Fatalf("ended multiplier=%d", got)
	}
	rounded := Summary{MultiplierBPS: 10_001, DiscountMultiplierBPS: 3_333, DiscountStartsAt: &store.updateInput.Discount.StartsAt, DiscountEndsAt: &store.updateInput.Discount.EndsAt}
	if got := rounded.MultiplierAt(store.updateInput.Discount.StartsAt); got != 3_334 {
		t.Fatalf("rounded multiplier=%d", got)
	}

	invalid := &DiscountInput{Name: "无效", MultiplierBPS: 10_000, StartsAt: start, EndsAt: end}
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{Discount: invalid}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid non-discount multiplier, got %v", err)
	}
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{Discount: discount, ClearDiscount: true}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected conflicting discount update, got %v", err)
	}
}

func TestDiscountSnapshotUpdatesAndDeletesWithoutDatabaseReads(t *testing.T) {
	groupID := uuid.New()
	start := time.Now().UTC().Add(-time.Hour)
	end := start.Add(2 * time.Hour)
	store := &fakeStore{updated: Record{
		ID: groupID, Code: "group-a", DisplayName: "A", MultiplierBPS: 10_000,
		DiscountName: "holiday", DiscountMultiplierBPS: 5_000, DiscountStartsAt: &start, DiscountEndsAt: &end,
	}}
	service := NewService(store)
	fallback := Summary{ID: groupID, Code: "group-a", DisplayName: "A", MultiplierBPS: 10_000}
	if got := service.MultiplierAt(fallback, time.Now().UTC()); got != 10_000 {
		t.Fatalf("initial cached multiplier=%d", got)
	}
	name := "A"
	if _, err := service.Update(context.Background(), groupID, UpdateInput{DisplayName: &name}); err != nil {
		t.Fatalf("update cached discount: %v", err)
	}
	if got := service.MultiplierAt(fallback, time.Now().UTC()); got != 5_000 {
		t.Fatalf("updated cached multiplier=%d", got)
	}
	if err := service.Delete(context.Background(), groupID); err != nil {
		t.Fatalf("delete cached discount: %v", err)
	}
	// Simulate a list or update that read the old row before Delete committed
	// and reached the cache afterward.
	service.remember(store.updated)
	refreshedFallback := fallback
	refreshedFallback.MultiplierBPS = 8_000
	if got := service.MultiplierAt(refreshedFallback, time.Now().UTC()); got != 8_000 {
		t.Fatalf("stale result restored deleted snapshot: multiplier=%d", got)
	}
}

/**
 * Delete 用于删除、撤销或释放指定资源。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Delete(_ context.Context, id uuid.UUID) error {
	f.deletedID = id
	return nil
}

/**
 * TestDeleteValidatesIDAndDelegates 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestDeleteValidatesIDAndDelegates(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store)
	if err := service.Delete(context.Background(), uuid.Nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid ID, got %v", err)
	}
	id := uuid.New()
	if err := service.Delete(context.Background(), id); err != nil || store.deletedID != id {
		t.Fatalf("delete id=%s stored=%s err=%v", id, store.deletedID, err)
	}
}
