package modelroute

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/provider"
)

type fakeStore struct {
	created        CreateInput
	updated        UpdateParams
	resolutions    []Resolution
	deletedID      uuid.UUID
	billingGroupID uuid.UUID
	err            error
}

/**
 * Create 用于创建并返回所需的对象或记录。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Create(_ context.Context, input CreateInput) (Record, error) {
	f.created = input
	return Record{PublicName: input.PublicName}, f.err
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
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) Update(_ context.Context, _ uuid.UUID, input UpdateParams) (Record, error) {
	f.updated = input
	return Record{}, f.err
}

/**
 * SetStatus 用于更新指定的数据或状态。
 * @param Status 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) SetStatus(context.Context, uuid.UUID, Status) (Record, error) {
	return Record{}, f.err
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
 * ResolveCandidates 封装该名称对应的业务处理逻辑。
 * @param billingGroupID 目标资源的一个或多个唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) ResolveCandidates(_ context.Context, _ string, billingGroupID uuid.UUID) ([]Resolution, error) {
	f.billingGroupID = billingGroupID
	return f.resolutions, f.err
}

/**
 * ListActive 用于筛选并返回数据列表。
 * @param billingGroupID 目标资源的一个或多个唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeStore) ListActive(_ context.Context, billingGroupID uuid.UUID) ([]Record, error) {
	f.billingGroupID = billingGroupID
	return nil, f.err
}

/**
 * TestCreateNormalizesAndValidatesModelRoute 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestCreateNormalizesAndValidatesModelRoute(t *testing.T) {
	cipher, _ := provider.NewCipher("01234567890123456789012345678901")
	store := &fakeStore{}
	service := NewService(store, cipher)
	upstreamModelID := uuid.New()
	providerID := uuid.New()
	billingGroupID := uuid.New()
	if _, err := service.Create(context.Background(), CreateInput{ProviderID: providerID, UpstreamModelID: upstreamModelID, BillingGroupID: billingGroupID, PublicName: "  deepseek-chat  ", DisplayName: " DeepSeek Chat "}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if store.created.PublicName != "deepseek-chat" || store.created.DisplayName != "DeepSeek Chat" || store.created.ProviderID != providerID || store.created.UpstreamModelID != upstreamModelID {
		t.Fatalf("not normalized: %+v", store.created)
	}
	for _, name := range []string{"GPT-5.6 Luna", "bad model", "/starts-wrong", "中文 模型（测试）", "x"} {
		if _, err := service.Create(context.Background(), CreateInput{ProviderID: providerID, UpstreamModelID: upstreamModelID, BillingGroupID: billingGroupID, PublicName: name, DisplayName: "name"}); err != nil {
			t.Fatalf("model name=%q should be accepted: %v", name, err)
		}
	}
	if _, err := service.Create(context.Background(), CreateInput{ProviderID: providerID, UpstreamModelID: upstreamModelID, BillingGroupID: billingGroupID, PublicName: "   ", DisplayName: "name"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank model name error=%v", err)
	}
	maxName := strings.Repeat("m", 256)
	if _, err := service.Create(context.Background(), CreateInput{ProviderID: providerID, UpstreamModelID: upstreamModelID, BillingGroupID: billingGroupID, PublicName: maxName, DisplayName: "Max Model"}); err != nil {
		t.Fatalf("256-rune model ID should be accepted: %v", err)
	}
	longName := strings.Repeat("m", 257)
	if _, err := service.Create(context.Background(), CreateInput{ProviderID: providerID, UpstreamModelID: upstreamModelID, BillingGroupID: billingGroupID, PublicName: longName, DisplayName: "Long Model"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("overlong model name error=%v", err)
	}
}

/**
 * TestResolveCandidatesDecryptsEveryProviderCredential 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestResolveCandidatesDecryptsEveryProviderCredential(t *testing.T) {
	cipher, _ := provider.NewCipher("01234567890123456789012345678901")
	firstEncrypted, _ := cipher.Encrypt("first-secret")
	secondEncrypted, _ := cipher.Encrypt("second-secret")
	store := &fakeStore{resolutions: []Resolution{
		{Record: Record{PublicName: "public"}, BaseURL: "https://first.example.com/v1", EncryptedAPIKey: firstEncrypted},
		{Record: Record{PublicName: "public"}, BaseURL: "https://second.example.com/v1", EncryptedAPIKey: secondEncrypted},
	}}
	groupID := uuid.New()
	resolved, err := NewService(store, cipher).ResolveCandidates(context.Background(), "public", groupID)
	if err != nil || len(resolved) != 2 || resolved[0].APIKey != "first-secret" || resolved[1].APIKey != "second-secret" || resolved[1].BaseURL != "https://second.example.com/v1" {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	if store.billingGroupID != groupID {
		t.Fatalf("billing group filter=%s want=%s", store.billingGroupID, groupID)
	}
}

/**
 * TestResolveCandidatesSkipsOneInvalidProviderCredential 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestResolveCandidatesSkipsOneInvalidProviderCredential(t *testing.T) {
	cipher, _ := provider.NewCipher("01234567890123456789012345678901")
	encrypted, _ := cipher.Encrypt("healthy-secret")
	store := &fakeStore{resolutions: []Resolution{
		{Record: Record{PublicName: "public"}, BaseURL: "https://broken.example.com/v1", EncryptedAPIKey: "not-encrypted"},
		{Record: Record{PublicName: "public"}, BaseURL: "https://healthy.example.com/v1", EncryptedAPIKey: encrypted},
	}}
	resolved, err := NewService(store, cipher).ResolveCandidates(context.Background(), "public", uuid.New())
	if err != nil || len(resolved) != 1 || resolved[0].APIKey != "healthy-secret" || resolved[0].BaseURL != "https://healthy.example.com/v1" {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
}

/**
 * TestUpdateRejectsNegativePrice 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestUpdateRejectsNegativePrice(t *testing.T) {
	cipher, _ := provider.NewCipher("01234567890123456789012345678901")
	value := int64(-1)
	_, err := NewService(&fakeStore{}, cipher).Update(context.Background(), uuid.New(), UpdateInput{InputPriceMicros: &value})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}
}

/**
 * TestDeleteValidatesIDAndDelegates 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestDeleteValidatesIDAndDelegates(t *testing.T) {
	cipher, _ := provider.NewCipher("01234567890123456789012345678901")
	store := &fakeStore{}
	service := NewService(store, cipher)
	if err := service.Delete(context.Background(), uuid.Nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid ID, got %v", err)
	}
	id := uuid.New()
	if err := service.Delete(context.Background(), id); err != nil || store.deletedID != id {
		t.Fatalf("delete id=%s stored=%s err=%v", id, store.deletedID, err)
	}
}
