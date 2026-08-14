package provider

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

var providerCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,62}[a-z0-9]$`)

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
	 * Delete 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Delete(context.Context, uuid.UUID) error
}

type CreateParams struct {
	Code            string
	DisplayName     string
	Protocol        Protocol
	BaseURL         string
	ModelListPath   string
	Weight          int
	EncryptedAPIKey string
	APIKeyHint      string
}

type Service struct {
	store  Store
	cipher *Cipher
}

/**
 * NewService 用于创建并返回所需的对象或记录。
 * @param store 用于持久化和查询数据的存储实现。
 * @param cipher 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewService(store Store, cipher *Cipher) *Service {
	return &Service{store: store, cipher: cipher}
}

/**
 * Create 用于创建并返回所需的对象或记录。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Create(ctx context.Context, input CreateInput) (Record, error) {
	code := strings.ToLower(strings.TrimSpace(input.Code))
	displayName := strings.TrimSpace(input.DisplayName)
	baseURL, ok := normalizeBaseURL(input.BaseURL)
	modelListPath, pathOK := normalizeModelListPath(input.ModelListPath)
	apiKey := strings.TrimSpace(input.APIKey)
	weight := input.Weight
	if weight == 0 {
		weight = DefaultWeight
	}
	if !providerCodePattern.MatchString(code) || displayName == "" || utf8.RuneCountInString(displayName) > 128 || !validProtocol(input.Protocol) || !ok || !pathOK || apiKey == "" || len(apiKey) > 1024 || weight <= 0 || weight > MaxWeight {
		return Record{}, ErrInvalidInput
	}
	encrypted, err := s.cipher.Encrypt(apiKey)
	if err != nil {
		return Record{}, err
	}
	return s.store.Create(ctx, CreateParams{
		Code: code, DisplayName: displayName, Protocol: input.Protocol, BaseURL: baseURL, ModelListPath: modelListPath, Weight: weight,
		EncryptedAPIKey: encrypted, APIKeyHint: secretHint(apiKey),
	})
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
	if id == uuid.Nil || (input.DisplayName == nil && input.Protocol == nil && input.BaseURL == nil && input.ModelListPath == nil && input.Weight == nil && input.APIKey == nil) {
		return Record{}, ErrInvalidInput
	}
	params := UpdateParams{Protocol: input.Protocol}
	if input.DisplayName != nil {
		value := strings.TrimSpace(*input.DisplayName)
		if value == "" || utf8.RuneCountInString(value) > 128 {
			return Record{}, ErrInvalidInput
		}
		params.DisplayName = &value
	}
	if input.Protocol != nil && !validProtocol(*input.Protocol) {
		return Record{}, ErrInvalidInput
	}
	if input.BaseURL != nil {
		value, ok := normalizeBaseURL(*input.BaseURL)
		if !ok {
			return Record{}, ErrInvalidInput
		}
		params.BaseURL = &value
	}
	if input.ModelListPath != nil {
		value, ok := normalizeModelListPath(*input.ModelListPath)
		if !ok {
			return Record{}, ErrInvalidInput
		}
		params.ModelListPath = &value
	}
	if input.Weight != nil {
		if *input.Weight <= 0 || *input.Weight > MaxWeight {
			return Record{}, ErrInvalidInput
		}
		params.Weight = input.Weight
	}
	if input.APIKey != nil {
		value := strings.TrimSpace(*input.APIKey)
		if value == "" || len(value) > 1024 {
			return Record{}, ErrInvalidInput
		}
		encrypted, err := s.cipher.Encrypt(value)
		if err != nil {
			return Record{}, err
		}
		hint := secretHint(value)
		params.EncryptedAPIKey = &encrypted
		params.APIKeyHint = &hint
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
 * validProtocol 封装该名称对应的业务处理逻辑。
 * @param protocol 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validProtocol(protocol Protocol) bool {
	return protocol == ProtocolOpenAI || protocol == ProtocolAnthropic
}

/**
 * normalizeBaseURL 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func normalizeBaseURL(value string) (string, bool) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return parsed.String(), true
}

/**
 * normalizeModelListPath 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func normalizeModelListPath(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", true
	}
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "?#") || len(value) > 512 {
		return "", false
	}
	return "/" + strings.Trim(value, "/"), true
}

/**
 * secretHint 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func secretHint(value string) string {
	runes := []rune(value)
	if len(runes) <= 4 {
		return string(runes)
	}
	return string(runes[len(runes)-4:])
}
