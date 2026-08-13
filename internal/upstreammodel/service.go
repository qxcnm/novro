package upstreammodel

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

type Store interface {
	/**
	 * Create 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 CreateInput 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Create(context.Context, CreateInput) (Record, error)
	/**
	 * List 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 ListFilter 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	List(context.Context, ListFilter) ([]Record, error)
	/**
	 * Update 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 UpdateInput 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Update(context.Context, uuid.UUID, UpdateInput) (Record, error)
	/**
	 * SetStatus 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 Status 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	SetStatus(context.Context, uuid.UUID, Status) (Record, error)
	/**
	 * Delete 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Delete(context.Context, uuid.UUID) error
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
 * Create 用于创建并返回所需的对象或记录。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Create(ctx context.Context, input CreateInput) (Record, error) {
	input.ProviderName = strings.TrimSpace(input.ProviderName)
	input.UpstreamName = strings.TrimSpace(input.UpstreamName)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !validText(input.ProviderName, 128) || !validText(input.UpstreamName, 256) || !validText(input.DisplayName, 128) || !validPrices(input.Prices) {
		return Record{}, ErrInvalidInput
	}
	return s.store.Create(ctx, input)
}

/**
 * List 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) List(ctx context.Context, filter ListFilter) ([]Record, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if filter.Status != "" && filter.Status != StatusActive && filter.Status != StatusDisabled {
		return nil, ErrInvalidInput
	}
	return s.store.List(ctx, filter)
}

/**
 * Update 用于更新指定的数据或状态。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (Record, error) {
	if id == uuid.Nil || emptyUpdate(input) {
		return Record{}, ErrInvalidInput
	}
	for value, max := range map[**string]int{&input.ProviderName: 128, &input.UpstreamName: 256, &input.DisplayName: 128} {
		if *value != nil {
			trimmed := strings.TrimSpace(**value)
			if !validText(trimmed, max) {
				return Record{}, ErrInvalidInput
			}
			**value = trimmed
		}
	}
	for _, price := range []*int64{input.InputMicros, input.OutputMicros, input.CacheReadMicros, input.CacheWriteMicros, input.CacheWrite1hMicros, input.RequestMicros} {
		if price != nil && !validPrice(*price) {
			return Record{}, ErrInvalidInput
		}
	}
	return s.store.Update(ctx, id, input)
}

/**
 * SetStatus 用于更新指定的数据或状态。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @param status 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, status Status) (Record, error) {
	if id == uuid.Nil || (status != StatusActive && status != StatusDisabled) {
		return Record{}, ErrInvalidInput
	}
	return s.store.SetStatus(ctx, id, status)
}

/**
 * Delete 用于删除、撤销或释放指定资源。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidInput
	}
	return s.store.Delete(ctx, id)
}

/**
 * emptyUpdate 封装该名称对应的业务处理逻辑。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func emptyUpdate(input UpdateInput) bool {
	return input.ProviderName == nil && input.UpstreamName == nil && input.DisplayName == nil && input.InputMicros == nil && input.OutputMicros == nil && input.CacheReadMicros == nil && input.CacheWriteMicros == nil && input.CacheWrite1hMicros == nil && input.RequestMicros == nil
}

/**
 * validText 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @param max 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validText(value string, max int) bool {
	return value != "" && utf8.RuneCountInString(value) <= max
}

/**
 * validPrice 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validPrice(value int64) bool { return value >= 0 && value <= 1_000_000_000_000 }

/**
 * validPrices 封装该名称对应的业务处理逻辑。
 * @param p 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validPrices(p Prices) bool {
	return validPrice(p.InputMicros) && validPrice(p.OutputMicros) && validPrice(p.CacheReadMicros) && validPrice(p.CacheWriteMicros) && validPrice(p.CacheWrite1hMicros) && validPrice(p.RequestMicros)
}
