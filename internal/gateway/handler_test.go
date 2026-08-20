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
	"net/http/httptrace"
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
	"github.com/novro-gateway/novro/internal/modelpricing"
	"github.com/novro-gateway/novro/internal/modelroute"
	"github.com/novro-gateway/novro/internal/provider"
	"github.com/novro-gateway/novro/internal/requestid"
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

type fakePricing struct {
	resolution modelpricing.Resolution
	modelID    uuid.UUID
	at         time.Time
	err        error
}

type fakeDiscounts struct {
	multiplierBPS int64
	multipliers   map[uuid.UUID]int64
	calls         int
	groupID       uuid.UUID
	groupIDs      []uuid.UUID
}

func (f *fakeDiscounts) MultiplierAt(group billinggroup.Summary, _ time.Time) int64 {
	f.calls++
	f.groupID = group.ID
	f.groupIDs = append(f.groupIDs, group.ID)
	if multiplier, ok := f.multipliers[group.ID]; ok {
		return multiplier
	}
	return f.multiplierBPS
}

func (f *fakePricing) Resolve(_ context.Context, modelID uuid.UUID, at time.Time) (modelpricing.Resolution, error) {
	f.modelID = modelID
	f.at = at
	return f.resolution, f.err
}

/**
 * Config 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f fakeGatewaySettings) Config(context.Context) (gatewaysettings.Config, error) {
	if f.err != nil {
		return gatewaysettings.Config{}, f.err
	}
	return f.config, nil
}

/**
 * Authenticate 用于校验用户凭据并建立登录会话。
 * @param string 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f fakeKeys) Authenticate(context.Context, string) (apikey.Actor, error) { return f.actor, f.err }

type fakeRoutes struct {
	route           modelroute.Resolved
	candidates      []modelroute.Resolved
	records         []modelroute.Record
	expectedGroupID uuid.UUID
	err             error
}

/**
 * ResolveCandidates 封装该名称对应的业务处理逻辑。
 * @param billingGroupID 目标资源的一个或多个唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * ListActive 用于筛选并返回数据列表。
 * @param billingGroupID 目标资源的一个或多个唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f fakeRoutes) ListActive(_ context.Context, billingGroupID uuid.UUID) ([]modelroute.Record, error) {
	if f.expectedGroupID != uuid.Nil && billingGroupID != f.expectedGroupID {
		return nil, fmt.Errorf("unexpected billing group %s", billingGroupID)
	}
	return f.records, f.err
}

type fakeBilling struct {
	reserveErr       error
	refundErrors     []error
	finalizeErrors   []error
	reserved         int64
	reserveCalls     int
	refunded         int64
	refundCalls      int
	finalizeCalls    int
	usage            billing.UsageInput
	failures         []billing.FailureInput
	operation        billing.Operation
	operationCreated bool
	pendingCalls     int
	pendingErrors    []error
	completeCalls    int
	failCalls        int
	unknownCalls     int
	replayOperation  *billing.Operation
}

/**
 * StartOperation 封装该名称对应的业务处理逻辑。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeBilling) StartOperation(_ context.Context, input billing.OperationStartInput) (billing.OperationStartResult, error) {
	if f.reserveErr != nil {
		return billing.OperationStartResult{}, f.reserveErr
	}
	if f.replayOperation != nil {
		return billing.OperationStartResult{Operation: *f.replayOperation}, nil
	}
	f.reserveCalls++
	f.reserved = input.ReservedMicros
	f.operation = billing.Operation{RequestID: input.RequestID, UserID: input.UserID, APIKeyID: input.APIKeyID, IdempotencyKeyHash: input.IdempotencyKeyHash, RequestHash: input.RequestHash, Endpoint: input.Endpoint, Status: billing.OperationProcessing, ReservedMicros: input.ReservedMicros}
	f.operationCreated = true
	return billing.OperationStartResult{Operation: f.operation, Created: true}, nil
}

/**
 * MarkOperationPendingSettlement 封装该名称对应的业务处理逻辑。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeBilling) MarkOperationPendingSettlement(_ context.Context, _ uuid.UUID, input billing.UsageInput) error {
	f.pendingCalls++
	if len(f.pendingErrors) > 0 {
		err := f.pendingErrors[0]
		f.pendingErrors = f.pendingErrors[1:]
		if err != nil {
			return err
		}
	}
	f.operation.Status = billing.OperationPendingSettlement
	f.usage = input
	return nil
}

/**
 * MarkOperationPendingUnknown 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeBilling) MarkOperationPendingUnknown(_ context.Context, _ uuid.UUID, _ string) error {
	f.unknownCalls++
	f.operation.Status = billing.OperationPendingUnknown
	return nil
}

/**
 * CompleteOperation 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeBilling) CompleteOperation(context.Context, uuid.UUID) error {
	f.completeCalls++
	f.operation.Status = billing.OperationCompleted
	return nil
}

/**
 * FailOperation 封装该名称对应的业务处理逻辑。
 * @param string 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeBilling) FailOperation(context.Context, uuid.UUID, string) error {
	f.failCalls++
	f.refundCalls++
	f.refunded += f.reserved
	f.operation.Status = billing.OperationFailed
	return nil
}

/**
 * Reserve 封装该名称对应的业务处理逻辑。
 * @param amount 本次操作使用的数值参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeBilling) Reserve(_ context.Context, _, _ uuid.UUID, amount int64, _ string) error {
	f.reserveCalls++
	f.reserved = amount
	return f.reserveErr
}

/**
 * Refund 封装该名称对应的业务处理逻辑。
 * @param amount 本次操作使用的数值参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * ReleaseReservation 封装该名称对应的业务处理逻辑。
 * @param amount 本次操作使用的数值参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeBilling) ReleaseReservation(_ context.Context, _, _ uuid.UUID, amount int64, _ string) error {
	f.refundCalls++
	f.refunded += amount
	return nil
}

/**
 * Finalize 封装该名称对应的业务处理逻辑。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * RecordFailure 封装该名称对应的业务处理逻辑。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f *fakeBilling) RecordFailure(_ context.Context, input billing.FailureInput) error {
	f.failures = append(f.failures, input)
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func jsonHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

/**
 * RoundTrip 封装该名称对应的业务处理逻辑。
 * @param request 当前请求数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type blockingBody struct {
	reader  *strings.Reader
	release chan struct{}
	once    sync.Once
}

/**
 * newBlockingBody 封装该名称对应的业务处理逻辑。
 * @param prefix 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func newBlockingBody(prefix string) *blockingBody {
	return &blockingBody{reader: strings.NewReader(prefix), release: make(chan struct{})}
}

/**
 * Read 用于查询并返回所需的数据。
 * @param target 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (b *blockingBody) Read(target []byte) (int, error) {
	if b.reader.Len() > 0 {
		return b.reader.Read(target)
	}
	<-b.release
	return 0, io.EOF
}

/**
 * Close 用于删除、撤销或释放指定资源。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (b *blockingBody) Close() error {
	b.once.Do(func() { close(b.release) })
	return nil
}

type noopBilling struct{}

/**
 * Finalize 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (noopBilling) Finalize(context.Context, billing.UsageInput) error { return nil }

/**
 * RecordFailure 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (noopBilling) RecordFailure(context.Context, billing.FailureInput) error { return nil }

/**
 * StartOperation 封装该名称对应的业务处理逻辑。
 * @param input 需要处理的输入数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (noopBilling) StartOperation(_ context.Context, input billing.OperationStartInput) (billing.OperationStartResult, error) {
	return billing.OperationStartResult{Created: true, Operation: billing.Operation{RequestID: input.RequestID, UserID: input.UserID, APIKeyID: input.APIKeyID, RequestHash: input.RequestHash, Endpoint: input.Endpoint, Status: billing.OperationProcessing, ReservedMicros: input.ReservedMicros}}, nil
}

/**
 * MarkOperationPendingSettlement 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (noopBilling) MarkOperationPendingSettlement(context.Context, uuid.UUID, billing.UsageInput) error {
	return nil
}

/**
 * MarkOperationPendingUnknown 封装该名称对应的业务处理逻辑。
 * @param string 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (noopBilling) MarkOperationPendingUnknown(context.Context, uuid.UUID, string) error { return nil }

/**
 * CompleteOperation 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (noopBilling) CompleteOperation(context.Context, uuid.UUID) error { return nil }

/**
 * FailOperation 封装该名称对应的业务处理逻辑。
 * @param string 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (noopBilling) FailOperation(context.Context, uuid.UUID, string) error { return nil }

type terminalErrorReader struct {
	reader *strings.Reader
	err    error
}

/**
 * Read 用于查询并返回所需的数据。
 * @param target 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (r *terminalErrorReader) Read(target []byte) (int, error) {
	if r.reader.Len() > 0 {
		read, _ := r.reader.Read(target)
		return read, nil
	}
	return 0, r.err
}

/**
 * gatewayActor 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func gatewayActor() apikey.Actor {
	groupID := uuid.New()
	return apikey.Actor{APIKey: apikey.Record{ID: uuid.New(), BillingGroupID: groupID, BillingGroup: billinggroup.Summary{ID: groupID, Code: "default", DisplayName: "默认", MultiplierBPS: 10_000}}, User: user.Record{ID: uuid.New(), Status: user.StatusActive}}
}

/**
 * gatewayActorWithMultiplier 封装该名称对应的业务处理逻辑。
 * @param multiplierBPS 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func gatewayActorWithMultiplier(multiplierBPS int64) apikey.Actor {
	actor := gatewayActor()
	actor.APIKey.BillingGroup.MultiplierBPS = multiplierBPS
	return actor
}

func compositeGatewayActor(memberGroupIDs ...uuid.UUID) apikey.Actor {
	actor := gatewayActor()
	actor.APIKey.BillingGroup.Kind = billinggroup.KindComposite
	actor.APIKey.BillingGroup.MemberGroupIDs = append([]uuid.UUID(nil), memberGroupIDs...)
	actor.APIKey.BillingGroup.MemberGroupCount = len(memberGroupIDs)
	return actor
}

func routeWithBillingGroup(route modelroute.Resolved, groupID uuid.UUID, code, displayName string, multiplierBPS int64) modelroute.Resolved {
	route.BillingGroupID = groupID
	route.BillingGroup = billinggroup.NewSummaryWithKind(groupID, code, displayName, billinggroup.KindStandard, multiplierBPS, "", billinggroup.DefaultMultiplierBPS, nil, nil, nil)
	return route
}

/**
 * openAIRoute 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func openAIRoute() modelroute.Resolved {
	routeID, upstreamID := uuid.New(), uuid.New()
	return modelroute.Resolved{Record: modelroute.Record{ID: routeID, UpstreamModelID: &upstreamID, PublicName: "deepseek-chat", UpstreamName: "deepseek-v3", InputPriceMicros: 2_000_000, OutputPriceMicros: 8_000_000, Provider: modelroute.ProviderSummary{Code: "deepseek", Weight: 100, Protocols: []provider.Protocol{provider.ProtocolOpenAI}}, UpstreamModel: &upstreammodel.Record{ID: upstreamID, UpstreamName: "deepseek-v3", Prices: upstreammodel.Prices{InputMicros: 2_000_000, OutputMicros: 8_000_000}}}, BaseURL: "https://api.example.com/v1", APIKey: "upstream-secret"}
}

/**
 * anthropicRoute 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func anthropicRoute() modelroute.Resolved {
	routeID, upstreamID := uuid.New(), uuid.New()
	return modelroute.Resolved{Record: modelroute.Record{ID: routeID, UpstreamModelID: &upstreamID, PublicName: "kimi-k3", UpstreamName: "kimi-k3-upstream", InputPriceMicros: 2_000_000, OutputPriceMicros: 8_000_000, Provider: modelroute.ProviderSummary{Code: "kimi", Protocols: []provider.Protocol{provider.ProtocolAnthropic}}, UpstreamModel: &upstreammodel.Record{ID: upstreamID, UpstreamName: "kimi-k3-upstream", Prices: upstreammodel.Prices{InputMicros: 2_000_000, OutputMicros: 8_000_000}}}, BaseURL: "https://api.anthropic.com/v1", APIKey: "anthropic-secret"}
}

func dualProtocolRoute() modelroute.Resolved {
	route := openAIRoute()
	route.Provider.Protocols = []provider.Protocol{provider.ProtocolOpenAI, provider.ProtocolAnthropic}
	return route
}

/**
 * openAIChannel 封装该名称对应的业务处理逻辑。
 * @param code 用于标识或筛选目标的文本值。
 * @param host 本次操作需要使用的输入参数。
 * @param upstreamName 用于标识或筛选目标的文本值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestProxyRoutesModelAndFinalizesExactUsage 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

func TestProxyPinsResolvedScheduledPriceForReservationAndSettlement(t *testing.T) {
	biller := &fakeBilling{}
	pricing := &fakePricing{resolution: modelpricing.Resolution{
		Rates: billing.RateCard{InputMicros: 3_000_000, OutputMicros: 9_000_000},
	}}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":10,"completion_tokens":20}}`))}, nil
	})}
	route := openAIRoute()
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: route}, Billing: biller, Pricing: pricing, Client: client})
	startedAt := time.Date(2026, time.August, 17, 1, 0, 0, 0, time.UTC)
	handler.now = func() time.Time { return startedAt }
	body := `{"model":"deepseek-chat","messages":[],"max_tokens":100}`
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	upstreamBody, err := buildUpstreamBody(payload, route, "chat_completions", false)
	if err != nil {
		t.Fatal(err)
	}
	wantReservation, err := billing.EstimateReservation(
		estimateInputTokens(upstreamBody),
		100,
		pricing.resolution.Rates,
		10_000,
	)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if pricing.modelID != *route.UpstreamModelID || !pricing.at.Equal(startedAt) {
		t.Fatalf("pricing was not resolved at request start: model=%s at=%s", pricing.modelID, pricing.at)
	}
	if biller.usage.Rates.InputMicros != 3_000_000 || biller.usage.Rates.OutputMicros != 9_000_000 || biller.usage.CostMicros != 210 {
		t.Fatalf("scheduled price was not pinned through settlement: %+v", biller.usage)
	}
	if biller.reserved != wantReservation.CostMicros {
		t.Fatalf("scheduled price was not pinned through reservation: got=%d want=%d", biller.reserved, wantReservation.CostMicros)
	}
}

/**
 * TestProxyUsesConfiguredInputAndOutputReservationCaps 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProxyUsesConfiguredInputAndOutputReservationCaps(t *testing.T) {
	biller := &fakeBilling{}
	route := openAIRoute()
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":10,"completion_tokens":20}}`))}, nil
	})}
	settings := gatewaysettings.DefaultConfig()
	settings.ReservationInputTokenCap = 96
	settings.ReservationOutputTokenCap = 128
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: route}, Billing: biller, Client: client, Settings: fakeGatewaySettings{config: settings}})
	body := `{"model":"deepseek-chat","messages":[{"role":"user","content":"` + strings.Repeat("long input ", 256) + `"}],"max_tokens":4096}`
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	upstreamBody, _ := buildUpstreamBody(payload, route, "chat_completions", false)
	if estimateInputTokens(upstreamBody) <= settings.ReservationInputTokenCap {
		t.Fatalf("test request estimate=%d must exceed cap=%d", estimateInputTokens(upstreamBody), settings.ReservationInputTokenCap)
	}
	want, _ := billing.EstimateReservation(96, 128, rateCardFor(route), 10_000)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
	if response.Code != http.StatusOK || biller.reserved != want.CostMicros || biller.completeCalls != 1 {
		t.Fatalf("status=%d reserved=%d want=%d billing=%+v", response.Code, biller.reserved, want.CostMicros, biller)
	}
}

/**
 * TestIdempotencyReplayDoesNotCallUpstreamOrReserveAgain 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestIdempotencyReplayDoesNotCallUpstreamOrReserveAgain(t *testing.T) {
	actor := gatewayActor()
	requestID := uuid.New()
	replay := billing.Operation{RequestID: requestID, UserID: actor.User.ID, APIKeyID: actor.APIKey.ID, Endpoint: "chat_completions", Status: billing.OperationCompleted, ReservedMicros: 1}
	biller := &fakeBilling{replayOperation: &replay}
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("must not call upstream") })}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: actor}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`))
	request.Header.Set("Idempotency-Key", "same-operation")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || calls != 0 || biller.reserveCalls != 0 || response.Header().Get(requestid.Header) != requestID.String() {
		t.Fatalf("status=%d calls=%d billing=%+v body=%s", response.Code, calls, biller, response.Body.String())
	}
}

/**
 * TestProxyAppliesScheduledBillingGroupDiscountToReservationAndCharge 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProxyAppliesScheduledBillingGroupDiscountToReservationAndCharge(t *testing.T) {
	startedAt := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	actor := gatewayActorWithMultiplier(4_000)
	discounts := &fakeDiscounts{multiplierBPS: 3_200}
	route := openAIRoute()
	biller := &fakeBilling{}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"up-multiplier","usage":{"prompt_tokens":10,"completion_tokens":20}}`)),
		}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: actor}, Routes: fakeRoutes{route: route, expectedGroupID: actor.APIKey.BillingGroupID}, Billing: biller, Discounts: discounts, Client: client})
	handler.now = func() time.Time { return startedAt }
	requestBody := `{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}],"max_tokens":100}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(requestBody)))

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if biller.reserveCalls != 1 || biller.finalizeCalls != 1 || biller.refundCalls != 0 {
		t.Fatalf("unexpected billing calls: %+v", biller)
	}
	if biller.usage.MultiplierBPS != 3_200 || biller.usage.BaseCostMicros != 180 || biller.usage.CostMicros != 58 {
		t.Fatalf("billing group discount was not applied to final charge: %+v", biller.usage)
	}
	if discounts.calls != 1 || discounts.groupID != actor.APIKey.BillingGroupID {
		t.Fatalf("shared discount resolver was not used: calls=%d group=%s", discounts.calls, discounts.groupID)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(requestBody), &payload); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	upstreamBody, err := buildUpstreamBody(payload, route, "chat_completions", false)
	if err != nil {
		t.Fatalf("build upstream body: %v", err)
	}
	expectedReservation, err := billing.EstimateReservation(estimateInputTokens(upstreamBody), 100, rateCardFor(route), 3_200)
	if err != nil {
		t.Fatalf("estimate expected reservation: %v", err)
	}
	if biller.reserved != expectedReservation.CostMicros {
		t.Fatalf("billing group discount was not applied to reservation: got=%d want=%d", biller.reserved, expectedReservation.CostMicros)
	}
}

func TestCompositeProxyReservesHighestCandidateAndChargesDirectHitGroup(t *testing.T) {
	startedAt := time.Date(2026, time.October, 2, 12, 0, 0, 0, time.UTC)
	fastGroupID, expensiveGroupID := uuid.New(), uuid.New()
	actor := compositeGatewayActor(fastGroupID, expensiveGroupID)
	fast := routeWithBillingGroup(openAIChannel("fast", "fast.example.com", "fast-upstream"), fastGroupID, "fast-group", "Fast Group", 3_000)
	fast.Provider.Weight = 20
	expensive := routeWithBillingGroup(openAIChannel("expensive", "expensive.example.com", "expensive-upstream"), expensiveGroupID, "expensive-group", "Expensive Group", 8_000)
	expensive.Provider.Weight = 10
	discounts := &fakeDiscounts{multipliers: map[uuid.UUID]int64{fastGroupID: 3_000, expensiveGroupID: 8_000}}
	biller := &fakeBilling{}
	hosts := make([]string, 0, 2)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		hosts = append(hosts, request.URL.Host)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"fast-hit","usage":{"prompt_tokens":10,"completion_tokens":20}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: actor}, Routes: fakeRoutes{candidates: []modelroute.Resolved{expensive, fast}, expectedGroupID: actor.APIKey.BillingGroupID}, Billing: biller, Discounts: discounts, Client: client})
	handler.now = func() time.Time { return startedAt }
	handler.upstreamRetryDelays = nil
	body := `{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}],"max_tokens":100}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
	if response.Code != http.StatusOK || strings.Join(hosts, ",") != "fast.example.com" {
		t.Fatalf("status=%d hosts=%v body=%s", response.Code, hosts, response.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	wantReserved := int64(0)
	for _, candidate := range []struct {
		route      modelroute.Resolved
		multiplier int64
	}{{route: fast, multiplier: 3_000}, {route: expensive, multiplier: 8_000}} {
		upstreamBody, err := buildUpstreamBody(payload, candidate.route, "chat_completions", false)
		if err != nil {
			t.Fatal(err)
		}
		reservation, err := billing.EstimateReservation(estimateInputTokens(upstreamBody), 100, rateCardFor(candidate.route), candidate.multiplier)
		if err != nil {
			t.Fatal(err)
		}
		wantReserved = max(wantReserved, reservation.CostMicros)
	}
	if biller.reserved != wantReserved {
		t.Fatalf("composite reservation=%d, want highest candidate %d", biller.reserved, wantReserved)
	}
	if biller.usage.BillingGroupID == nil || *biller.usage.BillingGroupID != fastGroupID || biller.usage.BillingGroupCode != "fast-group" || biller.usage.BillingGroupName != "Fast Group" || biller.usage.MultiplierBPS != 3_000 || biller.usage.ModelRouteID != fast.ID {
		t.Fatalf("direct hit did not use actual member billing snapshot: %+v", biller.usage)
	}
	if discounts.calls != 2 {
		t.Fatalf("candidate multipliers were not pinned once: calls=%d groups=%v", discounts.calls, discounts.groupIDs)
	}
}

func TestCompositeProxySafeFailoverChargesFallbackMember(t *testing.T) {
	primaryGroupID, fallbackGroupID := uuid.New(), uuid.New()
	actor := compositeGatewayActor(primaryGroupID, fallbackGroupID)
	primary := routeWithBillingGroup(openAIChannel("primary", "primary.example.com", "primary-upstream"), primaryGroupID, "primary-group", "Primary Group", 2_500)
	primary.Provider.Weight = 20
	fallback := routeWithBillingGroup(openAIChannel("fallback", "fallback.example.com", "fallback-upstream"), fallbackGroupID, "fallback-group", "Fallback Group", 6_000)
	fallback.Provider.Weight = 10
	discounts := &fakeDiscounts{multipliers: map[uuid.UUID]int64{primaryGroupID: 2_500, fallbackGroupID: 6_000}}
	biller := &fakeBilling{}
	hosts := make([]string, 0, 2)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		hosts = append(hosts, request.URL.Host)
		if request.URL.Host == "primary.example.com" {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection failed before request write")}
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"fallback-hit","usage":{"prompt_tokens":10,"completion_tokens":20}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: actor}, Routes: fakeRoutes{candidates: []modelroute.Resolved{fallback, primary}, expectedGroupID: actor.APIKey.BillingGroupID}, Billing: biller, Discounts: discounts, Client: client})
	handler.upstreamRetryDelays = nil
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":100}`)))
	if response.Code != http.StatusOK || strings.Join(hosts, ",") != "primary.example.com,fallback.example.com" {
		t.Fatalf("status=%d hosts=%v body=%s", response.Code, hosts, response.Body.String())
	}
	if biller.usage.BillingGroupID == nil || *biller.usage.BillingGroupID != fallbackGroupID || biller.usage.BillingGroupCode != "fallback-group" || biller.usage.MultiplierBPS != 6_000 || biller.usage.ModelRouteID != fallback.ID {
		t.Fatalf("safe failover did not settle against fallback member: %+v", biller.usage)
	}
}

func TestCompositeProxyFailureRecordsLastAttemptedMember(t *testing.T) {
	firstGroupID, secondGroupID := uuid.New(), uuid.New()
	actor := compositeGatewayActor(firstGroupID, secondGroupID)
	first := routeWithBillingGroup(openAIChannel("first-member", "first-member.example.com", "first-member-upstream"), firstGroupID, "first-member", "First Member", 3_000)
	first.Provider.Weight = 20
	second := routeWithBillingGroup(openAIChannel("second-member", "second-member.example.com", "second-member-upstream"), secondGroupID, "second-member", "Second Member", 7_000)
	second.Provider.Weight = 10
	discounts := &fakeDiscounts{multipliers: map[uuid.UUID]int64{firstGroupID: 3_000, secondGroupID: 7_000}}
	biller := &fakeBilling{}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection failed before request write")}
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: actor}, Routes: fakeRoutes{candidates: []modelroute.Resolved{second, first}}, Billing: biller, Discounts: discounts, Client: client})
	handler.upstreamRetryDelays = nil
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`)))
	if response.Code != http.StatusBadGateway || len(biller.failures) != 1 {
		t.Fatalf("status=%d failures=%+v body=%s", response.Code, biller.failures, response.Body.String())
	}
	failure := biller.failures[0]
	if failure.BillingGroupID == nil || *failure.BillingGroupID != secondGroupID || failure.BillingGroupCode != "second-member" || failure.BillingGroupName != "Second Member" || failure.MultiplierBPS != 7_000 || failure.ModelRouteID != second.ID {
		t.Fatalf("failure did not use last attempted member: %+v", failure)
	}
}

/**
 * TestProxySSEHeartbeatKeepsEstablishedStreamActive 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestProxyStopsIdleStream 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestProxyReturnsGatewayTimeoutForUpstreamTotalTimeout 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestProxyReturnsGatewayTimeoutWhenBufferedBodyExceedsTotalTimeout 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProxyReturnsGatewayTimeoutWhenBufferedBodyExceedsTotalTimeout(t *testing.T) {
	body := newBlockingBody("")
	biller := &fakeBilling{}
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
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client, Settings: fakeGatewaySettings{config: settings}})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Authorization", "Bearer nvr_test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("status=%d body=%s, want 504", response.Code, response.Body.String())
	}
	if biller.unknownCalls != 1 || biller.failCalls != 0 || biller.refundCalls != 0 || biller.finalizeCalls != 0 {
		t.Fatalf("buffered response timeout was refunded or settled: %+v", biller)
	}
}

/**
 * TestParseUsageRejectsMalformedReportedFields 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestParseUsageRejectsMalformedReportedFields(t *testing.T) {
	rates := rateCardFor(openAIRoute())
	tests := []struct {
		name         string
		body         string
		wantInput    int
		wantOutput   int
		wantEstimate bool
	}{
		{name: "string input", body: `{"usage":{"prompt_tokens":"10","completion_tokens":20}}`, wantInput: 0, wantOutput: 20, wantEstimate: true},
		{name: "negative output", body: `{"usage":{"prompt_tokens":10,"completion_tokens":-1}}`, wantInput: 10, wantOutput: 0, wantEstimate: true},
		{name: "fractional input", body: `{"usage":{"prompt_tokens":1.5,"completion_tokens":20}}`, wantInput: 0, wantOutput: 20, wantEstimate: true},
		{name: "large output", body: `{"usage":{"prompt_tokens":10,"completion_tokens":100000001}}`, wantInput: 10, wantOutput: 100000001, wantEstimate: false},
		{name: "invalid cache field", body: `{"usage":{"prompt_tokens":10,"completion_tokens":20,"cached_tokens":"4"}}`, wantInput: 0, wantOutput: 20, wantEstimate: true},
		{name: "invalid nested cache creation field", body: `{"usage":{"prompt_tokens":10,"completion_tokens":20,"prompt_tokens_details":{"cache_creation_input_tokens":"4"}}}`, wantInput: 0, wantOutput: 20, wantEstimate: true},
		{name: "invalid cache container", body: `{"usage":{"prompt_tokens":10,"completion_tokens":20,"cache_creation":"4"}}`, wantInput: 0, wantOutput: 20, wantEstimate: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := applyUsageFallback(parseUsage([]byte(tt.body), usageSemanticsOpenAITotal), 100, 50, rates)
			if usage.Input != tt.wantInput || usage.Output != tt.wantOutput || usage.Estimated != tt.wantEstimate {
				t.Fatalf("usage=%+v want input=%d output=%d estimated=%v", usage, tt.wantInput, tt.wantOutput, tt.wantEstimate)
			}
		})
	}
}

/**
 * TestParseIntValueRejectsNonFiniteAndInvalidNumbers 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestProxyAcceptsBodyAndOutputAboveFormerLimits 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestProxyAcceptsBufferedResponseAboveFormerLimit 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestProxyAlwaysStartsWithHighestWeightProvider 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProxyAlwaysStartsWithHighestWeightProvider(t *testing.T) {
	first := openAIChannel("first", "first.example.com", "first-upstream")
	second := openAIChannel("second", "second.example.com", "second-upstream")
	first.Provider.Weight = 200
	second.Provider.Weight = 100
	hosts := make([]string, 0, 4)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		hosts = append(hosts, request.URL.Host)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":1,"completion_tokens":1}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{candidates: []modelroute.Resolved{second, first}}, Billing: &fakeBilling{}, Client: client})
	for range 4 {
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`))
		request.Header.Set("Idempotency-Key", uuid.NewString())
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	}
	want := []string{"first.example.com", "first.example.com", "first.example.com", "first.example.com"}
	if strings.Join(hosts, ",") != strings.Join(want, ",") {
		t.Fatalf("weighted hosts=%v want=%v", hosts, want)
	}
}

/**
 * TestProxyWeightPriorityIsStableUnderConcurrentRequests 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProxyWeightPriorityIsStableUnderConcurrentRequests(t *testing.T) {
	first := openAIChannel("first", "first.example.com", "first-upstream")
	second := openAIChannel("second", "second.example.com", "second-upstream")
	first.Provider.Weight = 200
	second.Provider.Weight = 100
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
	if firstCalls.Load() != 100 || secondCalls.Load() != 0 {
		t.Fatalf("concurrent priority first=%d second=%d", firstCalls.Load(), secondCalls.Load())
	}
}

/**
 * TestProxyDoesNotReplayAcrossChannelsAfterHTTPResponse 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProxyDoesNotReplayAcrossChannelsAfterHTTPResponse(t *testing.T) {
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
	if response.Code != http.StatusBadGateway || strings.Join(hosts, ",") != "first.example.com" {
		t.Fatalf("status=%d hosts=%v body=%s", response.Code, hosts, response.Body.String())
	}
	if biller.reserveCalls != 1 || biller.finalizeCalls != 0 || biller.failCalls != 1 || biller.refundCalls != 1 {
		t.Fatalf("unexpected billing state: %+v", biller)
	}
}

/**
 * TestProxyDoesNotReplayAfterSuccessfulResponseStarts 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProxyDoesNotReplayAfterSuccessfulResponseStarts(t *testing.T) {
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
	if response.Code != http.StatusBadGateway || calls != 1 || biller.finalizeCalls != 0 || biller.refundCalls != 0 || biller.unknownCalls != 1 || !strings.Contains(response.Body.String(), "upstream_result_unknown") {
		t.Fatalf("status=%d calls=%d billing=%+v body=%s", response.Code, calls, biller, response.Body.String())
	}
}

/**
 * TestProxyReturnsFailureAndRefundsOnceAfterAllChannelsFail 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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
	handler.upstreamRetryDelays = []time.Duration{0, 0}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`)))
	if response.Code != http.StatusBadGateway || calls != 4 || biller.reserveCalls != 1 || biller.refundCalls != 1 || biller.refunded != biller.reserved || biller.finalizeCalls != 0 || len(biller.failures) != 1 {
		t.Fatalf("status=%d calls=%d billing=%+v body=%s", response.Code, calls, biller, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "upstream_unavailable") {
		t.Fatalf("unexpected error body: %s", response.Body.String())
	}
	if failure := biller.failures[0]; failure.StatusCode != http.StatusBadGateway || failure.ErrorCode != "upstream_http_error" || failure.ErrorMessage != "上游返回 HTTP 429：limited" || failure.MultiplierBPS != 10_000 || failure.ModelName != "deepseek-chat" {
		t.Fatalf("unexpected failure log: %+v", failure)
	}
}

func TestSummarizeUpstreamErrorExtractsSafeValidationMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "detail", body: `{"detail":"messages must be a non-empty array"}`, want: "messages must be a non-empty array"},
		{name: "nested message", body: `{"error":{"message":"invalid top_p: only 0.95 is allowed for this model"}}`, want: "invalid top_p: only 0.95 is allowed for this model"},
		{name: "plain text", body: "  invalid request\r\n  ", want: "invalid request"},
		{name: "redacts token-like value", body: `{"message":"invalid key gp-abcdefghijklmnopqrstuvwxyz0123456789"}`, want: "invalid key [redacted]"},
		{name: "ignores unrelated success JSON", body: `{"status":"failed","request":{"input":"private prompt"}}`, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarizeUpstreamError([]byte(tt.body)); got != tt.want {
				t.Fatalf("summarizeUpstreamError()=%q want=%q", got, tt.want)
			}
		})
	}
}

/**
 * TestProxyUsesHighestWeightWithoutReplayingHTTPFailure 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProxyUsesHighestWeightWithoutReplayingHTTPFailure(t *testing.T) {
	low := openAIChannel("low", "low.example.com", "low-upstream")
	high := openAIChannel("high", "high.example.com", "high-upstream")
	low.Provider.Weight = 10
	high.Provider.Weight = 20
	hosts := make([]string, 0, 4)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		hosts = append(hosts, request.URL.Host)
		if request.URL.Host == "high.example.com" {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"temporary"}`))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":1,"completion_tokens":1}}`))}, nil
	})}
	biller := &fakeBilling{}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{candidates: []modelroute.Resolved{low, high}}, Billing: biller, Client: client})
	handler.upstreamRetryDelays = []time.Duration{0, 0}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`)))
	if response.Code != http.StatusBadGateway || strings.Join(hosts, ",") != "high.example.com" {
		t.Fatalf("status=%d hosts=%v body=%s", response.Code, hosts, response.Body.String())
	}
	if biller.finalizeCalls != 0 || biller.failCalls != 1 || biller.refundCalls != 1 {
		t.Fatalf("unexpected billing state: %+v", biller)
	}
}

/**
 * TestProxyRetriesConnectionSetupBeforeRequestWrite 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestProxyDoesNotRetryAfterConnectionWasAcquired 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProxyDoesNotRetryAfterConnectionWasAcquired(t *testing.T) {
	first := openAIChannel("first", "first.example.com", "first-upstream")
	second := openAIChannel("second", "second.example.com", "second-upstream")
	biller := &fakeBilling{}
	hosts := make([]string, 0, 2)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		hosts = append(hosts, request.URL.Host)
		trace := httptrace.ContextClientTrace(request.Context())
		if trace == nil || trace.GotConn == nil {
			t.Fatal("connection trace is missing")
		}
		trace.GotConn(httptrace.GotConnInfo{})
		return nil, &net.OpError{Op: "write", Net: "tcp", Err: errors.New("connection lost before write completion")}
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{candidates: []modelroute.Resolved{first, second}}, Billing: biller, Client: client})
	handler.upstreamRetryDelays = []time.Duration{0, 0}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`)))
	if response.Code != http.StatusBadGateway || strings.Join(hosts, ",") != "first.example.com" || biller.unknownCalls != 1 || biller.failCalls != 0 || biller.refundCalls != 0 {
		t.Fatalf("status=%d hosts=%v billing=%+v body=%s", response.Code, hosts, biller, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "upstream_result_unknown") {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

/**
 * TestProxyDoesNotForwardRawClientIdempotencyKey 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProxyDoesNotForwardRawClientIdempotencyKey(t *testing.T) {
	biller := &fakeBilling{}
	var upstreamKey string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		upstreamKey = request.Header.Get("Idempotency-Key")
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":1,"completion_tokens":1}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10}`))
	request.Header.Set("Idempotency-Key", "shared-client-value")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || upstreamKey != "novro-"+biller.operation.RequestID.String() || upstreamKey == "shared-client-value" {
		t.Fatalf("status=%d upstream_key=%q operation=%+v body=%s", response.Code, upstreamKey, biller.operation, response.Body.String())
	}
}

/**
 * TestProxyBuffersChatStreamWhenUpstreamStreamCannotStart 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestProxyDoesNotFailOverAfterBufferedFallbackWasWritten 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProxyDoesNotFailOverAfterBufferedFallbackWasWritten(t *testing.T) {
	first := openAIChannel("first", "first.example.com", "first-upstream")
	second := openAIChannel("second", "second.example.com", "second-upstream")
	biller := &fakeBilling{}
	hosts := make([]string, 0, 3)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		hosts = append(hosts, request.URL.Host)
		body, _ := io.ReadAll(request.Body)
		if bytes.Contains(body, []byte(`"stream":true`)) {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("stream connection failed")}
		}
		trace := httptrace.ContextClientTrace(request.Context())
		if trace == nil || trace.WroteRequest == nil {
			t.Fatal("request write trace is missing")
		}
		trace.WroteRequest(httptrace.WroteRequestInfo{})
		return nil, &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection lost after write")}
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{candidates: []modelroute.Resolved{first, second}}, Billing: biller, Client: client})
	handler.upstreamRetryDelays = nil
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":10,"stream":true}`)))
	if response.Code != http.StatusBadGateway || strings.Join(hosts, ",") != "first.example.com,first.example.com" || biller.unknownCalls != 1 || biller.failCalls != 0 || biller.refundCalls != 0 {
		t.Fatalf("status=%d hosts=%v billing=%+v body=%s", response.Code, hosts, biller, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "upstream_result_unknown") {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

/**
 * TestProxyStreamRetriesTwiceThenFailsOverToNextChannel 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProxyStreamRetriesTwiceThenFailsOverToNextChannel(t *testing.T) {
	first := openAIChannel("first", "first.example.com", "first-upstream")
	second := openAIChannel("second", "second.example.com", "second-upstream")
	hosts := make([]string, 0, 4)
	streamModes := make([]bool, 0, 4)
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
	if response.Code != http.StatusOK || strings.Join(hosts, ",") != "first.example.com,first.example.com,first.example.com,second.example.com" {
		t.Fatalf("status=%d hosts=%v body=%s", response.Code, hosts, response.Body.String())
	}
	if len(streamModes) != 4 || !streamModes[0] || streamModes[1] || streamModes[2] || !streamModes[3] {
		t.Fatalf("unexpected upstream stream modes: %v", streamModes)
	}
	if biller.finalizeCalls != 1 || biller.usage.ModelRouteID != second.ID || biller.refundCalls != 0 {
		t.Fatalf("unexpected billing state: %+v", biller)
	}
}

/**
 * TestProxyStreamDoesNotReplayTemporaryHTTPFailureAsBufferedRequest 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestProxyStreamDoesNotReplayTemporaryHTTPFailureAsBufferedRequest(t *testing.T) {
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
	if response.Code != http.StatusBadGateway || calls != 1 || len(streamModes) != 1 || !streamModes[0] {
		t.Fatalf("status=%d calls=%d stream_modes=%v body=%s", response.Code, calls, streamModes, response.Body.String())
	}
	if biller.finalizeCalls != 0 || biller.failCalls != 1 || biller.refundCalls != 1 {
		t.Fatalf("unexpected billing state: %+v", biller)
	}
}

/**
 * TestProxyUsesBufferedUpstreamForReasonixStreams 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestProxyRejectsSelfReferentialUpstream 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestFinalizationRetriesTransientErrors 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestFinalizationDoesNotRetryBusinessConflict 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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
	if response.Code != http.StatusOK || response.Body.String() != body || biller.finalizeCalls != 1 || biller.refundCalls != 0 || biller.pendingCalls != 1 || biller.completeCalls != 0 {
		t.Fatalf("status=%d finalize_calls=%d refund_calls=%d body=%s", response.Code, biller.finalizeCalls, biller.refundCalls, response.Body.String())
	}
	if !strings.Contains(logs.String(), "forward successful response after usage finalization failure") {
		t.Fatalf("successful response after finalization failure was not logged: %s", logs.String())
	}
}

/**
 * TestBufferedResponseIsNotReturnedBeforeSettlementIntentPersists 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestBufferedResponseIsNotReturnedBeforeSettlementIntentPersists(t *testing.T) {
	biller := &fakeBilling{pendingErrors: []error{billing.ErrRequestConflict}}
	upstreamBody := `{"id":"up-1","choices":[{"message":{"content":"must not be returned"}}],"usage":{"prompt_tokens":10,"completion_tokens":20}}`
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(upstreamBody))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":100}`)))
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "must not be returned") || biller.finalizeCalls != 0 || biller.completeCalls != 0 || biller.refundCalls != 0 || biller.unknownCalls != 1 {
		t.Fatalf("status=%d billing=%+v body=%s", response.Code, biller, response.Body.String())
	}
}

/**
 * TestBufferedHTTP200FailureIsNotBilled 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestBufferedHTTP200FailureIsNotBilled(t *testing.T) {
	biller := &fakeBilling{}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"failed"}}`))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":100}`)))
	if response.Code != http.StatusBadGateway || biller.finalizeCalls != 0 || biller.failCalls != 1 || biller.refundCalls != 1 || len(biller.failures) != 1 {
		t.Fatalf("status=%d billing=%+v body=%s", response.Code, biller, response.Body.String())
	}
}

/**
 * TestSettlementRetryStopsWhenContextIsCanceled 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestProxyRoutesResponsesAndAnthropicMessages 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestDualProtocolProviderRoutesResponsesAndAnthropicMessages(t *testing.T) {
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
			route:             dualProtocolRoute(),
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
			body:         `{"model":"deepseek-chat","messages":[{"role":"user","content":"hello"}],"max_tokens":32}`,
			route:        dualProtocolRoute(),
			wantURL:      "https://api.example.com/v1/messages",
			wantAPIKey:   "upstream-secret",
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

func TestProviderOutboundFormatConvertsRequestResponseAndUsage(t *testing.T) {
	chatRoute := openAIRoute()
	chatRoute.Provider.OutboundFormat = provider.OutboundFormatChatCompletions
	responsesRoute := openAIRoute()
	responsesRoute.Provider.OutboundFormat = provider.OutboundFormatResponses
	messagesRoute := anthropicRoute()
	messagesRoute.Provider.OutboundFormat = provider.OutboundFormatMessages
	passthroughRoute := openAIRoute()

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
		wantUpstreamID    string
		wantClientFormat  string
		wantClientText    string
		wantClientInput   int
		wantClientCache   int
		wantInput         int
		wantUncached      int
		wantCacheRead     int
		wantCacheWrite    int
		wantOutput        int
		passthrough       bool
		assertUpstream    func(*testing.T, map[string]any)
	}{
		{
			name:              "responses to configured chat completions",
			path:              "/v1/responses",
			body:              `{"model":"deepseek-chat","instructions":"be concise","input":"hello chat","max_output_tokens":32}`,
			route:             chatRoute,
			wantURL:           "https://api.example.com/v1/chat/completions",
			wantAuthorization: "Bearer upstream-secret",
			wantEndpoint:      "responses",
			upstreamBody:      `{"id":"chat-up","object":"chat.completion","created":1,"model":"deepseek-v3","choices":[{"index":0,"message":{"role":"assistant","content":"chat answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":3}}}`,
			wantUpstreamID:    "chat-up",
			wantClientFormat:  "responses",
			wantClientText:    "chat answer",
			wantClientInput:   10,
			wantClientCache:   3,
			wantInput:         10,
			wantUncached:      7,
			wantCacheRead:     3,
			wantOutput:        4,
			assertUpstream: func(t *testing.T, payload map[string]any) {
				t.Helper()
				messages := sliceValue(payload["messages"])
				if intValue(payload["max_tokens"]) != 32 || len(messages) != 2 || stringValue(mapValue(messages[0])["role"]) != "system" || textFromContent(mapValue(messages[0])["content"]) != "be concise" || stringValue(mapValue(messages[1])["role"]) != "user" || textFromContent(mapValue(messages[1])["content"]) != "hello chat" {
					t.Fatalf("unexpected Chat Completions request: %+v", payload)
				}
			},
		},
		{
			name:              "messages to configured responses",
			path:              "/v1/messages",
			body:              `{"model":"deepseek-chat","system":"answer clearly","messages":[{"role":"user","content":"hello responses"}],"max_tokens":48}`,
			route:             responsesRoute,
			wantURL:           "https://api.example.com/v1/responses",
			wantAuthorization: "Bearer upstream-secret",
			wantEndpoint:      "messages",
			upstreamBody:      `{"id":"resp-up","object":"response","created_at":1,"status":"completed","model":"deepseek-v3","output":[{"id":"msg-up","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"responses answer","annotations":[]}]}],"usage":{"input_tokens":12,"output_tokens":5,"input_tokens_details":{"cached_tokens":5}}}`,
			wantUpstreamID:    "resp-up",
			wantClientFormat:  "messages",
			wantClientText:    "responses answer",
			wantClientInput:   7,
			wantClientCache:   5,
			wantInput:         12,
			wantUncached:      7,
			wantCacheRead:     5,
			wantOutput:        5,
			assertUpstream: func(t *testing.T, payload map[string]any) {
				t.Helper()
				input := sliceValue(payload["input"])
				if len(input) != 1 {
					t.Fatalf("unexpected Responses input: %+v", payload)
				}
				first := mapValue(input[0])
				if intValue(payload["max_output_tokens"]) != 48 || stringValue(payload["instructions"]) != "answer clearly" || stringValue(first["type"]) != "message" || stringValue(first["role"]) != "user" || textFromContent(first["content"]) != "hello responses" {
					t.Fatalf("unexpected Responses request: %+v", payload)
				}
			},
		},
		{
			name:             "chat completions to configured messages",
			path:             "/v1/chat/completions",
			body:             `{"model":"kimi-k3","messages":[{"role":"system","content":"think first"},{"role":"user","content":"hello messages"}],"max_tokens":64}`,
			route:            messagesRoute,
			wantURL:          "https://api.anthropic.com/v1/messages",
			wantAPIKey:       "anthropic-secret",
			wantEndpoint:     "chat_completions",
			upstreamBody:     `{"id":"msg-up","type":"message","role":"assistant","model":"kimi-k3-upstream","content":[{"type":"text","text":"messages answer"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"cache_read_input_tokens":3,"cache_creation_input_tokens":2,"output_tokens":6}}`,
			wantUpstreamID:   "msg-up",
			wantClientFormat: "chat_completions",
			wantClientText:   "messages answer",
			wantClientInput:  15,
			wantClientCache:  3,
			wantInput:        15,
			wantUncached:     10,
			wantCacheRead:    3,
			wantCacheWrite:   2,
			wantOutput:       6,
			assertUpstream: func(t *testing.T, payload map[string]any) {
				t.Helper()
				messages := sliceValue(payload["messages"])
				if len(messages) != 1 {
					t.Fatalf("unexpected Anthropic Messages input: %+v", payload)
				}
				first := mapValue(messages[0])
				if intValue(payload["max_tokens"]) != 64 || stringValue(payload["system"]) != "think first" || stringValue(first["role"]) != "user" || textFromContent(first["content"]) != "hello messages" {
					t.Fatalf("unexpected Anthropic Messages request: %+v", payload)
				}
			},
		},
		{
			name:              "unset outbound format preserves chat passthrough",
			path:              "/v1/chat/completions",
			body:              `{"model":"deepseek-chat","messages":[{"role":"user","content":"keep vendor fields"}],"max_tokens":16,"vendor_option":{"mode":"native"}}`,
			route:             passthroughRoute,
			wantURL:           "https://api.example.com/v1/chat/completions",
			wantAuthorization: "Bearer upstream-secret",
			wantEndpoint:      "chat_completions",
			upstreamBody:      `{"id":"pass-up","object":"chat.completion","created":1,"model":"deepseek-v3","choices":[{"index":0,"message":{"role":"assistant","content":"native answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":2},"vendor_response":{"kept":true}}`,
			wantUpstreamID:    "pass-up",
			wantClientFormat:  "chat_completions",
			wantClientText:    "native answer",
			wantClientInput:   9,
			wantInput:         9,
			wantUncached:      9,
			wantOutput:        2,
			passthrough:       true,
			assertUpstream: func(t *testing.T, payload map[string]any) {
				t.Helper()
				vendor := mapValue(payload["vendor_option"])
				if stringValue(vendor["mode"]) != "native" {
					t.Fatalf("same-protocol request lost vendor field: %+v", payload)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			biller := &fakeBilling{}
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != tt.wantURL {
					t.Fatalf("upstream URL=%s want=%s", request.URL, tt.wantURL)
				}
				if request.Header.Get("Authorization") != tt.wantAuthorization || request.Header.Get("X-API-Key") != tt.wantAPIKey {
					t.Fatalf("unexpected upstream auth Authorization=%q X-API-Key=%q", request.Header.Get("Authorization"), request.Header.Get("X-API-Key"))
				}
				if tt.wantAPIKey != "" && request.Header.Get("Anthropic-Version") != "2023-06-01" {
					t.Fatalf("Anthropic-Version=%q", request.Header.Get("Anthropic-Version"))
				}
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("read upstream request: %v", err)
				}
				var payload map[string]any
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatalf("decode upstream request: %v body=%s", err, body)
				}
				if stringValue(payload["model"]) != tt.route.UpstreamName {
					t.Fatalf("upstream model=%q want=%q body=%s", stringValue(payload["model"]), tt.route.UpstreamName, body)
				}
				tt.assertUpstream(t, payload)
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
			if biller.finalizeCalls != 1 || biller.usage.Estimated || biller.usage.Endpoint != tt.wantEndpoint || biller.usage.UpstreamRequestID != tt.wantUpstreamID || biller.usage.InputTokens != tt.wantInput || biller.usage.Tokens.UncachedInput != tt.wantUncached || biller.usage.Tokens.CacheRead != tt.wantCacheRead || biller.usage.Tokens.CacheWrite != tt.wantCacheWrite || biller.usage.OutputTokens != tt.wantOutput {
				t.Fatalf("unexpected usage: %+v", biller.usage)
			}
			if tt.passthrough && response.Body.String() != tt.upstreamBody {
				t.Fatalf("same-protocol response was not passed through: %s", response.Body.String())
			}
			var clientPayload map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &clientPayload); err != nil {
				t.Fatalf("decode client response: %v body=%s", err, response.Body.String())
			}
			switch tt.wantClientFormat {
			case "messages":
				usage := mapValue(clientPayload["usage"])
				if stringValue(clientPayload["type"]) != "message" || textFromContent(clientPayload["content"]) != tt.wantClientText || intValue(usage["input_tokens"]) != tt.wantClientInput || intValue(usage["cache_read_input_tokens"]) != tt.wantClientCache || intValue(usage["output_tokens"]) != tt.wantOutput {
					t.Fatalf("unexpected Messages client response: %+v", clientPayload)
				}
			case "responses":
				output := sliceValue(clientPayload["output"])
				usage := mapValue(clientPayload["usage"])
				if stringValue(clientPayload["object"]) != "response" || len(output) != 1 || textFromContent(mapValue(output[0])["content"]) != tt.wantClientText || intValue(usage["input_tokens"]) != tt.wantClientInput || intValue(mapValue(usage["input_tokens_details"])["cached_tokens"]) != tt.wantClientCache || intValue(usage["output_tokens"]) != tt.wantOutput {
					t.Fatalf("unexpected Responses client response: %+v", clientPayload)
				}
			case "chat_completions":
				choices := sliceValue(clientPayload["choices"])
				usage := mapValue(clientPayload["usage"])
				if stringValue(clientPayload["object"]) != "chat.completion" || len(choices) != 1 || textFromContent(mapValue(mapValue(choices[0])["message"])["content"]) != tt.wantClientText || intValue(usage["prompt_tokens"]) != tt.wantClientInput || intValue(mapValue(usage["prompt_tokens_details"])["cached_tokens"]) != tt.wantClientCache || intValue(usage["completion_tokens"]) != tt.wantOutput {
					t.Fatalf("unexpected Chat Completions client response: %+v", clientPayload)
				}
			default:
				t.Fatalf("unsupported client format %q", tt.wantClientFormat)
			}
		})
	}
}

func TestProviderOutboundFormatStreamsSSEThroughHTTPGateway(t *testing.T) {
	route := openAIRoute()
	route.Provider.OutboundFormat = provider.OutboundFormatChatCompletions
	biller := &fakeBilling{}
	upstreamSSE := strings.Join([]string{
		`data: {"id":"chat-sse","object":"chat.completion.chunk",`,
		`data: "created":1,"model":"deepseek-v3","choices":[{"index":0,"delta":{"role":"assistant","content":"hello over SSE"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chat-sse","object":"chat.completion.chunk","created":1,"model":"deepseek-v3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: {"id":"chat-sse","object":"chat.completion.chunk","created":1,"model":"deepseek-v3","choices":[],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`,
	}, "\n")
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.example.com/v1/chat/completions" {
			t.Fatalf("upstream URL=%s", request.URL)
		}
		if request.Header.Get("Authorization") != "Bearer upstream-secret" || request.Header.Get("X-API-Key") != "" {
			t.Fatalf("unexpected upstream auth Authorization=%q X-API-Key=%q", request.Header.Get("Authorization"), request.Header.Get("X-API-Key"))
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read upstream request: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode upstream request: %v body=%s", err, body)
		}
		options := mapValue(payload["stream_options"])
		if !boolValue(payload["stream"]) || !boolValue(options["include_usage"]) {
			t.Fatalf("Chat stream request did not require final usage: %s", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
		}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: route}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek-chat","input":"hello","max_output_tokens":32,"stream":true}`))
	request.Header.Set("Authorization", "Bearer nvr_test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status=%d content-type=%q body=%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{"event: response.output_text.delta", "hello over SSE", "event: response.completed"} {
		if !strings.Contains(body, want) {
			t.Fatalf("converted Responses SSE is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "[DONE]") {
		t.Fatalf("Responses SSE unexpectedly contains Chat sentinel:\n%s", body)
	}
	if biller.finalizeCalls != 1 || biller.usage.Estimated || biller.usage.UpstreamRequestID != "chat-sse" || biller.usage.InputTokens != 7 || biller.usage.OutputTokens != 2 {
		t.Fatalf("unexpected streamed usage: %+v", biller.usage)
	}
}

func TestProviderOutboundFormatStreamsSSEIncrementallyEndToEnd(t *testing.T) {
	firstEventSent := make(chan struct{})
	releaseUpstream := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(releaseUpstream)
		}
	}()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("upstream path = %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode upstream request: %v", err)
			return
		}
		if !boolValue(payload["stream"]) || !boolValue(mapValue(payload["stream_options"])["include_usage"]) {
			t.Errorf("upstream request did not enable streamed usage: %#v", payload)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("upstream response writer does not implement http.Flusher")
			return
		}
		_, _ = io.WriteString(w, "data: {\"id\":\"chat-e2e\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"deepseek-v3\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello incrementally\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		close(firstEventSent)
		select {
		case <-releaseUpstream:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "data: {\"id\":\"chat-e2e\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"deepseek-v3\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"id\":\"chat-e2e\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"deepseek-v3\",\"choices\":[],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":3,\"total_tokens\":11}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	route := openAIRoute()
	route.BaseURL = upstream.URL + "/v1"
	route.Provider.OutboundFormat = provider.OutboundFormatChatCompletions
	biller := &fakeBilling{}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: route}, Billing: biller, Client: upstream.Client()})
	gateway := httptest.NewServer(handler)
	defer gateway.Close()

	request, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/responses", strings.NewReader(`{"model":"deepseek-chat","input":"hello","max_output_tokens":32,"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer nvr_test")
	request.Host = "gateway.test"
	client := gateway.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("call gateway SSE endpoint: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status=%d content-type=%q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	select {
	case <-firstEventSent:
	default:
		t.Fatal("gateway returned before the upstream flushed its first SSE event")
	}

	reader := bufio.NewReader(response.Body)
	var firstFrame strings.Builder
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatalf("read first converted SSE frame: %v", readErr)
		}
		firstFrame.WriteString(line)
		if line == "\n" {
			break
		}
	}
	if !strings.Contains(firstFrame.String(), "event: response.created") {
		t.Fatalf("first converted frame = %q, want response.created", firstFrame.String())
	}
	close(releaseUpstream)
	released = true
	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read remaining converted SSE: %v", err)
	}
	fullStream := firstFrame.String() + string(rest)
	for _, want := range []string{"hello incrementally", "event: response.output_text.delta", "event: response.completed"} {
		if !strings.Contains(fullStream, want) {
			t.Fatalf("converted SSE is missing %q:\n%s", want, fullStream)
		}
	}
	if biller.finalizeCalls != 1 || biller.usage.Estimated || biller.usage.InputTokens != 8 || biller.usage.OutputTokens != 3 || biller.usage.UpstreamRequestID != "chat-e2e" {
		t.Fatalf("unexpected end-to-end streamed usage: %+v", biller.usage)
	}
}

func TestProviderOutboundFormatMessagesStreamsTextIncrementallyEndToEnd(t *testing.T) {
	firstEventSent := make(chan struct{})
	releaseUpstream := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(releaseUpstream)
		}
	}()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("upstream path = %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode upstream request: %v", err)
			return
		}
		if !boolValue(payload["stream"]) || !boolValue(mapValue(payload["stream_options"])["include_usage"]) {
			t.Errorf("upstream request did not enable streamed usage: %#v", payload)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("upstream response writer does not implement http.Flusher")
			return
		}
		_, _ = io.WriteString(w, "data: {\"id\":\"chat-messages-e2e\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"kimi-k3\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello incrementally\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		close(firstEventSent)
		select {
		case <-releaseUpstream:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "data: {\"id\":\"chat-messages-e2e\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"kimi-k3\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"id\":\"chat-messages-e2e\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"kimi-k3\",\"choices\":[],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":3,\"total_tokens\":11}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	route := openAIRoute()
	route.BaseURL = upstream.URL + "/v1"
	route.Provider.OutboundFormat = provider.OutboundFormatChatCompletions
	biller := &fakeBilling{}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: route}, Billing: biller, Client: upstream.Client()})
	gateway := httptest.NewServer(handler)
	defer gateway.Close()

	request, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/messages", strings.NewReader(`{"model":"kimi-k3","messages":[{"role":"user","content":"hello"}],"max_tokens":32,"stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-API-Key", "nvr_test")
	request.Header.Set("Anthropic-Version", "2023-06-01")
	request.Host = "gateway.test"
	client := gateway.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("call gateway Messages SSE endpoint: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status=%d content-type=%q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	select {
	case <-firstEventSent:
	default:
		t.Fatal("gateway returned before the upstream flushed its first SSE event")
	}

	reader := bufio.NewReader(response.Body)
	textFrame := make(chan string, 1)
	readFailure := make(chan error, 1)
	go func() {
		var stream strings.Builder
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				readFailure <- readErr
				return
			}
			stream.WriteString(line)
			if strings.Contains(stream.String(), `"type":"text_delta"`) {
				textFrame <- stream.String()
				return
			}
		}
	}()
	select {
	case frame := <-textFrame:
		if !strings.Contains(frame, "hello incrementally") {
			t.Fatalf("first Messages text frame = %q", frame)
		}
	case readErr := <-readFailure:
		t.Fatalf("read first Messages text frame: %v", readErr)
	case <-time.After(500 * time.Millisecond):
		close(releaseUpstream)
		released = true
		_ = response.Body.Close()
		t.Fatal("Messages text was buffered until the upstream stream finished")
	}

	close(releaseUpstream)
	released = true
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatalf("read remaining Messages stream: %v", err)
	}
	if biller.finalizeCalls != 1 || biller.usage.Estimated || biller.usage.InputTokens != 8 || biller.usage.OutputTokens != 3 || biller.usage.UpstreamRequestID != "chat-messages-e2e" {
		t.Fatalf("unexpected end-to-end streamed usage: %+v", biller.usage)
	}
}

func TestProviderOutboundFormatMessagesStreamsToolArgumentsIncrementallyEndToEnd(t *testing.T) {
	firstToolDeltaSent := make(chan struct{})
	releaseUpstream := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(releaseUpstream)
		}
	}()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("upstream path = %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode upstream request: %v", err)
			return
		}
		if !boolValue(payload["stream"]) || !boolValue(mapValue(payload["stream_options"])["include_usage"]) {
			t.Errorf("upstream request did not enable streamed usage: %#v", payload)
		}
		tools := sliceValue(payload["tools"])
		if len(tools) != 1 || stringValue(mapValue(mapValue(tools[0])["function"])["name"]) != "lookup" {
			t.Errorf("Messages function tool was not converted for Chat upstream: %#v", payload)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("upstream response writer does not implement http.Flusher")
			return
		}
		_, _ = io.WriteString(w, "data: {\"id\":\"chat-messages-tool-e2e\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"kimi-k3\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_lookup\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\\\"city\\\":\\\"\"}}]},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		close(firstToolDeltaSent)
		select {
		case <-releaseUpstream:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "data: {\"id\":\"chat-messages-tool-e2e\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"kimi-k3\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"Shanghai\\\"}\"}}]},\"finish_reason\":null}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"id\":\"chat-messages-tool-e2e\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"kimi-k3\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"id\":\"chat-messages-tool-e2e\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"kimi-k3\",\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":4,\"total_tokens\":16}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	route := openAIRoute()
	route.BaseURL = upstream.URL + "/v1"
	route.Provider.OutboundFormat = provider.OutboundFormatChatCompletions
	biller := &fakeBilling{}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: route}, Billing: biller, Client: upstream.Client()})
	gateway := httptest.NewServer(handler)
	defer gateway.Close()

	requestBody := `{"model":"kimi-k3","messages":[{"role":"user","content":"look up Shanghai"}],"max_tokens":32,"stream":true,"tools":[{"name":"lookup","description":"look up a city","input_schema":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}]}`
	request, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/messages", strings.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-API-Key", "nvr_test")
	request.Header.Set("Anthropic-Version", "2023-06-01")
	request.Host = "gateway.test"
	client := gateway.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("call gateway Messages tool SSE endpoint: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status=%d content-type=%q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	select {
	case <-firstToolDeltaSent:
	default:
		t.Fatal("gateway returned before the upstream flushed its first tool SSE event")
	}

	reader := bufio.NewReader(response.Body)
	toolFrame := make(chan string, 1)
	readFailure := make(chan error, 1)
	go func() {
		var stream strings.Builder
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				readFailure <- readErr
				return
			}
			stream.WriteString(line)
			if strings.Contains(stream.String(), `"type":"input_json_delta"`) {
				toolFrame <- stream.String()
				return
			}
		}
	}()
	select {
	case frame := <-toolFrame:
		if !strings.Contains(frame, `"partial_json":"{\"city\":\""`) {
			t.Fatalf("first Messages tool frame = %q", frame)
		}
	case readErr := <-readFailure:
		t.Fatalf("read first Messages tool frame: %v", readErr)
	case <-time.After(time.Second):
		close(releaseUpstream)
		released = true
		_ = response.Body.Close()
		t.Fatal("Messages tool arguments were buffered until the upstream stream finished")
	}

	close(releaseUpstream)
	released = true
	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read remaining Messages tool stream: %v", err)
	}
	for _, want := range []string{"Shanghai", "event: content_block_stop", "event: message_stop"} {
		if !strings.Contains(string(rest), want) {
			t.Fatalf("remaining Messages tool stream is missing %q:\n%s", want, rest)
		}
	}
	if biller.finalizeCalls != 1 || biller.usage.Estimated || biller.usage.InputTokens != 12 || biller.usage.OutputTokens != 4 || biller.usage.UpstreamRequestID != "chat-messages-tool-e2e" {
		t.Fatalf("unexpected end-to-end tool streamed usage: %+v", biller.usage)
	}
}

func TestProviderOutboundFormatRejectsMalformedFirstSSEEventBeforeCommittingHeaders(t *testing.T) {
	route := openAIRoute()
	route.Provider.OutboundFormat = provider.OutboundFormatChatCompletions
	biller := &fakeBilling{}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("data: this-is-not-json\n\n")),
		}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: route}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek-chat","input":"hello","stream":true}`))
	request.Header.Set("Authorization", "Bearer nvr_test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), `"code":"upstream_conversion_error"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("malformed first event committed SSE headers: %q", response.Header().Get("Content-Type"))
	}
	if biller.finalizeCalls != 0 || biller.unknownCalls != 1 {
		t.Fatalf("unexpected billing state finalize=%d unknown=%d", biller.finalizeCalls, biller.unknownCalls)
	}
}

func TestProviderOutboundFormatDropsOnlyUnrepresentableBuiltInTool(t *testing.T) {
	route := anthropicRoute()
	route.Provider.OutboundFormat = provider.OutboundFormatMessages
	called := false
	client := &http.Client{Transport: roundTripFunc(func(upstream *http.Request) (*http.Response, error) {
		called = true
		var payload map[string]any
		if err := json.NewDecoder(upstream.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		tools := sliceValue(payload["tools"])
		if len(tools) != 1 || stringValue(mapValue(tools[0])["name"]) != "lookup" {
			t.Fatalf("converted tools = %#v, want only portable lookup tool", tools)
		}
		return jsonHTTPResponse(http.StatusOK, `{"id":"msg-tools","type":"message","role":"assistant","model":"kimi-k3","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":1}}`), nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: route}, Billing: &fakeBilling{}, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"kimi-k3","input":"search","tools":[{"type":"web_search"},{"type":"function","name":"lookup","description":"look up data","parameters":{"type":"object"}}]}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"completed"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !called {
		t.Fatal("portable portion of mixed-tool request did not reach upstream")
	}
}

func TestProviderOutboundFormatRejectsUnknownResponsesServerState(t *testing.T) {
	route := openAIRoute()
	route.Provider.OutboundFormat = provider.OutboundFormatChatCompletions
	called := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("must not call upstream")
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: route}, Billing: &fakeBilling{}, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek-chat","input":"continue","previous_response_id":"resp_previous"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"previous_response_not_found"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if called {
		t.Fatal("stateful Responses request reached a non-Responses upstream")
	}
}

func TestProviderOutboundFormatExpandsPreviousResponseIDForChatUpstream(t *testing.T) {
	actor := gatewayActor()
	route := openAIRoute()
	route.Provider.OutboundFormat = provider.OutboundFormatChatCompletions
	call := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		call++
		var payload map[string]any
		decoder := json.NewDecoder(request.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&payload); err != nil {
			t.Fatalf("decode upstream request %d: %v", call, err)
		}
		if _, exists := payload["previous_response_id"]; exists {
			t.Fatalf("upstream request %d retained previous_response_id: %#v", call, payload)
		}
		messages := sliceValue(payload["messages"])
		switch call {
		case 1:
			if len(messages) != 1 || textFromContent(mapValue(messages[0])["content"]) != "first turn" {
				t.Fatalf("first upstream messages = %#v", messages)
			}
			return jsonHTTPResponse(http.StatusOK, `{"id":"chat-history-1","model":"deepseek-v3","choices":[{"index":0,"message":{"role":"assistant","content":"FIRST_OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`), nil
		case 2:
			if len(messages) != 3 {
				t.Fatalf("second upstream messages count = %d, want 3: %#v", len(messages), messages)
			}
			wantRoles := []string{"user", "assistant", "user"}
			wantText := []string{"first turn", "FIRST_OK", "second turn"}
			for index := range messages {
				message := mapValue(messages[index])
				if stringValue(message["role"]) != wantRoles[index] || textFromContent(message["content"]) != wantText[index] {
					t.Fatalf("second upstream message %d = %#v", index, message)
				}
			}
			return jsonHTTPResponse(http.StatusOK, `{"id":"chat-history-2","model":"deepseek-v3","choices":[{"index":0,"message":{"role":"assistant","content":"SECOND_OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11}}`), nil
		default:
			t.Fatalf("unexpected upstream request %d", call)
			return nil, nil
		}
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: actor}, Routes: fakeRoutes{route: route}, Billing: &fakeBilling{}, Client: client})

	firstRequest := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek-chat","input":"first turn"}`))
	firstResponse := httptest.NewRecorder()
	handler.ServeHTTP(firstResponse, firstRequest)
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first response status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}
	firstResult := decodeTestObject(t, firstResponse.Body.Bytes())
	if stringValue(firstResult["id"]) != "chat-history-1" || stringValue(firstResult["status"]) != "completed" {
		t.Fatalf("first Responses result = %#v", firstResult)
	}

	secondRequest := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek-chat","input":"second turn","previous_response_id":"chat-history-1"}`))
	secondResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusOK || !strings.Contains(secondResponse.Body.String(), "SECOND_OK") {
		t.Fatalf("second response status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}
	if call != 2 {
		t.Fatalf("upstream calls = %d, want 2", call)
	}
}

func TestProviderOutboundFormatExpandsPreviousResponseIDForMessagesUpstream(t *testing.T) {
	actor := gatewayActor()
	route := anthropicRoute()
	route.Provider.OutboundFormat = provider.OutboundFormatMessages
	call := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		call++
		if request.URL.Path != "/v1/messages" {
			t.Fatalf("upstream request %d path = %q, want /v1/messages", call, request.URL.Path)
		}
		var payload map[string]any
		decoder := json.NewDecoder(request.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&payload); err != nil {
			t.Fatalf("decode upstream request %d: %v", call, err)
		}
		if _, exists := payload["previous_response_id"]; exists {
			t.Fatalf("upstream request %d retained previous_response_id: %#v", call, payload)
		}
		messages := sliceValue(payload["messages"])
		switch call {
		case 1:
			if len(messages) != 1 || stringValue(mapValue(messages[0])["role"]) != "user" || textFromContent(mapValue(messages[0])["content"]) != "first turn" {
				t.Fatalf("first upstream Messages payload = %#v", messages)
			}
			return jsonHTTPResponse(http.StatusOK, `{"id":"msg-history-1","type":"message","role":"assistant","model":"kimi-k3-upstream","content":[{"type":"text","text":"FIRST_OK"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2}}`), nil
		case 2:
			if len(messages) != 3 {
				t.Fatalf("second upstream Messages count = %d, want 3: %#v", len(messages), messages)
			}
			wantRoles := []string{"user", "assistant", "user"}
			wantText := []string{"first turn", "FIRST_OK", "second turn"}
			for index := range messages {
				message := mapValue(messages[index])
				if stringValue(message["role"]) != wantRoles[index] || textFromContent(message["content"]) != wantText[index] {
					t.Fatalf("second upstream Messages item %d = %#v", index, message)
				}
			}
			return jsonHTTPResponse(http.StatusOK, `{"id":"msg-history-2","type":"message","role":"assistant","model":"kimi-k3-upstream","content":[{"type":"text","text":"SECOND_OK"}],"stop_reason":"end_turn","usage":{"input_tokens":9,"output_tokens":2}}`), nil
		default:
			t.Fatalf("unexpected upstream request %d", call)
			return nil, nil
		}
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: actor}, Routes: fakeRoutes{route: route}, Billing: &fakeBilling{}, Client: client})

	firstRequest := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"kimi-k3","input":"first turn"}`))
	firstResponse := httptest.NewRecorder()
	handler.ServeHTTP(firstResponse, firstRequest)
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first response status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}
	firstResult := decodeTestObject(t, firstResponse.Body.Bytes())
	if stringValue(firstResult["id"]) != "msg-history-1" || stringValue(firstResult["status"]) != "completed" {
		t.Fatalf("first Responses result = %#v", firstResult)
	}

	secondRequest := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"kimi-k3","input":"second turn","previous_response_id":"msg-history-1"}`))
	secondResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusOK || !strings.Contains(secondResponse.Body.String(), "SECOND_OK") {
		t.Fatalf("second response status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}
	if call != 2 {
		t.Fatalf("upstream calls = %d, want 2", call)
	}
}

func TestProviderOutboundFormatExpandsStreamedToolHistoryAndCompletes(t *testing.T) {
	actor := gatewayActor()
	route := openAIRoute()
	route.Provider.OutboundFormat = provider.OutboundFormatChatCompletions
	call := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		call++
		var payload map[string]any
		decoder := json.NewDecoder(request.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&payload); err != nil {
			t.Fatalf("decode upstream request %d: %v", call, err)
		}
		if _, exists := payload["previous_response_id"]; exists {
			t.Fatalf("upstream request %d retained previous_response_id", call)
		}
		switch call {
		case 1:
			upstreamSSE := strings.Join([]string{
				`data: {"id":"chat-tool-history","object":"chat.completion.chunk","created":1,"model":"kimi-k3","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
				``,
				`data: {"id":"chat-tool-history","object":"chat.completion.chunk","created":1,"model":"kimi-k3","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_write","type":"function","function":{"name":"bash","arguments":"{\"command\":\"write\"}"}}]},"finish_reason":null}]}`,
				``,
				`data: {"id":"chat-tool-history","object":"chat.completion.chunk","created":1,"model":"kimi-k3","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
				``,
				`data: {"id":"chat-tool-history","object":"chat.completion.chunk","created":1,"model":"kimi-k3","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`,
				``,
				`data: [DONE]`,
				``,
			}, "\n")
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(upstreamSSE))}, nil
		case 2:
			messages := sliceValue(payload["messages"])
			if len(messages) != 3 {
				t.Fatalf("tool follow-up messages count = %d, want 3: %#v", len(messages), messages)
			}
			first := mapValue(messages[0])
			assistant := mapValue(messages[1])
			toolResult := mapValue(messages[2])
			if stringValue(first["role"]) != "user" || textFromContent(first["content"]) != "write the file" {
				t.Fatalf("first history message = %#v", first)
			}
			calls := sliceValue(assistant["tool_calls"])
			if stringValue(assistant["role"]) != "assistant" || len(calls) != 1 || stringValue(mapValue(calls[0])["id"]) != "call_write" {
				t.Fatalf("assistant tool history = %#v", assistant)
			}
			if stringValue(toolResult["role"]) != "tool" || stringValue(toolResult["tool_call_id"]) != "call_write" || textFromContent(toolResult["content"]) != "123456" {
				t.Fatalf("tool result history = %#v", toolResult)
			}
			return jsonHTTPResponse(http.StatusOK, `{"id":"chat-tool-final","model":"kimi-k3","choices":[{"index":0,"message":{"role":"assistant","content":"TOOL_DONE"},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":3,"total_tokens":18}}`), nil
		default:
			t.Fatalf("unexpected upstream request %d", call)
			return nil, nil
		}
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: actor}, Routes: fakeRoutes{route: route}, Billing: &fakeBilling{}, Client: client})
	toolDefinition := `{"type":"function","name":"bash","description":"run a command","parameters":{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}}`

	firstRequestBody := `{"model":"deepseek-chat","input":"write the file","stream":true,"tools":[` + toolDefinition + `]}`
	firstRequest := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(firstRequestBody))
	firstResponse := httptest.NewRecorder()
	handler.ServeHTTP(firstResponse, firstRequest)
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first tool response status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}
	if !strings.Contains(firstResponse.Body.String(), `event: response.completed`) || strings.Contains(firstResponse.Body.String(), `event: response.incomplete`) {
		t.Fatalf("tool response did not end in exactly completed state:\n%s", firstResponse.Body.String())
	}
	if !strings.Contains(firstResponse.Body.String(), `"id":"chat-tool-history"`) || !strings.Contains(firstResponse.Body.String(), `"call_id":"call_write"`) {
		t.Fatalf("tool response lost response/call identity:\n%s", firstResponse.Body.String())
	}

	secondRequestBody := `{"model":"deepseek-chat","previous_response_id":"chat-tool-history","input":[{"type":"item_reference","id":"fc_reference"},{"type":"function_call_output","call_id":"call_write","output":"123456"}],"tools":[` + toolDefinition + `]}`
	secondRequest := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(secondRequestBody))
	secondResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusOK || !strings.Contains(secondResponse.Body.String(), "TOOL_DONE") {
		t.Fatalf("tool follow-up status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}
	if call != 2 {
		t.Fatalf("upstream calls = %d, want 2", call)
	}
}

func TestProviderOutboundFormatDoesNotFinalizeTruncatedChatStreamAfterFinishReason(t *testing.T) {
	route := openAIRoute()
	route.Provider.OutboundFormat = provider.OutboundFormatChatCompletions
	biller := &fakeBilling{}
	upstreamSSE := strings.Join([]string{
		`data: {"id":"chat-truncated","object":"chat.completion.chunk","created":1,"model":"deepseek-v3","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chat-truncated","object":"chat.completion.chunk","created":1,"model":"deepseek-v3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
	}, "\n")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/event-stream"}},
			Body: io.NopCloser(&terminalErrorReader{
				reader: strings.NewReader(upstreamSSE),
				err:    errors.New("truncated before usage and done"),
			}),
		}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: route}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek-chat","input":"hello","stream":true}`))
	request.Header.Set("Authorization", "Bearer nvr_test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "partial") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "response.completed") {
		t.Fatalf("truncated upstream was synthesized as completed:\n%s", response.Body.String())
	}
	if biller.finalizeCalls != 0 || biller.unknownCalls != 1 {
		t.Fatalf("truncated stream billing finalize=%d unknown=%d", biller.finalizeCalls, biller.unknownCalls)
	}
}

func TestBufferedResponsesUpstreamProducesResponsesSSELifecycle(t *testing.T) {
	route := openAIRoute()
	route.Provider.Code = "reasonix"
	biller := &fakeBilling{}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read upstream request: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if boolValue(payload["stream"]) {
			t.Fatalf("Reasonix upstream request remained streaming: %s", body)
		}
		response := `{"id":"resp-buffered","object":"response","created_at":1,"status":"completed","model":"deepseek-v3","output":[{"id":"msg-buffered","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"buffered responses ok","annotations":[]}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: route}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek-chat","input":"hello","stream":true}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	for _, want := range []string{"event: response.created", "event: response.output_text.delta", "buffered responses ok", "event: response.completed"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("buffered Responses SSE is missing %q:\n%s", want, response.Body.String())
		}
	}
	if biller.finalizeCalls != 1 || biller.usage.InputTokens != 5 || biller.usage.OutputTokens != 2 {
		t.Fatalf("unexpected buffered Responses usage: %+v", biller.usage)
	}
}

func TestProxyRejectsRoutesWithoutRequestedProtocol(t *testing.T) {
	tests := []struct {
		name, path, body string
		route            modelroute.Resolved
	}{
		{name: "Anthropic request with OpenAI route", path: "/v1/messages", body: `{"model":"deepseek-chat","messages":[],"max_tokens":20}`, route: openAIRoute()},
		{name: "OpenAI request with Anthropic route", path: "/v1/responses", body: `{"model":"kimi-k3","input":"hello","max_output_tokens":20}`, route: anthropicRoute()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: tt.route}, Billing: &fakeBilling{}, Client: &http.Client{}})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body)))
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "unsupported_endpoint") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

/**
 * TestProxyRejectsInsufficientBalanceBeforeUpstream 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestStreamingUsageIsCaptured 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestStreamingFinalizationFailureKeepsSuccessfulResponse 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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
	if response.Code != http.StatusOK || response.Body.String() != stream || biller.finalizeCalls != 1 || biller.refundCalls != 0 || biller.pendingCalls != 1 || biller.completeCalls != 0 {
		t.Fatalf("status=%d finalize_calls=%d refund_calls=%d body=%s", response.Code, biller.finalizeCalls, biller.refundCalls, response.Body.String())
	}
	if !strings.Contains(logs.String(), "stream usage finalization unavailable") {
		t.Fatalf("stream finalization failure was not logged: %s", logs.String())
	}
}

/**
 * TestStreamingSettlementIntentFailureKeepsReservationForReconciliation 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestStreamingSettlementIntentFailureKeepsReservationForReconciliation(t *testing.T) {
	biller := &fakeBilling{pendingErrors: []error{billing.ErrRequestConflict}}
	stream := "data: {\"id\":\"stream-1\",\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":9}}\n\ndata: [DONE]\n\n"
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
	})}
	var logs bytes.Buffer
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":20,"stream":true}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != stream || calls != 1 || biller.pendingCalls != 1 || biller.finalizeCalls != 0 || biller.completeCalls != 0 || biller.unknownCalls != 1 || biller.refundCalls != 0 {
		t.Fatalf("status=%d calls=%d billing=%+v body=%s", response.Code, calls, biller, response.Body.String())
	}
	if biller.operation.Status != billing.OperationPendingUnknown {
		t.Fatalf("operation status=%s want=%s", biller.operation.Status, billing.OperationPendingUnknown)
	}
	if !strings.Contains(logs.String(), "stream usage finalization unavailable") {
		t.Fatalf("stream settlement intent failure was not logged: %s", logs.String())
	}
}

/**
 * TestStreamingLargeDataLineIsRelayedAndBilled 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestInvalidStreamedUsageKeepsReservationForReconciliation 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestInvalidStreamedUsageKeepsReservationForReconciliation(t *testing.T) {
	biller := &fakeBilling{}
	stream := "data: {\"id\":\"stream-invalid\",\"usage\":{\"prompt_tokens\":2147483648,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":20,"stream":true}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != stream || biller.finalizeCalls != 0 || biller.refundCalls != 0 || biller.unknownCalls != 1 {
		t.Fatalf("status=%d finalize_calls=%d refund_calls=%d refunded=%d reserved=%d", response.Code, biller.finalizeCalls, biller.refundCalls, biller.refunded, biller.reserved)
	}
}

/**
 * TestStreamingUsageAcrossResponsesAndMessages 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestResponsesIncompleteIsBillableTerminalState 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestResponsesIncompleteIsBillableTerminalState(t *testing.T) {
	biller := &fakeBilling{}
	stream := "event: response.incomplete\n" +
		"data: {\"type\":\"response.incomplete\",\"response\":{\"id\":\"resp-incomplete\",\"status\":\"incomplete\",\"usage\":{\"input_tokens\":100,\"input_tokens_details\":{\"cached_tokens\":40},\"output_tokens\":9}}}\n\n"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek-chat","input":"hello","max_output_tokens":20,"stream":true}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || biller.finalizeCalls != 1 || biller.refundCalls != 0 {
		t.Fatalf("status=%d billing=%+v body=%s", response.Code, biller, response.Body.String())
	}
	if biller.usage.Estimated || biller.usage.InputTokens != 100 || biller.usage.Tokens.UncachedInput != 60 || biller.usage.Tokens.CacheRead != 40 || biller.usage.OutputTokens != 9 {
		t.Fatalf("unexpected incomplete response usage: %+v", biller.usage)
	}
}

/**
 * TestFailedStreamingTerminalStateReleasesReservation 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestFailedStreamingTerminalStateReleasesReservation(t *testing.T) {
	for _, endpoint := range []struct {
		name, path, body, stream string
		route                    modelroute.Resolved
	}{
		{name: "responses", path: "/v1/responses", body: `{"model":"deepseek-chat","input":"hello","max_output_tokens":20,"stream":true}`, stream: "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\"}}\n\n", route: openAIRoute()},
		{name: "messages", path: "/v1/messages", body: `{"model":"kimi-k3","messages":[],"max_tokens":20,"stream":true}`, stream: "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\"}}\n\n", route: anthropicRoute()},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			biller := &fakeBilling{}
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(endpoint.stream))}, nil
			})}
			handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: endpoint.route}, Billing: biller, Client: client})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(endpoint.body)))
			if response.Code != http.StatusOK || biller.finalizeCalls != 0 || biller.refundCalls != 1 || biller.refunded != biller.reserved || len(biller.failures) != 1 {
				t.Fatalf("status=%d billing=%+v body=%s", response.Code, biller, response.Body.String())
			}
			if biller.failures[0].StatusCode != http.StatusBadGateway || biller.failures[0].ErrorCode != "upstream_stream_failed" {
				t.Fatalf("failure=%+v", biller.failures[0])
			}
		})
	}
}

/**
 * TestFailedStreamingTerminalStateCannotBeOverriddenByDone 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestFailedStreamingTerminalStateCannotBeOverriddenByDone(t *testing.T) {
	biller := &fakeBilling{}
	stream := "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\"}}\n\ndata: [DONE]\n\n"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
	})}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek-chat","input":"hello","stream":true}`)))
	if response.Code != http.StatusOK || biller.finalizeCalls != 0 || biller.failCalls != 1 || biller.refundCalls != 1 {
		t.Fatalf("status=%d billing=%+v body=%s", response.Code, biller, response.Body.String())
	}
}

/**
 * TestInterruptedStreamingUsageKeepsReservationForReconciliation 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestInterruptedStreamingUsageKeepsReservationForReconciliation(t *testing.T) {
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
	if biller.finalizeCalls != 0 || biller.pendingCalls != 0 || biller.unknownCalls != 1 || biller.refundCalls != 0 {
		t.Fatalf("interrupted stream was automatically settled: %+v", biller)
	}
}

/**
 * TestStreamingReadErrorIsLoggedWithoutChangingCompletedUsage 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestModelListRequiresAPIKey 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestModelListRequiresAPIKey(t *testing.T) {
	handler := New(Dependencies{APIKeys: fakeKeys{err: apikey.ErrUnauthenticated}, Routes: fakeRoutes{}, Billing: &fakeBilling{}, Client: &http.Client{}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", response.Code)
	}
	assertErrorRequestID(t, response)
}

/**
 * TestModelListDeduplicatesFailoverRoutes 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestSingularModelListAliasUsesSelectedRoutes 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestGatewayErrorsIncludeRequestID 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * assertErrorRequestID 封装该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @param response 当前响应数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestBuildUpstreamURLAndPrivateAddressGuard 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestDefaultOutboundClientDoesNotFollowRedirects 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestUpstreamFailureRefundsReservation 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestUpstreamFailureRefundsReservation(t *testing.T) {
	biller := &fakeBilling{}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("timeout") })}
	handler := New(Dependencies{APIKeys: fakeKeys{actor: gatewayActor()}, Routes: fakeRoutes{route: openAIRoute()}, Billing: biller, Client: client})
	handler.settlementRetryDelays = []time.Duration{0, 0}
	handler.now = func() time.Time { return time.Unix(1, 0) }
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[],"max_tokens":20}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || biller.failCalls != 1 || biller.refundCalls != 1 || biller.refunded != biller.reserved {
		t.Fatalf("status=%d refund_calls=%d reserved=%d refunded=%d", response.Code, biller.refundCalls, biller.reserved, biller.refunded)
	}
}

/**
 * TestParseUsageSupportsProviderCacheShapes 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestParseUsageSupportsProviderCacheShapes(t *testing.T) {
	tests := []struct {
		name, body string
		semantics  usageSemantics
		want       tokenUsage
	}{
		{name: "glm cached details", body: `{"usage":{"prompt_tokens":2000,"completion_tokens":500,"prompt_tokens_details":{"cached_tokens":1200}}}`, semantics: usageSemanticsOpenAITotal, want: tokenUsage{Input: 2000, UncachedInput: 800, CacheRead: 1200, Output: 500}},
		{name: "deepseek hit and miss", body: `{"usage":{"prompt_tokens":2000,"completion_tokens":500,"prompt_cache_hit_tokens":1200,"prompt_cache_miss_tokens":800}}`, semantics: usageSemanticsOpenAITotal, want: tokenUsage{Input: 2000, UncachedInput: 800, CacheRead: 1200, Output: 500}},
		{name: "kimi top level cached", body: `{"usage":{"prompt_tokens":100,"completion_tokens":20,"cached_tokens":10}}`, semantics: usageSemanticsOpenAITotal, want: tokenUsage{Input: 100, UncachedInput: 90, CacheRead: 10, Output: 20}},
		{name: "openai cache read field is prompt subset", body: `{"usage":{"prompt_tokens":100,"completion_tokens":20,"cache_read_input_tokens":10}}`, semantics: usageSemanticsOpenAITotal, want: tokenUsage{Input: 100, UncachedInput: 90, CacheRead: 10, Output: 20}},
		{name: "responses cached details are input subset", body: `{"usage":{"input_tokens":100,"output_tokens":20,"input_tokens_details":{"cached_tokens":40}}}`, semantics: usageSemanticsOpenAITotal, want: tokenUsage{Input: 100, UncachedInput: 60, CacheRead: 40, Output: 20}},
		{name: "bailian explicit cache creation is prompt subset", body: `{"usage":{"prompt_tokens":2000,"completion_tokens":500,"prompt_tokens_details":{"cached_tokens":1000,"cache_creation_input_tokens":500}}}`, semantics: usageSemanticsOpenAITotal, want: tokenUsage{Input: 2000, UncachedInput: 500, CacheRead: 1000, CacheWrite: 500, Output: 500}},
		{name: "anthropic cache read is additional input", body: `{"usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":10}}`, semantics: usageSemanticsAnthropicAdditional, want: tokenUsage{Input: 110, UncachedInput: 100, CacheRead: 10, Output: 20}},
		{name: "anthropic cache creation", body: `{"usage":{"input_tokens":800,"output_tokens":500,"cache_read_input_tokens":1200,"cache_creation_input_tokens":300,"cache_creation":{"ephemeral_5m_input_tokens":200,"ephemeral_1h_input_tokens":100}}}`, semantics: usageSemanticsAnthropicAdditional, want: tokenUsage{Input: 2300, UncachedInput: 800, CacheRead: 1200, CacheWrite: 200, CacheWrite1h: 100, Output: 500}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseUsage([]byte(tt.body), tt.semantics)
			if got.Input != tt.want.Input || got.UncachedInput != tt.want.UncachedInput || got.CacheRead != tt.want.CacheRead || got.CacheWrite != tt.want.CacheWrite || got.CacheWrite1h != tt.want.CacheWrite1h || got.Output != tt.want.Output {
				t.Fatalf("got=%+v want=%+v", got, tt.want)
			}
		})
	}
}

/**
 * TestUsageFallbackNeverChargesUnreportedDimensions 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestUsageFallbackNeverChargesUnreportedDimensions(t *testing.T) {
	rates := billing.RateCard{InputMicros: 1, CacheReadMicros: 2, CacheWriteMicros: 3, CacheWrite1hMicros: 4, OutputMicros: 5}

	usage := applyUsageFallback(parseUsage([]byte(`{"usage":{"prompt_tokens":10,"completion_tokens":0}}`), usageSemanticsOpenAITotal), 100, 20, rates)
	if usage.Input != 10 || usage.UncachedInput != 10 || usage.Output != 0 || usage.Estimated {
		t.Fatalf("reported zero output must remain exact: %+v", usage)
	}

	usage = applyUsageFallback(parseUsage([]byte(`{"usage":{"completion_tokens":7}}`), usageSemanticsOpenAITotal), 100, 20, rates)
	if usage.Input != 0 || usage.UncachedInput != 0 || usage.CacheRead != 0 || usage.CacheWrite != 0 || usage.CacheWrite1h != 0 || usage.Output != 7 || !usage.Estimated {
		t.Fatalf("missing input was charged: %+v", usage)
	}

	usage = applyUsageFallback(parseUsage([]byte(`{"usage":{"prompt_tokens":10}}`), usageSemanticsOpenAITotal), 100, 20, rates)
	if usage.Input != 10 || usage.UncachedInput != 10 || usage.Output != 0 || !usage.Estimated {
		t.Fatalf("missing output was charged: %+v", usage)
	}
}

/**
 * TestUsageRejectsConflictingAliasTotals 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestUsageRejectsConflictingAliasTotals(t *testing.T) {
	tests := []struct {
		name string
		body string
		want tokenUsage
	}{
		{
			name: "conflicting input aliases",
			body: `{"usage":{"prompt_tokens":100,"input_tokens":100000,"completion_tokens":2,"output_tokens":2}}`,
			want: tokenUsage{Output: 2, OutputReported: true, Estimated: true},
		},
		{
			name: "conflicting output aliases",
			body: `{"usage":{"prompt_tokens":100,"input_tokens":100,"completion_tokens":2,"output_tokens":100000}}`,
			want: tokenUsage{Input: 100, UncachedInput: 100, InputReported: true, Estimated: true},
		},
		{
			name: "conflicting output aliases reverse order",
			body: `{"usage":{"prompt_tokens":100,"input_tokens":100,"completion_tokens":100000,"output_tokens":2}}`,
			want: tokenUsage{Input: 100, UncachedInput: 100, InputReported: true, Estimated: true},
		},
		{
			name: "conflicting cache creation aliases",
			body: `{"usage":{"prompt_tokens":100,"completion_tokens":2,"cache_creation_input_tokens":20,"prompt_tokens_details":{"cache_creation_input_tokens":30}}}`,
			want: tokenUsage{Output: 2, OutputReported: true, Estimated: true},
		},
		{
			name: "matching aliases",
			body: `{"usage":{"prompt_tokens":100,"input_tokens":100,"completion_tokens":2,"output_tokens":2}}`,
			want: tokenUsage{Input: 100, UncachedInput: 100, Output: 2, InputReported: true, OutputReported: true},
		},
		{
			name: "zero compatibility aliases and null optional details",
			body: `{"usage":{"prompt_tokens":100,"input_tokens":0,"completion_tokens":2,"output_tokens":0,"prompt_tokens_details":{},"input_tokens_details":null}}`,
			want: tokenUsage{Input: 100, UncachedInput: 100, Output: 2, InputReported: true, OutputReported: true},
		},
		{
			name: "zero canonical aliases",
			body: `{"usage":{"prompt_tokens":0,"input_tokens":100,"completion_tokens":0,"output_tokens":2}}`,
			want: tokenUsage{Input: 100, UncachedInput: 100, Output: 2, InputReported: true, OutputReported: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := applyUsageFallback(parseUsage([]byte(tt.body), usageSemanticsOpenAITotal), 500, 50, billing.RateCard{})
			if usage.Input != tt.want.Input || usage.UncachedInput != tt.want.UncachedInput || usage.Output != tt.want.Output || usage.InputReported != tt.want.InputReported || usage.OutputReported != tt.want.OutputReported || usage.Estimated != tt.want.Estimated {
				t.Fatalf("usage=%+v want=%+v", usage, tt.want)
			}
		})
	}
}

/**
 * TestParseUsageSupportsNewAPIBillingUsage 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestParseUsageSupportsNewAPIBillingUsage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want tokenUsage
	}{
		{
			name: "openai chat billing usage",
			body: `{"usage":{"completion_tokens":1941,"billing_usage":{"source":"oai_chat","semantic":"openai","openai_usage":{"prompt_tokens":71207,"completion_tokens":1941,"input_tokens":0,"output_tokens":0,"input_tokens_details":null}}}}`,
			want: tokenUsage{Input: 71207, UncachedInput: 71207, Output: 1941, InputReported: true, OutputReported: true},
		},
		{
			name: "openai responses billing usage",
			body: `{"usage":{"billing_usage":{"source":"oai_responses","semantic":"openai","openai_usage":{"prompt_tokens":0,"completion_tokens":0,"input_tokens":120,"output_tokens":8,"prompt_tokens_details":null}}}}`,
			want: tokenUsage{Input: 120, UncachedInput: 120, Output: 8, InputReported: true, OutputReported: true},
		},
		{
			name: "anthropic billing usage",
			body: `{"usage":{"billing_usage":{"source":"claude_messages","semantic":"anthropic","claude_usage":{"input_tokens":100,"cache_read_input_tokens":20,"cache_creation_input_tokens":30,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":20},"output_tokens":7}}}}`,
			want: tokenUsage{Input: 150, UncachedInput: 100, CacheRead: 20, CacheWrite: 10, CacheWrite1h: 20, Output: 7, InputReported: true, OutputReported: true},
		},
		{
			name: "gemini billing usage",
			body: `{"usage":{"billing_usage":{"source":"gemini_chat","semantic":"gemini","gemini_usage_metadata":{"promptTokenCount":90,"toolUsePromptTokenCount":10,"candidatesTokenCount":7,"thoughtsTokenCount":3,"cachedContentTokenCount":40,"totalTokenCount":110}}}}`,
			want: tokenUsage{Input: 100, UncachedInput: 60, CacheRead: 40, Output: 10, InputReported: true, OutputReported: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := applyUsageFallback(parseUsage([]byte(tt.body), usageSemanticsOpenAITotal), 500, 50, billing.RateCard{})
			if usage.Input != tt.want.Input || usage.UncachedInput != tt.want.UncachedInput || usage.CacheRead != tt.want.CacheRead || usage.CacheWrite != tt.want.CacheWrite || usage.CacheWrite1h != tt.want.CacheWrite1h || usage.Output != tt.want.Output || usage.InputReported != tt.want.InputReported || usage.OutputReported != tt.want.OutputReported || usage.Estimated {
				t.Fatalf("usage=%+v want=%+v", usage, tt.want)
			}
		})
	}
}

/**
 * TestParseUsageIgnoresUnrecognizedBillingUsage 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestParseUsageIgnoresUnrecognizedBillingUsage(t *testing.T) {
	body := `{"usage":{"prompt_tokens":10,"completion_tokens":2,"billing_usage":{"source":"unknown","semantic":"openai","openai_usage":{"prompt_tokens":100000,"completion_tokens":20000}}}}`
	usage := applyUsageFallback(parseUsage([]byte(body), usageSemanticsOpenAITotal), 500, 50, billing.RateCard{})
	if usage.Input != 10 || usage.UncachedInput != 10 || usage.Output != 2 || usage.Estimated {
		t.Fatalf("unrecognized billing usage replaced standard usage: %+v", usage)
	}
}

/**
 * TestParseUsagePreservesNewAPIEstimatedFlag 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestParseUsagePreservesNewAPIEstimatedFlag(t *testing.T) {
	body := `{"usage":{"billing_usage":{"source":"oai_chat","semantic":"openai","estimated":true,"openai_usage":{"prompt_tokens":10,"completion_tokens":2}}}}`
	usage := applyUsageFallback(parseUsage([]byte(body), usageSemanticsOpenAITotal), 500, 50, billing.RateCard{})
	if usage.Input != 10 || usage.Output != 2 || !usage.Estimated {
		t.Fatalf("estimated billing usage was treated as exact: %+v", usage)
	}
}

/**
 * TestOpenAIUsageRejectsCacheBreakdownAboveTotal 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestOpenAIUsageRejectsCacheBreakdownAboveTotal(t *testing.T) {
	for _, body := range []string{
		`{"usage":{"input_tokens":100,"output_tokens":2,"input_tokens_details":{"cached_tokens":101}}}`,
		`{"usage":{"prompt_tokens":100,"completion_tokens":2,"prompt_cache_hit_tokens":80,"prompt_cache_miss_tokens":30}}`,
		`{"usage":{"prompt_tokens":100,"completion_tokens":2,"prompt_tokens_details":{"cache_creation_input_tokens":101}}}`,
	} {
		usage := applyUsageFallback(parseUsage([]byte(body), usageSemanticsOpenAITotal), 500, 50, billing.RateCard{})
		if usage.Input != 0 || usage.UncachedInput != 0 || usage.CacheRead != 0 || usage.InputReported || !usage.Estimated {
			t.Fatalf("invalid cache breakdown increased billing usage: %+v", usage)
		}
		if usage.Output != 2 || !usage.OutputReported {
			t.Fatalf("valid output dimension was discarded: %+v", usage)
		}
	}
}

/**
 * TestAnthropicUsageRejectsInconsistentCacheCreationBreakdown 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestAnthropicUsageRejectsInconsistentCacheCreationBreakdown(t *testing.T) {
	body := `{"usage":{"input_tokens":10,"output_tokens":2,"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":80,"ephemeral_1h_input_tokens":30}}}`
	usage := applyUsageFallback(parseUsage([]byte(body), usageSemanticsAnthropicAdditional), 500, 50, billing.RateCard{})
	if usage.Input != 0 || usage.UncachedInput != 0 || usage.CacheWrite != 0 || usage.CacheWrite1h != 0 || usage.InputReported || !usage.Estimated || usage.Output != 2 {
		t.Fatalf("inconsistent Anthropic cache breakdown was charged: %+v", usage)
	}
}

/**
 * TestUsageMergeReplacesInputSnapshotAtomically 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestUsageMergeReplacesInputSnapshotAtomically(t *testing.T) {
	usage := tokenUsage{}
	usage.merge(parseUsage([]byte(`{"usage":{"prompt_tokens":100,"completion_tokens":1}}`), usageSemanticsOpenAITotal))
	usage.merge(parseUsage([]byte(`{"usage":{"prompt_tokens":100,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":80}}}`), usageSemanticsOpenAITotal))
	if usage.Input != 100 || usage.UncachedInput != 20 || usage.CacheRead != 80 || usage.Output != 2 {
		t.Fatalf("usage snapshots were combined across categories: %+v", usage)
	}
	if usage.breakdown().InputTotal() != usage.Input {
		t.Fatalf("input total=%d breakdown=%+v", usage.Input, usage.breakdown())
	}
}

/**
 * TestEstimateInputTokensDoesNotTreatEveryByteAsToken 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestEstimateInputTokensDoesNotTreatEveryByteAsToken(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 200_000)
	got := estimateInputTokens(body)
	want := 50_064
	if got != want || got >= len(body) {
		t.Fatalf("estimate=%d want=%d bytes=%d", got, want, len(body))
	}
}
