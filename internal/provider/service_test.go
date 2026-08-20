package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type fakeStore struct {
	createParams CreateParams
	updateParams UpdateParams
	status       Status
	deletedID    uuid.UUID
	err          error
}

/**
 * Create 用于创建并返回所需的对象或记录。
 * @param params 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Create(_ context.Context, params CreateParams) (Record, error) {
	f.createParams = params
	return Record{ID: uuid.New(), Code: params.Code, APIKeyHint: params.APIKeyHint}, f.err
}

/**
 * List 用于筛选并返回数据列表。
 * @param ListFilter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) List(context.Context, ListFilter) ([]Record, error) { return nil, f.err }

/**
 * Update 用于更新指定的数据或状态。
 * @param id 目标资源的唯一标识。
 * @param params 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Update(_ context.Context, id uuid.UUID, params UpdateParams) (Record, error) {
	f.updateParams = params
	return Record{ID: id}, f.err
}

/**
 * SetStatus 用于更新指定的数据或状态。
 * @param id 目标资源的唯一标识。
 * @param status 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) SetStatus(_ context.Context, id uuid.UUID, status Status) (Record, error) {
	f.status = status
	return Record{ID: id, Status: status}, f.err
}

/**
 * Delete 用于删除、撤销或释放指定资源。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Delete(_ context.Context, id uuid.UUID) error {
	f.deletedID = id
	return f.err
}

/**
 * testService 封装该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @param store 用于持久化和查询数据的存储实现。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func testService(t *testing.T, store *fakeStore) *Service {
	t.Helper()
	cipher, err := NewCipher("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	return NewService(store, cipher)
}

/**
 * TestCreateEncryptsCredentialAndNormalizesProvider 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestCreateEncryptsCredentialAndNormalizesProvider(t *testing.T) {
	store := &fakeStore{}
	service := testService(t, store)
	record, err := service.Create(context.Background(), CreateInput{
		Code: " DeepSeek ", DisplayName: " DeepSeek ", Protocols: []Protocol{ProtocolAnthropic, ProtocolOpenAI, ProtocolAnthropic},
		OutboundFormat: OutboundFormatMessages, BaseURL: "https://api.deepseek.com/", ModelListPath: " /api/models/ ", Weight: 250, APIKey: "provider-secret-1234",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if record.Code != "DeepSeek" || store.createParams.Code != "DeepSeek" || store.createParams.OutboundFormat != OutboundFormatMessages || store.createParams.BaseURL != "https://api.deepseek.com" || store.createParams.ModelListPath != "/api/models" || store.createParams.Weight != 250 || store.createParams.APIKeyHint != "1234" || store.createParams.PrimaryProtocol != ProtocolOpenAI || len(store.createParams.Protocols) != 2 || store.createParams.Protocols[0] != ProtocolOpenAI || store.createParams.Protocols[1] != ProtocolAnthropic {
		t.Fatalf("unexpected provider: record=%+v params=%+v", record, store.createParams)
	}
	if store.createParams.EncryptedAPIKey == "provider-secret-1234" || strings.Contains(store.createParams.EncryptedAPIKey, "provider-secret") {
		t.Fatal("provider credential was not encrypted")
	}
}

/**
 * TestProviderValidationRejectsInsecureOrInvalidInput 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProviderValidationRejectsInsecureOrInvalidInput(t *testing.T) {
	service := testService(t, &fakeStore{})
	inputs := []struct {
		input CreateInput
		field ValidationField
	}{
		{input: CreateInput{Code: "   ", DisplayName: "X", Protocols: []Protocol{ProtocolOpenAI}, BaseURL: "https://api.example.com", APIKey: "secret"}, field: ValidationFieldCode},
		{input: CreateInput{Code: strings.Repeat("a", 65), DisplayName: "X", Protocols: []Protocol{ProtocolOpenAI}, BaseURL: "https://api.example.com", APIKey: "secret"}, field: ValidationFieldCode},
		{input: CreateInput{Code: "valid-code", DisplayName: "", Protocols: []Protocol{ProtocolOpenAI}, BaseURL: "https://api.example.com", APIKey: "secret"}, field: ValidationFieldDisplayName},
		{input: CreateInput{Code: "valid-code", DisplayName: "X", Protocols: nil, BaseURL: "https://api.example.com", APIKey: "secret"}, field: ValidationFieldProtocols},
		{input: CreateInput{Code: "valid-code", DisplayName: "X", Protocols: []Protocol{Protocol("other")}, BaseURL: "https://api.example.com", APIKey: "secret"}, field: ValidationFieldProtocols},
		{input: CreateInput{Code: "valid-code", DisplayName: "X", Protocols: []Protocol{ProtocolOpenAI}, OutboundFormat: OutboundFormat("completions"), BaseURL: "https://api.example.com", APIKey: "secret"}, field: ValidationFieldOutboundFormat},
		{input: CreateInput{Code: "valid-code", DisplayName: "X", Protocols: []Protocol{ProtocolOpenAI}, BaseURL: "api.example.com", APIKey: "secret"}, field: ValidationFieldBaseURL},
		{input: CreateInput{Code: "valid-code", DisplayName: "X", Protocols: []Protocol{ProtocolOpenAI}, BaseURL: "https://api.example.com", ModelListPath: "models", APIKey: "secret"}, field: ValidationFieldModelListPath},
		{input: CreateInput{Code: "valid-code", DisplayName: "X", Protocols: []Protocol{ProtocolOpenAI}, BaseURL: "https://api.example.com", Weight: -1, APIKey: "secret"}, field: ValidationFieldWeight},
		{input: CreateInput{Code: "valid-code", DisplayName: "X", Protocols: []Protocol{ProtocolOpenAI}, BaseURL: "https://api.example.com"}, field: ValidationFieldAPIKey},
	}
	for _, test := range inputs {
		_, err := service.Create(context.Background(), test.input)
		var validationError *ValidationError
		if !errors.Is(err, ErrInvalidInput) || !errors.As(err, &validationError) || validationError.Field != test.field {
			t.Fatalf("expected field %s for %+v, got %v", test.field, test.input, err)
		}
	}
}

/**
 * TestProviderValidationAcceptsFlexibleProviderCode 验证提供商代码只做首尾空白清理并保留上游命名。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProviderValidationAcceptsFlexibleProviderCode(t *testing.T) {
	for _, test := range []struct {
		name string
		code string
		want string
	}{
		{name: "spaces Chinese and underscore", code: "  3DP TokenHub_国内  ", want: "3DP TokenHub_国内"},
		{name: "uppercase and punctuation", code: "Z.CODE v2/兼容", want: "Z.CODE v2/兼容"},
		{name: "single character", code: "x", want: "x"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeStore{}
			service := testService(t, store)
			_, err := service.Create(context.Background(), CreateInput{
				Code: test.code, DisplayName: "Vendor", Protocols: []Protocol{ProtocolOpenAI},
				BaseURL: "https://api.example.com/v1", APIKey: "secret",
			})
			if err != nil {
				t.Fatalf("expected provider code %q to be accepted: %v", test.code, err)
			}
			if store.createParams.Code != test.want {
				t.Fatalf("code=%q want %q", store.createParams.Code, test.want)
			}
		})
	}
}

/**
 * TestProviderValidationAcceptsHTTPForSelfHostedUpstream 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProviderValidationAcceptsHTTPForSelfHostedUpstream(t *testing.T) {
	store := &fakeStore{}
	service := testService(t, store)
	_, err := service.Create(context.Background(), CreateInput{
		Code: "self-hosted", DisplayName: "自建网关", Protocols: []Protocol{ProtocolOpenAI},
		BaseURL: "http://203.0.113.10:3000/v1/", APIKey: "secret",
	})
	if err != nil {
		t.Fatalf("expected HTTP self-hosted provider to be accepted: %v", err)
	}
	if store.createParams.BaseURL != "http://203.0.113.10:3000/v1" {
		t.Fatalf("base URL=%q", store.createParams.BaseURL)
	}
}

/**
 * TestUpdateReencryptsOnlyWhenCredentialProvided 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestUpdateReencryptsOnlyWhenCredentialProvided(t *testing.T) {
	store := &fakeStore{}
	service := testService(t, store)
	name := "New Name"
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{DisplayName: &name}); err != nil {
		t.Fatalf("update name: %v", err)
	}
	if store.updateParams.EncryptedAPIKey != nil {
		t.Fatal("name update replaced provider credential")
	}
	secret := "replacement-5678"
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{APIKey: &secret}); err != nil {
		t.Fatalf("update secret: %v", err)
	}
	if store.updateParams.EncryptedAPIKey == nil || store.updateParams.APIKeyHint == nil || *store.updateParams.APIKeyHint != "5678" {
		t.Fatalf("credential was not replaced safely: %+v", store.updateParams)
	}
	protocols := []Protocol{ProtocolAnthropic, ProtocolOpenAI}
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{Protocols: &protocols}); err != nil {
		t.Fatalf("update protocols: %v", err)
	}
	if store.updateParams.Protocols == nil || len(*store.updateParams.Protocols) != 2 || store.updateParams.PrimaryProtocol == nil || *store.updateParams.PrimaryProtocol != ProtocolOpenAI {
		t.Fatalf("protocol update was not normalized: %+v", store.updateParams)
	}
	outboundFormat := OutboundFormatResponses
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{OutboundFormat: &outboundFormat}); err != nil {
		t.Fatalf("update outbound format: %v", err)
	}
	if store.updateParams.OutboundFormat == nil || *store.updateParams.OutboundFormat != OutboundFormatResponses {
		t.Fatalf("outbound format update was not stored: %+v", store.updateParams)
	}
	outboundFormat = OutboundFormatNone
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{OutboundFormat: &outboundFormat}); err != nil {
		t.Fatalf("clear outbound format: %v", err)
	}
	if store.updateParams.OutboundFormat == nil || *store.updateParams.OutboundFormat != OutboundFormatNone {
		t.Fatalf("outbound format clear was not stored: %+v", store.updateParams)
	}
	outboundFormat = OutboundFormat("completions")
	_, err := service.Update(context.Background(), uuid.New(), UpdateInput{OutboundFormat: &outboundFormat})
	var validationError *ValidationError
	if !errors.Is(err, ErrInvalidInput) || !errors.As(err, &validationError) || validationError.Field != ValidationFieldOutboundFormat {
		t.Fatalf("invalid outbound format error=%v", err)
	}
	emptyProtocols := []Protocol{}
	if _, err := service.Update(context.Background(), uuid.New(), UpdateInput{Protocols: &emptyProtocols}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty protocol update error=%v", err)
	}
}

/**
 * TestProviderValidationRejectsNonPositiveWeight 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProviderValidationRejectsNonPositiveWeight(t *testing.T) {
	service := testService(t, &fakeStore{})
	for _, weight := range []int{-1, MaxWeight + 1} {
		_, err := service.Create(context.Background(), CreateInput{Code: "valid-code", DisplayName: "X", Protocols: []Protocol{ProtocolOpenAI}, BaseURL: "https://api.example.com", APIKey: "secret", Weight: weight})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("weight=%d err=%v", weight, err)
		}
	}
	weight := 300
	store := &fakeStore{}
	if _, err := testService(t, store).Update(context.Background(), uuid.New(), UpdateInput{Weight: &weight}); err != nil || store.updateParams.Weight == nil || *store.updateParams.Weight != weight {
		t.Fatalf("weight update=%+v err=%v", store.updateParams, err)
	}
}

/**
 * TestCipherRoundTripAndRejectsWrongKey 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestCipherRoundTripAndRejectsWrongKey(t *testing.T) {
	cipher, _ := NewCipher("01234567890123456789012345678901")
	encrypted, err := cipher.Encrypt("secret-value")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil || decrypted != "secret-value" {
		t.Fatalf("decrypt=%q err=%v", decrypted, err)
	}
	other, _ := NewCipher("different-secret-012345678901234567")
	if _, err := other.Decrypt(encrypted); err == nil {
		t.Fatal("wrong key decrypted provider credential")
	}
}

/**
 * TestDeleteValidatesIDAndDelegates 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestDeleteValidatesIDAndDelegates(t *testing.T) {
	store := &fakeStore{}
	service := testService(t, store)
	if err := service.Delete(context.Background(), uuid.Nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid ID, got %v", err)
	}
	id := uuid.New()
	if err := service.Delete(context.Background(), id); err != nil || store.deletedID != id {
		t.Fatalf("delete id=%s stored=%s err=%v", id, store.deletedID, err)
	}
}
