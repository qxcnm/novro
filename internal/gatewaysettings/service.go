package gatewaysettings

import "context"

type Store interface {
	GatewayRequestConfig(context.Context) (StoredConfig, error)
	SaveGatewayRequestConfig(context.Context, Config) (StoredConfig, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Config(ctx context.Context) (Config, error) {
	if s == nil || s.store == nil {
		return Config{}, ErrInvalidConfig
	}
	stored, err := s.store.GatewayRequestConfig(ctx)
	if err != nil {
		return Config{}, err
	}
	if !stored.Found {
		return DefaultConfig(), nil
	}
	if !stored.Config.Validate() {
		return Config{}, ErrInvalidConfig
	}
	config := stored.Config
	config.UpdatedAt = &stored.UpdatedAt
	return config, nil
}

func (s *Service) Update(ctx context.Context, config Config) (Config, error) {
	if s == nil || s.store == nil || !config.Validate() {
		return Config{}, ErrInvalidConfig
	}
	stored, err := s.store.SaveGatewayRequestConfig(ctx, config)
	if err != nil {
		return Config{}, err
	}
	result := stored.Config
	result.UpdatedAt = &stored.UpdatedAt
	return result, nil
}
