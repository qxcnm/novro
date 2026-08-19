package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	"github.com/novro-gateway/novro/ent/systemsetting"
	entuser "github.com/novro-gateway/novro/ent/user"
	"github.com/novro-gateway/novro/ent/usersession"
)

type EntStore struct {
	client *ent.Client
}

const initialAdminSettingKey = "initial_admin_created"

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
 * @param params 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) Create(ctx context.Context, params CreateParams) (Record, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin user creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	create := tx.User.Create().
		SetUsername(params.Username).
		SetEmail(params.Email).
		SetDisplayName(params.DisplayName).
		SetPasswordHash(params.PasswordHash).
		SetIsSystemAdmin(params.IsSystemAdmin).
		SetRole(entuser.Role(params.Role)).
		SetStatus(entuser.StatusActive)
	if params.ReferralCode != "" {
		referrer, referrerErr := tx.User.Query().
			Where(entuser.InviteCodeEQ(params.ReferralCode), entuser.StatusEQ(entuser.StatusActive)).
			Only(ctx)
		if ent.IsNotFound(referrerErr) {
			return Record{}, ErrInvalidReferralCode
		}
		if referrerErr != nil {
			return Record{}, fmt.Errorf("resolve referral code: %w", referrerErr)
		}
		create.SetReferredByUserID(referrer.ID)
	}
	created, err := create.Save(ctx)
	if ent.IsConstraintError(err) {
		return Record{}, identityConstraintError(err)
	}
	if err != nil {
		return Record{}, fmt.Errorf("create user: %w", err)
	}
	if _, err := tx.Wallet.Create().SetUserID(created.ID).Save(ctx); err != nil {
		return Record{}, fmt.Errorf("create user wallet: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("commit user creation: %w", err)
	}
	return fromEnt(created), nil
}

/**
 * CreateInitialAdmin 用于创建并返回所需的对象或记录。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param params 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) CreateInitialAdmin(ctx context.Context, params CreateParams) (Record, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin administrator initialization: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.SystemSetting.Create().
		SetID(initialAdminSettingKey).
		SetValue("complete").
		Save(ctx); err != nil {
		if ent.IsConstraintError(err) {
			return Record{}, ErrAlreadyInitialized
		}
		return Record{}, fmt.Errorf("lock administrator initialization: %w", err)
	}
	count, err := tx.User.Query().Count(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("count users during initialization: %w", err)
	}
	if count != 0 {
		return Record{}, ErrAlreadyInitialized
	}
	created, err := tx.User.Create().
		SetUsername(params.Username).
		SetEmail(params.Email).
		SetDisplayName(params.DisplayName).
		SetPasswordHash(params.PasswordHash).
		SetIsSystemAdmin(true).
		SetRole(entuser.RoleAdmin).
		SetStatus(entuser.StatusActive).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return Record{}, ErrAlreadyInitialized
	}
	if err != nil {
		return Record{}, fmt.Errorf("create initial administrator: %w", err)
	}
	if _, err := tx.Wallet.Create().SetUserID(created.ID).Save(ctx); err != nil {
		return Record{}, fmt.Errorf("create initial administrator wallet: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("commit administrator initialization: %w", err)
	}
	return fromEnt(created), nil
}

/**
 * EmailExists 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param email 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) EmailExists(ctx context.Context, email string) (bool, error) {
	exists, err := s.client.User.Query().Where(entuser.EmailEQ(email)).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("check existing user email: %w", err)
	}
	return exists, nil
}

/**
 * FindByUsername 用于查询并返回所需的数据。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param username 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) FindByUsername(ctx context.Context, username string) (Record, error) {
	entity, err := s.client.User.Query().Where(entuser.UsernameEQ(username)).Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("find user by username: %w", err)
	}
	return fromEnt(entity), nil
}

/**
 * IsInitialized 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) IsInitialized(ctx context.Context) (bool, error) {
	marked, err := s.client.SystemSetting.Query().
		Where(systemsetting.IDEQ(initialAdminSettingKey)).
		Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("query administrator initialization marker: %w", err)
	}
	if marked {
		return true, nil
	}
	exists, err := s.client.User.Query().Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("query existing users: %w", err)
	}
	return exists, nil
}

/**
 * List 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) List(ctx context.Context, filter ListFilter) (Page, error) {
	query := s.client.User.Query()
	if filter.Search != "" {
		query = query.Where(entuser.Or(
			entuser.UsernameContainsFold(filter.Search),
			entuser.EmailContainsFold(filter.Search),
			entuser.DisplayNameContainsFold(filter.Search),
		))
	}
	if filter.Status != "" {
		query = query.Where(entuser.StatusEQ(entuser.Status(filter.Status)))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("count users: %w", err)
	}
	entities, err := query.
		WithWallet().
		Order(ent.Desc(entuser.FieldCreatedAt)).
		Offset(filter.Offset).
		Limit(filter.Limit).
		All(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("list users: %w", err)
	}
	records := make([]AdminRecord, 0, len(entities))
	for _, entity := range entities {
		records = append(records, adminRecordFromEnt(entity))
	}
	return Page{Users: records, Total: total, Offset: filter.Offset, Limit: filter.Limit}, nil
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
		return Record{}, fmt.Errorf("begin user update transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	entity, err := tx.User.Query().Where(entuser.IDEQ(id)).Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read user for update: %w", err)
	}
	if params.Role != nil && entity.IsSystemAdmin && *params.Role != RoleAdmin {
		return Record{}, ErrProtectedAdmin
	}
	if params.Role != nil && entity.Role == entuser.RoleAdmin && entity.Status == entuser.StatusActive && *params.Role == RoleMember {
		activeAdminIDs, err := tx.User.Query().
			Where(entuser.RoleEQ(entuser.RoleAdmin), entuser.StatusEQ(entuser.StatusActive)).
			ForUpdate().
			IDs(ctx)
		if err != nil {
			return Record{}, fmt.Errorf("lock active administrators: %w", err)
		}
		if len(activeAdminIDs) <= 1 {
			return Record{}, ErrLastActiveAdmin
		}
	}

	update := tx.User.UpdateOneID(id)
	if params.DisplayName != nil {
		update.SetDisplayName(*params.DisplayName)
	}
	if params.Role != nil {
		update.SetRole(entuser.Role(*params.Role))
		if *params.Role == RoleAdmin {
			update.ClearAuthorizedBillingGroups()
		}
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("update user: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("commit user update: %w", err)
	}
	return fromEnt(updated), nil
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
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin user status transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	entity, err := tx.User.Query().Where(entuser.IDEQ(id)).Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read user for status update: %w", err)
	}
	if entity.IsSystemAdmin && status != StatusActive {
		return Record{}, ErrProtectedAdmin
	}
	if entity.Role == entuser.RoleAdmin && entity.Status == entuser.StatusActive && status == StatusDisabled {
		activeAdminIDs, err := tx.User.Query().
			Where(entuser.RoleEQ(entuser.RoleAdmin), entuser.StatusEQ(entuser.StatusActive)).
			ForUpdate().
			IDs(ctx)
		if err != nil {
			return Record{}, fmt.Errorf("lock active administrators: %w", err)
		}
		if len(activeAdminIDs) <= 1 {
			return Record{}, ErrLastActiveAdmin
		}
	}

	updated, err := tx.User.UpdateOneID(id).SetStatus(entuser.Status(status)).Save(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("update user status: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("commit user status: %w", err)
	}
	return fromEnt(updated), nil
}

/**
 * ResetPassword 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @param passwordHash 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) ResetPassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin password reset transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	updated, err := tx.User.UpdateOneID(id).SetPasswordHash(passwordHash).Save(ctx)
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.UserSession.Update().
		Where(usersession.UserIDEQ(updated.ID), usersession.RevokedAtIsNil()).
		SetRevokedAt(now).
		Save(ctx); err != nil {
		return fmt.Errorf("revoke sessions after password reset: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit password reset: %w", err)
	}
	return nil
}

/**
 * fromEnt 封装该名称对应的业务处理逻辑。
 * @param entity 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func fromEnt(entity *ent.User) Record {
	if entity == nil {
		return Record{}
	}
	record := Record{
		ID:            entity.ID,
		Username:      entity.Username,
		DisplayName:   entity.DisplayName,
		Role:          Role(entity.Role),
		Status:        Status(entity.Status),
		IsSystemAdmin: entity.IsSystemAdmin,
		LastLoginAt:   entity.LastLoginAt,
		CreatedAt:     entity.CreatedAt,
		UpdatedAt:     entity.UpdatedAt,
	}
	if entity.Email != nil {
		record.Email = *entity.Email
	}
	return record
}

func adminRecordFromEnt(entity *ent.User) AdminRecord {
	record := AdminRecord{Record: fromEnt(entity)}
	if entity != nil && entity.Edges.Wallet != nil {
		record.BalanceMicros = entity.Edges.Wallet.BalanceMicros
	}
	return record
}

/**
 * mapEntError 封装该名称对应的业务处理逻辑。
 * @param err 需要处理或转换的错误。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func mapEntError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrUsernameTaken), errors.Is(err, ErrEmailTaken):
		return err
	case ent.IsNotFound(err):
		return ErrNotFound
	case ent.IsConstraintError(err):
		return identityConstraintError(err)
	default:
		return err
	}
}

/**
 * identityConstraintError 封装该名称对应的业务处理逻辑。
 * @param err 需要处理或转换的错误。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func identityConstraintError(err error) error {
	if strings.Contains(strings.ToLower(err.Error()), "email") {
		return ErrEmailTaken
	}
	return ErrUsernameTaken
}
