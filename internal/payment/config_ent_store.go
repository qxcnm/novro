package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/novro-gateway/novro/ent"
)

const paymentConfigID = ProviderEPay

type ConfigEntStore struct {
	client *ent.Client
}

/**
 * NewConfigEntStore 用于创建并返回所需的对象或记录。
 * @param client 用于访问外部或底层服务的客户端。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewConfigEntStore(client *ent.Client) *ConfigEntStore {
	return &ConfigEntStore{client: client}
}

/**
 * Get 用于查询并返回所需的数据。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *ConfigEntStore) Get(ctx context.Context) (StoredConfig, error) {
	entity, err := s.client.PaymentConfig.Get(ctx, paymentConfigID)
	if ent.IsNotFound(err) {
		return StoredConfig{}, ErrConfigNotFound
	}
	if err != nil {
		return StoredConfig{}, err
	}
	return storedConfigFromEntity(entity)
}

/**
 * Upsert 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (s *ConfigEntStore) Upsert(ctx context.Context, input StoredConfigInput) (StoredConfig, error) {
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = paymentConfigID
	}
	methodsJSON, presetsJSON, bonusJSON, err := encodeStoredCollections(input)
	if err != nil {
		return StoredConfig{}, err
	}
	entity, err := s.client.PaymentConfig.Get(ctx, provider)
	if ent.IsNotFound(err) {
		created, createErr := s.client.PaymentConfig.Create().
			SetID(provider).
			SetEnabled(input.Enabled).
			SetAPIURL(input.APIURL).
			SetMerchantID(input.MerchantID).
			SetEncryptedMerchantKey(input.EncryptedMerchantKey).
			SetSiteName(input.SiteName).
			SetChannels(strings.Join(input.Channels, ",")).
			SetMethodsJSON(methodsJSON).
			SetMinTopUpMicros(input.MinMicros).
			SetMaxTopUpMicros(input.MaxMicros).
			SetPresetAmountsJSON(presetsJSON).
			SetBonusTiersJSON(bonusJSON).
			Save(ctx)
		if createErr == nil {
			return storedConfigFromEntity(created)
		}
		// Another administrator may have initialized the singleton between the
		// read and insert. Retry through the update path in that case.
		entity, err = s.client.PaymentConfig.Get(ctx, provider)
		if err != nil {
			return StoredConfig{}, createErr
		}
	} else if err != nil {
		return StoredConfig{}, err
	}
	updated, err := entity.Update().
		SetEnabled(input.Enabled).
		SetAPIURL(input.APIURL).
		SetMerchantID(input.MerchantID).
		SetEncryptedMerchantKey(input.EncryptedMerchantKey).
		SetSiteName(input.SiteName).
		SetChannels(strings.Join(input.Channels, ",")).
		SetMethodsJSON(methodsJSON).
		SetMinTopUpMicros(input.MinMicros).
		SetMaxTopUpMicros(input.MaxMicros).
		SetPresetAmountsJSON(presetsJSON).
		SetBonusTiersJSON(bonusJSON).
		Save(ctx)
	if err != nil {
		return StoredConfig{}, err
	}
	return storedConfigFromEntity(updated)
}

/**
 * storedConfigFromEntity 封装该名称对应的业务处理逻辑。
 * @param entity 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func storedConfigFromEntity(entity *ent.PaymentConfig) (StoredConfig, error) {
	channels := make([]string, 0)
	for _, channel := range strings.Split(entity.Channels, ",") {
		if channel = strings.TrimSpace(channel); channel != "" {
			channels = append(channels, channel)
		}
	}
	methods := make([]PaymentMethod, 0)
	if err := json.Unmarshal([]byte(entity.MethodsJSON), &methods); err != nil {
		return StoredConfig{}, fmt.Errorf("decode payment methods: %w", err)
	}
	if len(methods) == 0 && len(channels) > 0 {
		methods = defaultPaymentMethods(channels)
	}
	presets := make([]int64, 0)
	if err := json.Unmarshal([]byte(entity.PresetAmountsJSON), &presets); err != nil {
		return StoredConfig{}, fmt.Errorf("decode preset top-up amounts: %w", err)
	}
	if len(presets) == 0 {
		presets = defaultPresetAmounts()
	}
	bonusTiers := make([]BonusTier, 0)
	if err := json.Unmarshal([]byte(entity.BonusTiersJSON), &bonusTiers); err != nil {
		return StoredConfig{}, fmt.Errorf("decode top-up bonus tiers: %w", err)
	}
	return StoredConfig{
		Provider: entity.ID, Enabled: entity.Enabled, APIURL: entity.APIURL,
		MerchantID: entity.MerchantID, EncryptedMerchantKey: entity.EncryptedMerchantKey,
		SiteName: entity.SiteName, Channels: enabledMethodCodes(methods), Methods: methods,
		MinMicros: entity.MinTopUpMicros, MaxMicros: entity.MaxTopUpMicros,
		PresetAmountMicros: presets, BonusTiers: bonusTiers, UpdatedAt: entity.UpdatedAt,
	}, nil
}

/**
 * encodeStoredCollections 封装该名称对应的业务处理逻辑。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func encodeStoredCollections(input StoredConfigInput) (string, string, string, error) {
	methods, err := json.Marshal(input.Methods)
	if err != nil {
		return "", "", "", fmt.Errorf("encode payment methods: %w", err)
	}
	presets, err := json.Marshal(input.PresetAmountMicros)
	if err != nil {
		return "", "", "", fmt.Errorf("encode preset top-up amounts: %w", err)
	}
	bonusTiers, err := json.Marshal(input.BonusTiers)
	if err != nil {
		return "", "", "", fmt.Errorf("encode top-up bonus tiers: %w", err)
	}
	return string(methods), string(presets), string(bonusTiers), nil
}

var _ ConfigStore = (*ConfigEntStore)(nil)
