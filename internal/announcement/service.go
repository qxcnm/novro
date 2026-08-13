package announcement

import "context"

type Store interface {
	/**
	 * AnnouncementConfig 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	AnnouncementConfig(context.Context) (StoredConfig, error)
	/**
	 * SaveAnnouncementConfig 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 Config 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	SaveAnnouncementConfig(context.Context, Config) (StoredConfig, error)
}

type Service struct{ store Store }

/**
 * NewService 用于创建并返回所需的对象或记录。
 * @param store 用于持久化和查询数据的存储实现。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewService(store Store) *Service { return &Service{store: store} }

/**
 * Config 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * Public 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Public(ctx context.Context) (Public, error) {
	config, err := s.Config(ctx)
	if err != nil {
		return Public{}, err
	}
	return config.Public(), nil
}

/**
 * Update 用于更新指定的数据或状态。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param config 本次操作使用的配置。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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
