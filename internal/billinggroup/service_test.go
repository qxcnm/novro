package billinggroup

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakeStore struct {
	deletedID   uuid.UUID
	createInput CreateInput
	updateInput UpdateInput
}

/**
 * Create 用于创建并返回所需的对象或记录。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Create(_ context.Context, input CreateInput) (Record, error) {
	f.createInput = input
	return Record{}, nil
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
	return Record{}, nil
}

/**
 * SetStatus 用于更新指定的数据或状态。
 * @param Status 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (*fakeStore) SetStatus(context.Context, uuid.UUID, Status) (Record, error) {
	return Record{}, nil
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
