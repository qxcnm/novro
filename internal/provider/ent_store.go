package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entbillinggroup "github.com/novro-gateway/novro/ent/billinggroup"
	entmodelroute "github.com/novro-gateway/novro/ent/modelroute"
	entprovider "github.com/novro-gateway/novro/ent/provider"
	"github.com/novro-gateway/novro/internal/billinggroup"
)

type EntStore struct {
	client *ent.Client
}

func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) Create(ctx context.Context, params CreateParams) (Record, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin provider creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	group, err := tx.BillingGroup.Query().Where(entbillinggroup.IDEQ(params.BillingGroupID), entbillinggroup.StatusEQ(entbillinggroup.StatusActive), entbillinggroup.DeletedAtIsNil()).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrGroupUnavailable
	}
	if err != nil {
		return Record{}, fmt.Errorf("lock provider billing group: %w", err)
	}
	created, err := tx.Provider.Create().
		SetBillingGroupID(group.ID).
		SetCode(params.Code).
		SetDisplayName(params.DisplayName).
		SetProtocol(entprovider.Protocol(params.Protocol)).
		SetBaseURL(params.BaseURL).
		SetModelListPath(params.ModelListPath).
		SetEncryptedAPIKey(params.EncryptedAPIKey).
		SetAPIKeyHint(params.APIKeyHint).
		SetStatus(entprovider.StatusActive).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return Record{}, ErrCodeTaken
	}
	if err != nil {
		return Record{}, fmt.Errorf("create provider: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("commit provider creation: %w", err)
	}
	created.Edges.BillingGroup = group
	return fromEnt(created), nil
}

func (s *EntStore) List(ctx context.Context, filter ListFilter) ([]Record, error) {
	query := s.client.Provider.Query().Where(entprovider.DeletedAtIsNil())
	if filter.Search != "" {
		query = query.Where(entprovider.Or(
			entprovider.CodeContainsFold(filter.Search),
			entprovider.DisplayNameContainsFold(filter.Search),
			entprovider.BaseURLContainsFold(filter.Search),
		))
	}
	if filter.Status != "" {
		query = query.Where(entprovider.StatusEQ(entprovider.Status(filter.Status)))
	}
	entities, err := query.WithBillingGroup().Order(ent.Asc(entprovider.FieldDisplayName)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	records := make([]Record, 0, len(entities))
	for _, entity := range entities {
		records = append(records, fromEnt(entity))
	}
	return records, nil
}

func (s *EntStore) Update(ctx context.Context, id uuid.UUID, params UpdateParams) (Record, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin provider update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	update := tx.Provider.UpdateOneID(id).Where(entprovider.DeletedAtIsNil())
	if params.BillingGroupID != nil {
		group, err := tx.BillingGroup.Query().Where(entbillinggroup.IDEQ(*params.BillingGroupID), entbillinggroup.StatusEQ(entbillinggroup.StatusActive), entbillinggroup.DeletedAtIsNil()).ForUpdate().Only(ctx)
		if ent.IsNotFound(err) {
			return Record{}, ErrGroupUnavailable
		}
		if err != nil {
			return Record{}, fmt.Errorf("lock provider billing group: %w", err)
		}
		update.SetBillingGroupID(group.ID)
	}
	if params.DisplayName != nil {
		update.SetDisplayName(*params.DisplayName)
	}
	if params.Protocol != nil {
		update.SetProtocol(entprovider.Protocol(*params.Protocol))
	}
	if params.BaseURL != nil {
		update.SetBaseURL(*params.BaseURL)
	}
	if params.ModelListPath != nil {
		update.SetModelListPath(*params.ModelListPath)
	}
	if params.EncryptedAPIKey != nil {
		update.SetEncryptedAPIKey(*params.EncryptedAPIKey)
	}
	if params.APIKeyHint != nil {
		update.SetAPIKeyHint(*params.APIKeyHint)
	}
	updated, err := update.Save(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("update provider: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("commit provider update: %w", err)
	}
	return s.get(ctx, updated.ID)
}

func (s *EntStore) SetStatus(ctx context.Context, id uuid.UUID, status Status) (Record, error) {
	updated, err := s.client.Provider.UpdateOneID(id).Where(entprovider.DeletedAtIsNil()).SetStatus(entprovider.Status(status)).Save(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("update provider status: %w", err)
	}
	return s.get(ctx, updated.ID)
}

func (s *EntStore) Delete(ctx context.Context, id uuid.UUID) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin provider delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	entity, err := tx.Provider.Query().Where(entprovider.IDEQ(id), entprovider.DeletedAtIsNil()).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read provider before delete: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.ModelRoute.Update().Where(entmodelroute.ProviderIDEQ(id), entmodelroute.DeletedAtIsNil()).
		SetStatus(entmodelroute.StatusDisabled).SetDeletedAt(now).Save(ctx); err != nil {
		return fmt.Errorf("soft delete provider model routes: %w", err)
	}
	if _, err := entity.Update().SetStatus(entprovider.StatusDisabled).SetDeletedAt(now).Save(ctx); err != nil {
		return fmt.Errorf("soft delete provider: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit provider delete: %w", err)
	}
	return nil
}

func fromEnt(entity *ent.Provider) Record {
	record := Record{
		ID: entity.ID, Code: entity.Code, DisplayName: entity.DisplayName,
		BillingGroupID: entity.BillingGroupID,
		Protocol:       Protocol(entity.Protocol), BaseURL: entity.BaseURL, ModelListPath: entity.ModelListPath,
		APIKeyHint: entity.APIKeyHint, HasAPIKey: entity.EncryptedAPIKey != "",
		Status: Status(entity.Status), CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}
	if group, err := entity.Edges.BillingGroupOrErr(); err == nil {
		record.BillingGroup = billinggroup.Summary{ID: group.ID, Code: group.Code, DisplayName: group.DisplayName, MultiplierBPS: group.MultiplierBps}
	}
	return record
}

func (s *EntStore) get(ctx context.Context, id uuid.UUID) (Record, error) {
	entity, err := s.client.Provider.Query().Where(entprovider.IDEQ(id), entprovider.DeletedAtIsNil()).WithBillingGroup().Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read provider: %w", err)
	}
	return fromEnt(entity), nil
}
