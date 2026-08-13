package announcement

import "context"

type Store interface {
	AnnouncementConfig(context.Context) (StoredConfig, error)
	SaveAnnouncementConfig(context.Context, Config) (StoredConfig, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Config(ctx context.Context) (Config, error) {
	if s == nil || s.store == nil {
		return Config{}, ErrInvalidInput
	}
	stored, err := s.store.AnnouncementConfig(ctx)
	if err != nil {
		return Config{}, err
	}
	if !stored.Found {
		return DefaultConfig(), nil
	}
	config := stored.Config.Normalize()
	if !config.Validate() {
		return Config{}, ErrInvalidInput
	}
	config.UpdatedAt = &stored.UpdatedAt
	return config, nil
}

func (s *Service) Public(ctx context.Context) (Public, error) {
	config, err := s.Config(ctx)
	if err != nil {
		return Public{}, err
	}
	return config.Public(), nil
}

func (s *Service) Update(ctx context.Context, config Config) (Config, error) {
	config = config.Normalize()
	if s == nil || s.store == nil || !config.Validate() {
		return Config{}, ErrInvalidInput
	}
	stored, err := s.store.SaveAnnouncementConfig(ctx, config)
	if err != nil {
		return Config{}, err
	}
	result := stored.Config.Normalize()
	result.UpdatedAt = &stored.UpdatedAt
	return result, nil
}
