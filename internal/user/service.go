package user

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var (
	usernamePattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)
	emailPattern        = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	referralCodePattern = regexp.MustCompile(`^[A-Z0-9]{12}$`)
)

type Store interface {
	/**
	 * Create 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 CreateParams 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Create(context.Context, CreateParams) (Record, error)
	/**
	 * CreateInitialAdmin 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 CreateParams 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	CreateInitialAdmin(context.Context, CreateParams) (Record, error)
	/**
	 * EmailExists 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	EmailExists(context.Context, string) (bool, error)
	/**
	 * IsInitialized 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	IsInitialized(context.Context) (bool, error)
	/**
	 * List 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 ListFilter 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	List(context.Context, ListFilter) (Page, error)
	/**
	 * Update 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 UpdateParams 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Update(context.Context, uuid.UUID, UpdateParams) (Record, error)
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
	 * ResetPassword 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	ResetPassword(context.Context, uuid.UUID, string) error
	/**
	 * FindByUsername 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	FindByUsername(context.Context, string) (Record, error)
}

type PasswordHasher interface {
	/**
	 * Hash 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Hash(string) (string, error)
}

type CreateParams struct {
	Username      string
	Email         string
	DisplayName   string
	PasswordHash  string
	Role          Role
	ReferralCode  string
	IsSystemAdmin bool
}

type UpdateParams struct {
	DisplayName *string
	Role        *Role
}

type Service struct {
	store  Store
	hasher PasswordHasher
}

/**
 * NewService 用于创建并返回所需的对象或记录。
 * @param store 用于持久化和查询数据的存储实现。
 * @param hasher 控制对应行为是否启用的布尔值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewService(store Store, hasher PasswordHasher) *Service {
	return &Service{store: store, hasher: hasher}
}

/**
 * Create 用于创建并返回所需的对象或记录。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Create(ctx context.Context, input CreateInput) (Record, error) {
	return s.create(ctx, input, s.store.Create)
}

/**
 * Register 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Register(ctx context.Context, input RegisterInput) (Record, error) {
	referralCode := strings.ToUpper(strings.TrimSpace(input.ReferralCode))
	if referralCode != "" && !referralCodePattern.MatchString(referralCode) {
		return Record{}, ErrInvalidReferralCode
	}
	return s.create(ctx, CreateInput{
		Username: input.Username, Email: input.Email, DisplayName: input.Username, Password: input.Password, Role: RoleMember,
	}, func(ctx context.Context, params CreateParams) (Record, error) {
		params.ReferralCode = referralCode
		return s.store.Create(ctx, params)
	})
}

/**
 * InitializeAdmin 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) InitializeAdmin(ctx context.Context, input RegisterInput) (Record, error) {
	return s.create(ctx, CreateInput{
		Username: input.Username, Email: input.Email, DisplayName: input.DisplayName, Password: input.Password, Role: RoleAdmin,
	}, s.store.CreateInitialAdmin)
}

/**
 * EmailAvailable 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param rawEmail 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) EmailAvailable(ctx context.Context, rawEmail string) (bool, error) {
	email, ok := NormalizeEmail(rawEmail)
	if !ok {
		return false, ErrInvalidInput
	}
	exists, err := s.store.EmailExists(ctx, email)
	if err != nil {
		return false, fmt.Errorf("check email availability: %w", err)
	}
	return !exists, nil
}

/**
 * SetupRequired 用于更新指定的数据或状态。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	initialized, err := s.store.IsInitialized(ctx)
	if err != nil {
		return false, fmt.Errorf("check installation state: %w", err)
	}
	return !initialized, nil
}

/**
 * create 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param input 需要处理的输入数据。
 * @param save 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) create(ctx context.Context, input CreateInput, save func(context.Context, CreateParams) (Record, error)) (Record, error) {
	username := strings.ToLower(strings.TrimSpace(input.Username))
	email, emailOK := NormalizeEmail(input.Email)
	displayName := strings.TrimSpace(input.DisplayName)
	if !usernamePattern.MatchString(username) || !emailOK || len([]rune(displayName)) > 128 {
		return Record{}, ErrInvalidInput
	}
	role := input.Role
	if role == "" {
		role = RoleMember
	}
	if role != RoleAdmin && role != RoleMember {
		return Record{}, ErrInvalidInput
	}
	passwordHash, err := s.hasher.Hash(input.Password)
	if err != nil {
		return Record{}, fmt.Errorf("%w: password: %v", ErrInvalidInput, err)
	}
	return save(ctx, CreateParams{
		Username:     username,
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
		Role:         role,
	})
}

/**
 * NormalizeEmail 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NormalizeEmail(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	return value, value != "" && len(value) <= 320 && emailPattern.MatchString(value)
}

/**
 * List 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) List(ctx context.Context, filter ListFilter) (Page, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	if filter.Status != "" && filter.Status != StatusActive && filter.Status != StatusDisabled {
		return Page{}, ErrInvalidInput
	}
	if filter.Offset < 0 {
		return Page{}, ErrInvalidInput
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		return Page{}, ErrInvalidInput
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
	if id == uuid.Nil || (input.DisplayName == nil && input.Role == nil) {
		return Record{}, ErrInvalidInput
	}
	params := UpdateParams{Role: input.Role}
	if input.DisplayName != nil {
		displayName := strings.TrimSpace(*input.DisplayName)
		if len([]rune(displayName)) > 128 {
			return Record{}, ErrInvalidInput
		}
		params.DisplayName = &displayName
	}
	if input.Role != nil && *input.Role != RoleAdmin && *input.Role != RoleMember {
		return Record{}, ErrInvalidInput
	}
	return s.store.Update(ctx, id, params)
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
 * ResetPassword 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @param plainText 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) ResetPassword(ctx context.Context, id uuid.UUID, plainText string) error {
	if id == uuid.Nil {
		return ErrInvalidInput
	}
	passwordHash, err := s.hasher.Hash(plainText)
	if err != nil {
		return fmt.Errorf("%w: password: %v", ErrInvalidInput, err)
	}
	return s.store.ResetPassword(ctx, id, passwordHash)
}

/**
 * FindByUsername 用于查询并返回所需的数据。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param username 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) FindByUsername(ctx context.Context, username string) (Record, error) {
	return s.store.FindByUsername(ctx, strings.ToLower(strings.TrimSpace(username)))
}
