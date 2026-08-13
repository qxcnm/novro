package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-pay/gopay"
	"github.com/google/uuid"
)

type fakePaymentStore struct {
	created     CreateParams
	completed   CompleteParams
	orders      []Order
	listFilter  ListFilter
	adminFilter AdminListFilter
	err         error
}

/**
 * Create 用于创建并返回所需的对象或记录。
 * @param params 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakePaymentStore) Create(_ context.Context, params CreateParams) (Order, error) {
	f.created = params
	return Order{ID: params.ID, UserID: params.UserID, OutTradeNo: params.OutTradeNo, Provider: "epay", Channel: params.Channel, AmountMicros: params.AmountMicros, CreditedMicros: params.CreditedMicros, Status: StatusPending}, f.err
}

/**
 * Get 用于查询并返回所需的数据。
 * @param outTradeNo 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakePaymentStore) Get(_ context.Context, outTradeNo string) (Order, error) {
	for _, order := range f.orders {
		if order.OutTradeNo == outTradeNo {
			return order, f.err
		}
	}
	return Order{}, ErrOrderNotFound
}

/**
 * List 用于筛选并返回数据列表。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakePaymentStore) List(_ context.Context, _ uuid.UUID, filter ListFilter) (Page, error) {
	f.listFilter = filter
	return Page{Orders: f.orders, Total: len(f.orders), Offset: filter.Offset, Limit: filter.Limit}, f.err
}

/**
 * ListAll 用于筛选并返回数据列表。
 * @param filter 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakePaymentStore) ListAll(_ context.Context, filter AdminListFilter) (AdminPage, error) {
	f.adminFilter = filter
	return AdminPage{Orders: []AdminOrder{}}, f.err
}

/**
 * Complete 封装该名称对应的业务处理逻辑。
 * @param params 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakePaymentStore) Complete(_ context.Context, params CompleteParams) (Order, error) {
	f.completed = params
	return Order{OutTradeNo: params.OutTradeNo, Status: StatusPaid}, f.err
}

type fakePaymentGateway struct{}

/**
 * Channels 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (fakePaymentGateway) Channels() []string { return []string{"alipay"} }

/**
 * Checkout 用于校验输入或运行状态是否满足要求。
 * @param order 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (fakePaymentGateway) Checkout(order Order) (Checkout, error) {
	return Checkout{Action: "https://pay.example.com/submit.php", Method: "POST", Fields: map[string]string{"out_trade_no": order.OutTradeNo}}, nil
}

/**
 * ParseNotification 用于解析输入并转换为内部数据结构。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (fakePaymentGateway) ParseNotification(url.Values) (Notification, error) {
	return Notification{OutTradeNo: "NVR1", ProviderTradeNo: "EPAY1", Channel: "alipay", AmountMicros: 10_000_000}, nil
}

/**
 * Query 用于查询并返回所需的数据。
 * @param string 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (fakePaymentGateway) Query(context.Context, string) (Notification, bool, error) {
	return Notification{OutTradeNo: "NVR1", ProviderTradeNo: "EPAY1", Channel: "alipay", AmountMicros: 10_000_000}, true, nil
}

type fakePaymentConfigStore struct {
	record StoredConfig
	found  bool
}

/**
 * Get 用于查询并返回所需的数据。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakePaymentConfigStore) Get(context.Context) (StoredConfig, error) {
	if !f.found {
		return StoredConfig{}, ErrConfigNotFound
	}
	return f.record, nil
}

/**
 * Upsert 封装该名称对应的业务处理逻辑。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakePaymentConfigStore) Upsert(_ context.Context, input StoredConfigInput) (StoredConfig, error) {
	f.found = true
	f.record = StoredConfig{
		Provider: input.Provider, Enabled: input.Enabled, APIURL: input.APIURL, MerchantID: input.MerchantID,
		EncryptedMerchantKey: input.EncryptedMerchantKey, SiteName: input.SiteName, Channels: append([]string{}, input.Channels...),
		Methods: append([]PaymentMethod{}, input.Methods...), MinMicros: input.MinMicros, MaxMicros: input.MaxMicros,
		PresetAmountMicros: append([]int64{}, input.PresetAmountMicros...), BonusTiers: append([]BonusTier{}, input.BonusTiers...),
	}
	return f.record, nil
}

type fakePaymentCipher struct{}

/**
 * Encrypt 用于对敏感数据执行安全转换。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (fakePaymentCipher) Encrypt(value string) (string, error) { return "v1." + value, nil }

/**
 * Decrypt 用于解密并返回受保护的数据。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (fakePaymentCipher) Decrypt(value string) (string, error) { return value[len("v1."):], nil }

/**
 * paymentTestDefaults 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func paymentTestDefaults() EPayConfig {
	return EPayConfig{NotifyURL: "https://app.example.invalid/api/payments/epay/notify", ReturnURL: "https://app.example.invalid/console/billing"}
}

/**
 * TestServiceReadsLatestStoredPaymentChannels 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceReadsLatestStoredPaymentChannels(t *testing.T) {
	configStore := &fakePaymentConfigStore{found: true, record: StoredConfig{Provider: ProviderEPay, Enabled: true, APIURL: "https://pay.example.com", MerchantID: "1000", EncryptedMerchantKey: "v1.secret", SiteName: "Novro", Channels: []string{"alipay"}}}
	service := NewService(&fakePaymentStore{}, configStore, fakePaymentCipher{}, paymentTestDefaults())
	public, err := service.Config(context.Background())
	if err != nil || len(public.Channels) != 1 || public.Channels[0] != "alipay" {
		t.Fatalf("initial payment config = %+v, err=%v", public, err)
	}
	configStore.record.Channels = []string{"wxpay"}
	public, err = service.Config(context.Background())
	if err != nil || len(public.Channels) != 1 || public.Channels[0] != "wxpay" {
		t.Fatalf("updated payment config = %+v, err=%v", public, err)
	}
}

/**
 * TestServiceAdminConfigReturnsEmptyChannelsAsArray 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceAdminConfigReturnsEmptyChannelsAsArray(t *testing.T) {
	service := NewService(&fakePaymentStore{}, &fakePaymentConfigStore{}, fakePaymentCipher{}, paymentTestDefaults())
	admin, err := service.AdminConfig(context.Background())
	if err != nil {
		t.Fatalf("read payment config: %v", err)
	}
	if admin.Channels == nil || len(admin.Channels) != 0 {
		t.Fatalf("empty payment channels = %#v, want non-nil empty slice", admin.Channels)
	}
}

/**
 * TestServiceRaisesLegacyStoredTopUpMinimum 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceRaisesLegacyStoredTopUpMinimum(t *testing.T) {
	configStore := &fakePaymentConfigStore{found: true, record: StoredConfig{
		Provider: ProviderEPay, SiteName: "Novro", Channels: []string{"alipay"},
		Methods:            []PaymentMethod{{Code: "alipay", Name: "支付宝", Icon: "smartphone", MinMicros: 10_000, Enabled: true}},
		MinMicros:          10_000,
		MaxMicros:          MaxTopUpMicros,
		PresetAmountMicros: []int64{10_000_000},
		BonusTiers:         []BonusTier{},
	}}
	service := NewService(&fakePaymentStore{}, configStore, fakePaymentCipher{}, paymentTestDefaults())
	admin, err := service.AdminConfig(context.Background())
	if err != nil {
		t.Fatalf("read legacy payment config: %v", err)
	}
	if admin.MinMicros != MinTopUpMicros || len(admin.Methods) != 1 || admin.Methods[0].MinMicros != MinTopUpMicros {
		t.Fatalf("legacy minimum was not raised: %+v", admin)
	}
}

/**
 * TestServiceUpdateEncryptsMerchantKeyAndReturnsSafeAdminConfig 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceUpdateEncryptsMerchantKeyAndReturnsSafeAdminConfig(t *testing.T) {
	configStore := &fakePaymentConfigStore{}
	service := NewService(&fakePaymentStore{}, configStore, fakePaymentCipher{}, paymentTestDefaults())
	key := "merchant-secret"
	admin, err := service.UpdateConfig(context.Background(), ConfigInput{Enabled: true, APIURL: "https://pay.example.com", MerchantID: "1000", MerchantKey: &key, SiteName: "Novro", Channels: []string{"alipay", "wxpay"}})
	if err != nil {
		t.Fatalf("update payment config: %v", err)
	}
	if !admin.Enabled || !admin.Configured || !admin.HasMerchantKey || configStore.record.EncryptedMerchantKey != "v1.merchant-secret" {
		t.Fatalf("unexpected admin config: %+v stored=%+v", admin, configStore.record)
	}
	if strings.Contains(fmt.Sprintf("%+v", admin), key) {
		t.Fatal("merchant key leaked through admin config")
	}
}

/**
 * TestServiceCreatesCentAlignedTopUp 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceCreatesCentAlignedTopUp(t *testing.T) {
	store := &fakePaymentStore{}
	service := NewService(store, nil, nil, EPayConfig{
		APIURL: "https://pay.example.com", MerchantID: "1000", MerchantKey: "secret",
		SiteName: "Novro", Channels: []string{"alipay"}, NotifyURL: "https://app.example.invalid/api/payments/epay/notify", ReturnURL: "https://app.example.invalid/console/billing",
	})
	userID := uuid.New()
	result, err := service.Create(context.Background(), userID, MinTopUpMicros, " ALIPAY ")
	if err != nil {
		t.Fatalf("create top-up: %v", err)
	}
	if store.created.UserID != userID || store.created.AmountMicros != MinTopUpMicros || store.created.CreditedMicros != MinTopUpMicros || store.created.Channel != "alipay" || store.created.OutTradeNo == "" {
		t.Fatalf("unexpected create params: %+v", store.created)
	}
	if result.Checkout.Action == "" || result.Checkout.Fields["money"] != "1.00" || result.Order.Status != StatusPending {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, amount := range []int64{990_000, 1_000_001, MaxTopUpMicros + 10_000} {
		if _, err := service.Create(context.Background(), userID, amount, "alipay"); err != ErrInvalidInput {
			t.Fatalf("amount %d error = %v, want %v", amount, err, ErrInvalidInput)
		}
	}
}

/**
 * TestServiceAppliesHighestTopUpBonusWithoutChangingPaymentAmount 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceAppliesHighestTopUpBonusWithoutChangingPaymentAmount(t *testing.T) {
	store := &fakePaymentStore{}
	configStore := &fakePaymentConfigStore{found: true, record: StoredConfig{
		Provider: ProviderEPay, Enabled: true, APIURL: "https://pay.example.com", MerchantID: "1000",
		EncryptedMerchantKey: "v1.secret", SiteName: "Novro",
		Methods:   []PaymentMethod{{Code: "alipay", Name: "支付宝", Icon: "smartphone", MinMicros: 10_000_000, Enabled: true}},
		MinMicros: 10_000_000, MaxMicros: MaxTopUpMicros, PresetAmountMicros: []int64{100_000_000},
		BonusTiers: []BonusTier{{ThresholdMicros: 50_000_000, BonusBPS: 500}, {ThresholdMicros: 100_000_000, BonusBPS: 1000}},
	}}
	service := NewService(store, configStore, fakePaymentCipher{}, paymentTestDefaults())
	result, err := service.Create(context.Background(), uuid.New(), 100_000_000, "alipay")
	if err != nil {
		t.Fatalf("create top-up with bonus: %v", err)
	}
	if store.created.AmountMicros != 100_000_000 || store.created.CreditedMicros != 110_000_000 {
		t.Fatalf("create params = %+v", store.created)
	}
	if result.Order.AmountMicros != 100_000_000 || result.Order.CreditedMicros != 110_000_000 {
		t.Fatalf("order = %+v", result.Order)
	}
	if result.Checkout.Fields["money"] != "100.00" {
		t.Fatalf("checkout money = %q, want paid amount 100.00", result.Checkout.Fields["money"])
	}
}

/**
 * TestServiceEnforcesPaymentMethodMinimumAndAdminListFilter 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceEnforcesPaymentMethodMinimumAndAdminListFilter(t *testing.T) {
	store := &fakePaymentStore{}
	configStore := &fakePaymentConfigStore{found: true, record: StoredConfig{
		Provider: ProviderEPay, Enabled: true, APIURL: "https://pay.example.com", MerchantID: "1000",
		EncryptedMerchantKey: "v1.secret", SiteName: "Novro",
		Methods:   []PaymentMethod{{Code: "bankpay", Name: "银行卡", Icon: "card", MinMicros: 50_000_000, Enabled: true}},
		MinMicros: MinTopUpMicros, MaxMicros: MaxTopUpMicros, PresetAmountMicros: []int64{50_000_000}, BonusTiers: []BonusTier{},
	}}
	service := NewService(store, configStore, fakePaymentCipher{}, paymentTestDefaults())
	if _, err := service.Create(context.Background(), uuid.New(), 10_000_000, "bankpay"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("method minimum error = %v", err)
	}
	if _, err := service.ListAll(context.Background(), AdminListFilter{Search: " alice ", Status: StatusPaid, Channel: " BANKPAY ", Limit: 50}); err != nil {
		t.Fatalf("list all top-ups: %v", err)
	}
	if store.adminFilter.Search != "alice" || store.adminFilter.Channel != "bankpay" || store.adminFilter.Status != StatusPaid {
		t.Fatalf("admin list filter = %+v", store.adminFilter)
	}
	if _, err := service.ListAll(context.Background(), AdminListFilter{Limit: 101}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid admin list error = %v", err)
	}
}

/**
 * TestServiceListsUserTopUpsWithPagination 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceListsUserTopUpsWithPagination(t *testing.T) {
	store := &fakePaymentStore{orders: []Order{{ID: uuid.New()}}}
	service := NewService(store, nil, nil, EPayConfig{})
	page, err := service.List(context.Background(), uuid.New(), ListFilter{Offset: 20, Limit: 50})
	if err != nil {
		t.Fatalf("list user top-ups: %v", err)
	}
	if store.listFilter.Offset != 20 || store.listFilter.Limit != 50 || page.Total != 1 {
		t.Fatalf("page=%+v filter=%+v", page, store.listFilter)
	}
	if _, err := service.List(context.Background(), uuid.New(), ListFilter{Limit: 101}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid user list error = %v", err)
	}
}

/**
 * TestServiceCompletesVerifiedNotification 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceCompletesVerifiedNotification(t *testing.T) {
	store := &fakePaymentStore{}
	service := NewService(store, nil, nil, EPayConfig{
		APIURL: "https://pay.example.com", MerchantID: "1000", MerchantKey: "merchant-secret",
		SiteName: "Novro", Channels: []string{"alipay"}, NotifyURL: "https://app.example.invalid/api/payments/epay/notify", ReturnURL: "https://app.example.invalid/console/billing",
	})
	values := signedServiceNotification(t)
	if err := service.HandleNotification(context.Background(), values); err != nil {
		t.Fatalf("complete notification: %v", err)
	}
	if store.completed.OutTradeNo != "NVR1" || store.completed.ProviderTradeNo != "EPAY1" || store.completed.AmountMicros != 10_000_000 || store.completed.PaidAt.IsZero() {
		t.Fatalf("unexpected completion: %+v", store.completed)
	}
}

/**
 * TestServiceReconcilesProviderPaidOrder 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceReconcilesProviderPaidOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 1, "status": 1, "trade_no": "EPAY1", "out_trade_no": "NVR1", "type": "alipay", "money": "10.00",
		})
	}))
	defer server.Close()
	store := &fakePaymentStore{orders: []Order{{
		ID: uuid.New(), OutTradeNo: "NVR1", Channel: "alipay", AmountMicros: 10_000_000, CreditedMicros: 10_000_000, Status: StatusPending,
	}}}
	service := NewService(store, nil, nil, EPayConfig{
		APIURL: server.URL, MerchantID: "1000", MerchantKey: "merchant-secret",
		SiteName: "Novro", Channels: []string{"alipay"},
		NotifyURL: "https://app.example.invalid/api/payments/epay/notify",
		ReturnURL: "https://app.example.invalid/api/payments/epay/return",
	})
	order, err := service.Reconcile(context.Background(), "NVR1")
	if err != nil {
		t.Fatalf("reconcile paid order: %v", err)
	}
	if order.Status != StatusPaid || store.completed.OutTradeNo != "NVR1" || store.completed.ProviderTradeNo != "EPAY1" || store.completed.AmountMicros != 10_000_000 {
		t.Fatalf("unexpected reconciliation: order=%+v completed=%+v", order, store.completed)
	}
}

/**
 * TestServiceDoesNotQueryAlreadyPaidOrder 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceDoesNotQueryAlreadyPaidOrder(t *testing.T) {
	paid := Order{ID: uuid.New(), OutTradeNo: "NVR1", Status: StatusPaid}
	store := &fakePaymentStore{orders: []Order{paid}}
	service := NewService(store, nil, nil, EPayConfig{})
	order, err := service.Reconcile(context.Background(), "NVR1")
	if err != nil || order.Status != StatusPaid {
		t.Fatalf("reconcile already paid order: order=%+v err=%v", order, err)
	}
	if store.completed.OutTradeNo != "" {
		t.Fatalf("already paid order was completed again: %+v", store.completed)
	}
}

/**
 * TestServiceReconcileForUserRejectsAnotherUsersOrder 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceReconcileForUserRejectsAnotherUsersOrder(t *testing.T) {
	ownerID := uuid.New()
	store := &fakePaymentStore{orders: []Order{{ID: uuid.New(), UserID: ownerID, OutTradeNo: "NVR1", Status: StatusPending}}}
	service := NewService(store, nil, nil, EPayConfig{})

	if _, err := service.ReconcileForUser(context.Background(), uuid.New(), "NVR1"); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("reconcile another user's order error = %v, want %v", err, ErrOrderNotFound)
	}
	if store.completed.OutTradeNo != "" {
		t.Fatalf("another user's order was completed: %+v", store.completed)
	}
}

/**
 * TestServiceReconcileForUserReturnsOwnedPaidOrderWithoutQuery 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceReconcileForUserReturnsOwnedPaidOrderWithoutQuery(t *testing.T) {
	ownerID := uuid.New()
	paid := Order{ID: uuid.New(), UserID: ownerID, OutTradeNo: "NVR1", Status: StatusPaid}
	store := &fakePaymentStore{orders: []Order{paid}}
	service := NewService(store, nil, nil, EPayConfig{})

	order, err := service.ReconcileForUser(context.Background(), ownerID, "NVR1")
	if err != nil || order.Status != StatusPaid {
		t.Fatalf("reconcile owned paid order: order=%+v err=%v", order, err)
	}
	if store.completed.OutTradeNo != "" {
		t.Fatalf("owned paid order was completed again: %+v", store.completed)
	}
}

/**
 * TestServiceAcceptsPendingNotificationAfterPaymentsAreDisabled 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestServiceAcceptsPendingNotificationAfterPaymentsAreDisabled(t *testing.T) {
	store := &fakePaymentStore{}
	configStore := &fakePaymentConfigStore{found: true, record: StoredConfig{Provider: ProviderEPay, Enabled: false, APIURL: "https://pay.example.com", MerchantID: "1000", EncryptedMerchantKey: "v1.merchant-secret", SiteName: "Novro", Channels: []string{}}}
	service := NewService(store, configStore, fakePaymentCipher{}, paymentTestDefaults())
	if err := service.HandleNotification(context.Background(), signedServiceNotification(t)); err != nil {
		t.Fatalf("complete disabled payment notification: %v", err)
	}
	if store.completed.OutTradeNo != "NVR1" {
		t.Fatalf("notification was not completed: %+v", store.completed)
	}
	if _, err := service.Create(context.Background(), uuid.New(), 10_000_000, "alipay"); err != ErrDisabled {
		t.Fatalf("create while disabled error = %v, want %v", err, ErrDisabled)
	}
}

/**
 * signedServiceNotification 封装该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func signedServiceNotification(t *testing.T) url.Values {
	t.Helper()
	gateway := testEPayGateway(t)
	values := url.Values{"pid": {"1000"}, "trade_no": {"EPAY1"}, "out_trade_no": {"NVR1"}, "type": {"alipay"}, "name": {"Novro 余额充值"}, "money": {"10.00"}, "trade_status": {"TRADE_SUCCESS"}, "sign_type": {"MD5"}}
	params := gopay.BodyMap{}
	for key, entries := range values {
		if key != "sign_type" {
			params.Set(key, entries[0])
		}
	}
	values.Set("sign", gateway.sign(params))
	return values
}
