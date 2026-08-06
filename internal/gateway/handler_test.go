package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/apikey"
	"github.com/novro-gateway/novro/internal/billing"
	"github.com/novro-gateway/novro/internal/modelroute"
	"github.com/novro-gateway/novro/internal/provider"
	"github.com/novro-gateway/novro/internal/user"
)

type fakeKeys struct {
	actor apikey.Actor
	err   error
}

func (f fakeKeys) Authenticate(context.Context, string) (apikey.Actor, error) { return f.actor, f.err }

type fakeRoutes struct {
	route   modelroute.Resolved
	records []modelroute.Record
	err     error
}

func (f fakeRoutes) Resolve(context.Context, string) (modelroute.Resolved, error) {
	return f.route, f.err
}
func (f fakeRoutes) ListActive(context.Context) ([]modelroute.Record, error) { return f.records, f.err }

type fakeBilling struct {
	reserveErr     error
	refundErrors   []error
	finalizeErrors []error
	reserved       int64
	refunded       int64
	refundCalls    int
	finalizeCalls  int
	usage          billing.UsageInput
}

func (f *fakeBilling) Reserve(_ context.Context, _, _ uuid.UUID, amount int64, _ string) error {
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

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
	return apikey.Actor{APIKey: apikey.Record{ID: uuid.New()}, User: user.Record{ID: uuid.New(), Status: user.StatusActive}}
}

func openAIRoute() modelroute.Resolved {
	return modelroute.Resolved{Record: modelroute.Record{ID: uuid.New(), PublicName: "deepseek-chat", UpstreamName: "deepseek-v3", InputPriceMicros: 2_000_000, OutputPriceMicros: 8_000_000, Provider: modelroute.ProviderSummary{Code: "deepseek", Protocol: provider.ProtocolOpenAI}}, BaseURL: "https://api.example.com/v1", APIKey: "upstream-secret"}
}

func anthropicRoute() modelroute.Resolved {
	return modelroute.Resolved{Record: modelroute.Record{ID: uuid.New(), PublicName: "kimi-k3", UpstreamName: "kimi-k3-upstream", InputPriceMicros: 2_000_000, OutputPriceMicros: 8_000_000, Provider: modelroute.ProviderSummary{Code: "kimi", Protocol: provider.ProtocolAnthropic}}, BaseURL: "https://api.anthropic.com/v1", APIKey: "anthropic-secret"}
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
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"up-1","usage":{"prompt_tokens":10,"completion_tokens":20}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	handler.settlementRetryDelays = []time.Duration{0, 0}
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":100}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || biller.finalizeCalls != 1 {
		t.Fatalf("status=%d finalize_calls=%d body=%s", response.Code, biller.finalizeCalls, response.Body.String())
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
