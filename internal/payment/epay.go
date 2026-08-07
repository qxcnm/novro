package payment

import (
	"crypto/md5" // #nosec G501 -- EPay v1 mandates MD5 for protocol compatibility.
	"crypto/subtle"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-pay/gopay"
	"github.com/google/uuid"
)

type EPayConfig struct {
	APIURL      string
	MerchantID  string
	MerchantKey string
	SiteName    string
	Channels    []string
	NotifyURL   string
	ReturnURL   string
}

type EPayGateway struct {
	submitURL   string
	merchantID  string
	merchantKey string
	siteName    string
	channels    []string
	notifyURL   string
	returnURL   string
}

func NewEPayGateway(config EPayConfig) (*EPayGateway, error) {
	submitURL, err := epaySubmitURL(config.APIURL)
	if err != nil || strings.TrimSpace(config.MerchantID) == "" || strings.TrimSpace(config.MerchantKey) == "" || strings.TrimSpace(config.SiteName) == "" {
		return nil, ErrInvalidInput
	}
	if !validAbsoluteHTTPURL(config.NotifyURL) || !validAbsoluteHTTPURL(config.ReturnURL) {
		return nil, ErrInvalidInput
	}
	channels := make([]string, 0, len(config.Channels))
	for _, channel := range config.Channels {
		channel = strings.ToLower(strings.TrimSpace(channel))
		if channel == "" || contains(channels, channel) {
			return nil, ErrInvalidInput
		}
		channels = append(channels, channel)
	}
	return &EPayGateway{
		submitURL: submitURL, merchantID: strings.TrimSpace(config.MerchantID), merchantKey: strings.TrimSpace(config.MerchantKey),
		siteName: strings.TrimSpace(config.SiteName), channels: channels, notifyURL: config.NotifyURL, returnURL: config.ReturnURL,
	}, nil
}

func (g *EPayGateway) Channels() []string {
	return append([]string(nil), g.channels...)
}

func (g *EPayGateway) Checkout(order Order) (Checkout, error) {
	if order.ID == uuid.Nil || order.OutTradeNo == "" || order.AmountMicros <= 0 || !contains(g.channels, order.Channel) {
		return Checkout{}, ErrInvalidInput
	}
	money, err := formatEPayMoney(order.AmountMicros)
	if err != nil {
		return Checkout{}, err
	}
	params := gopay.BodyMap{}
	params.Set("pid", g.merchantID).
		Set("type", order.Channel).
		Set("out_trade_no", order.OutTradeNo).
		Set("notify_url", g.notifyURL).
		Set("return_url", g.returnURL).
		Set("name", "Novro 余额充值").
		Set("money", money).
		Set("sitename", g.siteName)
	fields := make(map[string]string, len(params)+2)
	params.Range(func(key string, value any) bool {
		fields[key] = params.GetString(key)
		return true
	})
	fields["sign"] = g.sign(params)
	fields["sign_type"] = "MD5"
	return Checkout{Action: g.submitURL, Method: "POST", Fields: fields}, nil
}

func (g *EPayGateway) ParseNotification(values url.Values) (Notification, error) {
	params := make(gopay.BodyMap, len(values))
	for key, entries := range values {
		if len(entries) != 1 {
			return Notification{}, ErrInvalidNotice
		}
		if key != "sign" && key != "sign_type" && entries[0] != "" {
			params.Set(key, entries[0])
		}
	}
	required := []string{"pid", "trade_no", "out_trade_no", "type", "money", "trade_status", "sign", "sign_type"}
	for _, key := range required {
		if strings.TrimSpace(values.Get(key)) == "" {
			return Notification{}, ErrInvalidNotice
		}
	}
	if !constantTimeEqual(values.Get("pid"), g.merchantID) || !strings.EqualFold(values.Get("sign_type"), "MD5") || !constantTimeEqual(strings.ToLower(values.Get("sign")), g.sign(params)) {
		return Notification{}, ErrInvalidNotice
	}
	if values.Get("trade_status") != "TRADE_SUCCESS" || !paymentChannelPattern.MatchString(values.Get("type")) {
		return Notification{}, ErrInvalidNotice
	}
	if len(values.Get("out_trade_no")) > 64 || len(values.Get("trade_no")) > 128 || len(values.Get("type")) > 32 {
		return Notification{}, ErrInvalidNotice
	}
	amount, err := parseEPayMoney(values.Get("money"))
	if err != nil {
		return Notification{}, ErrInvalidNotice
	}
	return Notification{
		OutTradeNo: values.Get("out_trade_no"), ProviderTradeNo: values.Get("trade_no"),
		Channel: values.Get("type"), AmountMicros: amount,
	}, nil
}

func (g *EPayGateway) sign(params gopay.BodyMap) string {
	// GoPay provides the same sorted key=value canonicalization used by EPay.
	payload := params.EncodeAliPaySignParams() + g.merchantKey
	digest := md5.Sum([]byte(payload)) // #nosec G401 -- required by the EPay v1 wire protocol.
	return fmt.Sprintf("%x", digest)
}

func epaySubmitURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrInvalidInput
	}
	if !strings.HasSuffix(strings.ToLower(parsed.Path), ".php") {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/submit.php"
	}
	return parsed.String(), nil
}

func validAbsoluteHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Host != "" && parsed.User == nil && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func formatEPayMoney(micros int64) (string, error) {
	if micros <= 0 || micros%10_000 != 0 {
		return "", ErrInvalidInput
	}
	cents := micros / 10_000
	return fmt.Sprintf("%d.%02d", cents/100, cents%100), nil
}

func parseEPayMoney(value string) (int64, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ".")
	if len(parts) > 2 || len(parts[0]) == 0 || len(parts[0]) > 12 || !asciiDigits(parts[0]) {
		return 0, ErrInvalidNotice
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) == 0 || len(fraction) > 2 || !asciiDigits(fraction) {
			return 0, ErrInvalidNotice
		}
	}
	if len(fraction) == 1 {
		fraction += "0"
	}
	if fraction == "" {
		fraction = "00"
	}
	var yuan, cents int64
	for _, char := range parts[0] {
		yuan = yuan*10 + int64(char-'0')
	}
	for _, char := range fraction {
		cents = cents*10 + int64(char-'0')
	}
	amount := (yuan*100 + cents) * 10_000
	if amount <= 0 || amount > MaxTopUpMicros {
		return 0, ErrInvalidNotice
	}
	return amount, nil
}

func asciiDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func constantTimeEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
