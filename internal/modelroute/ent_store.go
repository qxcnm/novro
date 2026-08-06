package modelroute

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entmodelroute "github.com/novro-gateway/novro/ent/modelroute"
	entprovider "github.com/novro-gateway/novro/ent/provider"
	"github.com/novro-gateway/novro/internal/provider"
)

type EntStore struct{ client *ent.Client }

func NewEntStore(client *ent.Client) *EntStore { return &EntStore{client: client} }

func (s *EntStore) Create(ctx context.Context, input CreateInput) (Record, error) {
	if _, err := s.client.Provider.Get(ctx, input.ProviderID); ent.IsNotFound(err) {
		return Record{}, ErrInvalidInput
	} else if err != nil {
		return Record{}, fmt.Errorf("read model provider: %w", err)
	}
	created, err := s.client.ModelRoute.Create().
		SetProviderID(input.ProviderID).SetPublicName(input.PublicName).SetDisplayName(input.DisplayName).
		SetUpstreamName(input.UpstreamName).SetInputPriceMicros(input.InputPriceMicros).
		SetOutputPriceMicros(input.OutputPriceMicros).SetStatus(entmodelroute.StatusActive).Save(ctx)
	if ent.IsConstraintError(err) {
		return Record{}, ErrNameTaken
	}
	if err != nil {
		return Record{}, fmt.Errorf("create model route: %w", err)
	}
	return s.get(ctx, created.ID)
}

func (s *EntStore) List(ctx context.Context, filter ListFilter) ([]Record, error) {
	query := s.client.ModelRoute.Query().WithProvider()
	if filter.Search != "" {
		query = query.Where(entmodelroute.Or(entmodelroute.PublicNameContainsFold(filter.Search), entmodelroute.DisplayNameContainsFold(filter.Search), entmodelroute.UpstreamNameContainsFold(filter.Search), entmodelroute.HasProviderWith(entprovider.Or(entprovider.CodeContainsFold(filter.Search), entprovider.DisplayNameContainsFold(filter.Search)))))
	}
	if filter.Status != "" {
		query = query.Where(entmodelroute.StatusEQ(entmodelroute.Status(filter.Status)))
	}
	entities, err := query.Order(ent.Asc(entmodelroute.FieldPublicName)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list model routes: %w", err)
	}
	return fromEntList(entities), nil
}

func (s *EntStore) Update(ctx context.Context, id uuid.UUID, params UpdateParams) (Record, error) {
	if params.ProviderID != nil {
		if _, err := s.client.Provider.Get(ctx, *params.ProviderID); ent.IsNotFound(err) {
			return Record{}, ErrInvalidInput
		} else if err != nil {
			return Record{}, fmt.Errorf("read model provider: %w", err)
		}
	}
	update := s.client.ModelRoute.UpdateOneID(id)
	if params.ProviderID != nil {
		update.SetProviderID(*params.ProviderID)
	}
	if params.DisplayName != nil {
		update.SetDisplayName(*params.DisplayName)
	}
	if params.UpstreamName != nil {
		update.SetUpstreamName(*params.UpstreamName)
	}
	if params.InputPriceMicros != nil {
		update.SetInputPriceMicros(*params.InputPriceMicros)
	}
	if params.OutputPriceMicros != nil {
		update.SetOutputPriceMicros(*params.OutputPriceMicros)
	}
	if _, err := update.Save(ctx); ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	} else if err != nil {
		return Record{}, fmt.Errorf("update model route: %w", err)
	}
	return s.get(ctx, id)
}

func (s *EntStore) SetStatus(ctx context.Context, id uuid.UUID, status Status) (Record, error) {
	if _, err := s.client.ModelRoute.UpdateOneID(id).SetStatus(entmodelroute.Status(status)).Save(ctx); ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	} else if err != nil {
		return Record{}, fmt.Errorf("update model route status: %w", err)
	}
	return s.get(ctx, id)
}

func (s *EntStore) Resolve(ctx context.Context, publicName string) (Record, string, string, error) {
	entity, err := s.client.ModelRoute.Query().Where(entmodelroute.PublicNameEQ(publicName), entmodelroute.StatusEQ(entmodelroute.StatusActive), entmodelroute.HasProviderWith(entprovider.StatusEQ(entprovider.StatusActive))).WithProvider().Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, "", "", ErrNotFound
	}
	if err != nil {
		return Record{}, "", "", fmt.Errorf("resolve model route: %w", err)
	}
	providerEntity, err := entity.Edges.ProviderOrErr()
	if err != nil {
		return Record{}, "", "", fmt.Errorf("read resolved provider: %w", err)
	}
	return fromEnt(entity), providerEntity.BaseURL, providerEntity.EncryptedAPIKey, nil
}

func (s *EntStore) ListActive(ctx context.Context) ([]Record, error) {
	entities, err := s.client.ModelRoute.Query().Where(entmodelroute.StatusEQ(entmodelroute.StatusActive), entmodelroute.HasProviderWith(entprovider.StatusEQ(entprovider.StatusActive))).WithProvider().Order(ent.Asc(entmodelroute.FieldPublicName)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active model routes: %w", err)
	}
	return fromEntList(entities), nil
}

func (s *EntStore) get(ctx context.Context, id uuid.UUID) (Record, error) {
	entity, err := s.client.ModelRoute.Query().Where(entmodelroute.IDEQ(id)).WithProvider().Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read model route: %w", err)
	}
	return fromEnt(entity), nil
}

func fromEntList(entities []*ent.ModelRoute) []Record {
	records := make([]Record, 0, len(entities))
	for _, entity := range entities {
		records = append(records, fromEnt(entity))
	}
	return records
}

func fromEnt(entity *ent.ModelRoute) Record {
	p, _ := entity.Edges.ProviderOrErr()
	summary := ProviderSummary{}
	if p != nil {
		summary = ProviderSummary{ID: p.ID, Code: p.Code, DisplayName: p.DisplayName, Protocol: provider.Protocol(p.Protocol), Status: provider.Status(p.Status)}
	}
	return Record{ID: entity.ID, ProviderID: entity.ProviderID, PublicName: entity.PublicName, DisplayName: entity.DisplayName, UpstreamName: entity.UpstreamName, InputPriceMicros: entity.InputPriceMicros, OutputPriceMicros: entity.OutputPriceMicros, Status: Status(entity.Status), Provider: summary, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
}
