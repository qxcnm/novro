package gatewaysettings

import "context"

type Store interface {
	/**
	 * GatewayRequestConfig 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	GatewayRequestConfig(context.Context) (StoredConfig, error)
	/**
	 * SaveGatewayRequestConfig 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 Config 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	SaveGatewayRequestConfig(context.Context, Config) (StoredConfig, error)
}

type Service struct {
	store Store
}

/**
 * NewService 用于创建并返回所需的对象或记录。
 * @param store 用于持久化和查询数据的存储实现。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewService(store Store) *Service {
	return &Service{store: store}
}

/**
 * Config 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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
	stored.Config = stored.Config.withDefaults()
	if !stored.Config.Validate() {
		return Config{}, ErrInvalidConfig
	}
	config := stored.Config
	config.UpdatedAt = &stored.UpdatedAt
	return config, nil
}

/**
 * Update 用于更新指定的数据或状态。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param config 本次操作使用的配置。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Update(ctx context.Context, config Config) (Config, error) {
	config = config.withDefaults()
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
