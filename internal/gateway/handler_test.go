package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/apikey"
	"github.com/novro-gateway/novro/internal/billing"
	"github.com/novro-gateway/novro/internal/billinggroup"
	"github.com/novro-gateway/novro/internal/gatewaysettings"
	"github.com/novro-gateway/novro/internal/modelroute"
	"github.com/novro-gateway/novro/internal/provider"
	"github.com/novro-gateway/novro/internal/upstreammodel"
	"github.com/novro-gateway/novro/internal/user"
)

type fakeKeys struct {
	actor apikey.Actor
	err   error
}

type fakeGatewaySettings struct {
	config gatewaysettings.Config
	err    error
}

func (f fakeGatewaySettings) Config(context.Context) (gatewaysettings.Config, error) {
	if f.err != nil {
		return gatewaysettings.Config{}, f.err
	}
	return f.config, nil
}

func (f fakeKeys) Authenticate(context.Context, string) (apikey.Actor, error) { return f.actor, f.err }

type fakeRoutes struct {
	route           modelroute.Resolved
	candidates      []modelroute.Resolved
	records         []modelroute.Record
	expectedGroupID uuid.UUID
	err             error
}

func (f fakeRoutes) ResolveCandidates(_ context.Context, _ string, billingGroupID uuid.UUID) ([]modelroute.Resolved, error) {
	if f.expectedGroupID != uuid.Nil && billingGroupID != f.expectedGroupID {
		return nil, fmt.Errorf("unexpected billing group %s", billingGroupID)
	}
	if f.err != nil {
		return nil, f.err
	}
	if f.candidates != nil {
		return f.candidates, nil
	}
	return []modelroute.Resolved{f.route}, nil
}
func (f fakeRoutes) ListActive(_ context.Context, billingGroupID uuid.UUID) ([]modelroute.Record, error) {
	if f.expectedGroupID != uuid.Nil && billingGroupID != f.expectedGroupID {
		return nil, fmt.Errorf("unexpected billing group %s", billingGroupID)
	}
	return f.records, f.err
}

type fakeBilling struct {
	reserveErr     error
	refundErrors   []error
	finalizeErrors []error
	reserved       int64
	reserveCalls   int
	refunded       int64
	refundCalls    int
	finalizeCalls  int
	usage          billing.UsageInput
	failures       []billing.FailureInput
}

func (f *fakeBilling) Reserve(_ context.Context, _, _ uuid.UUID, amount int64, _ string) error {
	f.reserveCalls++
	f.reserved = amount
	return f.reserveErr
}
func (f *fakeBilling) Refund(_ context.Context, _, _ uuid.UUID, amount int64, _ string) error {
	f.refundCalls++
	if len(f.refundErrors) > 0 {
		err := f.refundErrors[0]
		f.refundErrors = f.refundErrors[1:]
		if err != nil {
			return err
		}
	}
	f.refunded += amount
	return nil
}
func (f *fakeBilling) Finalize(_ context.Context, input billing.UsageInput) error {
	f.finalizeCalls++
	if len(f.finalizeErrors) > 0 {
		err := f.finalizeErrors[0]
		f.finalizeErrors = f.finalizeErrors[1:]
		if err != nil {
			return err
		}
	}
	f.usage = input
	return nil
}
func (f *fakeBilling) RecordFailure(_ context.Context, input billing.FailureInput) error {
	f.failures = append(f.failures, input)
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type blockingBody struct {
	reader  *strings.Reader
	release chan struct{}
	once    sync.Once
}

func newBlockingBody(prefix string) *blockingBody {
	return &blockingBody{reader: strings.NewReader(prefix), release: make(chan struct{})}
}

func (b *blockingBody) Read(target []byte) (int, error) {
	if b.reader.Len() > 0 {
		return b.reader.Read(target)
	}
	<-b.release
	return 0, io.EOF
}

func (b *blockingBody) Close() error {
	b.once.Do(func() { close(b.release) })
	return nil
}

type noopBilling struct{}

func (noopBilling) Reserve(context.Context, uuid.UUID, uuid.UUID, int64, string) error { return nil }
func (noopBilling) Refund(context.Context, uuid.UUID, uuid.UUID, int64, string) error  { return nil }
func (noopBilling) Finalize(context.Context, billing.UsageInput) error                 { return nil }
func (noopBilling) RecordFailure(context.Context, billing.FailureInput) error          { return nil }

type terminalErrorReader struct {
	reader *strings.Reader
	err    error
}

func (r *terminalErrorReader) Read(target []byte) (int, error) {
	if r.reader.Len() > 0 {
		read, _ := r.reader.Read(target)
		return read, nil
	}
	return 0, r.err
}

func gatewayActor() apikey.Actor {
	groupID := uuid.New()
	return apikey.Actor{APIKey: apikey.Record{ID: uuid.New(), BillingGroupID: groupID, BillingGroup: billinggroup.Summary{ID: groupID, Code: "default", DisplayName: "默认", MultiplierBPS: 10_000}}, User: user.Record{ID: uuid.New(), Status: user.StatusActive}}
}

func openAIRoute() modelroute.Resolved {
	routeID, upstreamID := uuid.New(), uuid.New()
	return modelroute.Resolved{Record: modelroute.Record{ID: routeID, UpstreamModelID: &upstreamID, PublicName: "deepseek-chat", UpstreamName: "deepseek-v3", InputPriceMicros: 2_000_000, OutputPriceMicros: 8_000_000, Provider: modelroute.ProviderSummary{Code: "deepseek", Protocol: provider.ProtocolOpenAI}, UpstreamModel: &upstreammodel.Record{ID: upstreamID, UpstreamName: "deepseek-v3", Prices: upstreammodel.Prices{InputMicros: 2_000_000, OutputMicros: 8_000_000}}}, BaseURL: "https://api.example.com/v1", APIKey: "upstream-secret"}
}

func anthropicRoute() modelroute.Resolved {
	routeID, upstreamID := uuid.New(), uuid.New()
	return modelroute.Resolved{Record: modelroute.Record{ID: routeID, UpstreamModelID: &upstreamID, PublicName: "kimi-k3", UpstreamName: "kimi-k3-upstream", InputPriceMicros: 2_000_000, OutputPriceMicros: 8_000_000, Provider: modelroute.ProviderSummary{Code: "kimi", Protocol: provider.ProtocolAnthropic}, UpstreamModel: &upstreammodel.Record{ID: upstreamID, UpstreamName: "kimi-k3-upstream", Prices: upstreammodel.Prices{InputMicros: 2_000_000, OutputMicros: 8_000_000}}}, BaseURL: "https://api.anthropic.com/v1", APIKey: "anthropic-secret"}
}

func openAIChannel(code, host, upstreamName string) modelroute.Resolved {
	route := openAIRoute()
	route.ID = uuid.New()
	upstreamID := uuid.New()
	route.UpstreamModelID = &upstreamID
	route.UpstreamModel.ID = upstreamID
	route.Provider.Code = code
	route.BaseURL = "https://" + host + "/v1"
	route.APIKey = code + "-secret"
	route.UpstreamName = upstreamName
	route.UpstreamModel.UpstreamName = upstreamName
	return route
}

func TestProxyRoutesModelAndFinalizesExactUsage(t *testing.T) {
	biller := &fakeBilling{}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.example.com/v1/chat/completions" {
			t.Fatalf("unexpected upstream URL %s", request.URL)
		}
		if request.Header.Get("Authorization") != "Bearer upstream-secret" {
			t.Fatalf("upstream credential missing")
		}
		body, _ := io.ReadAll(request.Body)
		if !strings.Contains(string(body), `"model":"deepseek-v3"`) {
			t.Fatalf("model was not mapped: %s", body)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"up-1","usage":{"prompt_tokens":10,"completion_tokens":20}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}],"max_tokens":100}`))
	request.Header.Set("Authorization", "Bearer nvr_test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if biller.reserved <= 180 || biller.usage.CostMicros != 180 || biller.usage.InputTokens != 10 || biller.usage.OutputTokens != 20 || biller.usage.UpstreamRequestID != "up-1" {
		t.Fatalf("unexpected billing reservation=%d usage=%+v", biller.reserved, biller.usage)
	}
	requestID, err := uuid.Parse(response.Header().Get("X-Novro-Request-ID"))
	if err != nil || biller.usage.RequestID != requestID {
		t.Fatalf("response and billing request ids differ: header=%q usage=%s err=%v", response.Header().Get("X-Novro-Request-ID"), biller.usage.RequestID, err)
	}
}

func TestProxySSEHeartbeatKeepsEstablishedStreamActive(t *testing.T) {
	body := newBlockingBody("data: {\"id\":\"heartbeat\",\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}, nil
	})}
	settings := gatewaysettings.DefaultConfig()
	settings.SSEHeartbeatIntervalMS = 20
	settings.UpstreamStreamIdleTimeoutMS = 0
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: &fakeBilling{}, Client: client, Settings: fakeGatewaySettings{config: settings}})
	server := httptest.NewServer(handler)
	defer server.Close()
	defer body.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer nvr_test")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if response.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("X-Accel-Buffering = %q, want no", response.Header.Get("X-Accel-Buffering"))
	}
	reader := bufio.NewReader(response.Body)
	first, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(first, `data: {"id":"heartbeat"`) {
		t.Fatalf("initial event = %q, err = %v", first, err)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("initial event separator error = %v", err)
	}
	heartbeat, err := reader.ReadString('\n')
	if err != nil || heartbeat != ": novro-keepalive\n" {
		t.Fatalf("heartbeat = %q, err = %v", heartbeat, err)
	}
	response.Body.Close()
}

func TestProxyStopsIdleStream(t *testing.T) {
	body := newBlockingBody("data: {\"id\":\"idle\",\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}, nil
	})}
	settings := gatewaysettings.DefaultConfig()
	settings.SSEHeartbeatEnabled = false
	settings.UpstreamStreamIdleTimeoutMS = 20
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: &fakeBilling{}, Client: client, Settings: fakeGatewaySettings{config: settings}})
	server := httptest.NewServer(handler)
	defer server.Close()
	defer body.Close()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer nvr_test")
	start := time.Now()
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatalf("read response error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("idle stream took too long: %s", elapsed)
	}
	if !strings.Contains(string(data), "data: {\"id\":\"idle\"") {
		t.Fatalf("response lost initial event: %q", data)
	}
}

func TestProxyReturnsGatewayTimeoutForUpstreamTotalTimeout(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	settings := gatewaysettings.DefaultConfig()
	settings.SSEHeartbeatEnabled = false
	settings.UpstreamTimeoutMS = 20
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: &fakeBilling{}, Client: client, Settings: fakeGatewaySettings{config: settings}})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Authorization", "Bearer nvr_test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("status=%d body=%s, want 504", response.Code, response.Body.String())
	}
}

func TestProxyReturnsGatewayTimeoutWhenBufferedBodyExceedsTotalTimeout(t *testing.T) {
	body := newBlockingBody("")
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		go func() {
			<-request.Context().Done()
			_ = body.Close()
		}()
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: body}, nil
	})}
	settings := gatewaysettings.DefaultConfig()
	settings.SSEHeartbeatEnabled = false
	settings.UpstreamTimeoutMS = 20
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: &fakeBilling{}, Client: client, Settings: fakeGatewaySettings{config: settings}})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Authorization", "Bearer nvr_test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("status=%d body=%s, want 504", response.Code, response.Body.String())
	}
}

func TestParseUsageRejectsMalformedReportedFields(t *testing.T) {
	rates := rateCardFor(openAIRoute())
	tests := []struct {
		name         string
		body         string
		wantInput    int
		wantOutput   int
		wantEstimate bool
	}{
		{name: "string input", body: `{"usage":{"prompt_tokens":"10","completion_tokens":20}}`, wantInput: 100, wantOutput: 20, wantEstimate: true},
		{name: "negative output", body: `{"usage":{"prompt_tokens":10,"completion_tokens":-1}}`, wantInput: 10, wantOutput: 50, wantEstimate: true},
		{name: "fractional input", body: `{"usage":{"prompt_tokens":1.5,"completion_tokens":20}}`, wantInput: 100, wantOutput: 20, wantEstimate: true},
		{name: "large output", body: `{"usage":{"prompt_tokens":10,"completion_tokens":100000001}}`, wantInput: 10, wantOutput: 100000001, wantEstimate: false},
		{name: "invalid cache field", body: `{"usage":{"prompt_tokens":10,"completion_tokens":20,"cached_tokens":"4"}}`, wantInput: 100, wantOutput: 20, wantEstimate: true},
		{name: "invalid cache container", body: `{"usage":{"prompt_tokens":10,"completion_tokens":20,"cache_creation":"4"}}`, wantInput: 100, wantOutput: 20, wantEstimate: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := applyUsageFallback(parseUsage([]byte(tt.body)), 100, 50, rates)
			if usage.Input != tt.wantInput || usage.Output != tt.wantOutput || usage.Estimated != tt.wantEstimate {
				t.Fatalf("usage=%+v want input=%d output=%d estimated=%v", usage, tt.wantInput, tt.wantOutput, tt.wantEstimate)
			}
		})
	}
}

func TestParseIntValueRejectsNonFiniteAndInvalidNumbers(t *testing.T) {
	for _, value := range []any{"10", -1, 1.5, math.NaN(), math.Inf(1), json.Number("999999999999999999999999999999")} {
		if parsed, ok := parseIntValue(value); ok {
			t.Fatalf("value=%v parsed as %d", value, parsed)
		}
	}
	if parsed, ok := parseIntValue(float64(10)); !ok || parsed != 10 {
		t.Fatalf("valid integer was rejected: parsed=%d ok=%v", parsed, ok)
	}
	if parsed, ok := parseIntValue(json.Number("1000001")); !ok || parsed != 1_000_001 {
		t.Fatalf("large valid integer was rejected: parsed=%d ok=%v", parsed, ok)
	}
}

func TestProxyAcceptsBodyAndOutputAboveFormerLimits(t *testing.T) {
	largeContent := strings.Repeat("x", (10<<20)+1024)
	upstreamCalled := false
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		upstreamCalled = true
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read upstream request: %v", err)
		}
		if !bytes.Contains(body, []byte(`"max_tokens":1000001`)) || !bytes.Contains(body, []byte(largeContent[:1024])) {
			t.Fatal("large request body or output token parameter was not forwarded")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":1,"completion_tokens":1}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: &fakeBilling{}, Client: client})
	body := `{"model":"deepseek-chat","messages":[{"role":"user","content":"` + largeContent + `"}],"max_tokens":1000001}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
	if response.Code != http.StatusOK || !upstreamCalled {
		t.Fatalf("status=%d upstream_called=%v body=%s", response.Code, upstreamCalled, response.Body.String())
	}
}

func TestProxyAcceptsBufferedResponseAboveFormerLimit(t *testing.T) {
	upstreamBody := `{"padding":"` + strings.Repeat("x", (32<<20)+1024) + `","usage":{"prompt_tokens":1,"completion_tokens":1}}`
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(upstreamBody))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: &fakeBilling{}, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.Len() != len(upstreamBody) {
		t.Fatalf("status=%d response_bytes=%d want=%d", response.Code, response.Body.Len(), len(upstreamBody))
	}
}

func TestProxyRoundRobinsStartingChannelAcrossRequests(t *testing.T) {
	first := openAIChannel("first", "first.example.com", "first-upstream")
	second := openAIChannel("second", "second.example.com", "second-upstream")
	hosts := make([]string, 0, 4)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		hosts = append(hosts, request.URL.Host)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":1,"completion_tokens":1}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{candidates: []modelroute.Resolved{first, second}}, Billing: &fakeBilling{}, Client: client})
	for range 4 {
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	}
	want := []string{"first.example.com", "second.example.com", "first.example.com", "second.example.com"}
	if strings.Join(hosts, ",") != strings.Join(want, ",") {
		t.Fatalf("round-robin hosts=%v want=%v", hosts, want)
	}
}

func TestProxyRoundRobinIsSafeUnderConcurrentRequests(t *testing.T) {
	first := openAIChannel("first", "first.example.com", "first-upstream")
	second := openAIChannel("second", "second.example.com", "second-upstream")
	for _, route := range []*modelroute.Resolved{&first, &second} {
		route.UpstreamModel.Prices = upstreammodel.Prices{}
	}
	var firstCalls atomic.Int64
	var secondCalls atomic.Int64
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case "first.example.com":
			firstCalls.Add(1)
		case "second.example.com":
			secondCalls.Add(1)
		default:
			t.Errorf("unexpected host %s", request.URL.Host)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":0,"completion_tokens":0}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{candidates: []modelroute.Resolved{first, second}}, Billing: noopBilling{}, Client: client})
	var group sync.WaitGroup
	for range 100 {
		group.Add(1)
		go func() {
			defer group.Done()
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Errorf("status=%d body=%s", response.Code, response.Body.String())
			}
		}()
	}
	group.Wait()
	if firstCalls.Load() != 50 || secondCalls.Load() != 50 {
		t.Fatalf("concurrent distribution first=%d second=%d", firstCalls.Load(), secondCalls.Load())
	}
}

func TestProxyFailsOverAndBillsOnlySuccessfulChannel(t *testing.T) {
	first := openAIChannel("first", "first.example.com", "first-upstream")
	second := openAIChannel("second", "second.example.com", "second-upstream")
	second.UpstreamModel.Prices.OutputMicros = 20_000_000
	biller := &fakeBilling{}
	hosts := make([]string, 0, 3)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		hosts = append(hosts, request.URL.Host)
		body, _ := io.ReadAll(request.Body)
		if request.URL.Host == "first.example.com" {
			if !strings.Contains(string(body), `"model":"first-upstream"`) {
				t.Fatalf("first model mapping missing: %s", body)
			}
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"unavailable"}`))}, nil
		}
		if !strings.Contains(string(body), `"model":"second-upstream"`) {
			t.Fatalf("second model mapping missing: %s", body)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"second-request","usage":{"prompt_tokens":10,"completion_tokens":20}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{candidates: []modelroute.Resolved{first, second}}, Billing: biller, Client: client})
	handler.upstreamRetryDelays = []time.Duration{0}
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":100}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Join(hosts, ",") != "first.example.com,first.example.com,second.example.com" {
		t.Fatalf("status=%d hosts=%v body=%s", response.Code, hosts, response.Body.String())
	}
	if biller.reserveCalls != 1 || biller.finalizeCalls != 1 || biller.refundCalls != 0 || biller.usage.ModelRouteID != second.ID || biller.usage.UpstreamRequestID != "second-request" {
		t.Fatalf("unexpected billing state: %+v", biller)
	}
	if biller.reserved != biller.usage.ReservedMicros || biller.reserved <= biller.usage.CostMicros {
		t.Fatalf("reservation did not cover the more expensive fallback: reserved=%d usage=%+v", biller.reserved, biller.usage)
	}
}

func TestProxyFailsOverAfterBufferedResponseReadError(t *testing.T) {
	first := openAIChannel("first", "first.example.com", "first-upstream")
	second := openAIChannel("second", "second.example.com", "second-upstream")
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(&terminalErrorReader{reader: strings.NewReader(`{"partial":`), err: errors.New("read failed")})}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":1,"completion_tokens":1}}`))}, nil
	})}
	biller := &fakeBilling{}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{candidates: []modelroute.Resolved{first, second}}, Billing: biller, Client: client})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`)))
	if response.Code != http.StatusOK || calls != 2 || biller.usage.ModelRouteID != second.ID || biller.refundCalls != 0 {
		t.Fatalf("status=%d calls=%d billing=%+v body=%s", response.Code, calls, biller, response.Body.String())
	}
}

func TestProxyReturnsFailureAndRefundsOnceAfterAllChannelsFail(t *testing.T) {
	first := openAIChannel("first", "first.example.com", "first-upstream")
	second := openAIChannel("second", "second.example.com", "second-upstream")
	biller := &fakeBilling{}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Host == "first.example.com" {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection failed")}
		}
		return &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"limited"}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{candidates: []modelroute.Resolved{first, second}}, Billing: biller, Client: client})
	handler.upstreamRetryDelays = []time.Duration{0}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`)))
	if response.Code != http.StatusBadGateway || calls != 4 || biller.reserveCalls != 1 || biller.refundCalls != 1 || biller.refunded != biller.reserved || biller.finalizeCalls != 0 || len(biller.failures) != 1 {
		t.Fatalf("status=%d calls=%d billing=%+v body=%s", response.Code, calls, biller, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "upstream_unavailable") {
		t.Fatalf("unexpected error body: %s", response.Body.String())
	}
	if failure := biller.failures[0]; failure.StatusCode != http.StatusBadGateway || failure.ErrorCode != "upstream_http_error" || failure.ErrorMessage != "上游返回 HTTP 429" || failure.MultiplierBPS != 10_000 || failure.ModelName != "deepseek-chat" {
		t.Fatalf("unexpected failure log: %+v", failure)
	}
}

func TestProxyRetriesConnectionSetupBeforeRequestWrite(t *testing.T) {
	biller := &fakeBilling{}
	calls := 0
	idempotencyKey := ""
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if value := request.Header.Get("Idempotency-Key"); value == "" {
			t.Fatal("upstream idempotency key is missing")
		} else if idempotencyKey == "" {
			idempotencyKey = value
		} else if value != idempotencyKey {
			t.Fatalf("idempotency key changed across retry: first=%q current=%q", idempotencyKey, value)
		}
		if calls < 2 {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("temporary connection failure")}
		}
		body, _ := io.ReadAll(request.Body)
		if !strings.Contains(string(body), `"model":"deepseek-v3"`) {
			t.Fatalf("model was not mapped after retry: %s", body)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":1,"completion_tokens":1}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	handler.upstreamRetryDelays = []time.Duration{0}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`)))
	if response.Code != http.StatusOK || calls != 2 || biller.finalizeCalls != 1 || biller.refundCalls != 0 {
		t.Fatalf("status=%d calls=%d billing=%+v body=%s", response.Code, calls, biller, response.Body.String())
	}
}

func TestProxyBuffersChatStreamWhenUpstreamStreamCannotStart(t *testing.T) {
	biller := &fakeBilling{}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(request.Body)
		if bytes.Contains(body, []byte(`"stream":true`)) {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("stream connection failed")}
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"buffered-1","object":"chat.completion","created":1,"model":"deepseek-v3","choices":[{"index":0,"message":{"role":"assistant","content":"buffered stream ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10,"stream":true}`)))
	if response.Code != http.StatusOK || calls != 2 || !strings.Contains(response.Body.String(), "buffered stream ok") || !strings.Contains(response.Body.String(), "data: [DONE]") || biller.finalizeCalls != 1 {
		t.Fatalf("status=%d calls=%d billing=%+v body=%s", response.Code, calls, biller, response.Body.String())
	}
}

func TestProxyStreamRetriesOnceThenFailsOverToNextChannel(t *testing.T) {
	first := openAIChannel("first", "first.example.com", "first-upstream")
	second := openAIChannel("second", "second.example.com", "second-upstream")
	hosts := make([]string, 0, 3)
	streamModes := make([]bool, 0, 3)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		hosts = append(hosts, request.URL.Host)
		body, _ := io.ReadAll(request.Body)
		streamModes = append(streamModes, bytes.Contains(body, []byte(`"stream":true`)))
		if request.URL.Host == "first.example.com" {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection failed")}
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"id\":\"second-request\",\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))}, nil
	})}
	biller := &fakeBilling{}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{candidates: []modelroute.Resolved{first, second}}, Billing: biller, Client: client})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10,"stream":true}`)))
	if response.Code != http.StatusOK || strings.Join(hosts, ",") != "first.example.com,first.example.com,second.example.com" {
		t.Fatalf("status=%d hosts=%v body=%s", response.Code, hosts, response.Body.String())
	}
	if len(streamModes) != 3 || !streamModes[0] || streamModes[1] || !streamModes[2] {
		t.Fatalf("unexpected upstream stream modes: %v", streamModes)
	}
	if biller.finalizeCalls != 1 || biller.usage.ModelRouteID != second.ID || biller.refundCalls != 0 {
		t.Fatalf("unexpected billing state: %+v", biller)
	}
}

func TestProxyStreamRetriesTemporaryHTTPFailureAsBufferedRequest(t *testing.T) {
	calls := 0
	streamModes := make([]bool, 0, 2)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(request.Body)
		streamModes = append(streamModes, bytes.Contains(body, []byte(`"stream":true`)))
		if calls == 1 {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"unavailable"}`))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"buffered-1","choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))}, nil
	})}
	biller := &fakeBilling{}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10,"stream":true}`)))
	if response.Code != http.StatusOK || calls != 2 || len(streamModes) != 2 || !streamModes[0] || streamModes[1] {
		t.Fatalf("status=%d calls=%d stream_modes=%v body=%s", response.Code, calls, streamModes, response.Body.String())
	}
	if biller.finalizeCalls != 1 || biller.refundCalls != 0 {
		t.Fatalf("unexpected billing state: %+v", biller)
	}
}

func TestProxyUsesBufferedUpstreamForReasonixStreams(t *testing.T) {
	route := openAIRoute()
	route.Provider.Code = "reasonix"
	biller := &fakeBilling{}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(request.Body)
		if bytes.Contains(body, []byte(`"stream":true`)) {
			return nil, errors.New("reasonix stream should be buffered")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"reasonix-buffered","object":"chat.completion","created":1,"model":"kimi-k3","choices":[{"index":0,"message":{"role":"assistant","content":"reasonix buffered ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`)),
		}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: route}, Billing: biller, Client: client})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"kimi-k3","messages":[],"max_tokens":10,"stream":true}`)))
	if response.Code != http.StatusOK || calls != 1 || !strings.Contains(response.Body.String(), "reasonix buffered ok") || !strings.Contains(response.Body.String(), "data: [DONE]") || biller.finalizeCalls != 1 {
		t.Fatalf("status=%d calls=%d billing=%+v body=%s", response.Code, calls, biller, response.Body.String())
	}
}

func TestProxyRejectsSelfReferentialUpstream(t *testing.T) {
	route := openAIRoute()
	route.BaseURL = "https://gateway.example/v1"
	biller := &fakeBilling{}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("self-reference should not be sent")
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: route}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "https://gateway.example/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`))
	request.Host = "gateway.example"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || calls != 0 || len(biller.failures) != 1 {
		t.Fatalf("status=%d calls=%d failures=%d body=%s", response.Code, calls, len(biller.failures), response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "upstream_self_reference") || biller.failures[0].ErrorCode != "upstream_self_reference" {
		t.Fatalf("self-reference error was not visible: body=%s failure=%+v", response.Body.String(), biller.failures[0])
	}
}

func TestFinalizationRetriesTransientErrors(t *testing.T) {
	biller := &fakeBilling{finalizeErrors: []error{errors.New("temporary database failure"), nil}}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"up-1","usage":{"prompt_tokens":10,"completion_tokens":20}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	handler.settlementRetryDelays = []time.Duration{0, 0}
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":100}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || biller.finalizeCalls != 2 || biller.usage.InputTokens != 10 || biller.usage.OutputTokens != 20 {
		t.Fatalf("status=%d finalize_calls=%d usage=%+v", response.Code, biller.finalizeCalls, biller.usage)
	}
}

func TestFinalizationDoesNotRetryBusinessConflict(t *testing.T) {
	biller := &fakeBilling{finalizeErrors: []error{billing.ErrRequestConflict}}
	body := `{"id":"up-1","usage":{"prompt_tokens":10,"completion_tokens":20}}`
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	var logs bytes.Buffer
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	handler.settlementRetryDelays = []time.Duration{0, 0}
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":100}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != body || biller.finalizeCalls != 1 || biller.refundCalls != 0 {
		t.Fatalf("status=%d finalize_calls=%d refund_calls=%d body=%s", response.Code, biller.finalizeCalls, biller.refundCalls, response.Body.String())
	}
	if !strings.Contains(logs.String(), "forward successful response after usage finalization failure") {
		t.Fatalf("successful response after finalization failure was not logged: %s", logs.String())
	}
}

func TestSettlementRetryStopsWhenContextIsCanceled(t *testing.T) {
	handler := New(Dependencies{})
	handler.settlementRetryDelays = []time.Duration{time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err, attempts := handler.retrySettlement(ctx, func() error {
		calls++
		cancel()
		return errors.New("temporary database failure")
	})
	if !errors.Is(err, context.Canceled) || attempts != 1 || calls != 1 {
		t.Fatalf("err=%v attempts=%d calls=%d", err, attempts, calls)
	}
}

func TestProxyRoutesResponsesAndAnthropicMessages(t *testing.T) {
	tests := []struct {
		name              string
		path              string
		body              string
		route             modelroute.Resolved
		wantURL           string
		wantAuthorization string
		wantAPIKey        string
		wantEndpoint      string
		upstreamBody      string
		wantInput         int
		wantOutput        int
	}{
		{
			name:              "responses",
			path:              "/v1/responses",
			body:              `{"model":"deepseek-chat","input":"hello","max_output_tokens":32}`,
			route:             openAIRoute(),
			wantURL:           "https://api.example.com/v1/responses",
			wantAuthorization: "Bearer upstream-secret",
			wantEndpoint:      "responses",
			upstreamBody:      `{"id":"resp-1","usage":{"input_tokens":11,"output_tokens":13}}`,
			wantInput:         11,
			wantOutput:        13,
		},
		{
			name:         "anthropic messages",
			path:         "/v1/messages",
			body:         `{"model":"kimi-k3","messages":[{"role":"user","content":"hello"}],"max_tokens":32}`,
			route:        anthropicRoute(),
			wantURL:      "https://api.anthropic.com/v1/messages",
			wantAPIKey:   "anthropic-secret",
			wantEndpoint: "messages",
			upstreamBody: `{"id":"msg-1","usage":{"input_tokens":5,"output_tokens":8}}`,
			wantInput:    5,
			wantOutput:   8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			biller := &fakeBilling{}
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != tt.wantURL {
					t.Fatalf("unexpected upstream URL %s", request.URL)
				}
				if request.Header.Get("Authorization") != tt.wantAuthorization {
					t.Fatalf("unexpected authorization header %q", request.Header.Get("Authorization"))
				}
				if request.Header.Get("X-API-Key") != tt.wantAPIKey {
					t.Fatalf("unexpected x-api-key header %q", request.Header.Get("X-API-Key"))
				}
				if tt.wantAPIKey != "" && request.Header.Get("Anthropic-Version") != "2023-06-01" {
					t.Fatalf("unexpected Anthropic-Version %q", request.Header.Get("Anthropic-Version"))
				}
				body, _ := io.ReadAll(request.Body)
				if !strings.Contains(string(body), `"model":"`+tt.route.UpstreamName+`"`) {
					t.Fatalf("model was not mapped: %s", body)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(tt.upstreamBody))}, nil
			})}
			handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: tt.route}, Billing: biller, Client: client})
			request := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			request.Header.Set("Authorization", "Bearer nvr_test")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if biller.usage.InputTokens != tt.wantInput || biller.usage.OutputTokens != tt.wantOutput || biller.usage.Endpoint != tt.wantEndpoint {
				t.Fatalf("unexpected usage %+v", biller.usage)
			}
		})
	}
}

func TestProxyRejectsInsufficientBalanceBeforeUpstream(t *testing.T) {
	biller := &fakeBilling{reserveErr: billing.ErrInsufficientBalance}
	called := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { called = true; return nil, errors.New("must not call") })}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":100}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusPaymentRequired || called {
		t.Fatalf("status=%d upstream_called=%v", response.Code, called)
	}
}

func TestStreamingUsageIsCaptured(t *testing.T) {
	biller := &fakeBilling{}
	stream := "data: {\"id\":\"stream-1\",\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":0}}\n\ndata: {\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":9}}\n\ndata: [DONE]\n\n"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":20,"stream":true}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "[DONE]") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if biller.usage.InputTokens != 7 || biller.usage.OutputTokens != 9 || biller.usage.Estimated {
		t.Fatalf("unexpected streamed usage %+v", biller.usage)
	}
}

func TestStreamingFinalizationFailureKeepsSuccessfulResponse(t *testing.T) {
	biller := &fakeBilling{finalizeErrors: []error{billing.ErrRequestConflict}}
	stream := "data: {\"id\":\"stream-1\",\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":9}}\n\ndata: [DONE]\n\n"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
	})}
	var logs bytes.Buffer
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":20,"stream":true}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != stream || biller.finalizeCalls != 1 || biller.refundCalls != 0 {
		t.Fatalf("status=%d finalize_calls=%d refund_calls=%d body=%s", response.Code, biller.finalizeCalls, biller.refundCalls, response.Body.String())
	}
	if !strings.Contains(logs.String(), "stream usage finalization unavailable") {
		t.Fatalf("stream finalization failure was not logged: %s", logs.String())
	}
}

func TestStreamingLargeDataLineIsRelayedAndBilled(t *testing.T) {
	biller := &fakeBilling{}
	padding := strings.Repeat("x", (4<<20)+1024)
	stream := "data: {\"id\":\"stream-large\",\"padding\":\"" + padding + "\",\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":9}}\n\ndata: [DONE]\n\n"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":20,"stream":true}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != stream {
		t.Fatalf("large SSE line was not relayed completely: status=%d bytes=%d want=%d", response.Code, response.Body.Len(), len(stream))
	}
	if biller.usage.InputTokens != 7 || biller.usage.OutputTokens != 9 || biller.usage.Estimated {
		t.Fatalf("unexpected large streamed usage %+v", biller.usage)
	}
}

func TestStreamingUsageAcrossResponsesAndMessages(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		body   string
		route  modelroute.Resolved
		stream string
	}{
		{
			name:  "responses",
			path:  "/v1/responses",
			body:  `{"model":"deepseek-chat","input":"hello","max_output_tokens":20,"stream":true}`,
			route: openAIRoute(),
			stream: "event: response.completed\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-stream-1\",\"usage\":{\"input_tokens\":7,\"output_tokens\":9}}}\n\n",
		},
		{
			name:  "anthropic messages",
			path:  "/v1/messages",
			body:  `{"model":"kimi-k3","messages":[{"role":"user","content":"hello"}],"max_tokens":20,"stream":true}`,
			route: anthropicRoute(),
			stream: "event: message_start\n" +
				"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-stream-1\",\"usage\":{\"input_tokens\":7,\"output_tokens\":1}}}\n\n" +
				"event: message_delta\n" +
				"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":9}}\n\n" +
				"event: message_stop\n" +
				"data: {\"type\":\"message_stop\"}\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			biller := &fakeBilling{}
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(tt.stream))}, nil
			})}
			handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: tt.route}, Billing: biller, Client: client})
			request := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "data:") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if biller.usage.InputTokens != 7 || biller.usage.OutputTokens != 9 || biller.usage.Estimated {
				t.Fatalf("unexpected streamed usage %+v", biller.usage)
			}
		})
	}
}

func TestInterruptedStreamingUsageIsMarkedEstimated(t *testing.T) {
	biller := &fakeBilling{}
	stream := "data: {\"id\":\"stream-partial\",\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2}}\n\n"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":20,"stream":true}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if biller.usage.InputTokens != 7 || biller.usage.OutputTokens != 2 || !biller.usage.Estimated {
		t.Fatalf("interrupted stream usage was not marked estimated: %+v", biller.usage)
	}
}

func TestStreamingReadErrorIsLoggedWithoutChangingCompletedUsage(t *testing.T) {
	biller := &fakeBilling{}
	stream := "data: {\"id\":\"stream-complete\",\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":9}}\n\ndata: [DONE]\n\n"
	readFailure := errors.New("upstream read failed")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(&terminalErrorReader{reader: strings.NewReader(stream), err: readFailure})}, nil
	})}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client, Logger: logger})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":20,"stream":true}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != stream {
		t.Fatalf("stream changed after terminal read error: status=%d bytes=%d want=%d", response.Code, response.Body.Len(), len(stream))
	}
	if biller.usage.InputTokens != 7 || biller.usage.OutputTokens != 9 || biller.usage.Estimated {
		t.Fatalf("unexpected completed usage after terminal read error %+v", biller.usage)
	}
	if !strings.Contains(logs.String(), "relay gateway stream") || !strings.Contains(logs.String(), readFailure.Error()) {
		t.Fatalf("terminal stream error was not logged: %s", logs.String())
	}
}

func TestModelListRequiresAPIKey(t *testing.T) {
	handler := New(Dependencies{APIKeys: fakeKeys{err: apikey.ErrUnauthenticated}, Routes: fakeRoutes{}, Billing: &fakeBilling{}, Client: &http.Client{}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", response.Code)
	}
	assertErrorRequestID(t, response)
}

func TestModelListDeduplicatesFailoverRoutes(t *testing.T) {
	first := openAIChannel("first", "first.example.com", "first-upstream")
	second := openAIChannel("second", "second.example.com", "second-upstream")
	first.UpstreamModel = &upstreammodel.Record{ProviderName: "DeepSeek", UpstreamName: "deepseek-chat", DisplayName: "DeepSeek Chat"}
	second.UpstreamModel = &upstreammodel.Record{ProviderName: "DeepSeek", UpstreamName: "deepseek-chat", DisplayName: "DeepSeek Chat"}
	actor := gatewayActor()
	handler := New(Dependencies{APIKeys: fakeKeys{actor: actor}, Routes: fakeRoutes{records: []modelroute.Record{first.Record, second.Record}, expectedGroupID: actor.APIKey.BillingGroupID}, Billing: &fakeBilling{}, Client: &http.Client{}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	var body struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode model list: %v", err)
	}
	if response.Code != http.StatusOK || len(body.Data) != 1 || body.Data[0].ID != "deepseek-chat" || body.Data[0].OwnedBy != "DeepSeek" {
		t.Fatalf("status=%d models=%+v body=%s", response.Code, body.Data, response.Body.String())
	}
}

func TestSingularModelListAliasUsesSelectedRoutes(t *testing.T) {
	route := openAIChannel("selected", "selected.example.com", "selected-upstream")
	route.UpstreamModel = &upstreammodel.Record{ProviderName: "Kimi", UpstreamName: "kimi-k3", DisplayName: "Kimi K3"}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{records: []modelroute.Record{route.Record}}, Billing: &fakeBilling{}, Client: &http.Client{}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/model", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"deepseek-chat"`) || !strings.Contains(response.Body.String(), `"owned_by":"Kimi"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestGatewayErrorsIncludeRequestID(t *testing.T) {
	tests := []struct {
		name    string
		routes  fakeRoutes
		billing *fakeBilling
		client  *http.Client
		status  int
	}{
		{
			name: "model not found", routes: fakeRoutes{err: modelroute.ErrNotFound},
			billing: &fakeBilling{}, client: &http.Client{}, status: http.StatusNotFound,
		},
		{
			name: "insufficient balance", routes: fakeRoutes{route: openAIRoute()},
			billing: &fakeBilling{reserveErr: billing.ErrInsufficientBalance}, client: &http.Client{}, status: http.StatusPaymentRequired,
		},
		{
			name: "upstream failure", routes: fakeRoutes{route: openAIRoute()}, billing: &fakeBilling{},
			client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("timeout") })},
			status: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: tt.routes, Billing: tt.billing, Client: tt.client})
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":20}`))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			assertErrorRequestID(t, response)
		})
	}
}

func assertErrorRequestID(t *testing.T, response *httptest.ResponseRecorder) uuid.UUID {
	t.Helper()
	requestID := response.Header().Get("X-Novro-Request-ID")
	parsed, err := uuid.Parse(requestID)
	if err != nil {
		t.Fatalf("invalid request id header %q: %v", requestID, err)
	}
	var body struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.RequestID != requestID {
		t.Fatalf("request id mismatch header=%q body=%q err=%v", requestID, body.RequestID, err)
	}
	return parsed
}

func TestBuildUpstreamURLAndPrivateAddressGuard(t *testing.T) {
	url, err := buildUpstreamURL("https://open.bigmodel.cn/api/paas/v4", provider.ProtocolOpenAI, "chat_completions")
	if err != nil || url != "https://open.bigmodel.cn/api/paas/v4/chat/completions" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	url, err = buildUpstreamURL("https://api.anthropic.com/v1", provider.ProtocolAnthropic, "messages")
	if err != nil || url != "https://api.anthropic.com/v1/messages" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	url, err = buildUpstreamURL("http://203.0.113.10:3000/v1", provider.ProtocolOpenAI, "chat_completions")
	if err != nil || url != "http://203.0.113.10:3000/v1/chat/completions" {
		t.Fatalf("HTTP self-hosted URL=%q err=%v", url, err)
	}
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.1", "::1"} {
		if !unsafeUpstreamIP(net.ParseIP(address)) {
			t.Fatalf("expected %s to be blocked", address)
		}
	}
}

func TestDefaultOutboundClientDoesNotFollowRedirects(t *testing.T) {
	client := newOutboundClient()
	if client.CheckRedirect == nil {
		t.Fatal("default outbound client must reject redirects")
	}
	request := httptest.NewRequest(http.MethodGet, "https://provider.example/redirect", nil)
	if err := client.CheckRedirect(request, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("redirect error=%v, want http.ErrUseLastResponse", err)
	}
}

func TestUpstreamFailureRefundsReservation(t *testing.T) {
	biller := &fakeBilling{refundErrors: []error{errors.New("temporary database failure"), nil}}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("timeout") })}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	handler.settlementRetryDelays = []time.Duration{0, 0}
	handler.now = func() time.Time { return time.Unix(1, 0) }
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":20}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || biller.refundCalls != 2 || biller.refunded != biller.reserved {
		t.Fatalf("status=%d refund_calls=%d reserved=%d refunded=%d", response.Code, biller.refundCalls, biller.reserved, biller.refunded)
	}
}

func TestParseUsageSupportsProviderCacheShapes(t *testing.T) {
	tests := []struct {
		name, body string
		want       tokenUsage
	}{
		{name: "glm cached details", body: `{"usage":{"prompt_tokens":2000,"completion_tokens":500,"prompt_tokens_details":{"cached_tokens":1200}}}`, want: tokenUsage{Input: 2000, UncachedInput: 800, CacheRead: 1200, Output: 500}},
		{name: "deepseek hit and miss", body: `{"usage":{"prompt_tokens":2000,"completion_tokens":500,"prompt_cache_hit_tokens":1200,"prompt_cache_miss_tokens":800}}`, want: tokenUsage{Input: 2000, UncachedInput: 800, CacheRead: 1200, Output: 500}},
		{name: "kimi top level cached", body: `{"usage":{"prompt_tokens":100,"completion_tokens":20,"cached_tokens":10}}`, want: tokenUsage{Input: 100, UncachedInput: 90, CacheRead: 10, Output: 20}},
		{name: "openai cache read field is prompt subset", body: `{"usage":{"prompt_tokens":100,"completion_tokens":20,"cache_read_input_tokens":10}}`, want: tokenUsage{Input: 100, UncachedInput: 90, CacheRead: 10, Output: 20}},
		{name: "anthropic cache read is additional input", body: `{"usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":10}}`, want: tokenUsage{Input: 110, UncachedInput: 100, CacheRead: 10, Output: 20}},
		{name: "anthropic cache creation", body: `{"usage":{"input_tokens":800,"output_tokens":500,"cache_read_input_tokens":1200,"cache_creation_input_tokens":300,"cache_creation":{"ephemeral_5m_input_tokens":200,"ephemeral_1h_input_tokens":100}}}`, want: tokenUsage{Input: 2300, UncachedInput: 800, CacheRead: 1200, CacheWrite: 200, CacheWrite1h: 100, Output: 500}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseUsage([]byte(tt.body))
			if got.Input != tt.want.Input || got.UncachedInput != tt.want.UncachedInput || got.CacheRead != tt.want.CacheRead || got.CacheWrite != tt.want.CacheWrite || got.CacheWrite1h != tt.want.CacheWrite1h || got.Output != tt.want.Output {
				t.Fatalf("got=%+v want=%+v", got, tt.want)
			}
		})
	}
}

func TestUsageFallbackOnlyEstimatesMissingDimensionsAtHighestRate(t *testing.T) {
	rates := billing.RateCard{InputMicros: 1, CacheReadMicros: 2, CacheWriteMicros: 3, CacheWrite1hMicros: 4, OutputMicros: 5}

	usage := applyUsageFallback(parseUsage([]byte(`{"usage":{"prompt_tokens":10,"completion_tokens":0}}`)), 100, 20, rates)
	if usage.Input != 10 || usage.UncachedInput != 10 || usage.Output != 0 || usage.Estimated {
		t.Fatalf("reported zero output must remain exact: %+v", usage)
	}

	usage = applyUsageFallback(parseUsage([]byte(`{"usage":{"completion_tokens":7}}`)), 100, 20, rates)
	if usage.Input != 100 || usage.CacheWrite1h != 100 || usage.Output != 7 || !usage.Estimated {
		t.Fatalf("missing input was not conservatively estimated: %+v", usage)
	}

	usage = applyUsageFallback(parseUsage([]byte(`{"usage":{"prompt_tokens":10}}`)), 100, 20, rates)
	if usage.Input != 10 || usage.UncachedInput != 10 || usage.Output != 20 || !usage.Estimated {
		t.Fatalf("missing output was not conservatively estimated: %+v", usage)
	}
}
