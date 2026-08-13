package email

import (
	"context"
	"fmt"

	"github.com/novro-gateway/novro/ent"
	entsmtp "github.com/novro-gateway/novro/ent/emailsmtpconfig"
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
 * Get 用于查询并返回所需的数据。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) Get(ctx context.Context) (StoredConfig, error) {
	entity, err := s.client.EmailSMTPConfig.Query().Where(entsmtp.IDEQ(configID)).Only(ctx)
	if ent.IsNotFound(err) {
		return StoredConfig{}, ErrConfigNotFound
	}
	if err != nil {
		return StoredConfig{}, fmt.Errorf("read SMTP configuration: %w", err)
	}
	return storedConfigFromEnt(entity), nil
}

/**
 * Upsert 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) Upsert(ctx context.Context, input StoredConfigInput) (StoredConfig, error) {
	entity, err := s.client.EmailSMTPConfig.Get(ctx, configID)
	if ent.IsNotFound(err) {
		created, createErr := s.client.EmailSMTPConfig.Create().
			SetID(configID).SetEnabled(input.Enabled).SetHost(input.Host).SetPort(input.Port).
			SetUsername(input.Username).SetEncryptedPassword(input.EncryptedPassword).
			SetFromAddress(input.FromAddress).SetSecurity(input.Security).Save(ctx)
		if createErr == nil {
			return storedConfigFromEnt(created), nil
		}
		entity, err = s.client.EmailSMTPConfig.Get(ctx, configID)
		if err != nil {
			return StoredConfig{}, fmt.Errorf("save SMTP configuration: %w", createErr)
		}
	} else if err != nil {
		return StoredConfig{}, fmt.Errorf("read SMTP configuration before update: %w", err)
	}
	entity, err = entity.Update().
		SetEnabled(input.Enabled).SetHost(input.Host).SetPort(input.Port).
		SetUsername(input.Username).SetEncryptedPassword(input.EncryptedPassword).
		SetFromAddress(input.FromAddress).SetSecurity(input.Security).Save(ctx)
	if err != nil {
		return StoredConfig{}, fmt.Errorf("save SMTP configuration: %w", err)
	}
	return storedConfigFromEnt(entity), nil
}

/**
 * storedConfigFromEnt 封装该名称对应的业务处理逻辑。
 * @param entity 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func storedConfigFromEnt(entity *ent.EmailSMTPConfig) StoredConfig {
	return StoredConfig{ID: entity.ID, Enabled: entity.Enabled, Host: entity.Host, Port: entity.Port, Username: entity.Username, EncryptedPassword: entity.EncryptedPassword, FromAddress: entity.FromAddress, Security: entity.Security, CreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt}
}
