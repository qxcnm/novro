package apikey

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entapikey "github.com/novro-gateway/novro/ent/apikey"
	entbillinggroup "github.com/novro-gateway/novro/ent/billinggroup"
	"github.com/novro-gateway/novro/ent/predicate"
	entuser "github.com/novro-gateway/novro/ent/user"
	"github.com/novro-gateway/novro/internal/billinggroup"
	"github.com/novro-gateway/novro/internal/user"
)

type EntStore struct {
	client *ent.Client
}

/**
 * AuthenticateHash 用于校验用户凭据并建立登录会话。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param hash 控制对应行为是否启用的布尔值。
 * @param now 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) AuthenticateHash(ctx context.Context, hash string, now time.Time) (Actor, error) {
	entity, err := s.client.APIKey.Query().
		Where(
			entapikey.KeyHashEQ(hash),
			entapikey.StatusEQ(entapikey.StatusActive),
			entapikey.HasUserWith(entuser.StatusEQ(entuser.StatusActive)),
			entapikey.HasBillingGroupWith(entbillinggroup.StatusEQ(entbillinggroup.StatusActive), entbillinggroup.DeletedAtIsNil()),
		).
		WithUser().WithBillingGroup(func(query *ent.BillingGroupQuery) { query.WithCompositions() }).Only(ctx)
	if ent.IsNotFound(err) {
		return Actor{}, ErrUnauthenticated
	}
	if err != nil {
		return Actor{}, fmt.Errorf("authenticate API key: %w", err)
	}
	owner, err := entity.Edges.UserOrErr()
	if err != nil {
		return Actor{}, fmt.Errorf("read API key owner: %w", err)
	}
	group, err := entity.Edges.BillingGroupOrErr()
	if err != nil {
		return Actor{}, fmt.Errorf("read API key billing group: %w", err)
	}
	if group.IsHidden && owner.Role != entuser.RoleAdmin {
		authorized, err := group.QueryAuthorizedUsers().Where(entuser.IDEQ(owner.ID)).Exist(ctx)
		if err != nil {
			return Actor{}, fmt.Errorf("check API key billing group authorization: %w", err)
		}
		if !authorized {
			return Actor{}, ErrUnauthenticated
		}
	}
	if _, err := s.client.APIKey.UpdateOneID(entity.ID).SetLastUsedAt(now).Save(ctx); err != nil {
		return Actor{}, fmt.Errorf("update API key last use: %w", err)
	}
	userRecord := user.Record{ID: owner.ID, Username: owner.Username, DisplayName: owner.DisplayName, Role: user.Role(owner.Role), Status: user.Status(owner.Status), IsSystemAdmin: owner.IsSystemAdmin, LastLoginAt: owner.LastLoginAt, CreatedAt: owner.CreatedAt, UpdatedAt: owner.UpdatedAt}
	return Actor{APIKey: fromEnt(entity), User: userRecord}, nil
}

/**
 * NewEntStore 用于创建并返回所需的对象或记录。
 * @param client 用于访问外部或底层服务的客户端。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

/**
 * Create 用于创建并返回所需的对象或记录。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param billingGroupID 目标资源的一个或多个唯一标识。
 * @param name 用于标识或筛选目标的文本值。
 * @param prefix 本次操作需要使用的输入参数。
 * @param hash 控制对应行为是否启用的布尔值。
 * @param encryptedSecret 本次操作需要使用的输入参数。
 * @param maxActive 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) Create(ctx context.Context, userID, billingGroupID uuid.UUID, name, prefix, hash, encryptedSecret string, maxActive int) (Record, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin API key creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	owner, err := tx.User.Query().Where(entuser.IDEQ(userID), entuser.StatusEQ(entuser.StatusActive)).ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	} else if err != nil {
		return Record{}, fmt.Errorf("lock API key owner: %w", err)
	}
	group, err := tx.BillingGroup.Query().Where(entbillinggroup.IDEQ(billingGroupID), entbillinggroup.StatusEQ(entbillinggroup.StatusActive), entbillinggroup.DeletedAtIsNil()).WithCompositions().ForUpdate().Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrGroupUnavailable
	}
	if err != nil {
		return Record{}, fmt.Errorf("lock API key billing group: %w", err)
	}
	if group.Kind == entbillinggroup.KindComposite && len(group.Edges.Compositions) == 0 {
		return Record{}, ErrGroupUnavailable
	}
	if group.IsHidden && owner.Role != entuser.RoleAdmin {
		authorized, err := group.QueryAuthorizedUsers().Where(entuser.IDEQ(owner.ID)).Exist(ctx)
		if err != nil {
			return Record{}, fmt.Errorf("check API key billing group authorization: %w", err)
		}
		if !authorized {
			return Record{}, ErrGroupUnavailable
		}
	}
	count, err := tx.APIKey.Query().Where(entapikey.UserIDEQ(userID), entapikey.StatusEQ(entapikey.StatusActive)).Count(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("count active API keys: %w", err)
	}
	if count >= maxActive {
		return Record{}, ErrLimitReached
	}
	created, err := tx.APIKey.Create().
		SetUserID(userID).
		SetBillingGroupID(group.ID).
		SetName(name).
		SetKeyPrefix(prefix).
		SetKeyHash(hash).
		SetKeySecretCiphertext(encryptedSecret).
		SetStatus(entapikey.StatusActive).
		Save(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("create API key: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("commit API key creation: %w", err)
	}
	created.Edges.BillingGroup = group
	return fromEnt(created), nil
}

/**
 * ListByUser 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]Record, error) {
	entities, err := s.client.APIKey.Query().
		Where(entapikey.UserIDEQ(userID), accessibleBillingGroup(userID)).
		WithBillingGroup(func(query *ent.BillingGroupQuery) { query.WithCompositions() }).
		Order(ent.Desc(entapikey.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list user API keys: %w", err)
	}
	records := make([]Record, 0, len(entities))
	for _, entity := range entities {
		records = append(records, fromEnt(entity))
	}
	return records, nil
}

/**
 * GetByUser 用于查询并返回所需的数据。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) GetByUser(ctx context.Context, userID, id uuid.UUID) (Record, error) {
	entity, err := s.client.APIKey.Query().
		Where(entapikey.IDEQ(id), entapikey.UserIDEQ(userID), entapikey.StatusEQ(entapikey.StatusActive), accessibleBillingGroup(userID)).
		WithBillingGroup(func(query *ent.BillingGroupQuery) { query.WithCompositions() }).
		Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("find user API key: %w", err)
	}
	return fromEnt(entity), nil
}

/**
 * RevokeByUser 用于删除、撤销或释放指定资源。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param id 目标资源的唯一标识。
 * @param now 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) RevokeByUser(ctx context.Context, userID, id uuid.UUID, now time.Time) error {
	entity, err := s.client.APIKey.Query().Where(entapikey.IDEQ(id), entapikey.UserIDEQ(userID), accessibleBillingGroup(userID)).Only(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find user API key: %w", err)
	}
	return s.revoke(ctx, entity, now)
}

/**
 * ListAll 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) ListAll(ctx context.Context, filter ListFilter) (Page, error) {
	query := s.client.APIKey.Query()
	if filter.Search != "" {
		query = query.Where(entapikey.Or(
			entapikey.NameContainsFold(filter.Search),
			entapikey.KeyPrefixContainsFold(filter.Search),
			entapikey.HasUserWith(entuser.Or(
				entuser.UsernameContainsFold(filter.Search),
				entuser.DisplayNameContainsFold(filter.Search),
			)),
		))
	}
	if filter.Status != "" {
		query = query.Where(entapikey.StatusEQ(entapikey.Status(filter.Status)))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("count API keys: %w", err)
	}
	entities, err := query.WithUser().WithBillingGroup(func(query *ent.BillingGroupQuery) { query.WithCompositions() }).
		Order(ent.Desc(entapikey.FieldCreatedAt)).
		Offset(filter.Offset).
		Limit(filter.Limit).
		All(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("list API keys: %w", err)
	}
	records := make([]AdminRecord, 0, len(entities))
	for _, entity := range entities {
		owner, err := entity.Edges.UserOrErr()
		if err != nil {
			return Page{}, fmt.Errorf("read API key owner: %w", err)
		}
		records = append(records, AdminRecord{
			Record: fromEnt(entity),
			Owner:  Owner{ID: owner.ID, Username: owner.Username, DisplayName: owner.DisplayName},
		})
	}
	return Page{APIKeys: records, Total: total, Offset: filter.Offset, Limit: filter.Limit}, nil
}

/**
 * Revoke 用于删除、撤销或释放指定资源。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @param now 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) Revoke(ctx context.Context, id uuid.UUID, now time.Time) error {
	entity, err := s.client.APIKey.Get(ctx, id)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find API key: %w", err)
	}
	return s.revoke(ctx, entity, now)
}

/**
 * revoke 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param entity 本次操作需要使用的输入参数。
 * @param now 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) revoke(ctx context.Context, entity *ent.APIKey, now time.Time) error {
	if entity.Status == entapikey.StatusRevoked {
		return nil
	}
	if _, err := entity.Update().SetStatus(entapikey.StatusRevoked).SetRevokedAt(now).Save(ctx); err != nil {
		return fmt.Errorf("revoke API key: %w", err)
	}
	return nil
}

/**
 * fromEnt 封装该名称对应的业务处理逻辑。
 * @param entity 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func fromEnt(entity *ent.APIKey) Record {
	record := Record{
		ID:                  entity.ID,
		UserID:              entity.UserID,
		BillingGroupID:      entity.BillingGroupID,
		Name:                entity.Name,
		KeyPrefix:           entity.KeyPrefix,
		CanCopySecret:       entity.KeySecretCiphertext != "" && entity.Status == entapikey.StatusActive,
		Status:              Status(entity.Status),
		LastUsedAt:          entity.LastUsedAt,
		CreatedAt:           entity.CreatedAt,
		RevokedAt:           entity.RevokedAt,
		KeySecretCiphertext: entity.KeySecretCiphertext,
	}
	if group, err := entity.Edges.BillingGroupOrErr(); err == nil {
		memberIDs := make([]uuid.UUID, 0, len(group.Edges.Compositions))
		for _, composition := range group.Edges.Compositions {
			memberIDs = append(memberIDs, composition.MemberGroupID)
		}
		record.BillingGroup = billinggroup.NewSummaryWithKind(group.ID, group.Code, group.DisplayName, billinggroup.Kind(group.Kind), group.MultiplierBps, group.DiscountName, group.DiscountMultiplierBps, group.DiscountStartsAt, group.DiscountEndsAt, memberIDs)
	}
	return record
}

/**
 * accessibleBillingGroup 封装该名称对应的业务处理逻辑。
 * @param userID 目标用户的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func accessibleBillingGroup(userID uuid.UUID) predicate.APIKey {
	return entapikey.Or(
		entapikey.HasBillingGroupWith(entbillinggroup.IsHiddenEQ(false)),
		entapikey.HasUserWith(entuser.RoleEQ(entuser.RoleAdmin)),
		entapikey.HasBillingGroupWith(entbillinggroup.HasAuthorizedUsersWith(entuser.IDEQ(userID))),
	)
}
