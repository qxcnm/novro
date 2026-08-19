package modelroute

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entbillinggroup "github.com/novro-gateway/novro/ent/billinggroup"
	entbillinggroupcomposition "github.com/novro-gateway/novro/ent/billinggroupcomposition"
	entmodelroute "github.com/novro-gateway/novro/ent/modelroute"
	entprovider "github.com/novro-gateway/novro/ent/provider"
	entupstreammodel "github.com/novro-gateway/novro/ent/upstreammodel"
	"github.com/novro-gateway/novro/internal/billinggroup"
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
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin model route creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.BillingGroup.Query().Where(entbillinggroup.IDEQ(input.BillingGroupID), entbillinggroup.KindEQ(entbillinggroup.KindStandard), entbillinggroup.StatusEQ(entbillinggroup.StatusActive), entbillinggroup.DeletedAtIsNil()).ForUpdate().Only(ctx); ent.IsNotFound(err) {
		return Record{}, ErrGroupUnavailable
	} else if err != nil {
		return Record{}, fmt.Errorf("read model route billing group: %w", err)
	}
	upstream, err := tx.UpstreamModel.Query().Where(entupstreammodel.IDEQ(input.UpstreamModelID), entupstreammodel.DeletedAtIsNil()).Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrInvalidInput
	} else if err != nil {
		return Record{}, fmt.Errorf("read upstream model: %w", err)
	}
	if _, err := tx.Provider.Query().Where(entprovider.IDEQ(input.ProviderID), entprovider.DeletedAtIsNil()).Only(ctx); ent.IsNotFound(err) {
		return Record{}, ErrInvalidInput
	} else if err != nil {
		return Record{}, fmt.Errorf("read provider: %w", err)
	}
	status := entmodelroute.StatusActive
	if upstream.Status != entupstreammodel.StatusActive || !upstream.PricingConfigured {
		status = entmodelroute.StatusDisabled
	}
	created, err := tx.ModelRoute.Create().
		SetProviderID(input.ProviderID).SetUpstreamModelID(upstream.ID).SetBillingGroupID(input.BillingGroupID).SetPublicName(input.PublicName).SetDisplayName(input.DisplayName).
		SetUpstreamName(upstream.UpstreamName).SetInputPriceMicros(upstream.InputPriceMicros).
		SetOutputPriceMicros(upstream.OutputPriceMicros).SetStatus(status).Save(ctx)
	if ent.IsConstraintError(err) {
		return Record{}, ErrNameTaken
	}
	if err != nil {
		return Record{}, fmt.Errorf("create model route: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("commit model route creation: %w", err)
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
	).WithProvider().WithUpstreamModel().WithBillingGroup()
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
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin model route update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := tx.ModelRoute.Query().Where(entmodelroute.IDEQ(id), entmodelroute.DeletedAtIsNil()).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read model route before update: %w", err)
	}
	groupIDs := []uuid.UUID{current.BillingGroupID}
	if params.BillingGroupID != nil {
		groupIDs = append(groupIDs, *params.BillingGroupID)
	}
	if err := lockRouteGroups(ctx, tx, groupIDs); err != nil {
		return Record{}, err
	}
	disableForUpstream := false
	if params.UpstreamModelID != nil {
		upstream, err := tx.UpstreamModel.Query().Where(entupstreammodel.IDEQ(*params.UpstreamModelID), entupstreammodel.DeletedAtIsNil()).Only(ctx)
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
		if _, err := tx.Provider.Query().Where(entprovider.IDEQ(*params.ProviderID), entprovider.DeletedAtIsNil()).Only(ctx); ent.IsNotFound(err) {
			return Record{}, ErrInvalidInput
		} else if err != nil {
			return Record{}, fmt.Errorf("read model provider: %w", err)
		}
	}
	if params.BillingGroupID != nil {
		if _, err := tx.BillingGroup.Query().Where(entbillinggroup.IDEQ(*params.BillingGroupID), entbillinggroup.KindEQ(entbillinggroup.KindStandard), entbillinggroup.StatusEQ(entbillinggroup.StatusActive), entbillinggroup.DeletedAtIsNil()).Only(ctx); ent.IsNotFound(err) {
			return Record{}, ErrGroupUnavailable
		} else if err != nil {
			return Record{}, fmt.Errorf("read model route billing group: %w", err)
		}
	}
	update := tx.ModelRoute.UpdateOneID(id).Where(entmodelroute.DeletedAtIsNil())
	if params.UpstreamModelID != nil {
		update.SetUpstreamModelID(*params.UpstreamModelID)
	}
	if params.ProviderID != nil {
		update.SetProviderID(*params.ProviderID)
	}
	if params.BillingGroupID != nil {
		update.SetBillingGroupID(*params.BillingGroupID)
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
	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("commit model route update: %w", err)
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
		route, err := s.client.ModelRoute.Query().Where(entmodelroute.IDEQ(id), entmodelroute.DeletedAtIsNil()).WithUpstreamModel().WithBillingGroup().Only(ctx)
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
		group, groupErr := route.Edges.BillingGroupOrErr()
		if groupErr != nil || group.Kind != entbillinggroup.KindStandard || group.Status != entbillinggroup.StatusActive || group.DeletedAt != nil {
			return Record{}, ErrGroupUnavailable
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
	groupIDs, err := s.resolveRouteGroupIDs(ctx, billingGroupID)
	if err != nil {
		return nil, err
	}
	if len(groupIDs) == 0 {
		return nil, ErrNotFound
	}
	entities, err := s.client.ModelRoute.Query().Where(entmodelroute.PublicNameEQ(publicName), entmodelroute.BillingGroupIDIn(groupIDs...), entmodelroute.StatusEQ(entmodelroute.StatusActive), entmodelroute.DeletedAtIsNil(), entmodelroute.HasBillingGroupWith(entbillinggroup.KindEQ(entbillinggroup.KindStandard), entbillinggroup.StatusEQ(entbillinggroup.StatusActive), entbillinggroup.DeletedAtIsNil()), entmodelroute.HasProviderWith(entprovider.StatusEQ(entprovider.StatusActive), entprovider.DeletedAtIsNil()), entmodelroute.HasUpstreamModelWith(entupstreammodel.StatusEQ(entupstreammodel.StatusActive), entupstreammodel.PricingConfiguredEQ(true), entupstreammodel.DeletedAtIsNil())).WithProvider().WithUpstreamModel().WithBillingGroup().Order(ent.Asc(entmodelroute.FieldCreatedAt), ent.Asc(entmodelroute.FieldID)).All(ctx)
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
	groupIDs, err := s.resolveRouteGroupIDs(ctx, billingGroupID)
	if err != nil {
		return nil, err
	}
	if len(groupIDs) == 0 {
		return []Record{}, nil
	}
	entities, err := s.client.ModelRoute.Query().Where(entmodelroute.BillingGroupIDIn(groupIDs...), entmodelroute.StatusEQ(entmodelroute.StatusActive), entmodelroute.DeletedAtIsNil(), entmodelroute.HasBillingGroupWith(entbillinggroup.KindEQ(entbillinggroup.KindStandard), entbillinggroup.StatusEQ(entbillinggroup.StatusActive), entbillinggroup.DeletedAtIsNil()), entmodelroute.HasProviderWith(entprovider.StatusEQ(entprovider.StatusActive), entprovider.DeletedAtIsNil()), entmodelroute.HasUpstreamModelWith(entupstreammodel.StatusEQ(entupstreammodel.StatusActive), entupstreammodel.PricingConfiguredEQ(true), entupstreammodel.DeletedAtIsNil())).WithProvider().WithUpstreamModel().WithBillingGroup().Order(ent.Asc(entmodelroute.FieldPublicName), ent.Asc(entmodelroute.FieldCreatedAt), ent.Asc(entmodelroute.FieldID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active model routes: %w", err)
	}
	return fromEntList(entities), nil
}

func (s *EntStore) resolveRouteGroupIDs(ctx context.Context, billingGroupID uuid.UUID) ([]uuid.UUID, error) {
	group, err := s.client.BillingGroup.Query().Where(
		entbillinggroup.IDEQ(billingGroupID),
		entbillinggroup.StatusEQ(entbillinggroup.StatusActive),
		entbillinggroup.DeletedAtIsNil(),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrGroupUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("resolve route billing group: %w", err)
	}
	if group.Kind != entbillinggroup.KindComposite {
		return []uuid.UUID{group.ID}, nil
	}
	compositions, err := s.client.BillingGroupComposition.Query().Where(
		entbillinggroupcomposition.CompositeGroupIDEQ(group.ID),
		entbillinggroupcomposition.HasMemberGroupWith(
			entbillinggroup.KindEQ(entbillinggroup.KindStandard),
			entbillinggroup.StatusEQ(entbillinggroup.StatusActive),
			entbillinggroup.DeletedAtIsNil(),
		),
	).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve composite billing group members: %w", err)
	}
	ids := make([]uuid.UUID, 0, len(compositions))
	for _, composition := range compositions {
		ids = append(ids, composition.MemberGroupID)
	}
	return ids, nil
}

// lockRouteGroups serializes route writes with billing-group kind changes. A
// composite group must never become committed while a route write can still
// attach a route to it, so both the current and target groups are locked in a
// deterministic order before the route mutation runs.
func lockRouteGroups(ctx context.Context, tx *ent.Tx, ids []uuid.UUID) error {
	unique := make(map[uuid.UUID]struct{}, len(ids))
	ordered := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return ErrGroupUnavailable
		}
		if _, exists := unique[id]; exists {
			continue
		}
		unique[id] = struct{}{}
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].String() < ordered[j].String() })
	for _, id := range ordered {
		if _, err := tx.BillingGroup.Query().Where(entbillinggroup.IDEQ(id), entbillinggroup.DeletedAtIsNil()).ForUpdate().Only(ctx); ent.IsNotFound(err) {
			return ErrGroupUnavailable
		} else if err != nil {
			return fmt.Errorf("lock model route billing group: %w", err)
		}
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
	entity, err := s.client.ModelRoute.Query().Where(
		entmodelroute.IDEQ(id),
		entmodelroute.DeletedAtIsNil(),
		entmodelroute.HasProviderWith(entprovider.DeletedAtIsNil()),
		entmodelroute.Or(entmodelroute.UpstreamModelIDIsNil(), entmodelroute.HasUpstreamModelWith(entupstreammodel.DeletedAtIsNil())),
	).WithProvider().WithUpstreamModel().WithBillingGroup().Only(ctx)
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
		summary = ProviderSummary{ID: p.ID, Code: p.Code, DisplayName: p.DisplayName, Weight: p.Weight, Protocols: provider.ProtocolsFromStrings(p.Protocols, provider.Protocol(p.Protocol)), Status: provider.Status(p.Status)}
	}
	groupSummary := billinggroup.Summary{}
	if group, err := entity.Edges.BillingGroupOrErr(); err == nil {
		groupSummary = billinggroup.NewSummaryWithKind(group.ID, group.Code, group.DisplayName, billinggroup.Kind(group.Kind), group.MultiplierBps, group.DiscountName, group.DiscountMultiplierBps, group.DiscountStartsAt, group.DiscountEndsAt, nil)
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
	return Record{ID: entity.ID, ProviderID: entity.ProviderID, UpstreamModelID: entity.UpstreamModelID, BillingGroupID: entity.BillingGroupID, BillingGroup: groupSummary, PublicName: entity.PublicName, DisplayName: entity.DisplayName, UpstreamName: upstreamName, InputPriceMicros: inputPrice, OutputPriceMicros: outputPrice, Status: Status(entity.Status), Provider: summary, UpstreamModel: upstreamRecord, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
}
