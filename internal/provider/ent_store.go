package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entmodelroute "github.com/novro-gateway/novro/ent/modelroute"
	entprovider "github.com/novro-gateway/novro/ent/provider"
)

type EntStore struct {
	client *ent.Client
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
 * @param params 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) Create(ctx context.Context, params CreateParams) (Record, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin provider creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	created, err := tx.Provider.Create().
		SetCode(params.Code).
		SetDisplayName(params.DisplayName).
		SetProtocol(entprovider.Protocol(params.Protocol)).
		SetBaseURL(params.BaseURL).
		SetModelListPath(params.ModelListPath).
		SetWeight(params.Weight).
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
	return fromEnt(created), nil
}

/**
 * List 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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
		return Record{}, fmt.Errorf("begin provider update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	update := tx.Provider.UpdateOneID(id).Where(entprovider.DeletedAtIsNil())
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
	if params.Weight != nil {
		update.SetWeight(*params.Weight)
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

/**
 * SetStatus 用于更新指定的数据或状态。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @param status 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * fromEnt 封装该名称对应的业务处理逻辑。
 * @param entity 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func fromEnt(entity *ent.Provider) Record {
	record := Record{
		ID: entity.ID, Code: entity.Code, DisplayName: entity.DisplayName,
		Protocol: Protocol(entity.Protocol), BaseURL: entity.BaseURL, ModelListPath: entity.ModelListPath,
		Weight:     entity.Weight,
		APIKeyHint: entity.APIKeyHint, HasAPIKey: entity.EncryptedAPIKey != "",
		Status: Status(entity.Status), CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,
	}
	return record
}

/**
 * get 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) get(ctx context.Context, id uuid.UUID) (Record, error) {
	entity, err := s.client.Provider.Query().Where(entprovider.IDEQ(id), entprovider.DeletedAtIsNil()).Only(ctx)
	if ent.IsNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read provider: %w", err)
	}
	return fromEnt(entity), nil
}
