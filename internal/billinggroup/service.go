package billinggroup

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

var codePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

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
 * NewService 执行该名称对应的业务处理逻辑。
 * @param store 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewService(store Store) *Service { return &Service{store: store} }

/**
 * Create 执行该名称对应的业务处理逻辑。
 * @param ctx 本次操作需要使用的输入参数。
 * @param input 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Create(ctx context.Context, input CreateInput) (Record, error) {
	input.Code = strings.ToLower(strings.TrimSpace(input.Code))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !codePattern.MatchString(input.Code) || !validName(input.DisplayName) || !validMultiplier(input.MultiplierBPS) || !validAuthorizedUserIDs(input.AuthorizedUserIDs) || (!input.IsHidden && len(input.AuthorizedUserIDs) > 0) {
		return Record{}, ErrInvalidInput
	}
	return s.store.Create(ctx, input)
}

/**
 * List 执行该名称对应的业务处理逻辑。
 * @param ctx 本次操作需要使用的输入参数。
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
 * Update 执行该名称对应的业务处理逻辑。
 * @param ctx 本次操作需要使用的输入参数。
 * @param id 本次操作需要使用的输入参数。
 * @param input 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Update(ctx context.Context, id uuid.UUID, input UpdateInput) (Record, error) {
	if id == uuid.Nil || (input.DisplayName == nil && input.MultiplierBPS == nil && input.IsHidden == nil && input.AuthorizedUserIDs == nil) {
		return Record{}, ErrInvalidInput
	}
	if input.DisplayName != nil {
		value := strings.TrimSpace(*input.DisplayName)
		if !validName(value) {
			return Record{}, ErrInvalidInput
		}
		input.DisplayName = &value
	}
	if input.MultiplierBPS != nil && !validMultiplier(*input.MultiplierBPS) {
		return Record{}, ErrInvalidInput
	}
	if input.AuthorizedUserIDs != nil && !validAuthorizedUserIDs(*input.AuthorizedUserIDs) {
		return Record{}, ErrInvalidInput
	}
	return s.store.Update(ctx, id, input)
}

/**
 * SetStatus 执行该名称对应的业务处理逻辑。
 * @param ctx 本次操作需要使用的输入参数。
 * @param id 本次操作需要使用的输入参数。
 * @param status 本次操作需要使用的输入参数。
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
 * Delete 执行该名称对应的业务处理逻辑。
 * @param ctx 本次操作需要使用的输入参数。
 * @param id 本次操作需要使用的输入参数。
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
 * validName 执行该名称对应的业务处理逻辑。
 * @param value 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validName(value string) bool      { return value != "" && utf8.RuneCountInString(value) <= 128 }
/**
 * validMultiplier 执行该名称对应的业务处理逻辑。
 * @param value 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validMultiplier(value int64) bool { return value >= 1 && value <= 1_000_000 }

/**
 * validAuthorizedUserIDs 执行该名称对应的业务处理逻辑。
 * @param ids 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validAuthorizedUserIDs(ids []uuid.UUID) bool {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return false
		}
		if _, exists := seen[id]; exists {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}
