package upstreammodel

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakeStore struct{ deletedID uuid.UUID }

/**
 * Create 执行该名称对应的业务处理逻辑。
 * @param CreateInput 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (*fakeStore) Create(context.Context, CreateInput) (Record, error) { return Record{}, nil }
/**
 * List 执行该名称对应的业务处理逻辑。
 * @param ListFilter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (*fakeStore) List(context.Context, ListFilter) ([]Record, error)  { return nil, nil }
/**
 * Update 执行该名称对应的业务处理逻辑。
 * @param UpdateInput 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (*fakeStore) Update(context.Context, uuid.UUID, UpdateInput) (Record, error) {
	return Record{}, nil
}
/**
 * SetStatus 执行该名称对应的业务处理逻辑。
 * @param Status 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (*fakeStore) SetStatus(context.Context, uuid.UUID, Status) (Record, error) {
	return Record{}, nil
}
/**
 * Delete 执行该名称对应的业务处理逻辑。
 * @param id 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Delete(_ context.Context, id uuid.UUID) error {
	f.deletedID = id
	return nil
}

/**
 * TestDeleteValidatesIDAndDelegates 执行该名称对应的业务处理逻辑。
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
