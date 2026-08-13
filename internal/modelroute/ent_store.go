package modelroute

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entmodelroute "github.com/novro-gateway/novro/ent/modelroute"
	entprovider "github.com/novro-gateway/novro/ent/provider"
	entupstreammodel "github.com/novro-gateway/novro/ent/upstreammodel"
	"github.com/novro-gateway/novro/internal/provider"
	"github.com/novro-gateway/novro/internal/upstreammodel"
)

type EntStore struct{ client *ent.Client }

/**
 * NewEntStore 用于创建并返回所需的对象或记录。
 * @param client 用于访问外部或底层服务的客户端。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewEntStore(client *ent.Client) *EntStore { return &EntStore{client: client} }

/**
 * Create 用于创建并返回所需的对象或记录。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) Create(ctx context.Context, input CreateInput) (Record, error) {
	upstream, err := s.client.UpstreamModel.Query().Where(entupstreammodel.IDEQ(input.UpstreamModelID), entupstreammodel.DeletedAtIsNil()).Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrInvalidInput
	} else if err != nil {
		return Record{}, fmt.Errorf("read upstream model: %w", err)
	}
	if _, err := s.client.Provider.Query().Where(entprovider.IDEQ(input.ProviderID), entprovider.DeletedAtIsNil()).Only(ctx); ent.IsNotFound(err) {
		return Record{}, ErrInvalidInput
	} else if err != nil {
		return Record{}, fmt.Errorf("read provider: %w", err)
	}
	status := entmodelroute.StatusActive
	if upstream.Status != entupstreammodel.StatusActive || !upstream.PricingConfigured {
		status = entmodelroute.StatusDisabled
	}
	created, err := s.client.ModelRoute.Create().
		SetProviderID(input.ProviderID).SetUpstreamModelID(upstream.ID).SetPublicName(input.PublicName).SetDisplayName(input.DisplayName).
		SetUpstreamName(upstream.UpstreamName).SetInputPriceMicros(upstream.InputPriceMicros).
		SetOutputPriceMicros(upstream.OutputPriceMicros).SetStatus(status).Save(ctx)
	if ent.IsConstraintError(err) {
		return Record{}, ErrNameTaken
	}
	if err != nil {
		return Record{}, fmt.Errorf("create model route: %w", err)
	}
	return s.get(ctx, created.ID)
}

/**
 * List 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) List(ctx context.Context, filter ListFilter) ([]Record, error) {
	query := s.client.ModelRoute.Query().Where(
		entmodelroute.DeletedAtIsNil(),
		entmodelroute.HasProviderWith(entprovider.DeletedAtIsNil()),
		entmodelroute.Or(entmodelroute.UpstreamModelIDIsNil(), entmodelroute.HasUpstreamModelWith(entupstreammodel.DeletedAtIsNil())),
	).WithProvider().WithUpstreamModel()
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

/**
 * Update 用于更新指定的数据或状态。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @param params 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) Update(ctx context.Context, id uuid.UUID, params UpdateParams) (Record, error) {
	disableForUpstream := false
	if params.UpstreamModelID != nil {
		upstream, err := s.client.UpstreamModel.Query().Where(entupstreammodel.IDEQ(*params.UpstreamModelID), entupstreammodel.DeletedAtIsNil()).Only(ctx)
		if ent.IsNotFound(err) {
			return Record{}, ErrInvalidInput
		}
		if err != nil {
			return Record{}, fmt.Errorf("read upstream model: %w", err)
		}
		params.UpstreamName = &upstream.UpstreamName
		params.InputPriceMicros = &upstream.InputPriceMicros
		params.OutputPriceMicros = &upstream.OutputPriceMicros
		if upstream.Status != entupstreammodel.StatusActive || !upstream.PricingConfigured {
			disableForUpstream = true
		}
	}
	if params.ProviderID != nil {
		if _, err := s.client.Provider.Query().Where(entprovider.IDEQ(*params.ProviderID), entprovider.DeletedAtIsNil()).Only(ctx); ent.IsNotFound(err) {
			return Record{}, ErrInvalidInput
		} else if err != nil {
			return Record{}, fmt.Errorf("read model provider: %w", err)
		}
	}
	update := s.client.ModelRoute.UpdateOneID(id).Where(entmodelroute.DeletedAtIsNil())
	if params.UpstreamModelID != nil {
		update.SetUpstreamModelID(*params.UpstreamModelID)
	}
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
	if disableForUpstream {
		update.SetStatus(entmodelroute.StatusDisabled)
	}
	if _, err := update.Save(ctx); ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	} else if err != nil {
		return Record{}, fmt.Errorf("update model route: %w", err)
	}
	return s.get(ctx, id)
}

/**
 * SetStatus 用于更新指定的数据或状态。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @param status 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) SetStatus(ctx context.Context, id uuid.UUID, status Status) (Record, error) {
	if status == StatusActive {
		route, err := s.client.ModelRoute.Query().Where(entmodelroute.IDEQ(id), entmodelroute.DeletedAtIsNil()).WithUpstreamModel().Only(ctx)
		if ent.IsNotFound(err) {
			return Record{}, ErrNotFound
		}
		if err != nil {
			return Record{}, fmt.Errorf("read model route before status update: %w", err)
		}
		if route.UpstreamModelID != nil {
			upstream, upstreamErr := route.Edges.UpstreamModelOrErr()
			if upstreamErr != nil {
				return Record{}, fmt.Errorf("read model route pricing status: %w", upstreamErr)
			}
			if upstream.Status != entupstreammodel.StatusActive || !upstream.PricingConfigured || upstream.DeletedAt != nil {
				return Record{}, ErrPricingRequired
			}
		}
	}
	if _, err := s.client.ModelRoute.UpdateOneID(id).Where(entmodelroute.DeletedAtIsNil()).SetStatus(entmodelroute.Status(status)).Save(ctx); ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	} else if err != nil {
		return Record{}, fmt.Errorf("update model route status: %w", err)
	}
	return s.get(ctx, id)
}

/**
 * Delete 用于删除、撤销或释放指定资源。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.client.ModelRoute.UpdateOneID(id).Where(entmodelroute.DeletedAtIsNil()).
		SetStatus(entmodelroute.StatusDisabled).SetDeletedAt(time.Now().UTC()).Save(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("soft delete model route: %w", err)
	}
	return nil
}

/**
 * ResolveCandidates 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param publicName 用于标识或筛选目标的文本值。
 * @param billingGroupID 目标资源的一个或多个唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) ResolveCandidates(ctx context.Context, publicName string, billingGroupID uuid.UUID) ([]Resolution, error) {
	entities, err := s.client.ModelRoute.Query().Where(entmodelroute.PublicNameEQ(publicName), entmodelroute.StatusEQ(entmodelroute.StatusActive), entmodelroute.DeletedAtIsNil(), entmodelroute.HasProviderWith(entprovider.BillingGroupIDEQ(billingGroupID), entprovider.StatusEQ(entprovider.StatusActive), entprovider.DeletedAtIsNil()), entmodelroute.HasUpstreamModelWith(entupstreammodel.StatusEQ(entupstreammodel.StatusActive), entupstreammodel.PricingConfiguredEQ(true), entupstreammodel.DeletedAtIsNil())).WithProvider().WithUpstreamModel().Order(ent.Asc(entmodelroute.FieldCreatedAt), ent.Asc(entmodelroute.FieldID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve model routes: %w", err)
	}
	if len(entities) == 0 {
		return nil, ErrNotFound
	}
	resolutions := make([]Resolution, 0, len(entities))
	for _, entity := range entities {
		providerEntity, err := entity.Edges.ProviderOrErr()
		if err != nil {
			return nil, fmt.Errorf("read resolved provider: %w", err)
		}
		resolutions = append(resolutions, Resolution{Record: fromEnt(entity), BaseURL: providerEntity.BaseURL, EncryptedAPIKey: providerEntity.EncryptedAPIKey})
	}
	return resolutions, nil
}

/**
 * ListActive 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param billingGroupID 目标资源的一个或多个唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) ListActive(ctx context.Context, billingGroupID uuid.UUID) ([]Record, error) {
	entities, err := s.client.ModelRoute.Query().Where(entmodelroute.StatusEQ(entmodelroute.StatusActive), entmodelroute.DeletedAtIsNil(), entmodelroute.HasProviderWith(entprovider.BillingGroupIDEQ(billingGroupID), entprovider.StatusEQ(entprovider.StatusActive), entprovider.DeletedAtIsNil()), entmodelroute.HasUpstreamModelWith(entupstreammodel.StatusEQ(entupstreammodel.StatusActive), entupstreammodel.PricingConfiguredEQ(true), entupstreammodel.DeletedAtIsNil())).WithProvider().WithUpstreamModel().Order(ent.Asc(entmodelroute.FieldPublicName), ent.Asc(entmodelroute.FieldCreatedAt), ent.Asc(entmodelroute.FieldID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active model routes: %w", err)
	}
	return fromEntList(entities), nil
}

/**
 * get 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) get(ctx context.Context, id uuid.UUID) (Record, error) {
	entity, err := s.client.ModelRoute.Query().Where(
		entmodelroute.IDEQ(id),
		entmodelroute.DeletedAtIsNil(),
		entmodelroute.HasProviderWith(entprovider.DeletedAtIsNil()),
		entmodelroute.Or(entmodelroute.UpstreamModelIDIsNil(), entmodelroute.HasUpstreamModelWith(entupstreammodel.DeletedAtIsNil())),
	).WithProvider().WithUpstreamModel().Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read model route: %w", err)
	}
	return fromEnt(entity), nil
}

/**
 * fromEntList 封装该名称对应的业务处理逻辑。
 * @param entities 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func fromEntList(entities []*ent.ModelRoute) []Record {
	records := make([]Record, 0, len(entities))
	for _, entity := range entities {
		records = append(records, fromEnt(entity))
	}
	return records
}

/**
 * fromEnt 封装该名称对应的业务处理逻辑。
 * @param entity 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func fromEnt(entity *ent.ModelRoute) Record {
	p, _ := entity.Edges.ProviderOrErr()
	summary := ProviderSummary{}
	if p != nil {
		summary = ProviderSummary{ID: p.ID, Code: p.Code, DisplayName: p.DisplayName, Weight: p.Weight, Protocol: provider.Protocol(p.Protocol), Status: provider.Status(p.Status)}
	}
	var upstreamRecord *upstreammodel.Record
	upstreamName, inputPrice, outputPrice := entity.UpstreamName, entity.InputPriceMicros, entity.OutputPriceMicros
	if upstream, err := entity.Edges.UpstreamModelOrErr(); err == nil {
		record := upstreammodel.Record{ID: upstream.ID, ProviderName: upstream.ProviderName, UpstreamName: upstream.UpstreamName, DisplayName: upstream.DisplayName,
			Prices:            upstreammodel.Prices{InputMicros: upstream.InputPriceMicros, OutputMicros: upstream.OutputPriceMicros, CacheReadMicros: upstream.CacheReadPriceMicros, CacheWriteMicros: upstream.CacheWritePriceMicros, CacheWrite1hMicros: upstream.CacheWrite1hPriceMicros, RequestMicros: upstream.RequestPriceMicros},
			PricingConfigured: upstream.PricingConfigured, Status: upstreammodel.Status(upstream.Status), CreatedAt: upstream.CreatedAt, UpdatedAt: upstream.UpdatedAt}
		upstreamRecord = &record
		upstreamName, inputPrice, outputPrice = upstream.UpstreamName, upstream.InputPriceMicros, upstream.OutputPriceMicros
	}
	return Record{ID: entity.ID, ProviderID: entity.ProviderID, UpstreamModelID: entity.UpstreamModelID, PublicName: entity.PublicName, DisplayName: entity.DisplayName, UpstreamName: upstreamName, InputPriceMicros: inputPrice, OutputPriceMicros: outputPrice, Status: Status(entity.Status), Provider: summary, UpstreamModel: upstreamRecord, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
}
