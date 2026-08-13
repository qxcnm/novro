package upstreammodel

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entmodelroute "github.com/novro-gateway/novro/ent/modelroute"
	entupstreammodel "github.com/novro-gateway/novro/ent/upstreammodel"
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
	created, err := s.client.UpstreamModel.Create().SetProviderName(input.ProviderName).SetUpstreamName(input.UpstreamName).
		SetDisplayName(input.DisplayName).SetInputPriceMicros(input.InputMicros).SetOutputPriceMicros(input.OutputMicros).
		SetCacheReadPriceMicros(input.CacheReadMicros).SetCacheWritePriceMicros(input.CacheWriteMicros).
		SetCacheWrite1hPriceMicros(input.CacheWrite1hMicros).SetRequestPriceMicros(input.RequestMicros).
		SetStatus(entupstreammodel.StatusActive).Save(ctx)
	if ent.IsConstraintError(err) {
		return Record{}, ErrNameTaken
	}
	if err != nil {
		return Record{}, fmt.Errorf("create upstream model: %w", err)
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
	query := s.client.UpstreamModel.Query().Where(entupstreammodel.DeletedAtIsNil())
	if filter.Search != "" {
		query = query.Where(entupstreammodel.Or(entupstreammodel.UpstreamNameContainsFold(filter.Search), entupstreammodel.DisplayNameContainsFold(filter.Search), entupstreammodel.ProviderNameContainsFold(filter.Search)))
	}
	if filter.Status != "" {
		query = query.Where(entupstreammodel.StatusEQ(entupstreammodel.Status(filter.Status)))
	}
	entities, err := query.Order(ent.Asc(entupstreammodel.FieldDisplayName)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list upstream models: %w", err)
	}
	return fromEntList(entities), nil
}

/**
 * Update 用于更新指定的数据或状态。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (Record, error) {
	update := s.client.UpstreamModel.UpdateOneID(id).Where(entupstreammodel.DeletedAtIsNil())
	if input.ProviderName != nil {
		update.SetProviderName(*input.ProviderName)
	}
	if input.UpstreamName != nil {
		update.SetUpstreamName(*input.UpstreamName)
	}
	if input.DisplayName != nil {
		update.SetDisplayName(*input.DisplayName)
	}
	if input.InputMicros != nil {
		update.SetInputPriceMicros(*input.InputMicros)
	}
	if input.OutputMicros != nil {
		update.SetOutputPriceMicros(*input.OutputMicros)
	}
	if input.CacheReadMicros != nil {
		update.SetCacheReadPriceMicros(*input.CacheReadMicros)
	}
	if input.CacheWriteMicros != nil {
		update.SetCacheWritePriceMicros(*input.CacheWriteMicros)
	}
	if input.CacheWrite1hMicros != nil {
		update.SetCacheWrite1hPriceMicros(*input.CacheWrite1hMicros)
	}
	if input.RequestMicros != nil {
		update.SetRequestPriceMicros(*input.RequestMicros)
	}
	if input.InputMicros != nil || input.OutputMicros != nil || input.CacheReadMicros != nil || input.CacheWriteMicros != nil || input.CacheWrite1hMicros != nil || input.RequestMicros != nil {
		update.SetPricingConfigured(true)
	}
	if _, err := update.Save(ctx); ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	} else if ent.IsConstraintError(err) {
		return Record{}, ErrNameTaken
	} else if err != nil {
		return Record{}, fmt.Errorf("update upstream model: %w", err)
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
		model, err := s.client.UpstreamModel.Query().Where(entupstreammodel.IDEQ(id), entupstreammodel.DeletedAtIsNil()).Only(ctx)
		if ent.IsNotFound(err) {
			return Record{}, ErrNotFound
		}
		if err != nil {
			return Record{}, fmt.Errorf("read upstream model pricing status: %w", err)
		}
		if !model.PricingConfigured {
			return Record{}, ErrPricingRequired
		}
	}
	if _, err := s.client.UpstreamModel.UpdateOneID(id).Where(entupstreammodel.DeletedAtIsNil()).SetStatus(entupstreammodel.Status(status)).Save(ctx); ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	} else if err != nil {
		return Record{}, fmt.Errorf("update upstream model status: %w", err)
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
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin upstream model delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	entity, err := tx.UpstreamModel.Query().Where(entupstreammodel.IDEQ(id), entupstreammodel.DeletedAtIsNil()).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read upstream model before delete: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.ModelRoute.Update().Where(entmodelroute.UpstreamModelIDEQ(id), entmodelroute.DeletedAtIsNil()).
		SetStatus(entmodelroute.StatusDisabled).SetDeletedAt(now).Save(ctx); err != nil {
		return fmt.Errorf("soft delete upstream model routes: %w", err)
	}
	if _, err := entity.Update().SetStatus(entupstreammodel.StatusDisabled).SetDeletedAt(now).Save(ctx); err != nil {
		return fmt.Errorf("soft delete upstream model: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upstream model delete: %w", err)
	}
	return nil
}

/**
 * get 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) get(ctx context.Context, id uuid.UUID) (Record, error) {
	entity, err := s.client.UpstreamModel.Query().Where(entupstreammodel.IDEQ(id), entupstreammodel.DeletedAtIsNil()).Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read upstream model: %w", err)
	}
	return fromEnt(entity), nil
}

/**
 * fromEntList 封装该名称对应的业务处理逻辑。
 * @param entities 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func fromEntList(entities []*ent.UpstreamModel) []Record {
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
func fromEnt(entity *ent.UpstreamModel) Record {
	return Record{ID: entity.ID, ProviderName: entity.ProviderName, UpstreamName: entity.UpstreamName, DisplayName: entity.DisplayName,
		Prices:            Prices{InputMicros: entity.InputPriceMicros, OutputMicros: entity.OutputPriceMicros, CacheReadMicros: entity.CacheReadPriceMicros, CacheWriteMicros: entity.CacheWritePriceMicros, CacheWrite1hMicros: entity.CacheWrite1hPriceMicros, RequestMicros: entity.RequestPriceMicros},
		PricingConfigured: entity.PricingConfigured, Status: Status(entity.Status), CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
}
