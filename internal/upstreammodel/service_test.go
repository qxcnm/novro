package upstreammodel

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakeStore struct{ deletedID uuid.UUID }

func (*fakeStore) Create(context.Context, CreateInput) (Record, error) { return Record{}, nil }
func (*fakeStore) List(context.Context, ListFilter) ([]Record, error)  { return nil, nil }
func (*fakeStore) Update(context.Context, uuid.UUID, UpdateInput) (Record, error) {
	return Record{}, nil
}
func (*fakeStore) SetStatus(context.Context, uuid.UUID, Status) (Record, error) {
	return Record{}, nil
}
func (f *fakeStore) Delete(_ context.Context, id uuid.UUID) error {
	f.deletedID = id
	return nil
}

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
