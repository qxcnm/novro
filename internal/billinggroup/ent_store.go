package billinggroup

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entapikey "github.com/novro-gateway/novro/ent/apikey"
	entbillinggroup "github.com/novro-gateway/novro/ent/billinggroup"
	entmodelroute "github.com/novro-gateway/novro/ent/modelroute"
	entuser "github.com/novro-gateway/novro/ent/user"
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
		return Record{}, fmt.Errorf("begin billing group creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := validateAuthorizedUsers(ctx, tx, input.AuthorizedUserIDs); err != nil {
		return Record{}, err
	}
	create := tx.BillingGroup.Create().SetCode(input.Code).SetDisplayName(input.DisplayName).
		SetMultiplierBps(input.MultiplierBPS).SetIsHidden(input.IsHidden).SetStatus(entbillinggroup.StatusActive)
	if input.Discount != nil {
		create.SetDiscountName(input.Discount.Name).SetDiscountMultiplierBps(input.Discount.MultiplierBPS).
			SetDiscountStartsAt(input.Discount.StartsAt).SetDiscountEndsAt(input.Discount.EndsAt)
	}
	if input.IsHidden {
		create.AddAuthorizedUserIDs(input.AuthorizedUserIDs...)
	}
	created, err := create.Save(ctx)
	if ent.IsConstraintError(err) {
		return Record{}, ErrCodeTaken
	}
	if err != nil {
		return Record{}, fmt.Errorf("create billing group: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("commit billing group creation: %w", err)
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
	query := s.client.BillingGroup.Query().Where(entbillinggroup.DeletedAtIsNil())
	if !filter.IncludeHidden {
		visible := entbillinggroup.IsHiddenEQ(false)
		if filter.AuthorizedUserID != uuid.Nil {
			visible = entbillinggroup.Or(visible, entbillinggroup.HasAuthorizedUsersWith(entuser.IDEQ(filter.AuthorizedUserID)))
		}
		query = query.Where(visible)
	}
	if filter.Search != "" {
		query = query.Where(entbillinggroup.Or(entbillinggroup.CodeContainsFold(filter.Search), entbillinggroup.DisplayNameContainsFold(filter.Search)))
	}
	if filter.Status != "" {
		query = query.Where(entbillinggroup.StatusEQ(entbillinggroup.Status(filter.Status)))
	}
	entities, err := query.
		WithAPIKeys(func(query *ent.APIKeyQuery) { query.Where(entapikey.StatusEQ(entapikey.StatusActive)) }).
		WithModelRoutes(func(query *ent.ModelRouteQuery) { query.Where(entmodelroute.DeletedAtIsNil()) }).
		WithAuthorizedUsers(func(query *ent.UserQuery) { query.Order(ent.Asc(entuser.FieldUsername)) }).
		Order(ent.Desc(entbillinggroup.FieldIsDefault), ent.Asc(entbillinggroup.FieldDisplayName)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list billing groups: %w", err)
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
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin billing group update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	entity, err := tx.BillingGroup.Query().Where(entbillinggroup.IDEQ(id), entbillinggroup.DeletedAtIsNil()).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("lock billing group for update: %w", err)
	}
	willBeHidden := entity.IsHidden
	if input.IsHidden != nil {
		willBeHidden = *input.IsHidden
	}
	if willBeHidden && entity.IsDefault {
		return Record{}, ErrProtected
	}
	if input.AuthorizedUserIDs != nil {
		if !willBeHidden && len(*input.AuthorizedUserIDs) > 0 {
			return Record{}, ErrInvalidInput
		}
		if err := validateAuthorizedUsers(ctx, tx, *input.AuthorizedUserIDs); err != nil {
			return Record{}, err
		}
	}
	update := tx.BillingGroup.UpdateOneID(id)
	if input.DisplayName != nil {
		update.SetDisplayName(*input.DisplayName)
	}
	if input.MultiplierBPS != nil {
		update.SetMultiplierBps(*input.MultiplierBPS)
	}
	if input.Discount != nil {
		update.SetDiscountName(input.Discount.Name).SetDiscountMultiplierBps(input.Discount.MultiplierBPS).
			SetDiscountStartsAt(input.Discount.StartsAt).SetDiscountEndsAt(input.Discount.EndsAt)
	} else if input.ClearDiscount {
		update.SetDiscountName("").SetDiscountMultiplierBps(DefaultMultiplierBPS).ClearDiscountStartsAt().ClearDiscountEndsAt()
	}
	if input.IsHidden != nil {
		update.SetIsHidden(*input.IsHidden)
	}
	if !willBeHidden {
		update.ClearAuthorizedUsers()
	} else if input.AuthorizedUserIDs != nil {
		update.ClearAuthorizedUsers().AddAuthorizedUserIDs((*input.AuthorizedUserIDs)...)
	}
	if _, err := update.Save(ctx); err != nil {
		return Record{}, fmt.Errorf("update billing group: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("commit billing group update: %w", err)
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
	hasAPIKeys, err := tx.APIKey.Query().Where(entapikey.BillingGroupIDEQ(id), entapikey.StatusEQ(entapikey.StatusActive)).Exist(ctx)
	if err != nil {
		return fmt.Errorf("check billing group API keys: %w", err)
	}
	hasModelRoutes, err := tx.ModelRoute.Query().Where(entmodelroute.BillingGroupIDEQ(id), entmodelroute.DeletedAtIsNil()).Exist(ctx)
	if err != nil {
		return fmt.Errorf("check billing group model routes: %w", err)
	}
	if hasAPIKeys || hasModelRoutes {
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

/**
 * get 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) get(ctx context.Context, id uuid.UUID) (Record, error) {
	entity, err := s.client.BillingGroup.Query().Where(entbillinggroup.IDEQ(id), entbillinggroup.DeletedAtIsNil()).
		WithAPIKeys(func(query *ent.APIKeyQuery) { query.Where(entapikey.StatusEQ(entapikey.StatusActive)) }).
		WithModelRoutes(func(query *ent.ModelRouteQuery) { query.Where(entmodelroute.DeletedAtIsNil()) }).
		WithAuthorizedUsers(func(query *ent.UserQuery) { query.Order(ent.Asc(entuser.FieldUsername)) }).Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read billing group: %w", err)
	}
	return fromEnt(entity), nil
}

/**
 * fromEntList 封装该名称对应的业务处理逻辑。
 * @param entities 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func fromEntList(entities []*ent.BillingGroup) []Record {
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
func fromEnt(entity *ent.BillingGroup) Record {
	summary := NewSummary(entity.ID, entity.Code, entity.DisplayName, entity.MultiplierBps, entity.DiscountName, entity.DiscountMultiplierBps, entity.DiscountStartsAt, entity.DiscountEndsAt)
	record := Record{ID: entity.ID, Code: entity.Code, DisplayName: entity.DisplayName, MultiplierBPS: entity.MultiplierBps,
		DiscountName: entity.DiscountName, DiscountMultiplierBPS: entity.DiscountMultiplierBps, DiscountStartsAt: entity.DiscountStartsAt, DiscountEndsAt: entity.DiscountEndsAt,
		EffectiveMultiplierBPS: summary.EffectiveMultiplierBPS, DiscountActive: summary.DiscountActive,
		IsDefault: entity.IsDefault, IsHidden: entity.IsHidden, Status: Status(entity.Status), APIKeyCount: len(entity.Edges.APIKeys), ModelRouteCount: len(entity.Edges.ModelRoutes), AuthorizedUsers: make([]AuthorizedUser, 0, len(entity.Edges.AuthorizedUsers)), CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
	for _, authorized := range entity.Edges.AuthorizedUsers {
		record.AuthorizedUsers = append(record.AuthorizedUsers, AuthorizedUser{ID: authorized.ID, Username: authorized.Username, DisplayName: authorized.DisplayName, Status: string(authorized.Status)})
	}
	return record
}

/**
 * validateAuthorizedUsers 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param tx 本次操作需要使用的输入参数。
 * @param ids 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validateAuthorizedUsers(ctx context.Context, tx *ent.Tx, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	count, err := tx.User.Query().Where(
		entuser.IDIn(ids...),
		entuser.RoleEQ(entuser.RoleMember),
	).Count(ctx)
	if err != nil {
		return fmt.Errorf("validate billing group authorized users: %w", err)
	}
	if count != len(ids) {
		return ErrInvalidInput
	}
	return nil
}
