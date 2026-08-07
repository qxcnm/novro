package billinggroup

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entbillinggroup "github.com/novro-gateway/novro/ent/billinggroup"
	entuser "github.com/novro-gateway/novro/ent/user"
)

type EntStore struct{ client *ent.Client }

func NewEntStore(client *ent.Client) *EntStore { return &EntStore{client: client} }

func (s *EntStore) Create(ctx context.Context, input CreateInput) (Record, error) {
	created, err := s.client.BillingGroup.Create().SetCode(input.Code).SetDisplayName(input.DisplayName).
		SetMultiplierBps(input.MultiplierBPS).SetStatus(entbillinggroup.StatusActive).Save(ctx)
	if ent.IsConstraintError(err) {
		return Record{}, ErrCodeTaken
	}
	if err != nil {
		return Record{}, fmt.Errorf("create billing group: %w", err)
	}
	return s.get(ctx, created.ID)
}

func (s *EntStore) List(ctx context.Context, filter ListFilter) ([]Record, error) {
	query := s.client.BillingGroup.Query().Where(entbillinggroup.DeletedAtIsNil())
	if filter.Search != "" {
		query = query.Where(entbillinggroup.Or(entbillinggroup.CodeContainsFold(filter.Search), entbillinggroup.DisplayNameContainsFold(filter.Search)))
	}
	if filter.Status != "" {
		query = query.Where(entbillinggroup.StatusEQ(entbillinggroup.Status(filter.Status)))
	}
	entities, err := query.WithUsers().Order(ent.Desc(entbillinggroup.FieldIsDefault), ent.Asc(entbillinggroup.FieldDisplayName)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list billing groups: %w", err)
	}
	return fromEntList(entities), nil
}

func (s *EntStore) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (Record, error) {
	update := s.client.BillingGroup.UpdateOneID(id).Where(entbillinggroup.DeletedAtIsNil())
	if input.DisplayName != nil {
		update.SetDisplayName(*input.DisplayName)
	}
	if input.MultiplierBPS != nil {
		update.SetMultiplierBps(*input.MultiplierBPS)
	}
	if _, err := update.Save(ctx); ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	} else if err != nil {
		return Record{}, fmt.Errorf("update billing group: %w", err)
	}
	return s.get(ctx, id)
}

func (s *EntStore) SetStatus(ctx context.Context, id uuid.UUID, status Status) (Record, error) {
	entity, err := s.client.BillingGroup.Query().Where(entbillinggroup.IDEQ(id), entbillinggroup.DeletedAtIsNil()).Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read billing group: %w", err)
	}
	if entity.IsDefault && status == StatusDisabled {
		return Record{}, ErrProtected
	}
	if _, err := entity.Update().SetStatus(entbillinggroup.Status(status)).Save(ctx); err != nil {
		return Record{}, fmt.Errorf("update billing group status: %w", err)
	}
	return s.get(ctx, id)
}

func (s *EntStore) Delete(ctx context.Context, id uuid.UUID) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin billing group delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	entity, err := tx.BillingGroup.Query().Where(entbillinggroup.IDEQ(id), entbillinggroup.DeletedAtIsNil()).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read billing group before delete: %w", err)
	}
	if entity.IsDefault {
		return ErrProtected
	}
	hasUsers, err := tx.User.Query().Where(entuser.BillingGroupIDEQ(id)).Exist(ctx)
	if err != nil {
		return fmt.Errorf("check billing group users: %w", err)
	}
	if hasUsers {
		return ErrInUse
	}
	if _, err := entity.Update().SetStatus(entbillinggroup.StatusDisabled).SetDeletedAt(time.Now().UTC()).Save(ctx); err != nil {
		return fmt.Errorf("soft delete billing group: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit billing group delete: %w", err)
	}
	return nil
}

func (s *EntStore) get(ctx context.Context, id uuid.UUID) (Record, error) {
	entity, err := s.client.BillingGroup.Query().Where(entbillinggroup.IDEQ(id), entbillinggroup.DeletedAtIsNil()).WithUsers().Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read billing group: %w", err)
	}
	return fromEnt(entity), nil
}

func fromEntList(entities []*ent.BillingGroup) []Record {
	records := make([]Record, 0, len(entities))
	for _, entity := range entities {
		records = append(records, fromEnt(entity))
	}
	return records
}

func fromEnt(entity *ent.BillingGroup) Record {
	return Record{ID: entity.ID, Code: entity.Code, DisplayName: entity.DisplayName, MultiplierBPS: entity.MultiplierBps,
		IsDefault: entity.IsDefault, Status: Status(entity.Status), UserCount: len(entity.Edges.Users), CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
}
