package payment

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Store interface {
	/**
	 * Create 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 CreateParams 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Create(context.Context, CreateParams) (Order, error)
	/**
	 * Get 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Get(context.Context, string) (Order, error)
	/**
	 * List 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 ListFilter 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	List(context.Context, uuid.UUID, ListFilter) (Page, error)
	/**
	 * ListAll 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 AdminListFilter 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	ListAll(context.Context, AdminListFilter) (AdminPage, error)
	/**
	 * Complete 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 CompleteParams 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Complete(context.Context, CompleteParams) (Order, error)
}

type ConfigStore interface {
	/**
	 * Get 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Get(context.Context) (StoredConfig, error)
	/**
	 * Upsert 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 StoredConfigInput 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Upsert(context.Context, StoredConfigInput) (StoredConfig, error)
}

type SecretCipher interface {
	/**
	 * Encrypt 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Encrypt(string) (string, error)
	/**
	 * Decrypt 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Decrypt(string) (string, error)
}

type StoredConfig struct {
	Provider   string
	Enabled    bool
	APIURL     string
	MerchantID string
	// MerchantKey is populated only for an environment bootstrap fallback and
	// is never persisted or returned through an API response.
	MerchantKey          string
	EncryptedMerchantKey string
	SiteName             string
	Channels             []string
	Methods              []PaymentMethod
	MinMicros            int64
	MaxMicros            int64
	PresetAmountMicros   []int64
	BonusTiers           []BonusTier
	UpdatedAt            time.Time
}

type StoredConfigInput struct {
	Provider             string
	Enabled              bool
	APIURL               string
	MerchantID           string
	EncryptedMerchantKey string
	SiteName             string
	Channels             []string
	Methods              []PaymentMethod
	MinMicros            int64
	MaxMicros            int64
	PresetAmountMicros   []int64
	BonusTiers           []BonusTier
}

type Gateway interface {
	/**
	 * Channels 声明该接口方法需要提供的业务能力。
	 * @param none 无参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Channels() []string
	/**
	 * Checkout 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 Order 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Checkout(Order) (Checkout, error)
	/**
	 * ParseNotification 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 url.Values 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	ParseNotification(url.Values) (Notification, error)
	/**
	 * Query 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Query(context.Context, string) (Notification, bool, error)
}

type Service struct {
	store         Store
	configStore   ConfigStore
	cipher        SecretCipher
	defaultConfig EPayConfig
	now           func() time.Time
}

var paymentChannelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

/**
 * NewService 用于创建并返回所需的对象或记录。
 * @param store 用于持久化和查询数据的存储实现。
 * @param configStore 本次操作需要使用的输入参数。
 * @param cipher 本次操作需要使用的输入参数。
 * @param defaultConfig 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewService(store Store, configStore ConfigStore, cipher SecretCipher, defaultConfig EPayConfig) *Service {
	return &Service{store: store, configStore: configStore, cipher: cipher, defaultConfig: defaultConfig, now: time.Now}
}

/**
 * Config 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Config(ctx context.Context) (PublicConfig, error) {
	config := publicConfigFromStored(defaultStoredConfig())
	if s == nil {
		return config, nil
	}
	record, err := s.currentStoredConfig(ctx)
	if err != nil {
		return config, err
	}
	config = publicConfigFromStored(record)
	if !record.Enabled {
		config.Enabled = false
		config.Channels = []string{}
		config.Methods = []PaymentMethod{}
		return config, nil
	}
	gateway, err := s.gatewayFromStored(record, true)
	if err != nil {
		return config, err
	}
	config.Enabled = true
	config.Provider = ProviderEPay
	config.Channels = append([]string{}, gateway.Channels()...)
	return config, nil
}

/**
 * Create 用于创建并返回所需的对象或记录。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param amountMicros 本次操作需要使用的输入参数。
 * @param channel 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Create(ctx context.Context, userID uuid.UUID, amountMicros int64, channel string) (CreateResult, error) {
	record, err := s.currentStoredConfig(ctx)
	if err != nil {
		return CreateResult{}, err
	}
	gateway, err := s.gatewayFromStored(record, true)
	if err != nil {
		return CreateResult{}, err
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	method, ok := enabledMethod(record.Methods, channel)
	if userID == uuid.Nil || amountMicros < record.MinMicros || amountMicros > record.MaxMicros || amountMicros < method.MinMicros || amountMicros%10_000 != 0 || !ok || !contains(gateway.Channels(), channel) {
		return CreateResult{}, ErrInvalidInput
	}
	creditedMicros := creditedAmount(amountMicros, record.BonusTiers)
	id := uuid.New()
	created, err := s.store.Create(ctx, CreateParams{
		ID: id, UserID: userID, OutTradeNo: "NVR" + strings.ReplaceAll(id.String(), "-", ""),
		Channel: channel, AmountMicros: amountMicros, CreditedMicros: creditedMicros,
	})
	if err != nil {
		return CreateResult{}, fmt.Errorf("create top-up order: %w", err)
	}
	checkout, err := gateway.Checkout(created)
	if err != nil {
		return CreateResult{}, fmt.Errorf("build top-up checkout: %w", err)
	}
	return CreateResult{Order: created, Checkout: checkout}, nil
}

/**
 * List 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) List(ctx context.Context, userID uuid.UUID, filter ListFilter) (Page, error) {
	if userID == uuid.Nil || filter.Offset < 0 || filter.Limit < 1 || filter.Limit > 100 {
		return Page{}, ErrInvalidInput
	}
	return s.store.List(ctx, userID, filter)
}

/**
 * ListAll 用于筛选并返回数据列表。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) ListAll(ctx context.Context, filter AdminListFilter) (AdminPage, error) {
	filter.Search = strings.TrimSpace(filter.Search)
	filter.Channel = strings.ToLower(strings.TrimSpace(filter.Channel))
	if s == nil || s.store == nil || filter.Offset < 0 || filter.Limit < 1 || filter.Limit > 100 || len([]rune(filter.Search)) > 128 {
		return AdminPage{}, ErrInvalidInput
	}
	if filter.Status != "" && filter.Status != StatusPending && filter.Status != StatusPaid {
		return AdminPage{}, ErrInvalidInput
	}
	if filter.Channel != "" && !paymentChannelPattern.MatchString(filter.Channel) {
		return AdminPage{}, ErrInvalidInput
	}
	return s.store.ListAll(ctx, filter)
}

/**
 * HandleNotification 用于处理对应的 HTTP 请求并写入响应。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param values 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) HandleNotification(ctx context.Context, values url.Values) error {
	gateway, err := s.gateway(ctx, false)
	if err != nil {
		return err
	}
	notification, err := gateway.ParseNotification(values)
	if err != nil {
		return err
	}
	_, err = s.store.Complete(ctx, CompleteParams{
		OutTradeNo: notification.OutTradeNo, ProviderTradeNo: notification.ProviderTradeNo,
		Channel: notification.Channel, AmountMicros: notification.AmountMicros, PaidAt: s.now().UTC(),
	})
	return err
}

// Reconcile queries EPay for an existing order and runs the same idempotent
// completion transaction used by signed callbacks. It never creates a new
// payment and returns paid orders without querying the provider again.
/**
 * Reconcile 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param outTradeNo 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Reconcile(ctx context.Context, outTradeNo string) (Order, error) {
	outTradeNo = strings.TrimSpace(outTradeNo)
	if s == nil || s.store == nil || outTradeNo == "" || len(outTradeNo) > 64 {
		return Order{}, ErrInvalidInput
	}
	order, err := s.store.Get(ctx, outTradeNo)
	if err != nil {
		return Order{}, err
	}
	return s.reconcileOrder(ctx, order)
}

// ReconcileForUser exposes the same provider query to an order owner without
// revealing or touching another user's order.
/**
 * ReconcileForUser 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param userID 目标用户的唯一标识。
 * @param outTradeNo 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) ReconcileForUser(ctx context.Context, userID uuid.UUID, outTradeNo string) (Order, error) {
	outTradeNo = strings.TrimSpace(outTradeNo)
	if s == nil || s.store == nil || userID == uuid.Nil || outTradeNo == "" || len(outTradeNo) > 64 {
		return Order{}, ErrInvalidInput
	}
	order, err := s.store.Get(ctx, outTradeNo)
	if err != nil {
		return Order{}, err
	}
	if order.UserID != userID {
		return Order{}, ErrOrderNotFound
	}
	return s.reconcileOrder(ctx, order)
}

/**
 * reconcileOrder 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param order 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) reconcileOrder(ctx context.Context, order Order) (Order, error) {
	if order.Status == StatusPaid {
		return order, nil
	}
	gateway, err := s.gateway(ctx, false)
	if err != nil {
		return Order{}, err
	}
	notification, paid, err := gateway.Query(ctx, order.OutTradeNo)
	if err != nil {
		return Order{}, err
	}
	if !paid {
		return Order{}, ErrOrderUnpaid
	}
	if notification.OutTradeNo != order.OutTradeNo {
		return Order{}, ErrOrderConflict
	}
	return s.store.Complete(ctx, CompleteParams{
		OutTradeNo: notification.OutTradeNo, ProviderTradeNo: notification.ProviderTradeNo,
		Channel: notification.Channel, AmountMicros: notification.AmountMicros, PaidAt: s.now().UTC(),
	})
}

/**
 * AdminConfig 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) AdminConfig(ctx context.Context) (AdminConfig, error) {
	record, err := s.currentStoredConfig(ctx)
	if err != nil {
		return AdminConfig{}, err
	}
	config := adminConfigFromStored(record)
	config.NotifyURL = s.defaultConfig.NotifyURL
	config.ReturnURL = s.defaultConfig.ReturnURL
	return config, nil
}

/**
 * UpdateConfig 用于更新指定的数据或状态。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) UpdateConfig(ctx context.Context, input ConfigInput) (AdminConfig, error) {
	if s == nil || s.configStore == nil || s.cipher == nil {
		return AdminConfig{}, ErrInvalidInput
	}
	current, err := s.currentStoredConfig(ctx)
	if err != nil {
		return AdminConfig{}, err
	}
	normalized, err := normalizeConfigInput(input, current)
	if err != nil {
		return AdminConfig{}, err
	}
	if normalized.MerchantKey != "" {
		encrypted, encryptErr := s.cipher.Encrypt(normalized.MerchantKey)
		if encryptErr != nil {
			return AdminConfig{}, fmt.Errorf("encrypt payment merchant key: %w", encryptErr)
		}
		current.EncryptedMerchantKey = encrypted
		current.MerchantKey = ""
	} else if current.MerchantKey != "" && current.EncryptedMerchantKey == "" {
		encrypted, encryptErr := s.cipher.Encrypt(current.MerchantKey)
		if encryptErr != nil {
			return AdminConfig{}, fmt.Errorf("encrypt payment bootstrap key: %w", encryptErr)
		}
		current.EncryptedMerchantKey = encrypted
		current.MerchantKey = ""
	}
	current.Provider = ProviderEPay
	current.Enabled = normalized.Enabled
	current.APIURL = normalized.APIURL
	current.MerchantID = normalized.MerchantID
	current.SiteName = normalized.SiteName
	current.Methods = append([]PaymentMethod{}, normalized.Methods...)
	current.Channels = enabledMethodCodes(current.Methods)
	current.MinMicros = normalized.MinMicros
	current.MaxMicros = normalized.MaxMicros
	current.PresetAmountMicros = append([]int64{}, normalized.PresetAmountMicros...)
	current.BonusTiers = append([]BonusTier{}, normalized.BonusTiers...)
	updated, err := s.configStore.Upsert(ctx, StoredConfigInput{
		Provider: current.Provider, Enabled: current.Enabled, APIURL: current.APIURL,
		MerchantID: current.MerchantID, EncryptedMerchantKey: current.EncryptedMerchantKey,
		SiteName: current.SiteName, Channels: current.Channels, Methods: current.Methods,
		MinMicros: current.MinMicros, MaxMicros: current.MaxMicros,
		PresetAmountMicros: current.PresetAmountMicros, BonusTiers: current.BonusTiers,
	})
	if err != nil {
		return AdminConfig{}, err
	}
	config := adminConfigFromStored(updated)
	config.NotifyURL = s.defaultConfig.NotifyURL
	config.ReturnURL = s.defaultConfig.ReturnURL
	return config, nil
}

// Bootstrap stores an environment configuration only when the database has
// never had a payment configuration. Subsequent page edits are authoritative.
/**
 * Bootstrap 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) Bootstrap(ctx context.Context) error {
	if s == nil || s.configStore == nil || s.cipher == nil || !epayConfigEnabled(s.defaultConfig) {
		return nil
	}
	if _, err := s.configStore.Get(ctx); err == nil {
		return nil
	} else if !errors.Is(err, ErrConfigNotFound) {
		return err
	}
	encrypted, err := s.cipher.Encrypt(strings.TrimSpace(s.defaultConfig.MerchantKey))
	if err != nil {
		return fmt.Errorf("encrypt payment bootstrap key: %w", err)
	}
	_, err = s.configStore.Upsert(ctx, StoredConfigInput{
		Provider: ProviderEPay, Enabled: true, APIURL: strings.TrimRight(strings.TrimSpace(s.defaultConfig.APIURL), "/"),
		MerchantID: strings.TrimSpace(s.defaultConfig.MerchantID), EncryptedMerchantKey: encrypted,
		SiteName: strings.TrimSpace(s.defaultConfig.SiteName), Channels: append([]string{}, s.defaultConfig.Channels...),
		Methods: defaultPaymentMethods(s.defaultConfig.Channels), MinMicros: MinTopUpMicros, MaxMicros: MaxTopUpMicros,
		PresetAmountMicros: defaultPresetAmounts(), BonusTiers: []BonusTier{},
	})
	return err
}

/**
 * currentStoredConfig 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) currentStoredConfig(ctx context.Context) (StoredConfig, error) {
	if s == nil || s.configStore == nil {
		if s != nil && epayConfigEnabled(s.defaultConfig) {
			return storedConfigFromEPay(s.defaultConfig), nil
		}
		return defaultStoredConfig(), nil
	}
	record, err := s.configStore.Get(ctx)
	if errors.Is(err, ErrConfigNotFound) {
		if epayConfigEnabled(s.defaultConfig) {
			return storedConfigFromEPay(s.defaultConfig), nil
		}
		return defaultStoredConfig(), nil
	}
	if err != nil {
		return StoredConfig{}, err
	}
	return normalizeStoredConfig(record), nil
}

/**
 * gateway 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param requireEnabled 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) gateway(ctx context.Context, requireEnabled bool) (Gateway, error) {
	record, err := s.currentStoredConfig(ctx)
	if err != nil {
		return nil, err
	}
	return s.gatewayFromStored(record, requireEnabled)
}

/**
 * gatewayFromStored 封装该名称对应的业务处理逻辑。
 * @param record 本次操作需要使用的输入参数。
 * @param requireEnabled 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *Service) gatewayFromStored(record StoredConfig, requireEnabled bool) (Gateway, error) {
	if requireEnabled && !record.Enabled {
		return nil, ErrDisabled
	}
	merchantKey := record.MerchantKey
	if merchantKey == "" {
		merchantKey = record.EncryptedMerchantKey
	}
	if strings.HasPrefix(merchantKey, "v1.") {
		if s.cipher == nil {
			return nil, ErrDisabled
		}
		decrypted, err := s.cipher.Decrypt(merchantKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt payment merchant key: %w", err)
		}
		merchantKey = decrypted
	}
	config := EPayConfig{APIURL: record.APIURL, MerchantID: record.MerchantID, MerchantKey: merchantKey, SiteName: record.SiteName, Channels: enabledMethodCodes(record.Methods), NotifyURL: s.defaultConfig.NotifyURL, ReturnURL: s.defaultConfig.ReturnURL}
	return NewEPayGateway(config)
}

/**
 * normalizeConfigInput 封装该名称对应的业务处理逻辑。
 * @param input 需要处理的输入数据。
 * @param current 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func normalizeConfigInput(input ConfigInput, current StoredConfig) (normalizedConfig, error) {
	result := normalizedConfig{
		Enabled: input.Enabled, APIURL: strings.TrimRight(strings.TrimSpace(input.APIURL), "/"),
		MerchantID: strings.TrimSpace(input.MerchantID), SiteName: strings.TrimSpace(input.SiteName),
		MinMicros: input.MinMicros, MaxMicros: input.MaxMicros,
	}
	if result.SiteName == "" {
		result.SiteName = "Novro"
	}
	if result.MinMicros == 0 {
		result.MinMicros = current.MinMicros
	}
	if result.MinMicros == 0 {
		result.MinMicros = MinTopUpMicros
	}
	if result.MaxMicros == 0 {
		result.MaxMicros = current.MaxMicros
	}
	if result.MaxMicros == 0 {
		result.MaxMicros = MaxTopUpMicros
	}
	if input.MerchantKey != nil {
		result.MerchantKey = strings.TrimSpace(*input.MerchantKey)
	} else if current.EncryptedMerchantKey != "" {
		result.MerchantKey = "preserve"
	}
	if result.APIURL != "" {
		if _, err := epaySubmitURL(result.APIURL); err != nil {
			return normalizedConfig{}, ErrInvalidInput
		}
	}
	if result.MerchantID == "" || len(result.MerchantID) > 128 || len([]rune(result.SiteName)) > 64 || len(result.SiteName) == 0 {
		if result.Enabled {
			return normalizedConfig{}, ErrInvalidInput
		}
	}
	if result.MinMicros < MinTopUpMicros || result.MaxMicros > MaxTopUpMicros || result.MinMicros > result.MaxMicros || result.MinMicros%10_000 != 0 || result.MaxMicros%10_000 != 0 {
		return normalizedConfig{}, ErrInvalidInput
	}

	methods := input.Methods
	if methods == nil && input.Channels != nil {
		methods = defaultPaymentMethods(input.Channels)
	}
	if methods == nil {
		methods = current.Methods
	}
	seenMethods := make(map[string]struct{}, len(methods))
	result.Methods = make([]PaymentMethod, 0, len(methods))
	for _, method := range methods {
		method.Code = strings.ToLower(strings.TrimSpace(method.Code))
		method.Name = strings.TrimSpace(method.Name)
		method.Icon = strings.ToLower(strings.TrimSpace(method.Icon))
		if method.Icon == "" {
			method.Icon = "wallet"
		}
		if method.MinMicros == 0 {
			method.MinMicros = result.MinMicros
		}
		if method.Code == "" || !paymentChannelPattern.MatchString(method.Code) || len([]rune(method.Name)) < 1 || len([]rune(method.Name)) > 32 || !validPaymentIcon(method.Icon) || method.MinMicros < result.MinMicros || method.MinMicros > result.MaxMicros || method.MinMicros%10_000 != 0 {
			return normalizedConfig{}, ErrInvalidInput
		}
		if _, exists := seenMethods[method.Code]; exists {
			return normalizedConfig{}, ErrInvalidInput
		}
		seenMethods[method.Code] = struct{}{}
		result.Methods = append(result.Methods, method)
	}
	if result.Enabled && (result.APIURL == "" || result.MerchantID == "" || (result.MerchantKey == "" && current.EncryptedMerchantKey == "") || len(enabledMethodCodes(result.Methods)) == 0) {
		return normalizedConfig{}, ErrInvalidInput
	}

	presets := input.PresetAmountMicros
	if presets == nil {
		presets = current.PresetAmountMicros
	}
	if presets == nil {
		presets = defaultPresetAmounts()
	}
	seenPresets := make(map[int64]struct{}, len(presets))
	result.PresetAmountMicros = make([]int64, 0, len(presets))
	for _, amount := range presets {
		if amount < result.MinMicros || amount > result.MaxMicros || amount%10_000 != 0 {
			return normalizedConfig{}, ErrInvalidInput
		}
		if _, exists := seenPresets[amount]; exists {
			return normalizedConfig{}, ErrInvalidInput
		}
		seenPresets[amount] = struct{}{}
		result.PresetAmountMicros = append(result.PresetAmountMicros, amount)
	}
	if len(result.PresetAmountMicros) == 0 || len(result.PresetAmountMicros) > 12 {
		return normalizedConfig{}, ErrInvalidInput
	}

	bonusTiers := input.BonusTiers
	if bonusTiers == nil {
		bonusTiers = current.BonusTiers
	}
	result.BonusTiers = append([]BonusTier{}, bonusTiers...)
	sort.Slice(result.BonusTiers, func(i, j int) bool {
		return result.BonusTiers[i].ThresholdMicros < result.BonusTiers[j].ThresholdMicros
	})
	for index, tier := range result.BonusTiers {
		if tier.ThresholdMicros < result.MinMicros || tier.ThresholdMicros > result.MaxMicros || tier.ThresholdMicros%10_000 != 0 || tier.BonusBPS < 1 || tier.BonusBPS > 10_000 {
			return normalizedConfig{}, ErrInvalidInput
		}
		if index > 0 && result.BonusTiers[index-1].ThresholdMicros == tier.ThresholdMicros {
			return normalizedConfig{}, ErrInvalidInput
		}
	}
	if len(result.Methods) > 20 || len(result.BonusTiers) > 20 {
		return normalizedConfig{}, ErrInvalidInput
	}
	if result.MerchantKey == "preserve" {
		result.MerchantKey = ""
	}
	return result, nil
}

type normalizedConfig struct {
	Enabled            bool
	APIURL             string
	MerchantID         string
	MerchantKey        string
	SiteName           string
	Methods            []PaymentMethod
	MinMicros          int64
	MaxMicros          int64
	PresetAmountMicros []int64
	BonusTiers         []BonusTier
}

/**
 * adminConfigFromStored 封装该名称对应的业务处理逻辑。
 * @param record 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func adminConfigFromStored(record StoredConfig) AdminConfig {
	hasMerchantKey := record.EncryptedMerchantKey != "" || record.MerchantKey != ""
	return AdminConfig{
		Provider: ProviderEPay, Enabled: record.Enabled,
		Configured: record.APIURL != "" && record.MerchantID != "" && hasMerchantKey && len(enabledMethodCodes(record.Methods)) > 0,
		APIURL:     record.APIURL, MerchantID: record.MerchantID, SiteName: record.SiteName,
		Channels: enabledMethodCodes(record.Methods), Methods: append([]PaymentMethod{}, record.Methods...),
		MinMicros: record.MinMicros, MaxMicros: record.MaxMicros,
		PresetAmountMicros: append([]int64{}, record.PresetAmountMicros...), BonusTiers: append([]BonusTier{}, record.BonusTiers...),
		HasMerchantKey: hasMerchantKey, UpdatedAt: record.UpdatedAt,
	}
}

/**
 * storedConfigFromEPay 封装该名称对应的业务处理逻辑。
 * @param config 本次操作使用的配置。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func storedConfigFromEPay(config EPayConfig) StoredConfig {
	methods := defaultPaymentMethods(config.Channels)
	return StoredConfig{
		Provider: ProviderEPay, Enabled: epayConfigEnabled(config), APIURL: config.APIURL, MerchantID: config.MerchantID,
		MerchantKey: config.MerchantKey, SiteName: config.SiteName, Channels: enabledMethodCodes(methods), Methods: methods,
		MinMicros: MinTopUpMicros, MaxMicros: MaxTopUpMicros, PresetAmountMicros: defaultPresetAmounts(), BonusTiers: []BonusTier{},
	}
}

/**
 * defaultStoredConfig 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func defaultStoredConfig() StoredConfig {
	return StoredConfig{
		Provider: ProviderEPay, SiteName: "Novro", Channels: []string{}, Methods: []PaymentMethod{},
		MinMicros: MinTopUpMicros, MaxMicros: MaxTopUpMicros,
		PresetAmountMicros: defaultPresetAmounts(), BonusTiers: []BonusTier{},
	}
}

/**
 * normalizeStoredConfig 封装该名称对应的业务处理逻辑。
 * @param record 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func normalizeStoredConfig(record StoredConfig) StoredConfig {
	if record.Provider == "" {
		record.Provider = ProviderEPay
	}
	if record.SiteName == "" {
		record.SiteName = "Novro"
	}
	if len(record.Methods) == 0 && len(record.Channels) > 0 {
		record.Methods = defaultPaymentMethods(record.Channels)
	}
	record.Channels = enabledMethodCodes(record.Methods)
	if record.MinMicros == 0 {
		record.MinMicros = MinTopUpMicros
	}
	if record.MinMicros < MinTopUpMicros {
		record.MinMicros = MinTopUpMicros
	}
	if record.MaxMicros == 0 {
		record.MaxMicros = MaxTopUpMicros
	}
	if record.MaxMicros < record.MinMicros {
		record.MaxMicros = MaxTopUpMicros
	}
	for index := range record.Methods {
		if record.Methods[index].MinMicros < record.MinMicros {
			record.Methods[index].MinMicros = record.MinMicros
		}
	}
	if len(record.PresetAmountMicros) == 0 {
		record.PresetAmountMicros = defaultPresetAmounts()
	}
	if record.BonusTiers == nil {
		record.BonusTiers = []BonusTier{}
	}
	return record
}

/**
 * defaultPresetAmounts 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func defaultPresetAmounts() []int64 {
	return []int64{10_000_000, 50_000_000, 100_000_000, 500_000_000}
}

/**
 * defaultPaymentMethods 封装该名称对应的业务处理逻辑。
 * @param channels 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func defaultPaymentMethods(channels []string) []PaymentMethod {
	methods := make([]PaymentMethod, 0, len(channels))
	seen := make(map[string]struct{}, len(channels))
	for _, channel := range channels {
		channel = strings.ToLower(strings.TrimSpace(channel))
		if channel == "" {
			continue
		}
		if _, exists := seen[channel]; exists {
			continue
		}
		seen[channel] = struct{}{}
		name, icon := channel, "wallet"
		switch channel {
		case "alipay":
			name, icon = "支付宝", "smartphone"
		case "wxpay":
			name, icon = "微信支付", "qr-code"
		case "qqpay":
			name, icon = "QQ 钱包", "wallet"
		case "bank", "bankpay":
			name, icon = "银行卡", "landmark"
		}
		methods = append(methods, PaymentMethod{Code: channel, Name: name, Icon: icon, MinMicros: MinTopUpMicros, Enabled: true})
	}
	return methods
}

/**
 * enabledMethodCodes 封装该名称对应的业务处理逻辑。
 * @param methods 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func enabledMethodCodes(methods []PaymentMethod) []string {
	codes := make([]string, 0, len(methods))
	for _, method := range methods {
		if method.Enabled {
			codes = append(codes, method.Code)
		}
	}
	return codes
}

/**
 * enabledMethod 封装该名称对应的业务处理逻辑。
 * @param methods 本次操作需要使用的输入参数。
 * @param code 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func enabledMethod(methods []PaymentMethod, code string) (PaymentMethod, bool) {
	for _, method := range methods {
		if method.Enabled && method.Code == code {
			return method, true
		}
	}
	return PaymentMethod{}, false
}

/**
 * validPaymentIcon 封装该名称对应的业务处理逻辑。
 * @param icon 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func validPaymentIcon(icon string) bool {
	switch icon {
	case "wallet", "smartphone", "qr-code", "card", "landmark":
		return true
	default:
		return false
	}
}

/**
 * creditedAmount 封装该名称对应的业务处理逻辑。
 * @param amountMicros 本次操作需要使用的输入参数。
 * @param tiers 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func creditedAmount(amountMicros int64, tiers []BonusTier) int64 {
	bonusBPS := 0
	for _, tier := range tiers {
		if amountMicros >= tier.ThresholdMicros {
			bonusBPS = tier.BonusBPS
		}
	}
	return amountMicros + amountMicros*int64(bonusBPS)/10_000
}

/**
 * publicConfigFromStored 封装该名称对应的业务处理逻辑。
 * @param record 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func publicConfigFromStored(record StoredConfig) PublicConfig {
	methods := make([]PaymentMethod, 0, len(record.Methods))
	for _, method := range record.Methods {
		if method.Enabled {
			methods = append(methods, method)
		}
	}
	return PublicConfig{
		Enabled: record.Enabled, Provider: ProviderEPay, Channels: enabledMethodCodes(methods), Methods: methods,
		MinMicros: record.MinMicros, MaxMicros: record.MaxMicros,
		PresetAmountMicros: append([]int64{}, record.PresetAmountMicros...), BonusTiers: append([]BonusTier{}, record.BonusTiers...),
	}
}

/**
 * epayConfigEnabled 封装该名称对应的业务处理逻辑。
 * @param config 本次操作使用的配置。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func epayConfigEnabled(config EPayConfig) bool {
	return strings.TrimSpace(config.APIURL) != "" && strings.TrimSpace(config.MerchantID) != "" && strings.TrimSpace(config.MerchantKey) != ""
}

const ProviderEPay = "epay"

/**
 * contains 封装该名称对应的业务处理逻辑。
 * @param values 本次操作需要使用的输入参数。
 * @param target 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
