package payment

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/go-pay/gopay"
	"github.com/google/uuid"
)

type fakePaymentStore struct {
	created    CreateParams
	completed  CompleteParams
	orders     []Order
	listFilter AdminListFilter
	err        error
}

func (f *fakePaymentStore) Create(_ context.Context, params CreateParams) (Order, error) {
	f.created = params
	return Order{ID: params.ID, UserID: params.UserID, OutTradeNo: params.OutTradeNo, Provider: "epay", Channel: params.Channel, AmountMicros: params.AmountMicros, CreditedMicros: params.CreditedMicros, Status: StatusPending}, f.err
}

func (f *fakePaymentStore) List(context.Context, uuid.UUID, int) ([]Order, error) {
	return f.orders, f.err
}

func (f *fakePaymentStore) ListAll(_ context.Context, filter AdminListFilter) (AdminPage, error) {
	f.listFilter = filter
	return AdminPage{Orders: []AdminOrder{}}, f.err
}

func (f *fakePaymentStore) Complete(_ context.Context, params CompleteParams) (Order, error) {
	f.completed = params
	return Order{OutTradeNo: params.OutTradeNo, Status: StatusPaid}, f.err
}

type fakePaymentGateway struct{}

func (fakePaymentGateway) Channels() []string { return []string{"alipay"} }
func (fakePaymentGateway) Checkout(order Order) (Checkout, error) {
	return Checkout{Action: "https://pay.example.com/submit.php", Method: "POST", Fields: map[string]string{"out_trade_no": order.OutTradeNo}}, nil
}
func (fakePaymentGateway) ParseNotification(url.Values) (Notification, error) {
	return Notification{OutTradeNo: "NVR1", ProviderTradeNo: "EPAY1", Channel: "alipay", AmountMicros: 10_000_000}, nil
}

type fakePaymentConfigStore struct {
	record StoredConfig
	found  bool
}

func (f *fakePaymentConfigStore) Get(context.Context) (StoredConfig, error) {
	if !f.found {
		return StoredConfig{}, ErrConfigNotFound
	}
	return f.record, nil
}

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

func (fakePaymentCipher) Encrypt(value string) (string, error) { return "v1." + value, nil }
func (fakePaymentCipher) Decrypt(value string) (string, error) { return value[len("v1."):], nil }

func paymentTestDefaults() EPayConfig {
	return EPayConfig{NotifyURL: "https://novro.example.com/api/payments/epay/notify", ReturnURL: "https://novro.example.com/console/billing"}
}

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

func TestServiceCreatesCentAlignedTopUp(t *testing.T) {
	store := &fakePaymentStore{}
	service := NewService(store, nil, nil, EPayConfig{
		APIURL: "https://pay.example.com", MerchantID: "1000", MerchantKey: "secret",
		SiteName: "Novro", Channels: []string{"alipay"}, NotifyURL: "https://novro.example.com/api/payments/epay/notify", ReturnURL: "https://novro.example.com/console/billing",
	})
	userID := uuid.New()
	result, err := service.Create(context.Background(), userID, 10_000_000, " ALIPAY ")
	if err != nil {
		t.Fatalf("create top-up: %v", err)
	}
	if store.created.UserID != userID || store.created.AmountMicros != 10_000_000 || store.created.CreditedMicros != 10_000_000 || store.created.Channel != "alipay" || store.created.OutTradeNo == "" {
		t.Fatalf("unexpected create params: %+v", store.created)
	}
	if result.Checkout.Action == "" || result.Order.Status != StatusPending {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, amount := range []int64{999_999, 1_000_001, MaxTopUpMicros + 10_000} {
		if _, err := service.Create(context.Background(), userID, amount, "alipay"); err != ErrInvalidInput {
			t.Fatalf("amount %d error = %v, want %v", amount, err, ErrInvalidInput)
		}
	}
}

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
	if store.listFilter.Search != "alice" || store.listFilter.Channel != "bankpay" || store.listFilter.Status != StatusPaid {
		t.Fatalf("admin list filter = %+v", store.listFilter)
	}
	if _, err := service.ListAll(context.Background(), AdminListFilter{Limit: 101}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid admin list error = %v", err)
	}
}

func TestServiceCompletesVerifiedNotification(t *testing.T) {
	store := &fakePaymentStore{}
	service := NewService(store, nil, nil, EPayConfig{
		APIURL: "https://pay.example.com", MerchantID: "1000", MerchantKey: "merchant-secret",
		SiteName: "Novro", Channels: []string{"alipay"}, NotifyURL: "https://novro.example.com/api/payments/epay/notify", ReturnURL: "https://novro.example.com/console/billing",
	})
	values := signedServiceNotification(t)
	if err := service.HandleNotification(context.Background(), values); err != nil {
		t.Fatalf("complete notification: %v", err)
	}
	if store.completed.OutTradeNo != "NVR1" || store.completed.ProviderTradeNo != "EPAY1" || store.completed.AmountMicros != 10_000_000 || store.completed.PaidAt.IsZero() {
		t.Fatalf("unexpected completion: %+v", store.completed)
	}
}

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
