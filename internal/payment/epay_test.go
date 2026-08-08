package payment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-pay/gopay"
	"github.com/google/uuid"
)

func testEPayGateway(t *testing.T) *EPayGateway {
	t.Helper()
	gateway, err := NewEPayGateway(EPayConfig{
		APIURL: "https://pay.example.com", MerchantID: "1000", MerchantKey: "merchant-secret",
		SiteName: "Novro", Channels: []string{"alipay", "wxpay"},
		NotifyURL: "https://novro.example.com/api/payments/epay/notify",
		ReturnURL: "https://novro.example.com/console/billing?payment=returned",
	})
	if err != nil {
		t.Fatalf("create EPay gateway: %v", err)
	}
	return gateway
}

func TestEPayCheckoutUsesSignedSubmitForm(t *testing.T) {
	gateway := testEPayGateway(t)
	checkout, err := gateway.Checkout(Order{
		ID: uuid.New(), OutTradeNo: "NVR0123456789", Channel: "alipay", AmountMicros: MinTopUpMicros,
	})
	if err != nil {
		t.Fatalf("build checkout: %v", err)
	}
	if checkout.Action != "https://pay.example.com/submit.php" || checkout.Method != "POST" {
		t.Fatalf("unexpected checkout target: %+v", checkout)
	}
	if checkout.Fields["money"] != "0.01" || checkout.Fields["pid"] != "1000" || checkout.Fields["sign"] == "" {
		t.Fatalf("unexpected checkout fields: %#v", checkout.Fields)
	}
	for key, value := range checkout.Fields {
		if key == "merchant-secret" || value == "merchant-secret" {
			t.Fatal("merchant key leaked into checkout fields")
		}
	}
}

func TestEPayNotificationRequiresValidSignatureAndExactMoney(t *testing.T) {
	gateway := testEPayGateway(t)
	values := url.Values{
		"pid": {"1000"}, "trade_no": {"EPAY-123"}, "out_trade_no": {"NVR0123456789"},
		"type": {"alipay"}, "name": {"Novro 余额充值"}, "money": {"10.00"},
		"trade_status": {"TRADE_SUCCESS"}, "sign_type": {"MD5"},
	}
	params := gopay.BodyMap{}
	for key, entries := range values {
		if key != "sign_type" {
			params.Set(key, entries[0])
		}
	}
	values.Set("sign", gateway.sign(params))

	notification, err := gateway.ParseNotification(values)
	if err != nil {
		t.Fatalf("parse signed notification: %v", err)
	}
	if notification.AmountMicros != 10_000_000 || notification.ProviderTradeNo != "EPAY-123" || notification.OutTradeNo != "NVR0123456789" {
		t.Fatalf("unexpected notification: %+v", notification)
	}

	values.Set("money", "100.00")
	if _, err := gateway.ParseNotification(values); err != ErrInvalidNotice {
		t.Fatalf("tampered notification error = %v, want %v", err, ErrInvalidNotice)
	}
}

func TestEPayNotificationRejectsAmbiguousAndImpreciseValues(t *testing.T) {
	gateway := testEPayGateway(t)
	if _, err := gateway.ParseNotification(url.Values{"pid": {"1000", "other"}}); err != ErrInvalidNotice {
		t.Fatalf("duplicate field error = %v, want %v", err, ErrInvalidNotice)
	}
	for _, value := range []string{"0", "1.001", "-1.00", "nan", "50000.01"} {
		if _, err := parseEPayMoney(value); err == nil {
			t.Fatalf("money %q was accepted", value)
		}
	}
}

func TestEPayQueryReturnsVerifiedPaidOrder(t *testing.T) {
	var received url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 1, "status": 1, "trade_no": "EPAY-123", "out_trade_no": "NVR0123456789", "type": "alipay", "money": "10.00",
		})
	}))
	defer server.Close()

	gateway, err := NewEPayGateway(EPayConfig{
		APIURL: server.URL, MerchantID: "1000", MerchantKey: "merchant-secret",
		SiteName: "Novro", Channels: []string{"alipay"},
		NotifyURL: "https://novro.example.com/api/payments/epay/notify",
		ReturnURL: "https://novro.example.com/api/payments/epay/return",
	})
	if err != nil {
		t.Fatalf("create EPay gateway: %v", err)
	}
	notification, paid, err := gateway.Query(context.Background(), "NVR0123456789")
	if err != nil || !paid {
		t.Fatalf("query paid order: paid=%v err=%v", paid, err)
	}
	if notification.ProviderTradeNo != "EPAY-123" || notification.AmountMicros != 10_000_000 || notification.Channel != "alipay" {
		t.Fatalf("unexpected notification: %+v", notification)
	}
	if received.Get("act") != "order" || received.Get("pid") != "1000" || received.Get("key") != "merchant-secret" || received.Get("out_trade_no") != "NVR0123456789" {
		t.Fatalf("unexpected query: %#v", received)
	}
}
