package modelpricing

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entbillinggroup "github.com/novro-gateway/novro/ent/billinggroup"
	entmodelpriceplan "github.com/novro-gateway/novro/ent/modelpriceplan"
	entmodelpricewindow "github.com/novro-gateway/novro/ent/modelpricewindow"
	entmodelroute "github.com/novro-gateway/novro/ent/modelroute"
	entprovider "github.com/novro-gateway/novro/ent/provider"
	entupstreammodel "github.com/novro-gateway/novro/ent/upstreammodel"
	"github.com/novro-gateway/novro/internal/billing"
)

/**
 * EntStore 使用 Ent 持久化价格方案、版本和峰谷窗口。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type EntStore struct{ client *ent.Client }

/**
 * NewEntStore 创建基于 Ent 客户端的价格方案存储。
 * @param client Ent 数据库客户端。
 * @return 价格方案存储实例。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func NewEntStore(client *ent.Client) *EntStore { return &EntStore{client: client} }

/**
 * List 查询模型的全部价格方案，并预加载每个方案的窗口。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param modelID 上游模型的唯一标识。
 * @return 按版本倒序排列的价格方案列表。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *EntStore) List(ctx context.Context, modelID uuid.UUID) ([]Plan, error) {
	entities, err := s.client.ModelPricePlan.Query().
		Where(entmodelpriceplan.UpstreamModelIDEQ(modelID)).
		WithWindows(func(query *ent.ModelPriceWindowQuery) {
			query.Order(ent.Asc(entmodelpricewindow.FieldStartMinute), ent.Asc(entmodelpricewindow.FieldLabel))
		}).
		Order(ent.Desc(entmodelpriceplan.FieldVersion)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list model price plans: %w", err)
	}
	result := make([]Plan, 0, len(entities))
	for _, entity := range entities {
		result = append(result, planFromEnt(entity))
	}
	return result, nil
}

/**
 * CreateDraft 在事务中锁定模型并创建价格方案草稿及其窗口。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param modelID 上游模型的唯一标识。
 * @param input 已校验的价格方案草稿内容。
 * @return 创建后的价格方案。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *EntStore) CreateDraft(ctx context.Context, modelID uuid.UUID, input PlanInput) (Plan, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Plan{}, fmt.Errorf("begin model price plan creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.UpstreamModel.Query().Where(entupstreammodel.IDEQ(modelID), entupstreammodel.DeletedAtIsNil()).ForUpdate().Only(ctx); ent.IsNotFound(err) {
		return Plan{}, ErrModelMissing
	} else if err != nil {
		return Plan{}, fmt.Errorf("lock upstream model for pricing: %w", err)
	}
	version := 1
	latest, err := tx.ModelPricePlan.Query().Where(entmodelpriceplan.UpstreamModelIDEQ(modelID)).Order(ent.Desc(entmodelpriceplan.FieldVersion)).First(ctx)
	if err == nil {
		version = latest.Version + 1
	} else if !ent.IsNotFound(err) {
		return Plan{}, fmt.Errorf("read latest model price version: %w", err)
	}
	created, err := createPlan(tx, ctx, modelID, version, input)
	if ent.IsConstraintError(err) {
		return Plan{}, ErrConflict
	}
	if err != nil {
		return Plan{}, fmt.Errorf("create model price plan: %w", err)
	}
	if err := createWindows(tx, ctx, created.ID, input.Windows); err != nil {
		return Plan{}, err
	}
	if err := tx.Commit(); err != nil {
		return Plan{}, fmt.Errorf("commit model price plan creation: %w", err)
	}
	return s.get(ctx, created.ID)
}

/**
 * UpdateDraft 在事务中替换草稿基础费率和全部峰谷窗口。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param planID 价格方案的唯一标识。
 * @param input 已校验的更新内容。
 * @return 更新后的价格方案。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *EntStore) UpdateDraft(ctx context.Context, planID uuid.UUID, input PlanInput) (Plan, error) {
	input.EffectiveFrom = time.Unix(0, 0).UTC()
	input.EffectiveTo = nil
	if input.Mode == ModeFixed {
		input.Timezone = "UTC"
		input.Windows = nil
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Plan{}, fmt.Errorf("begin model price plan update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	plan, err := tx.ModelPricePlan.Query().Where(entmodelpriceplan.IDEQ(planID)).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, fmt.Errorf("lock model price plan: %w", err)
	}
	if plan.Status != entmodelpriceplan.StatusDraft {
		return Plan{}, ErrImmutable
	}
	update := tx.ModelPricePlan.UpdateOne(plan).
		SetMode(entmodelpriceplan.Mode(input.Mode)).SetTimezone(input.Timezone).
		SetEffectiveFrom(input.EffectiveFrom).SetDefaultInputPriceMicros(input.DefaultRates.InputMicros).
		SetDefaultOutputPriceMicros(input.DefaultRates.OutputMicros).SetDefaultCacheReadPriceMicros(input.DefaultRates.CacheReadMicros).
		SetDefaultCacheWritePriceMicros(input.DefaultRates.CacheWriteMicros).SetDefaultCacheWrite1hPriceMicros(input.DefaultRates.CacheWrite1hMicros).
		SetDefaultRequestPriceMicros(input.DefaultRates.RequestMicros)
	if input.EffectiveTo == nil {
		update.ClearEffectiveTo()
	} else {
		update.SetEffectiveTo(*input.EffectiveTo)
	}
	if _, err := update.Save(ctx); err != nil {
		return Plan{}, fmt.Errorf("update model price plan: %w", err)
	}
	if _, err := tx.ModelPriceWindow.Delete().Where(entmodelpricewindow.PricePlanIDEQ(planID)).Exec(ctx); err != nil {
		return Plan{}, fmt.Errorf("replace model price windows: %w", err)
	}
	if err := createWindows(tx, ctx, planID, input.Windows); err != nil {
		return Plan{}, err
	}
	if err := tx.Commit(); err != nil {
		return Plan{}, fmt.Errorf("commit model price plan update: %w", err)
	}
	return s.get(ctx, planID)
}

/**
 * Publish 在单个事务中发布草稿、收口相邻版本并同步模型兼容价格。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param planID 待发布价格方案的唯一标识。
 * @return 已发布的价格方案。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *EntStore) Publish(ctx context.Context, planID uuid.UUID) (Plan, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Plan{}, fmt.Errorf("begin model price plan publication: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	plan, err := tx.ModelPricePlan.Query().Where(entmodelpriceplan.IDEQ(planID)).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, fmt.Errorf("lock model price plan for publication: %w", err)
	}
	if plan.Status != entmodelpriceplan.StatusDraft {
		return Plan{}, ErrImmutable
	}
	if err := s.publishDraftTx(tx, ctx, plan, time.Now().UTC()); err != nil {
		return Plan{}, err
	}
	if err := tx.Commit(); err != nil {
		return Plan{}, fmt.Errorf("commit model price plan publication: %w", err)
	}
	return s.get(ctx, planID)
}

/**
 * Republish 直接切换已有历史版本的生效区间，不创建新的价格版本。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param planID 要切换到的历史价格版本 ID。
 * @param at 新版本的生效时间，必须使用 UTC。
 * @return 切换后的价格方案；Created 始终为 false，表示没有生成新版本。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *EntStore) Republish(ctx context.Context, planID uuid.UUID, at time.Time) (RepublishResult, error) {
	if at.IsZero() {
		return RepublishResult{}, ErrInvalidInput
	}
	at = at.UTC()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return RepublishResult{}, fmt.Errorf("begin model price plan republish: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	sourceRef, err := tx.ModelPricePlan.Query().Where(entmodelpriceplan.IDEQ(planID)).Only(ctx)
	if ent.IsNotFound(err) {
		return RepublishResult{}, ErrNotFound
	}
	if err != nil {
		return RepublishResult{}, fmt.Errorf("read historical model price plan: %w", err)
	}
	if _, err := tx.UpstreamModel.Query().Where(entupstreammodel.IDEQ(sourceRef.UpstreamModelID), entupstreammodel.DeletedAtIsNil()).ForUpdate().Only(ctx); ent.IsNotFound(err) {
		return RepublishResult{}, ErrModelMissing
	} else if err != nil {
		return RepublishResult{}, fmt.Errorf("lock model for price plan republish: %w", err)
	}
	published, err := tx.ModelPricePlan.Query().Where(
		entmodelpriceplan.UpstreamModelIDEQ(sourceRef.UpstreamModelID),
		entmodelpriceplan.StatusEQ(entmodelpriceplan.StatusPublished),
	).WithWindows(func(query *ent.ModelPriceWindowQuery) {
		query.Order(ent.Asc(entmodelpricewindow.FieldStartMinute), ent.Asc(entmodelpricewindow.FieldLabel))
	}).Order(ent.Asc(entmodelpriceplan.FieldEffectiveFrom)).ForUpdate().All(ctx)
	if err != nil {
		return RepublishResult{}, fmt.Errorf("lock published model price plans for republish: %w", err)
	}
	var source *ent.ModelPricePlan
	for _, plan := range published {
		if plan.ID == planID {
			source = plan
			break
		}
	}
	if source == nil {
		return RepublishResult{}, ErrImmutable
	}
	current := effectivePlanAt(published, at)
	currentIsSource := current != nil && current.ID == source.ID
	hadFuturePlan := false
	for _, plan := range published {
		if plan.ID == source.ID {
			continue
		}
		if plan.EffectiveFrom.Equal(at) {
			return RepublishResult{}, ErrConflict
		}
		if plan.EffectiveFrom.After(at) {
			hadFuturePlan = true
			if _, err := tx.ModelPricePlan.UpdateOne(plan).SetStatus(entmodelpriceplan.StatusRetired).Save(ctx); err != nil {
				return RepublishResult{}, fmt.Errorf("retire future model price plan during switch: %w", err)
			}
		}
	}
	if currentIsSource && !hadFuturePlan {
		return RepublishResult{Plan: planFromEnt(current), Created: false}, nil
	}
	for _, plan := range published {
		if plan.ID == source.ID || !plan.EffectiveFrom.Before(at) {
			continue
		}
		if plan.EffectiveTo == nil || plan.EffectiveTo.After(at) {
			if _, err := tx.ModelPricePlan.UpdateOne(plan).SetEffectiveTo(at).Save(ctx); err != nil {
				return RepublishResult{}, fmt.Errorf("close active model price plan during switch: %w", err)
			}
		}
	}
	update := tx.ModelPricePlan.UpdateOne(source).ClearEffectiveTo()
	if !currentIsSource {
		update.SetEffectiveFrom(at)
	}
	if _, err := update.Save(ctx); err != nil {
		return RepublishResult{}, fmt.Errorf("switch model price plan effective interval: %w", err)
	}
	if err := activatePricedModelTx(tx, ctx, source.UpstreamModelID, defaultRates(source)); err != nil {
		return RepublishResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RepublishResult{}, fmt.Errorf("commit model price plan switch: %w", err)
	}
	plan, err := s.get(ctx, source.ID)
	if err != nil {
		return RepublishResult{}, err
	}
	return RepublishResult{Plan: plan, Created: false}, nil
}

/**
 * publishDraftTx 在当前事务中立即发布草稿并关闭当前版本。
 * 管理员不能为模型价格安排未来发布时间；发现旧数据中的未来版本时将其退役，
 * 避免它随后覆盖本次明确发布的价格。
 * @param tx 当前数据库事务。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param plan 已锁定的待发布草稿。
 * @return 发布结果或业务错误。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *EntStore) publishDraftTx(tx *ent.Tx, ctx context.Context, plan *ent.ModelPricePlan, at time.Time) error {
	published, err := tx.ModelPricePlan.Query().Where(
		entmodelpriceplan.UpstreamModelIDEQ(plan.UpstreamModelID),
		entmodelpriceplan.StatusEQ(entmodelpriceplan.StatusPublished),
	).ForUpdate().All(ctx)
	if err != nil {
		return fmt.Errorf("lock published model price plans: %w", err)
	}
	for _, existing := range published {
		if existing.EffectiveFrom.After(at) {
			if _, err := tx.ModelPricePlan.UpdateOne(existing).SetStatus(entmodelpriceplan.StatusRetired).Save(ctx); err != nil {
				return fmt.Errorf("retire future model price plan: %w", err)
			}
			continue
		}
		if existing.EffectiveTo == nil || existing.EffectiveTo.After(at) {
			if _, err := tx.ModelPricePlan.UpdateOne(existing).SetEffectiveTo(at).Save(ctx); err != nil {
				return fmt.Errorf("close previous model price plan: %w", err)
			}
		}
	}
	if _, err := tx.ModelPricePlan.UpdateOne(plan).SetStatus(entmodelpriceplan.StatusPublished).SetEffectiveFrom(at).ClearEffectiveTo().Save(ctx); err != nil {
		return fmt.Errorf("publish model price plan: %w", err)
	}
	return activatePricedModelTx(tx, ctx, plan.UpstreamModelID, defaultRates(plan))
}

// activatePricedModelTx keeps pricing publication and route availability atomic.
func activatePricedModelTx(tx *ent.Tx, ctx context.Context, modelID uuid.UUID, rates billing.RateCard) error {
	if _, err := tx.UpstreamModel.UpdateOneID(modelID).
		SetPricingConfigured(true).
		SetStatus(entupstreammodel.StatusActive).
		SetInputPriceMicros(rates.InputMicros).
		SetOutputPriceMicros(rates.OutputMicros).
		SetCacheReadPriceMicros(rates.CacheReadMicros).
		SetCacheWritePriceMicros(rates.CacheWriteMicros).
		SetCacheWrite1hPriceMicros(rates.CacheWrite1hMicros).
		SetRequestPriceMicros(rates.RequestMicros).
		Save(ctx); err != nil {
		return fmt.Errorf("activate priced upstream model: %w", err)
	}
	if _, err := tx.ModelRoute.Update().Where(
		entmodelroute.UpstreamModelIDEQ(modelID),
		entmodelroute.DeletedAtIsNil(),
		entmodelroute.HasProviderWith(entprovider.StatusEQ(entprovider.StatusActive), entprovider.DeletedAtIsNil()),
		entmodelroute.HasBillingGroupWith(entbillinggroup.KindEQ(entbillinggroup.KindStandard), entbillinggroup.StatusEQ(entbillinggroup.StatusActive), entbillinggroup.DeletedAtIsNil()),
	).SetInputPriceMicros(rates.InputMicros).
		SetOutputPriceMicros(rates.OutputMicros).
		SetStatus(entmodelroute.StatusActive).
		Save(ctx); err != nil {
		return fmt.Errorf("activate routes for priced upstream model: %w", err)
	}
	return nil
}

/**
 * DeleteDraft 在事务中删除草稿及其窗口，已发布版本不可删除。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param planID 待删除价格方案的唯一标识。
 * @return 删除结果或业务错误。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *EntStore) DeleteDraft(ctx context.Context, planID uuid.UUID) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin model price plan deletion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	plan, err := tx.ModelPricePlan.Query().Where(entmodelpriceplan.IDEQ(planID)).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock model price plan for deletion: %w", err)
	}
	if plan.Status != entmodelpriceplan.StatusDraft {
		return ErrImmutable
	}
	if _, err := tx.ModelPriceWindow.Delete().Where(entmodelpricewindow.PricePlanIDEQ(planID)).Exec(ctx); err != nil {
		return fmt.Errorf("delete model price windows: %w", err)
	}
	if err := tx.ModelPricePlan.DeleteOne(plan).Exec(ctx); err != nil {
		return fmt.Errorf("delete model price plan: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit model price plan deletion: %w", err)
	}
	return nil
}

/**
 * Resolve 从数据库解析请求时间对应的已发布方案和峰谷窗口。
 * 数据库兼容路径加载已发布版本后复用内存解析函数，保证版本边界和窗口选择完全一致。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param modelID 上游模型的唯一标识。
 * @param at 已统一为 UTC 的请求开始时间。
 * @return 解析后的费率和方案来源信息。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *EntStore) Resolve(ctx context.Context, modelID uuid.UUID, at time.Time) (Resolution, error) {
	plans, err := s.client.ModelPricePlan.Query().Where(
		entmodelpriceplan.UpstreamModelIDEQ(modelID),
		entmodelpriceplan.StatusEQ(entmodelpriceplan.StatusPublished),
	).WithWindows(func(query *ent.ModelPriceWindowQuery) {
		query.Order(ent.Asc(entmodelpricewindow.FieldStartMinute), ent.Asc(entmodelpricewindow.FieldLabel))
	}).Order(ent.Desc(entmodelpriceplan.FieldVersion)).All(ctx)
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve model price plans: %w", err)
	}
	if len(plans) > 0 {
		published := make([]Plan, 0, len(plans))
		for _, plan := range plans {
			published = append(published, planFromEnt(plan))
		}
		return resolvePublishedPlans(published, at.UTC())
	}
	return s.resolveLegacy(ctx, modelID)
}

/**
 * resolveLegacy 读取没有已发布价格方案的旧模型价格字段作为兼容回退。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param modelID 上游模型的唯一标识。
 * @return 旧版六维费率，并标记为兼容回退来源。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *EntStore) resolveLegacy(ctx context.Context, modelID uuid.UUID) (Resolution, error) {
	model, err := s.client.UpstreamModel.Query().Where(entupstreammodel.IDEQ(modelID), entupstreammodel.DeletedAtIsNil(), entupstreammodel.PricingConfiguredEQ(true)).Only(ctx)
	if ent.IsNotFound(err) {
		return Resolution{}, ErrNoPrice
	}
	if err != nil {
		return Resolution{}, fmt.Errorf("resolve legacy model price: %w", err)
	}
	return Resolution{
		Rates: billing.RateCard{
			InputMicros: model.InputPriceMicros, OutputMicros: model.OutputPriceMicros,
			CacheReadMicros: model.CacheReadPriceMicros, CacheWriteMicros: model.CacheWritePriceMicros,
			CacheWrite1hMicros: model.CacheWrite1hPriceMicros, RequestMicros: model.RequestPriceMicros,
		},
		Timezone: "UTC", LegacyFallback: true,
	}, nil
}

/**
 * get 读取单个价格方案及其按开始分钟排序的窗口。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param planID 价格方案的唯一标识。
 * @return 价格方案详情。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func (s *EntStore) get(ctx context.Context, planID uuid.UUID) (Plan, error) {
	entity, err := s.client.ModelPricePlan.Query().Where(entmodelpriceplan.IDEQ(planID)).WithWindows(func(query *ent.ModelPriceWindowQuery) {
		query.Order(ent.Asc(entmodelpricewindow.FieldStartMinute), ent.Asc(entmodelpricewindow.FieldLabel))
	}).Only(ctx)
	if ent.IsNotFound(err) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, fmt.Errorf("read model price plan: %w", err)
	}
	return planFromEnt(entity), nil
}

/**
 * effectivePlanAt 选择指定时刻正在生效的已发布方案。
 * 该辅助方法用于切换历史版本，必须与 Resolve 使用相同的 [from, to) 判断，避免“当前版本”判定和请求计费不一致。
 * @param plans 同一模型的已发布方案，按生效时间升序排列。
 * @param at 待解析的 UTC 时刻。
 * @return 命中的方案，未命中时返回 nil。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func effectivePlanAt(plans []*ent.ModelPricePlan, at time.Time) *ent.ModelPricePlan {
	var selected *ent.ModelPricePlan
	for _, plan := range plans {
		if plan.EffectiveFrom.After(at) || (plan.EffectiveTo != nil && !at.Before(*plan.EffectiveTo)) {
			continue
		}
		if selected == nil || plan.EffectiveFrom.After(selected.EffectiveFrom) || (plan.EffectiveFrom.Equal(selected.EffectiveFrom) && plan.Version > selected.Version) {
			selected = plan
		}
	}
	return selected
}

/**
 * createPlan 在事务中创建价格方案主记录并写入默认六维费率。
 * @param tx 当前数据库事务。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param modelID 上游模型的唯一标识。
 * @param version 新方案版本号。
 * @param input 已校验的价格方案输入。
 * @return Ent 价格方案实体。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func createPlan(tx *ent.Tx, ctx context.Context, modelID uuid.UUID, version int, input PlanInput) (*ent.ModelPricePlan, error) {
	input.EffectiveFrom = time.Unix(0, 0).UTC()
	input.EffectiveTo = nil
	if input.Mode == ModeFixed {
		input.Timezone = "UTC"
	}
	create := tx.ModelPricePlan.Create().SetUpstreamModelID(modelID).SetVersion(version).
		SetMode(entmodelpriceplan.Mode(input.Mode)).SetTimezone(input.Timezone).SetEffectiveFrom(input.EffectiveFrom).
		SetStatus(entmodelpriceplan.StatusDraft).SetDefaultInputPriceMicros(input.DefaultRates.InputMicros).
		SetDefaultOutputPriceMicros(input.DefaultRates.OutputMicros).SetDefaultCacheReadPriceMicros(input.DefaultRates.CacheReadMicros).
		SetDefaultCacheWritePriceMicros(input.DefaultRates.CacheWriteMicros).SetDefaultCacheWrite1hPriceMicros(input.DefaultRates.CacheWrite1hMicros).
		SetDefaultRequestPriceMicros(input.DefaultRates.RequestMicros)
	return create.Save(ctx)
}

/**
 * createWindows 批量创建方案的峰谷窗口及其完整六维费率。
 * @param tx 当前数据库事务。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param planID 所属价格方案的唯一标识。
 * @param windows 已校验的窗口输入列表。
 * @return 批量写入结果或数据库错误。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func createWindows(tx *ent.Tx, ctx context.Context, planID uuid.UUID, windows []WindowInput) error {
	if len(windows) == 0 {
		return nil
	}
	builders := make([]*ent.ModelPriceWindowCreate, 0, len(windows))
	for _, window := range windows {
		builders = append(builders, tx.ModelPriceWindow.Create().SetPricePlanID(planID).SetLabel(window.Label).
			SetWeekdayMask(window.WeekdayMask).SetStartMinute(window.StartMinute).SetEndMinute(window.EndMinute).
			SetInputPriceMicros(window.Rates.InputMicros).SetOutputPriceMicros(window.Rates.OutputMicros).
			SetCacheReadPriceMicros(window.Rates.CacheReadMicros).SetCacheWritePriceMicros(window.Rates.CacheWriteMicros).
			SetCacheWrite1hPriceMicros(window.Rates.CacheWrite1hMicros).SetRequestPriceMicros(window.Rates.RequestMicros))
	}
	if _, err := tx.ModelPriceWindow.CreateBulk(builders...).Save(ctx); err != nil {
		return fmt.Errorf("create model price windows: %w", err)
	}
	return nil
}

/**
 * planFromEnt 将 Ent 价格方案实体转换为应用层模型，并复制排序后的窗口。
 * @param entity Ent 价格方案实体。
 * @return 应用层价格方案。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func planFromEnt(entity *ent.ModelPricePlan) Plan {
	windows, _ := entity.Edges.WindowsOrErr()
	result := Plan{
		ID: entity.ID, UpstreamModelID: entity.UpstreamModelID, Version: entity.Version,
		Mode: Mode(entity.Mode), Timezone: entity.Timezone, EffectiveFrom: entity.EffectiveFrom,
		EffectiveTo: entity.EffectiveTo, Status: Status(entity.Status), DefaultRates: defaultRates(entity),
		Windows: make([]Window, 0, len(windows)), CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}
	for _, window := range windows {
		result.Windows = append(result.Windows, Window{
			ID: window.ID, Label: window.Label, WeekdayMask: window.WeekdayMask,
			StartMinute: window.StartMinute, EndMinute: window.EndMinute,
			Rates: windowRates(window), CreatedAt: window.CreatedAt,
		})
	}
	sort.SliceStable(result.Windows, func(i, j int) bool { return result.Windows[i].StartMinute < result.Windows[j].StartMinute })
	return result
}

/**
 * defaultRates 提取价格方案的默认六维费率。
 * @param plan Ent 价格方案实体。
 * @return 默认计费费率卡。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func defaultRates(plan *ent.ModelPricePlan) billing.RateCard {
	return billing.RateCard{
		InputMicros: plan.DefaultInputPriceMicros, OutputMicros: plan.DefaultOutputPriceMicros,
		CacheReadMicros: plan.DefaultCacheReadPriceMicros, CacheWriteMicros: plan.DefaultCacheWritePriceMicros,
		CacheWrite1hMicros: plan.DefaultCacheWrite1hPriceMicros, RequestMicros: plan.DefaultRequestPriceMicros,
	}
}

/**
 * windowRates 提取峰谷窗口覆盖的完整六维费率。
 * @param window Ent 峰谷窗口实体。
 * @return 窗口计费费率卡。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func windowRates(window *ent.ModelPriceWindow) billing.RateCard {
	return billing.RateCard{
		InputMicros: window.InputPriceMicros, OutputMicros: window.OutputPriceMicros,
		CacheReadMicros: window.CacheReadPriceMicros, CacheWriteMicros: window.CacheWritePriceMicros,
		CacheWrite1hMicros: window.CacheWrite1hPriceMicros, RequestMicros: window.RequestPriceMicros,
	}
}
