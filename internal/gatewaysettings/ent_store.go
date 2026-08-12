package gatewaysettings

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/novro-gateway/novro/ent"
	entsystemsetting "github.com/novro-gateway/novro/ent/systemsetting"
)

type EntStore struct {
	client *ent.Client
}

func NewEntStore(client *ent.Client) *EntStore {
	return &EntStore{client: client}
}

func (s *EntStore) GatewayRequestConfig(ctx context.Context) (StoredConfig, error) {
	entity, err := s.client.SystemSetting.Query().Where(entsystemsetting.IDEQ(SettingKey)).Only(ctx)
	if ent.IsNotFound(err) {
		return StoredConfig{}, nil
	}
	if err != nil {
		return StoredConfig{}, fmt.Errorf("read gateway request settings: %w", err)
	}
	var config Config
	if err := json.Unmarshal([]byte(entity.Value), &config); err != nil {
		return StoredConfig{}, fmt.Errorf("read gateway request settings: invalid stored value: %w", err)
	}
	config = config.withDefaults()
	return StoredConfig{Config: config, UpdatedAt: entity.UpdatedAt, Found: true}, nil
}

func (s *EntStore) SaveGatewayRequestConfig(ctx context.Context, config Config) (StoredConfig, error) {
	config = config.withDefaults()
	if !config.Validate() {
		return StoredConfig{}, ErrInvalidConfig
	}
	value := Config{
		SSEHeartbeatEnabled:         config.SSEHeartbeatEnabled,
		SSEHeartbeatIntervalMS:      config.SSEHeartbeatIntervalMS,
		UpstreamTimeoutMS:           config.UpstreamTimeoutMS,
		UpstreamStreamIdleTimeoutMS: config.UpstreamStreamIdleTimeoutMS,
		ReservationInputTokenCap:    config.ReservationInputTokenCap,
		ReservationOutputTokenCap:   config.ReservationOutputTokenCap,
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return StoredConfig{}, fmt.Errorf("encode gateway request settings: %w", err)
	}
	entity, err := s.client.SystemSetting.Query().Where(entsystemsetting.IDEQ(SettingKey)).Only(ctx)
	if ent.IsNotFound(err) {
		entity, err = s.client.SystemSetting.Create().SetID(SettingKey).SetValue(string(encoded)).Save(ctx)
	} else if err == nil {
		entity, err = s.client.SystemSetting.UpdateOne(entity).SetValue(string(encoded)).Save(ctx)
	}
	if err != nil {
		return StoredConfig{}, fmt.Errorf("save gateway request settings: %w", err)
	}
	return StoredConfig{Config: value, UpdatedAt: entity.UpdatedAt, Found: true}, nil
}
