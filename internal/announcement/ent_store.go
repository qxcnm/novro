package announcement

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/novro-gateway/novro/ent"
	entsystemsetting "github.com/novro-gateway/novro/ent/systemsetting"
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
 * AnnouncementConfig 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) AnnouncementConfig(ctx context.Context) (StoredConfig, error) {
	if s == nil || s.client == nil {
		return StoredConfig{}, fmt.Errorf("announcement store is unavailable")
	}
	entity, err := s.client.SystemSetting.Query().Where(entsystemsetting.IDEQ(SettingKey)).Only(ctx)
	if ent.IsNotFound(err) {
		return StoredConfig{}, nil
	}
	if err != nil {
		return StoredConfig{}, fmt.Errorf("read system announcement: %w", err)
	}
	var config Config
	if err := json.Unmarshal([]byte(entity.Value), &config); err != nil {
		return StoredConfig{}, fmt.Errorf("read system announcement: invalid stored value: %w", err)
	}
	return StoredConfig{Config: config.Normalize(), UpdatedAt: entity.UpdatedAt, Found: true}, nil
}

/**
 * SaveAnnouncementConfig 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param config 本次操作使用的配置。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *EntStore) SaveAnnouncementConfig(ctx context.Context, config Config) (StoredConfig, error) {
	config = config.Normalize()
	if !config.Validate() {
		return StoredConfig{}, ErrInvalidInput
	}
	encoded, err := json.Marshal(Config{Enabled: config.Enabled, Title: config.Title, Body: config.Body})
	if err != nil {
		return StoredConfig{}, fmt.Errorf("encode system announcement: %w", err)
	}
	entity, err := s.client.SystemSetting.Query().Where(entsystemsetting.IDEQ(SettingKey)).Only(ctx)
	if ent.IsNotFound(err) {
		entity, err = s.client.SystemSetting.Create().SetID(SettingKey).SetValue(string(encoded)).Save(ctx)
	} else if err == nil {
		entity, err = s.client.SystemSetting.UpdateOne(entity).SetValue(string(encoded)).Save(ctx)
	}
	if err != nil {
		return StoredConfig{}, fmt.Errorf("save system announcement: %w", err)
	}
	return StoredConfig{Config: config, UpdatedAt: entity.UpdatedAt, Found: true}, nil
}
