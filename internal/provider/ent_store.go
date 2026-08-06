package provider

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entprovider "github.com/novro-gateway/novro/ent/provider"
)

type EntStore struct {
	client *ent.Client
}

func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) Create(ctx context.Context, params CreateParams) (Record, error) {
	created, err := s.client.Provider.Create().
		SetCode(params.Code).
		SetDisplayName(params.DisplayName).
		SetProtocol(entprovider.Protocol(params.Protocol)).
		SetBaseURL(params.BaseURL).
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
	return fromEnt(created), nil
}

func (s *EntStore) List(ctx context.Context, filter ListFilter) ([]Record, error) {
	query := s.client.Provider.Query()
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
	entities, err := query.Order(ent.Asc(entprovider.FieldDisplayName)).All(ctx)
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
	update := s.client.Provider.UpdateOneID(id)
	if params.DisplayName != nil {
		update.SetDisplayName(*params.DisplayName)
	}
	if params.Protocol != nil {
		update.SetProtocol(entprovider.Protocol(*params.Protocol))
	}
	if params.BaseURL != nil {
		update.SetBaseURL(*params.BaseURL)
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
	return fromEnt(updated), nil
}

func (s *EntStore) SetStatus(ctx context.Context, id uuid.UUID, status Status) (Record, error) {
	updated, err := s.client.Provider.UpdateOneID(id).SetStatus(entprovider.Status(status)).Save(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("update provider status: %w", err)
	}
	return fromEnt(updated), nil
}

func fromEnt(entity *ent.Provider) Record {
	return Record{
		ID: entity.ID, Code: entity.Code, DisplayName: entity.DisplayName,
		Protocol: Protocol(entity.Protocol), BaseURL: entity.BaseURL,
		APIKeyHint: entity.APIKeyHint, HasAPIKey: entity.EncryptedAPIKey != "",
		Status: Status(entity.Status), CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}
}
