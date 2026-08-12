package gatewaysettings

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryStore struct {
	stored StoredConfig
	err    error
}

func (s *memoryStore) GatewayRequestConfig(context.Context) (StoredConfig, error) {
	if s.err != nil {
		return StoredConfig{}, s.err
	}
	return s.stored, nil
}

func (s *memoryStore) SaveGatewayRequestConfig(_ context.Context, config Config) (StoredConfig, error) {
	if s.err != nil {
		return StoredConfig{}, s.err
	}
	now := time.Now()
	s.stored = StoredConfig{Config: config, UpdatedAt: now, Found: true}
	return s.stored, nil
}

func TestConfigReturnsDefaultsWhenNotStored(t *testing.T) {
	config, err := NewService(&memoryStore{}).Config(context.Background())
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}
	if config != DefaultConfig() {
		t.Fatalf("Config() = %+v, want %+v", config, DefaultConfig())
	}
}

func TestUpdatePersistsValidatedConfig(t *testing.T) {
	store := &memoryStore{}
	want := Config{SSEHeartbeatEnabled: false, SSEHeartbeatIntervalMS: 30_000, UpstreamTimeoutMS: 120_000, UpstreamStreamIdleTimeoutMS: 45_000, ReservationInputTokenCap: 32_768, ReservationOutputTokenCap: 2048}
	got, err := NewService(store).Update(context.Background(), want)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got.SSEHeartbeatEnabled != want.SSEHeartbeatEnabled || got.SSEHeartbeatIntervalMS != want.SSEHeartbeatIntervalMS || got.UpstreamTimeoutMS != want.UpstreamTimeoutMS || got.UpstreamStreamIdleTimeoutMS != want.UpstreamStreamIdleTimeoutMS || got.ReservationInputTokenCap != want.ReservationInputTokenCap || got.ReservationOutputTokenCap != want.ReservationOutputTokenCap || got.UpdatedAt == nil {
		t.Fatalf("Update() = %+v", got)
	}
}

func TestUpdateRejectsInvalidConfig(t *testing.T) {
	store := &memoryStore{}
	_, err := NewService(store).Update(context.Background(), Config{SSEHeartbeatEnabled: true, SSEHeartbeatIntervalMS: 0})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Update() error = %v, want ErrInvalidConfig", err)
	}
}
