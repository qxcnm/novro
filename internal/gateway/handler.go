package gateway

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
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
	"github.com/novro-gateway/novro/internal/upstreamhttp"
	"github.com/novro-gateway/novro/internal/upstreammodel"
)

const (
	maxGatewayBodyBytes          int64 = 0
	maxUpstreamBodyBytes         int64 = 0
	defaultMaxOutputTokens             = 4096
	streamSettlementDrainTimeout       = 30 * time.Second
)

var errSettlementIntentNotPersisted = errors.New("billing settlement intent was not persisted")

type KeyAuthenticator interface {

	/**
	 * Authenticate 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Authenticate(context.Context, string) (apikey.Actor, error)
}
type RouteService interface {

	/**
	 * ResolveCandidates 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 string 的接口输入参数。
	 * @param arg3 类型为 uuid.UUID 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	ResolveCandidates(context.Context, string, uuid.UUID) ([]modelroute.Resolved, error)

	/**
	 * ListActive 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	ListActive(context.Context, uuid.UUID) ([]modelroute.Record, error)
}
type BillingService interface {

	/**
	 * Finalize 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 billing.UsageInput 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Finalize(context.Context, billing.UsageInput) error

	/**
	 * RecordFailure 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 billing.FailureInput 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	RecordFailure(context.Context, billing.FailureInput) error

	/**
	 * StartOperation 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 billing.OperationStartInput 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	StartOperation(context.Context, billing.OperationStartInput) (billing.OperationStartResult, error)

	/**
	 * MarkOperationPendingSettlement 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 billing.UsageInput 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	MarkOperationPendingSettlement(context.Context, uuid.UUID, billing.UsageInput) error

	/**
	 * MarkOperationPendingUnknown 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	MarkOperationPendingUnknown(context.Context, uuid.UUID, string) error

	/**
	 * CompleteOperation 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	CompleteOperation(context.Context, uuid.UUID) error

	/**
	 * FailOperation 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 uuid.UUID 的接口输入参数。
	 * @param arg3 类型为 string 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	FailOperation(context.Context, uuid.UUID, string) error
}
type SettingsService interface {
	/**
	 * Config 声明该接口方法需要提供的业务能力。
	 * @param none 无参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	Config(context.Context) (gatewaysettings.Config, error)
}

/**
 * PriceResolver 定义网关在请求开始时读取模型价格的能力。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
type PriceResolver interface {
	Resolve(context.Context, uuid.UUID, time.Time) (modelpricing.Resolution, error)
}

type MultiplierResolver interface {
	MultiplierAt(billinggroup.Summary, time.Time) int64
}

type Dependencies struct {
	APIKeys   KeyAuthenticator
	Routes    RouteService
	Billing   BillingService
	Settings  SettingsService
	Pricing   PriceResolver
	Discounts MultiplierResolver
	Client    *http.Client
	Logger    *slog.Logger
}

type Handler struct {
	apiKeys               KeyAuthenticator
	routes                RouteService
	billing               BillingService
	settings              SettingsService
	pricing               PriceResolver
	discounts             MultiplierResolver
	client                *http.Client
	logger                *slog.Logger
	now                   func() time.Time
	settlementRetryDelays []time.Duration
	upstreamRetryDelays   []time.Duration
}

/**
 * New 执行该名称对应的业务处理逻辑。
 * @param deps 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func New(deps Dependencies) *Handler {
	client := deps.Client
	if client == nil {
		client = newOutboundClient()
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		apiKeys: deps.APIKeys, routes: deps.Routes, billing: deps.Billing, settings: deps.Settings, pricing: deps.Pricing, discounts: deps.Discounts, client: client, logger: logger,
		now: func() time.Time { return time.Now().UTC() }, settlementRetryDelays: []time.Duration{100 * time.Millisecond, 300 * time.Millisecond},
		upstreamRetryDelays: []time.Duration{250 * time.Millisecond, 500 * time.Millisecond},
	}
}

func (h *Handler) multiplierAt(actor apikey.Actor, at time.Time) int64 {
	if actor.PinnedMultiplierBPS > 0 {
		return actor.PinnedMultiplierBPS
	}
	if h.discounts != nil {
		return h.discounts.MultiplierAt(actor.APIKey.BillingGroup, at)
	}
	return actor.APIKey.BillingGroup.MultiplierAt(at)
}

/**
 * ServeHTTP 执行该名称对应的业务处理逻辑。
 * @param w 本次操作需要使用的输入参数。
 * @param r 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, id := requestid.Ensure(r.Context())
	r = r.WithContext(ctx)
	w.Header().Set(requestid.Header, id.String())
	actor, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet && (r.URL.Path == "/v1/models" || r.URL.Path == "/v1/model") {
		h.listModels(w, r, actor)
		return
	}
	if r.Method == http.MethodPost {
		switch r.URL.Path {
		case "/v1/chat/completions":
			h.proxy(w, r, actor, "chat_completions")
			return
		case "/v1/responses":
			h.proxy(w, r, actor, "responses")
			return
		case "/v1/messages":
			h.proxy(w, r, actor, "messages")
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "API 路径不存在")
}

/**
 * authenticate 执行该名称对应的业务处理逻辑。
 * @param w 本次操作需要使用的输入参数。
 * @param r 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (apikey.Actor, bool) {
	token := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if authorization := strings.TrimSpace(r.Header.Get("Authorization")); authorization != "" {
		parts := strings.SplitN(authorization, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token = strings.TrimSpace(parts[1])
		}
	}
	actor, err := h.apiKeys.Authenticate(r.Context(), token)
	if err != nil {
		if errors.Is(err, apikey.ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, "invalid_api_key", "API Key 无效或已撤销")
			return apikey.Actor{}, false
		}
		h.logger.Error("authenticate gateway request", "request_id", requestid.FromContext(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return apikey.Actor{}, false
	}
	return actor, true
}

/**
 * listModels 执行该名称对应的业务处理逻辑。
 * @param w 本次操作需要使用的输入参数。
 * @param r 本次操作需要使用的输入参数。
 * @param actor 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) listModels(w http.ResponseWriter, r *http.Request, actor apikey.Actor) {
	routes, err := h.routes.ListActive(r.Context(), actor.APIKey.BillingGroupID)
	if err != nil {
		h.logger.Error("list gateway models", "request_id", requestid.FromContext(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
	}
	data := make([]map[string]any, 0, len(routes))
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		if _, exists := seen[route.PublicName]; exists {
			continue
		}
		seen[route.PublicName] = struct{}{}
		ownedBy := route.Provider.Code
		if route.UpstreamModel != nil && strings.TrimSpace(route.UpstreamModel.ProviderName) != "" {
			ownedBy = route.UpstreamModel.ProviderName
		}
		data = append(data, map[string]any{"id": route.PublicName, "object": "model", "created": route.CreatedAt.Unix(), "owned_by": ownedBy})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

/**
 * proxy 执行统一模型网关的完整调用流程：解析候选路由、固定每条路由的价格、按最坏候选预占余额，
 * 再依权重顺序调用上游，并仅用实际命中渠道的确认 usage 结算。预占和结算由同一个 requestID 关联。
 * @param w 本次操作需要使用的输入参数。
 * @param r 本次操作需要使用的输入参数。
 * @param actor 本次操作需要使用的输入参数。
 * @param endpoint 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func (h *Handler) proxy(w http.ResponseWriter, r *http.Request, actor apikey.Actor, endpoint string) {
	startedAt := h.now()
	body, err := readLimited(r.Body, maxGatewayBodyBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求体读取失败")
		return
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求体必须是 JSON 对象")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求体必须是 JSON 对象")
		return
	}
	model, ok := payload["model"].(string)
	if !ok || strings.TrimSpace(model) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "model 不能为空")
		return
	}
	publicModel := strings.TrimSpace(model)
	routes, err := h.routes.ResolveCandidates(r.Context(), publicModel, actor.APIKey.BillingGroupID)
	if err != nil {
		if errors.Is(err, modelroute.ErrNotFound) {
			writeError(w, http.StatusNotFound, "model_not_found", "模型不存在或当前不可用")
			return
		}
		h.logger.Error("resolve gateway model", "request_id", requestid.FromContext(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
	}
	if actor.APIKey.BillingGroupID == uuid.Nil || actor.APIKey.BillingGroup.ID == uuid.Nil {
		h.logger.Error("resolve gateway billing context", "request_id", requestid.FromContext(r.Context()), "model", publicModel)
		writeError(w, http.StatusInternalServerError, "billing_configuration_error", "模型计费配置暂时不可用")
		return
	}
	// Pin the shared discount snapshot once per request so an administrator
	// update during a long upstream call cannot change the final multiplier
	// after the reservation was calculated.
	actor.PinnedMultiplierBPS = h.multiplierAt(actor, startedAt)
	stream, _ := payload["stream"].(bool)
	requestSettings := gatewaysettings.DefaultConfig()
	if h.settings != nil {
		requestSettings, err = h.settings.Config(r.Context())
		if err != nil {
			h.logger.Error("read gateway request settings", "request_id", requestid.FromContext(r.Context()), "error", err)
			writeError(w, http.StatusInternalServerError, "gateway_settings_unavailable", "请求设置暂时不可用")
			return
		}
	}
	maximum, ok := readMaxOutput(payload, endpoint)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "最大输出 token 参数无效")
		return
	}
	compatible := make([]modelroute.Resolved, 0, len(routes))
	invalidBillingRoute := false
	for _, route := range routes {
		if !protocolSupports(route.Provider.Protocol, endpoint) {
			continue
		}
		if route.UpstreamModel == nil || route.UpstreamModelID == nil {
			invalidBillingRoute = true
			h.logger.Error("skip gateway route without billing context", "request_id", requestid.FromContext(r.Context()), "model", publicModel, "route_id", route.ID, "provider", route.Provider.Code)
			continue
		}
		compatible = append(compatible, route)
	}
	if len(compatible) == 0 {
		if invalidBillingRoute {
			writeError(w, http.StatusInternalServerError, "billing_configuration_error", "模型计费配置暂时不可用")
		} else {
			writeError(w, http.StatusBadRequest, "unsupported_endpoint", "该模型不支持当前 API 协议")
		}
		return
	}
	sort.SliceStable(compatible, func(i, j int) bool {
		return compatible[i].Provider.Weight > compatible[j].Provider.Weight
	})
	type upstreamAttempt struct {
		route         modelroute.Resolved
		inputEstimate int
	}
	attempts := make([]upstreamAttempt, 0, len(compatible))
	reserved := int64(0)
	reservationInputCap := requestSettings.ReservationInputTokenCap
	if reservationInputCap <= 0 {
		reservationInputCap = gatewaysettings.DefaultReservationInputTokenCap
	}
	reservationOutputCap := requestSettings.ReservationOutputTokenCap
	if reservationOutputCap <= 0 {
		reservationOutputCap = gatewaysettings.DefaultReservationOutputTokenCap
	}
	// 先按请求开始时间解析并固定每条候选路由的费率；后续预扣、重试和结算
	// 都只读取 route.ResolvedPrices，避免请求执行期间跨窗口或发布新版本造成价格不一致。
	for _, route := range compatible {
		if h.pricing != nil {
			resolution, err := h.pricing.Resolve(r.Context(), *route.UpstreamModelID, startedAt)
			if err != nil {
				h.logger.Error("resolve gateway model pricing", "request_id", requestid.FromContext(r.Context()), "route_id", route.ID, "error", err)
				continue
			}
			prices := upstreammodel.Prices{
				InputMicros: resolution.Rates.InputMicros, OutputMicros: resolution.Rates.OutputMicros,
				CacheReadMicros: resolution.Rates.CacheReadMicros, CacheWriteMicros: resolution.Rates.CacheWriteMicros,
				CacheWrite1hMicros: resolution.Rates.CacheWrite1hMicros, RequestMicros: resolution.Rates.RequestMicros,
			}
			route.ResolvedPrices = &prices
			route.PricingPlanID = resolution.PlanID
			route.PricingWindowID = resolution.WindowID
			route.PricingWindowLabel = resolution.WindowLabel
		}
		upstreamBody, err := buildUpstreamBody(payload, route, endpoint, stream)
		if err != nil {
			h.logger.Error("build gateway upstream payload", "request_id", requestid.FromContext(r.Context()), "route_id", route.ID, "error", err)
			continue
		}
		inputEstimate := estimateInputTokens(upstreamBody)
		// 输入和输出分别被上限截断，防止超大请求或 max_tokens 使一次预占无限增长。
		reservation, err := billing.EstimateReservation(min(inputEstimate, reservationInputCap), min(maximum, reservationOutputCap), rateCardFor(route), h.multiplierAt(actor, startedAt))
		if err != nil {
			h.logger.Error("estimate gateway reservation", "request_id", requestid.FromContext(r.Context()), "route_id", route.ID, "error", err)
			continue
		}
		attempts = append(attempts, upstreamAttempt{route: route, inputEstimate: inputEstimate})
		// 可能因故障切换到任意候选路由，故冻结所有候选估算中的最大值；
		// 成功后仍按实际命中路由的已确认 usage 退差额或补扣，不会按这个最大值收费。
		reserved = max(reserved, reservation.CostMicros)
	}
	if len(attempts) == 0 {
		writeError(w, http.StatusInternalServerError, "billing_configuration_error", "模型计费配置暂时不可用")
		return
	}
	requestID := requestid.FromContext(r.Context())
	if requestID == uuid.Nil {
		requestID = requestid.New()
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) > 255 {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key 不能超过 255 个字符")
		return
	}
	if idempotencyKey == "" {
		// 未提供显式幂等键时，requestID 仍会让当前请求的内部重试共享同一笔 operation。
		idempotencyKey = requestID.String()
	}
	operation, err := h.billing.StartOperation(r.Context(), billing.OperationStartInput{
		RequestID: requestID, UserID: actor.User.ID, APIKeyID: actor.APIKey.ID,
		IdempotencyKeyHash: sha256String(idempotencyKey), RequestHash: gatewayRequestHash(endpoint, body), Endpoint: endpoint, ReservedMicros: reserved,
	})
	if err != nil {
		if errors.Is(err, billing.ErrInsufficientBalance) {
			writeError(w, http.StatusPaymentRequired, "insufficient_balance", "余额不足")
			return
		}
		if errors.Is(err, billing.ErrRequestConflict) {
			writeError(w, http.StatusConflict, "idempotency_conflict", "同一 Idempotency-Key 已用于不同请求")
			return
		}
		h.logger.Error("start gateway billing operation", "request_id", requestID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
	}
	requestID = operation.Operation.RequestID
	w.Header().Set(requestid.Header, requestID.String())
	if !operation.Created {
		// 已存在的 operation 绝不再次调用上游，避免网络重试产生两次模型调用或两次扣费。
		writeOperationReplay(w, operation.Operation)
		return
	}
	upstreamContext := context.WithoutCancel(r.Context())
	cancelUpstreamContext := func() {}
	if timeout := requestSettings.UpstreamTimeout(); timeout > 0 {
		upstreamContext, cancelUpstreamContext = context.WithTimeout(upstreamContext, timeout)
	}
	defer cancelUpstreamContext()
	lastRoute := modelroute.Resolved{}
	failureCode := "upstream_unavailable"
	failureMessage := "所有上游渠道均暂时不可用"
	failureStatus := http.StatusBadGateway
	for index, attempt := range attempts {
		route := attempt.route
		lastRoute = route
		upstreamStream := stream && !bufferedUpstreamStream(route)
		upstreamBody, err := buildUpstreamBody(payload, route, endpoint, upstreamStream)
		if err != nil {
			failureCode, failureMessage = "upstream_payload_error", "上游请求内容构建失败"
			h.logger.Warn("build gateway upstream request body", "request_id", requestID, "provider", route.Provider.Code, "route_id", route.ID, "attempt", index+1, "candidates", len(attempts), "error", err)
			continue
		}
		upstreamURL, err := buildUpstreamURL(route.BaseURL, route.Provider.Protocol, endpoint)
		if err != nil {
			failureCode, failureMessage = "upstream_configuration_error", "上游地址配置无效"
			h.logger.Warn("gateway route has invalid upstream URL", "request_id", requestID, "provider", route.Provider.Code, "route_id", route.ID, "attempt", index+1, "candidates", len(attempts))
			continue
		}
		if selfReferentialUpstream(r, route.BaseURL) {
			failureCode, failureMessage = "upstream_self_reference", "上游地址指向当前网关，请改为实际的上游服务地址"
			h.logger.Warn("gateway route points back to this gateway", "request_id", requestID, "provider", route.Provider.Code, "route_id", route.ID, "attempt", index+1, "candidates", len(attempts))
			continue
		}
		attemptContext, cancelAttempt := context.WithCancel(upstreamContext)
		upstreamRequest, err := http.NewRequestWithContext(attemptContext, http.MethodPost, upstreamURL, bytes.NewReader(upstreamBody))
		if err != nil {
			cancelAttempt()
			h.logger.Warn("create gateway upstream request", "request_id", requestID, "provider", route.Provider.Code, "route_id", route.ID, "attempt", index+1, "candidates", len(attempts), "error", err)
			continue
		}
		setUpstreamHeaders(upstreamRequest, r, route)
		setUpstreamIdempotencyKey(upstreamRequest, requestID)
		retryDelays := h.upstreamRetryDelays
		if stream && upstreamStream {
			// A failed upstream stream can be safely retried as a buffered chat
			// completion below, without waiting through several long header timeouts.
			retryDelays = nil
		}
		response, err, requestWritten := h.doUpstreamWithRetries(upstreamRequest, retryDelays)
		streamRetry := stream && upstreamStream && endpoint == "chat_completions" && err != nil
		if streamRetry && !requestWritten {
			if response != nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
				_ = response.Body.Close()
			}
			fallbackBody, fallbackErr := buildUpstreamBody(payload, route, endpoint, false)
			if fallbackErr == nil {
				fallbackRequest, requestErr := http.NewRequestWithContext(attemptContext, http.MethodPost, upstreamURL, bytes.NewReader(fallbackBody))
				if requestErr == nil {
					setUpstreamHeaders(fallbackRequest, r, route)
					setUpstreamIdempotencyKey(fallbackRequest, requestID)
					fallbackRetryDelays := h.upstreamRetryDelays
					if len(fallbackRetryDelays) > 0 {
						fallbackRetryDelays = fallbackRetryDelays[:len(fallbackRetryDelays)-1]
					}
					fallbackResponse, fallbackRequestErr, fallbackWritten := h.doUpstreamWithRetries(fallbackRequest, fallbackRetryDelays)
					response, err, requestWritten = fallbackResponse, fallbackRequestErr, fallbackWritten
					if fallbackRequestErr == nil {
						h.logger.Warn("fallback to buffered upstream completion", "request_id", requestID, "provider", route.Provider.Code, "route_id", route.ID)
					}
				}
			}
		}
		if err != nil {
			cancelAttempt()
			if requestWritten {
				// 请求字节已经写出时无法判断上游是否执行成功；保留预占并等待人工核对，
				// 不能切换渠道重放，否则用户可能在未知情况下被执行两次。
				h.markOperationPendingUnknown(requestID, "upstream_result_unknown")
				writeError(w, http.StatusBadGateway, "upstream_result_unknown", "上游请求结果无法确认；预占已保留，系统不会自动重放请求")
				return
			}
			failureCode, failureMessage = "upstream_connection_error", "连接上游失败"
			h.logger.Warn("gateway upstream request failed", "request_id", requestID, "provider", route.Provider.Code, "route_id", route.ID, "attempt", index+1, "candidates", len(attempts), "error", err)
			if errors.Is(upstreamContext.Err(), context.DeadlineExceeded) {
				failureStatus = http.StatusGatewayTimeout
				failureCode, failureMessage = "upstream_timeout", "上游请求超过总超时时间"
				break
			}
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			failureCode = "upstream_http_error"
			failureMessage = fmt.Sprintf("上游返回 HTTP %d", response.StatusCode)
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
			_ = response.Body.Close()
			cancelAttempt()
			h.logger.Warn("gateway upstream rejected request", "request_id", requestID, "provider", route.Provider.Code, "route_id", route.ID, "status", response.StatusCode, "attempt", index+1, "candidates", len(attempts))
			h.failOperation(requestID, failureCode)
			h.recordFailure(actor, route, requestID, endpoint, failureStatus, failureCode, failureMessage, publicModel, startedAt)
			writeError(w, failureStatus, "upstream_unavailable", fmt.Sprintf("上游渠道返回失败：%s（%s）", failureMessage, failureCode))
			return
		}
		if stream && !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
			responseBody, readErr := readLimited(response.Body, maxUpstreamBodyBytes)
			_ = response.Body.Close()
			cancelAttempt()
			if errors.Is(upstreamContext.Err(), context.DeadlineExceeded) {
				h.markOperationPendingUnknown(requestID, "upstream_response_timeout")
				writeError(w, http.StatusGatewayTimeout, "upstream_result_unknown", "上游响应读取超时；预占已保留，系统不会自动重放请求")
				return
			}
			if readErr != nil {
				h.logger.Warn("read buffered stream fallback", "request_id", requestID, "provider", route.Provider.Code, "route_id", route.ID, "attempt", index+1, "candidates", len(attempts), "error", readErr)
				h.markOperationPendingUnknown(requestID, "upstream_response_incomplete")
				writeError(w, http.StatusBadGateway, "upstream_result_unknown", "上游响应不完整；预占已保留，系统不会自动重放请求")
				return
			}
			h.bufferedStreamResponse(w, r, response, responseBody, actor, route, requestID, endpoint, reserved, attempt.inputEstimate, maximum, startedAt)
			return
		}
		if stream || strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
			h.streamResponse(w, r, response, attemptContext, cancelAttempt, requestSettings, actor, route, requestID, endpoint, reserved, attempt.inputEstimate, maximum, startedAt)
			_ = response.Body.Close()
			return
		}
		responseBody, readErr := readLimited(response.Body, maxUpstreamBodyBytes)
		_ = response.Body.Close()
		cancelAttempt()
		if errors.Is(upstreamContext.Err(), context.DeadlineExceeded) {
			h.markOperationPendingUnknown(requestID, "upstream_response_timeout")
			writeError(w, http.StatusGatewayTimeout, "upstream_result_unknown", "上游响应读取超时；预占已保留，系统不会自动重放请求")
			return
		}
		if readErr != nil {
			h.logger.Warn("read gateway upstream response", "request_id", requestID, "provider", route.Provider.Code, "route_id", route.ID, "attempt", index+1, "candidates", len(attempts), "error", readErr)
			h.markOperationPendingUnknown(requestID, "upstream_response_incomplete")
			writeError(w, http.StatusBadGateway, "upstream_result_unknown", "上游响应不完整；预占已保留，系统不会自动重放请求")
			return
		}
		h.bufferedResponse(w, r, response, responseBody, actor, route, requestID, endpoint, reserved, attempt.inputEstimate, maximum, startedAt)
		return
	}
	h.failOperation(requestID, failureCode)
	if lastRoute.ID != uuid.Nil {
		h.recordFailure(actor, lastRoute, requestID, endpoint, failureStatus, failureCode, failureMessage, publicModel, startedAt)
	}
	writeError(w, failureStatus, "upstream_unavailable", fmt.Sprintf("所有上游渠道均暂时不可用：%s（%s）", failureMessage, failureCode))
}

/**
 * doUpstream 执行该名称对应的业务处理逻辑。
 * @param request 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) doUpstream(request *http.Request) (*http.Response, error) {
	response, err, _ := h.doUpstreamWithRetries(request, h.upstreamRetryDelays)
	return response, err
}

/**
 * doUpstreamWithRetries 执行该名称对应的业务处理逻辑。
 * @param request 本次操作需要使用的输入参数。
 * @param retryDelays 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) doUpstreamWithRetries(request *http.Request, retryDelays []time.Duration) (*http.Response, error, bool) {
	for attempt := 0; ; attempt++ {
		attemptRequest := request
		if attempt > 0 {
			body, err := request.GetBody()
			if err != nil {
				return nil, fmt.Errorf("reopen upstream request body: %w", err), false
			}
			attemptRequest = request.Clone(request.Context())
			attemptRequest.Body = body
		}
		var connected, written atomic.Bool
		trace := &httptrace.ClientTrace{
			GotConn:      func(httptrace.GotConnInfo) { connected.Store(true) },
			WroteRequest: func(httptrace.WroteRequestInfo) { written.Store(true) },
		}
		attemptRequest = attemptRequest.WithContext(httptrace.WithClientTrace(attemptRequest.Context(), trace))
		response, err := h.client.Do(attemptRequest)
		requestMayHaveBeenSent := connected.Load() || written.Load() || response != nil
		retryable := !requestMayHaveBeenSent && retryableUpstreamConnectionError(err)
		if !retryable || attempt >= len(retryDelays) {
			return response, err, requestMayHaveBeenSent
		}
		if response != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
			_ = response.Body.Close()
		}
		delay := retryDelays[attempt]
		if delay <= 0 {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-request.Context().Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, request.Context().Err(), requestMayHaveBeenSent
		}
	}
}

/**
 * bufferedStreamResponse 执行该名称对应的业务处理逻辑。
 * @param w 本次操作需要使用的输入参数。
 * @param r 本次操作需要使用的输入参数。
 * @param response 本次操作需要使用的输入参数。
 * @param body 本次操作需要使用的输入参数。
 * @param actor 本次操作需要使用的输入参数。
 * @param route 本次操作需要使用的输入参数。
 * @param requestID 本次操作需要使用的输入参数。
 * @param endpoint 本次操作需要使用的输入参数。
 * @param reserved 本次操作需要使用的输入参数。
 * @param inputEstimate 本次操作需要使用的输入参数。
 * @param outputMaximum 本次操作需要使用的输入参数。
 * @param startedAt 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) bufferedStreamResponse(w http.ResponseWriter, r *http.Request, response *http.Response, body []byte, actor apikey.Actor, route modelroute.Resolved, requestID uuid.UUID, endpoint string, reserved int64, inputEstimate, outputMaximum int, startedAt time.Time) {
	if !h.acceptBufferedOutcome(w, actor, route, requestID, endpoint, body, startedAt) {
		return
	}
	usage := parseUsage(body, usageSemanticsFor(endpoint, route.Provider.Protocol))
	rates := rateCardFor(route)
	usage = applyUsageFallback(usage, inputEstimate, min(len(body)+128, outputMaximum), rates)
	quote, err := billing.CalculateCost(usage.breakdown(), rates, h.multiplierAt(actor, startedAt))
	if err != nil {
		h.markOperationPendingUnknown(requestID, "billing_calculation_error")
		writeError(w, http.StatusInternalServerError, "billing_error", "调用已完成但计费记录失败")
		return
	}
	if err := h.settle(r.Context(), actor, route, requestID, endpoint, reserved, quote, usage, startedAt); err != nil {
		h.logger.Error("forward buffered stream after usage finalization failure", "request_id", requestID, "error", err)
		if errors.Is(err, errSettlementIntentNotPersisted) {
			h.markOperationPendingUnknown(requestID, "settlement_intent_unavailable")
			writeError(w, http.StatusInternalServerError, "billing_settlement_pending", "调用已完成但结算状态未能持久化；预占已保留，请使用请求 ID 联系管理员核对")
			return
		}
	}
	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Novro-Request-ID", requestID.String())
	w.WriteHeader(response.StatusCode)
	flusher, _ := w.(http.Flusher)
	if endpoint == "chat_completions" {
		h.writeBufferedChatEvents(w, body, flusher)
	} else {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", bytes.TrimSpace(body))
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}
}

/**
 * writeBufferedChatEvents 执行该名称对应的业务处理逻辑。
 * @param w 本次操作需要使用的输入参数。
 * @param body 本次操作需要使用的输入参数。
 * @param flusher 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) writeBufferedChatEvents(w http.ResponseWriter, body []byte, flusher http.Flusher) {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", bytes.TrimSpace(body))
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		return
	}
	choices, _ := root["choices"].([]any)
	if len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		message, _ := choice["message"].(map[string]any)
		delta := map[string]any{"role": "assistant"}
		if content, ok := message["content"].(string); ok {
			delta["content"] = content
		}
		if reasoning, ok := message["reasoning_content"].(string); ok && reasoning != "" {
			delta["reasoning_content"] = reasoning
		}
		writeSSEJSON(w, map[string]any{"id": root["id"], "object": "chat.completion.chunk", "created": root["created"], "model": root["model"], "choices": []any{map[string]any{"index": choice["index"], "delta": delta, "finish_reason": nil}}})
		finish := choice["finish_reason"]
		final := map[string]any{"index": choice["index"], "delta": map[string]any{}, "finish_reason": finish}
		finalEvent := map[string]any{"id": root["id"], "object": "chat.completion.chunk", "created": root["created"], "model": root["model"], "choices": []any{final}}
		if usage, ok := root["usage"]; ok {
			finalEvent["usage"] = usage
		}
		writeSSEJSON(w, finalEvent)
	} else {
		writeSSEJSON(w, root)
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

/**
 * writeSSEJSON 执行该名称对应的业务处理逻辑。
 * @param w 本次操作需要使用的输入参数。
 * @param value 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func writeSSEJSON(w io.Writer, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

/**
 * retryableUpstreamConnectionError 执行该名称对应的业务处理逻辑。
 * @param err 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func retryableUpstreamConnectionError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var urlError *url.Error
	if errors.As(err, &urlError) {
		err = urlError.Err
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError)
}

// Any response outside the success range counts as a failed provider attempt.
/**
 * retryableUpstreamStatus 执行该名称对应的业务处理逻辑。
 * @param status 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func retryableUpstreamStatus(status int) bool {
	return status < http.StatusOK || status >= http.StatusMultipleChoices
}

/**
 * bufferedResponse 执行该名称对应的业务处理逻辑。
 * @param w 本次操作需要使用的输入参数。
 * @param r 本次操作需要使用的输入参数。
 * @param response 本次操作需要使用的输入参数。
 * @param body 本次操作需要使用的输入参数。
 * @param actor 本次操作需要使用的输入参数。
 * @param route 本次操作需要使用的输入参数。
 * @param requestID 本次操作需要使用的输入参数。
 * @param endpoint 本次操作需要使用的输入参数。
 * @param reserved 本次操作需要使用的输入参数。
 * @param inputEstimate 本次操作需要使用的输入参数。
 * @param outputMaximum 本次操作需要使用的输入参数。
 * @param startedAt 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) bufferedResponse(w http.ResponseWriter, r *http.Request, response *http.Response, body []byte, actor apikey.Actor, route modelroute.Resolved, requestID uuid.UUID, endpoint string, reserved int64, inputEstimate, outputMaximum int, startedAt time.Time) {
	if !h.acceptBufferedOutcome(w, actor, route, requestID, endpoint, body, startedAt) {
		return
	}
	usage := parseUsage(body, usageSemanticsFor(endpoint, route.Provider.Protocol))
	rates := rateCardFor(route)
	usage = applyUsageFallback(usage, inputEstimate, min(len(body)+128, outputMaximum), rates)
	quote, err := billing.CalculateCost(usage.breakdown(), rates, h.multiplierAt(actor, startedAt))
	if err != nil {
		h.markOperationPendingUnknown(requestID, "billing_calculation_error")
		writeError(w, http.StatusInternalServerError, "billing_error", "调用已完成但计费记录失败")
		return
	}
	if err := h.settle(r.Context(), actor, route, requestID, endpoint, reserved, quote, usage, startedAt); err != nil {
		// The pending settlement was persisted before the successful response is
		// returned. Keep the hold until the recovery loop commits the same usage.
		h.logger.Error("forward successful response after usage finalization failure", "request_id", requestID, "error", err)
		if errors.Is(err, errSettlementIntentNotPersisted) {
			h.markOperationPendingUnknown(requestID, "settlement_intent_unavailable")
			writeError(w, http.StatusInternalServerError, "billing_settlement_pending", "调用已完成但结算状态未能持久化；预占已保留，请使用请求 ID 联系管理员核对")
			return
		}
	}
	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Set("X-Novro-Request-ID", requestID.String())
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(body)
}

/**
 * acceptBufferedOutcome 执行该名称对应的业务处理逻辑。
 * @param w 本次操作需要使用的输入参数。
 * @param actor 本次操作需要使用的输入参数。
 * @param route 本次操作需要使用的输入参数。
 * @param requestID 本次操作需要使用的输入参数。
 * @param endpoint 本次操作需要使用的输入参数。
 * @param body 本次操作需要使用的输入参数。
 * @param startedAt 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) acceptBufferedOutcome(w http.ResponseWriter, actor apikey.Actor, route modelroute.Resolved, requestID uuid.UUID, endpoint string, body []byte, startedAt time.Time) bool {
	switch classifyBufferedOutcome(endpoint, body) {
	case streamOutcomeBillable:
		return true
	case streamOutcomeFailed:
		h.failOperation(requestID, "upstream_response_failed")
		h.recordFailure(actor, route, requestID, endpoint, http.StatusBadGateway, "upstream_response_failed", "上游调用失败", route.PublicName, startedAt)
		writeError(w, http.StatusBadGateway, "upstream_response_failed", "上游调用失败")
	default:
		h.markOperationPendingUnknown(requestID, "upstream_response_unknown")
		writeError(w, http.StatusBadGateway, "upstream_result_unknown", "上游响应状态无法确认；预占已保留，系统不会自动重放请求")
	}
	return false
}

/**
 * classifyBufferedOutcome 执行该名称对应的业务处理逻辑。
 * @param endpoint 本次操作需要使用的输入参数。
 * @param body 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func classifyBufferedOutcome(endpoint string, body []byte) streamOutcome {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil || root == nil {
		return streamOutcomeOpen
	}
	if errorValue, exists := root["error"]; exists && errorValue != nil {
		return streamOutcomeFailed
	}
	if endpoint == "responses" {
		switch strings.ToLower(strings.TrimSpace(stringValue(root["status"]))) {
		case "failed", "cancelled":
			return streamOutcomeFailed
		case "queued", "in_progress":
			return streamOutcomeOpen
		}
	}
	if endpoint == "messages" && strings.EqualFold(stringValue(root["type"]), "error") {
		return streamOutcomeFailed
	}
	return streamOutcomeBillable
}

/**
 * buildUpstreamBody 执行该名称对应的业务处理逻辑。
 * @param payload 本次操作需要使用的输入参数。
 * @param route 本次操作需要使用的输入参数。
 * @param endpoint 本次操作需要使用的输入参数。
 * @param stream 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func buildUpstreamBody(payload map[string]any, route modelroute.Resolved, endpoint string, stream bool) ([]byte, error) {
	upstreamPayload := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		upstreamPayload[key] = value
	}
	upstreamPayload["model"] = route.UpstreamName
	if _, exists := payload["stream"]; exists {
		upstreamPayload["stream"] = stream
	}
	if endpoint == "chat_completions" && stream {
		options := make(map[string]any)
		if current, ok := payload["stream_options"].(map[string]any); ok {
			for key, value := range current {
				options[key] = value
			}
		}
		options["include_usage"] = true
		upstreamPayload["stream_options"] = options
	}
	return json.Marshal(upstreamPayload)
}

// The Reasonix/Kimi gateway can take a long time before producing its first
// streaming chunk. Generate a buffered completion upstream and translate it
// back to SSE below so clients with shorter HTTP/2 header deadlines can still
// receive a complete response.
/**
 * bufferedUpstreamStream 执行该名称对应的业务处理逻辑。
 * @param route 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func bufferedUpstreamStream(route modelroute.Resolved) bool {
	if strings.EqualFold(strings.TrimSpace(route.Provider.Code), "reasonix") {
		return true
	}
	parsed, err := url.Parse(route.BaseURL)
	return err == nil && strings.EqualFold(parsed.Hostname(), "1024token.net")
}

/**
 * setUpstreamHeaders 执行该名称对应的业务处理逻辑。
 * @param upstreamRequest 本次操作需要使用的输入参数。
 * @param inboundRequest 本次操作需要使用的输入参数。
 * @param route 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func setUpstreamHeaders(upstreamRequest, inboundRequest *http.Request, route modelroute.Resolved) {
	upstreamRequest.Header.Set("Content-Type", "application/json")
	upstreamRequest.Header.Set("Accept", "application/json, text/event-stream")
	upstreamRequest.Header.Set("User-Agent", "Novro-Gateway/1")
	if route.Provider.Protocol == provider.ProtocolAnthropic {
		upstreamRequest.Header.Set("X-API-Key", route.APIKey)
		version := strings.TrimSpace(inboundRequest.Header.Get("Anthropic-Version"))
		if version == "" {
			version = "2023-06-01"
		}
		upstreamRequest.Header.Set("Anthropic-Version", version)
		if beta := strings.TrimSpace(inboundRequest.Header.Get("Anthropic-Beta")); beta != "" {
			upstreamRequest.Header.Set("Anthropic-Beta", beta)
		}
		return
	}
	upstreamRequest.Header.Set("Authorization", "Bearer "+route.APIKey)
}

/**
 * setUpstreamIdempotencyKey 执行该名称对应的业务处理逻辑。
 * @param upstreamRequest 本次操作需要使用的输入参数。
 * @param requestID 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func setUpstreamIdempotencyKey(upstreamRequest *http.Request, requestID uuid.UUID) {
	upstreamRequest.Header.Set("Idempotency-Key", "novro-"+requestID.String())
}

type streamReadResult struct {
	fragment []byte
	isPrefix bool
	readErr  error
}

type streamActivityReader struct {
	reader   io.Reader
	activity chan<- struct{}
}

type streamOutcome uint8

const (
	streamOutcomeOpen streamOutcome = iota
	streamOutcomeBillable
	streamOutcomeFailed
)

/**
 * Read 执行该名称对应的业务处理逻辑。
 * @param target 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (r streamActivityReader) Read(target []byte) (int, error) {
	read, err := r.reader.Read(target)
	if read > 0 {
		select {
		case r.activity <- struct{}{}:
		default:
		}
	}
	return read, err
}

/**
 * streamResponse 执行该名称对应的业务处理逻辑。
 * @param w 本次操作需要使用的输入参数。
 * @param r 本次操作需要使用的输入参数。
 * @param response 本次操作需要使用的输入参数。
 * @param upstreamContext 本次操作需要使用的输入参数。
 * @param cancelUpstream 本次操作需要使用的输入参数。
 * @param settings 本次操作需要使用的输入参数。
 * @param actor 本次操作需要使用的输入参数。
 * @param route 本次操作需要使用的输入参数。
 * @param requestID 本次操作需要使用的输入参数。
 * @param endpoint 本次操作需要使用的输入参数。
 * @param reserved 本次操作需要使用的输入参数。
 * @param inputEstimate 本次操作需要使用的输入参数。
 * @param outputMaximum 本次操作需要使用的输入参数。
 * @param startedAt 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) streamResponse(w http.ResponseWriter, r *http.Request, response *http.Response, upstreamContext context.Context, cancelUpstream context.CancelFunc, settings gatewaysettings.Config, actor apikey.Actor, route modelroute.Resolved, requestID uuid.UUID, endpoint string, reserved int64, inputEstimate, outputMaximum int, startedAt time.Time) {
	defer cancelUpstream()
	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Novro-Request-ID", requestID.String())
	w.WriteHeader(response.StatusCode)
	flusher, _ := w.(http.Flusher)
	usage := tokenUsage{}
	outcome := streamOutcomeOpen
	failureCode, failureMessage := "", ""
	line := make([]byte, 0, 64<<10)
	lineTooLarge := false
	atEventBoundary := true
	var relayErr error
	clientConnected := true
	clientDone := r.Context().Done()
	var drainTimer *time.Timer
	var drainC <-chan time.Time
	defer func() {
		if drainTimer != nil {
			drainTimer.Stop()
		}
	}()
	var heartbeatTicker *time.Ticker
	var heartbeatC <-chan time.Time
	if settings.SSEHeartbeatEnabled {
		heartbeatTicker = time.NewTicker(settings.HeartbeatInterval())
		heartbeatC = heartbeatTicker.C
		defer heartbeatTicker.Stop()
	}
	var idleTimer *time.Timer
	var idleC <-chan time.Time
	var activityC chan struct{}
	if idleTimeout := settings.UpstreamStreamIdleTimeout(); idleTimeout > 0 {
		idleTimer = time.NewTimer(idleTimeout)
		idleC = idleTimer.C
		activityC = make(chan struct{}, 1)
		defer idleTimer.Stop()
	}
	streamReader := io.Reader(response.Body)
	if activityC != nil {
		streamReader = streamActivityReader{reader: response.Body, activity: activityC}
	}
	reader := bufio.NewReaderSize(streamReader, 64<<10)
	readResults := make(chan streamReadResult)
	go func() {
		for {
			fragment, isPrefix, readErr := reader.ReadLine()
			result := streamReadResult{fragment: bytes.Clone(fragment), isPrefix: isPrefix, readErr: readErr}
			select {
			case readResults <- result:
			case <-upstreamContext.Done():
				return
			}
			if readErr != nil {
				return
			}
		}
	}()
	resetIdleTimer := func() {
		if idleTimer == nil {
			return
		}
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(settings.UpstreamStreamIdleTimeout())
	}
streamLoop:
	for {
		var result streamReadResult
		select {
		case result = <-readResults:
		case <-clientDone:
			clientConnected = false
			clientDone = nil
			drainTimer = time.NewTimer(streamSettlementDrainTimeout)
			drainC = drainTimer.C
			continue
		case <-drainC:
			relayErr = fmt.Errorf("settlement drain timeout after %s", streamSettlementDrainTimeout)
			cancelUpstream()
			_ = response.Body.Close()
			break streamLoop
		case <-activityC:
			resetIdleTimer()
			continue
		case <-heartbeatC:
			if clientConnected && atEventBoundary && len(line) == 0 && !lineTooLarge {
				if _, writeErr := io.WriteString(w, ": novro-keepalive\n\n"); writeErr != nil {
					relayErr = writeErr
					clientConnected = false
					clientDone = nil
					if drainTimer == nil {
						drainTimer = time.NewTimer(streamSettlementDrainTimeout)
						drainC = drainTimer.C
					}
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			continue
		case <-idleC:
			relayErr = fmt.Errorf("upstream stream idle timeout after %s", settings.UpstreamStreamIdleTimeout())
			cancelUpstream()
			_ = response.Body.Close()
			break streamLoop
		case <-upstreamContext.Done():
			if errors.Is(upstreamContext.Err(), context.DeadlineExceeded) {
				relayErr = fmt.Errorf("upstream total timeout after %s", settings.UpstreamTimeout())
			}
			_ = response.Body.Close()
			break streamLoop
		}
		fragment, isPrefix, readErr := result.fragment, result.isPrefix, result.readErr
		if len(fragment) == 0 && readErr != nil && len(line) == 0 && !lineTooLarge {
			if readErr != io.EOF {
				relayErr = readErr
			}
			break
		}
		if !lineTooLarge {
			if maxUpstreamBodyBytes <= 0 || int64(len(line))+int64(len(fragment)) <= maxUpstreamBodyBytes {
				line = append(line, fragment...)
			} else {
				if bytes.HasPrefix(line, []byte("data:")) {
					usage.Estimated = true
				}
				line = nil
				lineTooLarge = true
			}
		}
		if !isPrefix && !lineTooLarge && bytes.HasPrefix(line, []byte("data:")) {
			data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			eventOutcome, eventCode, eventMessage := classifyStreamEvent(endpoint, data)
			if eventOutcome == streamOutcomeFailed || (eventOutcome != streamOutcomeOpen && outcome == streamOutcomeOpen) {
				outcome = eventOutcome
			}
			if eventCode != "" {
				failureCode, failureMessage = eventCode, eventMessage
			}
			if !bytes.Equal(data, []byte("[DONE]")) {
				usage.merge(parseUsage(data, usageSemanticsFor(endpoint, route.Provider.Protocol)))
			}
		}
		if clientConnected && len(fragment) > 0 {
			written, writeErr := w.Write(fragment)
			if writeErr == nil && written != len(fragment) {
				writeErr = io.ErrShortWrite
			}
			if writeErr != nil {
				relayErr = writeErr
				clientConnected = false
				clientDone = nil
				if drainTimer == nil {
					drainTimer = time.NewTimer(streamSettlementDrainTimeout)
					drainC = drainTimer.C
				}
			}
		}
		if !isPrefix {
			if clientConnected {
				_, writeErr := w.Write([]byte{'\n'})
				if writeErr != nil {
					relayErr = writeErr
					clientConnected = false
					clientDone = nil
					if drainTimer == nil {
						drainTimer = time.NewTimer(streamSettlementDrainTimeout)
						drainC = drainTimer.C
					}
				}
			}
			atEventBoundary = len(line) == 0
			line = line[:0]
			lineTooLarge = false
		}
		if clientConnected && flusher != nil {
			flusher.Flush()
		}
		if readErr != nil {
			if readErr != io.EOF {
				relayErr = readErr
			}
			break
		}
	}
	if relayErr != nil {
		h.logger.Warn("relay gateway stream", "request_id", requestID, "provider", route.Provider.Code, "error", relayErr)
	}
	if outcome == streamOutcomeFailed {
		h.failOperation(requestID, "upstream_stream_failed")
		if failureCode == "" {
			failureCode, failureMessage = "upstream_stream_failed", "上游流式调用失败"
		}
		h.recordFailure(actor, route, requestID, endpoint, http.StatusBadGateway, failureCode, failureMessage, route.PublicName, startedAt)
		return
	}
	if outcome != streamOutcomeBillable {
		h.markOperationPendingUnknown(requestID, "upstream_stream_incomplete")
		return
	}
	usage = applyUsageFallback(usage, inputEstimate, outputMaximum, rateCardFor(route))
	quote, err := billing.CalculateCost(usage.breakdown(), rateCardFor(route), h.multiplierAt(actor, startedAt))
	if err != nil {
		h.logger.Error("calculate streamed usage", "request_id", requestID, "error", err)
		h.markOperationPendingUnknown(requestID, "billing_calculation_error")
		return
	}
	if err := h.settle(r.Context(), actor, route, requestID, endpoint, reserved, quote, usage, startedAt); err != nil {
		// A successful stream is billable even when the client disconnects or the
		// usage row cannot be persisted after the response has started.
		h.logger.Error("stream usage finalization unavailable", "request_id", requestID, "error", err)
		if errors.Is(err, errSettlementIntentNotPersisted) {
			h.markOperationPendingUnknown(requestID, "settlement_intent_unavailable")
		}
	}
}

/**
 * settle 按“持久化意图 -> 写入资金和 usage -> 完成 operation”的顺序完成结算。
 * 第一阶段完成后即使进程崩溃，后台也能根据 settlement JSON 恢复；成功响应只应在该流程完成后返回。
 * @param ctx 本次操作需要使用的输入参数。
 * @param actor 本次操作需要使用的输入参数。
 * @param route 本次操作需要使用的输入参数。
 * @param requestID 本次操作需要使用的输入参数。
 * @param endpoint 本次操作需要使用的输入参数。
 * @param reserved 本次操作需要使用的输入参数。
 * @param quote 本次操作需要使用的输入参数。
 * @param usage 本次操作需要使用的输入参数。
 * @param startedAt 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func (h *Handler) settle(ctx context.Context, actor apikey.Actor, route modelroute.Resolved, requestID uuid.UUID, endpoint string, reserved int64, quote billing.Quote, usage tokenUsage, startedAt time.Time) error {
	input := h.usageInput(actor, route, requestID, endpoint, reserved, quote, usage, startedAt)
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer persistCancel()
	// 先确保可恢复的结算意图已经入库，防止上游成功但进程崩溃后丢失应收费用。
	if err, _ := h.retrySettlement(persistCtx, func() error { return h.billing.MarkOperationPendingSettlement(persistCtx, requestID, input) }); err != nil {
		return fmt.Errorf("%w: %v", errSettlementIntentNotPersisted, err)
	}
	// Finalize 在同一事务中处理差额退款/超额补扣并写 usage；它本身支持按 requestID 幂等重试。
	if err := h.finalizeInput(ctx, input); err != nil {
		return err
	}
	completeCtx, completeCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer completeCancel()
	if err, _ := h.retrySettlement(completeCtx, func() error { return h.billing.CompleteOperation(completeCtx, requestID) }); err != nil {
		return fmt.Errorf("complete gateway operation: %w", err)
	}
	return nil
}

/**
 * usageInput 执行该名称对应的业务处理逻辑。
 * @param actor 本次操作需要使用的输入参数。
 * @param route 本次操作需要使用的输入参数。
 * @param requestID 本次操作需要使用的输入参数。
 * @param endpoint 本次操作需要使用的输入参数。
 * @param reserved 本次操作需要使用的输入参数。
 * @param quote 本次操作需要使用的输入参数。
 * @param usage 本次操作需要使用的输入参数。
 * @param startedAt 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) usageInput(actor apikey.Actor, route modelroute.Resolved, requestID uuid.UUID, endpoint string, reserved int64, quote billing.Quote, usage tokenUsage, startedAt time.Time) billing.UsageInput {
	billingGroupID := actor.APIKey.BillingGroupID
	return billing.UsageInput{UserID: actor.User.ID, APIKeyID: actor.APIKey.ID, ModelRouteID: route.ID, UpstreamModelID: route.UpstreamModelID, BillingGroupID: &billingGroupID, RequestID: requestID, Endpoint: endpoint,
		StatusCode: http.StatusOK, InputTokens: usage.Input, Tokens: usage.breakdown(), OutputTokens: usage.Output, Rates: rateCardFor(route), BaseCostMicros: quote.BaseCostMicros, MultiplierBPS: h.multiplierAt(actor, startedAt), CostMicros: quote.CostMicros, ReservedMicros: reserved,
		Estimated: usage.Estimated, UpstreamRequestID: usage.UpstreamID, ModelName: route.PublicName, UpstreamModelName: route.UpstreamModel.UpstreamName, BillingGroupCode: actor.APIKey.BillingGroup.Code, BillingGroupName: actor.APIKey.BillingGroup.DisplayName, CalculationVersion: billing.CalculationVersion, CreatedAt: startedAt, FinishedAt: h.now()}
}

/**
 * classifyStreamEvent 执行该名称对应的业务处理逻辑。
 * @param endpoint 本次操作需要使用的输入参数。
 * @param data 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func classifyStreamEvent(endpoint string, data []byte) (streamOutcome, string, string) {
	if bytes.Equal(data, []byte("[DONE]")) {
		return streamOutcomeBillable, "", ""
	}
	var event struct {
		Type     string `json:"type"`
		Error    any    `json:"error"`
		Response struct {
			Status string `json:"status"`
		} `json:"response"`
	}
	if json.Unmarshal(data, &event) != nil {
		return streamOutcomeOpen, "", ""
	}
	switch endpoint {
	case "responses":
		switch event.Type {
		case "response.completed", "response.incomplete":
			return streamOutcomeBillable, "", ""
		case "response.failed", "response.cancelled", "error":
			return streamOutcomeFailed, "upstream_stream_failed", "上游流式调用失败"
		}
		if event.Response.Status == "failed" || event.Response.Status == "cancelled" {
			return streamOutcomeFailed, "upstream_stream_failed", "上游流式调用失败"
		}
	case "messages":
		switch event.Type {
		case "message_stop":
			return streamOutcomeBillable, "", ""
		case "error":
			return streamOutcomeFailed, "upstream_stream_failed", "上游流式调用失败"
		}
	}
	return streamOutcomeOpen, "", ""
}

/**
 * finalizeInput 执行该名称对应的业务处理逻辑。
 * @param ctx 本次操作需要使用的输入参数。
 * @param input 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) finalizeInput(ctx context.Context, input billing.UsageInput) error {
	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err, attempts := h.retrySettlement(finalizeCtx, func() error { return h.billing.Finalize(finalizeCtx, input) })
	if err != nil {
		h.logger.Error("finalize gateway usage", "request_id", input.RequestID, "attempts", attempts, "error", err)
	}
	return err
}

/**
 * recordFailure 执行该名称对应的业务处理逻辑。
 * @param actor 本次操作需要使用的输入参数。
 * @param route 本次操作需要使用的输入参数。
 * @param requestID 本次操作需要使用的输入参数。
 * @param endpoint 本次操作需要使用的输入参数。
 * @param statusCode 本次操作需要使用的输入参数。
 * @param errorCode 本次操作需要使用的输入参数。
 * @param errorMessage 本次操作需要使用的输入参数。
 * @param modelName 本次操作需要使用的输入参数。
 * @param startedAt 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) recordFailure(actor apikey.Actor, route modelroute.Resolved, requestID uuid.UUID, endpoint string, statusCode int, errorCode, errorMessage, modelName string, startedAt time.Time) {
	if route.UpstreamModel == nil || route.UpstreamModelID == nil || actor.APIKey.BillingGroupID == uuid.Nil || actor.APIKey.BillingGroup.ID == uuid.Nil {
		return
	}
	finishedAt := h.now()
	duration := finishedAt.Sub(startedAt)
	if duration < 0 {
		duration = 0
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 10*time.Second)
	defer cancel()
	billingGroupID := actor.APIKey.BillingGroupID
	input := billing.FailureInput{
		UserID: actor.User.ID, APIKeyID: actor.APIKey.ID, ModelRouteID: route.ID, UpstreamModelID: route.UpstreamModelID, BillingGroupID: &billingGroupID,
		RequestID: requestID, Endpoint: endpoint, StatusCode: statusCode, ErrorCode: errorCode, ErrorMessage: errorMessage,
		MultiplierBPS: h.multiplierAt(actor, startedAt), ModelName: modelName, UpstreamModelName: route.UpstreamModel.UpstreamName, BillingGroupCode: actor.APIKey.BillingGroup.Code, BillingGroupName: actor.APIKey.BillingGroup.DisplayName,
		CreatedAt: startedAt, FinishedAt: finishedAt, DurationMS: duration.Milliseconds(),
	}
	if err := h.billing.RecordFailure(ctx, input); err != nil {
		h.logger.Error("record failed gateway usage", "request_id", requestID, "error", err)
	}
}

/**
 * failOperation 执行该名称对应的业务处理逻辑。
 * @param requestID 本次操作需要使用的输入参数。
 * @param code 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) failOperation(requestID uuid.UUID, code string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err, attempts := h.retrySettlement(ctx, func() error { return h.billing.FailOperation(ctx, requestID, code) }); err != nil {
		h.logger.Error("fail gateway operation", "request_id", requestID, "attempts", attempts, "error", err)
	}
}

/**
 * markOperationPendingUnknown 执行该名称对应的业务处理逻辑。
 * @param requestID 本次操作需要使用的输入参数。
 * @param reason 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) markOperationPendingUnknown(requestID uuid.UUID, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err, attempts := h.retrySettlement(ctx, func() error { return h.billing.MarkOperationPendingUnknown(ctx, requestID, reason) }); err != nil {
		h.logger.Error("mark gateway operation result unknown", "request_id", requestID, "attempts", attempts, "error", err)
	}
}

/**
 * retrySettlement 执行该名称对应的业务处理逻辑。
 * @param ctx 本次操作需要使用的输入参数。
 * @param operation 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h *Handler) retrySettlement(ctx context.Context, operation func() error) (error, int) {
	for attempt := 0; ; attempt++ {
		err := operation()
		if err == nil || !retryableSettlementError(err) || attempt >= len(h.settlementRetryDelays) {
			return err, attempt + 1
		}
		delay := h.settlementRetryDelays[attempt]
		if delay <= 0 {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return fmt.Errorf("wait to retry settlement after %v: %w", err, ctx.Err()), attempt + 1
		}
	}
}

/**
 * retryableSettlementError 执行该名称对应的业务处理逻辑。
 * @param err 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func retryableSettlementError(err error) bool {
	return err != nil &&
		!errors.Is(err, billing.ErrInvalidInput) && !errors.Is(err, billing.ErrWalletNotFound) &&
		!errors.Is(err, billing.ErrInsufficientBalance) && !errors.Is(err, billing.ErrRequestConflict) &&
		!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// tokenUsage 是把不同上游协议的 usage 归一化后的中间表示。
// Input 始终等于四种输入维度之和；Reported 与 Invalid 分开记录，避免“字段缺失”和“字段矛盾”被混为零 Token。
type tokenUsage struct {
	Input, UncachedInput, CacheRead, CacheWrite, CacheWrite1h, Output int
	InputReported, OutputReported                                     bool
	InputInvalid, OutputInvalid                                       bool
	Estimated                                                         bool
	UpstreamID                                                        string
}

/**
 * merge 将一份 usage 快照并入当前累计结果。
 * 流式上游通常在每个事件中发送“截至当前”的累计值而不是增量，因此已报告的维度必须整体替换，不能累加。
 * 无效字段优先级高于已报告字段，避免前一个事件的旧数据掩盖后一个事件发现的协议矛盾。
 * @param other 新收到的 usage 快照。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func (u *tokenUsage) merge(other tokenUsage) {
	if other.InputInvalid {
		u.Input, u.UncachedInput, u.CacheRead, u.CacheWrite, u.CacheWrite1h = 0, 0, 0, 0, 0
		u.InputReported = false
		u.InputInvalid = true
		u.Estimated = true
	} else if other.InputReported {
		// Usage events are cumulative snapshots. Replace the complete input
		// breakdown atomically so cache details from a later event cannot be
		// combined with uncached input from an earlier event.
		u.Input = other.Input
		u.UncachedInput = other.UncachedInput
		u.CacheRead = other.CacheRead
		u.CacheWrite = other.CacheWrite
		u.CacheWrite1h = other.CacheWrite1h
		u.InputReported = true
		u.InputInvalid = false
	}
	if other.OutputInvalid {
		u.Output = 0
		u.OutputReported = false
		u.OutputInvalid = true
		u.Estimated = true
	} else if other.OutputReported {
		u.Output = other.Output
		u.OutputReported = true
		u.OutputInvalid = false
	}
	if other.UpstreamID != "" {
		u.UpstreamID = other.UpstreamID
	}
	u.Estimated = u.Estimated || other.Estimated
}

type usageSemantics uint8

const (
	// OpenAI 的 prompt/input total 已包含缓存相关 Token，缓存维度必须从总数中扣除。
	usageSemanticsOpenAITotal usageSemantics = iota
	// Anthropic 的 input_tokens 是未缓存输入，缓存维度是额外的互斥部分，需要相加。
	usageSemanticsAnthropicAdditional
)

/**
 * usageSemanticsFor 执行该名称对应的业务处理逻辑。
 * @param endpoint 本次操作需要使用的输入参数。
 * @param protocol 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func usageSemanticsFor(endpoint string, protocol provider.Protocol) usageSemantics {
	if endpoint == "messages" && protocol == provider.ProtocolAnthropic {
		return usageSemanticsAnthropicAdditional
	}
	return usageSemanticsOpenAITotal
}

/**
 * parseUsage 从普通响应、Responses 包装响应或 Anthropic message 中读取 usage。
 * 同一响应可能同时包含通用 usage 与 NewAPI billing_usage；两者被归一化后按累计快照语义合并。
 * @param body 上游响应或 SSE 事件的 JSON 正文。
 * @param semantics 当前 endpoint 和上游协议的输入 Token 语义。
 * @return 可用于最终结算的确认 Token、完整性和上游请求 ID。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func parseUsage(body []byte, semantics usageSemantics) tokenUsage {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return tokenUsage{}
	}
	usage := tokenUsage{UpstreamID: stringValue(root["id"])}
	for _, candidate := range []map[string]any{mapValue(root["usage"]), mapValue(mapValue(root["response"])["usage"]), mapValue(mapValue(root["message"])["usage"])} {
		if candidate == nil {
			continue
		}
		parsed := parseUsageCandidate(candidate, semantics)
		if billingUsage, ok := parseNewAPIBillingUsage(candidate); ok {
			parsed.merge(billingUsage)
		}
		usage.merge(parsed)
	}
	if response := mapValue(root["response"]); response != nil && usage.UpstreamID == "" {
		usage.UpstreamID = stringValue(response["id"])
	}
	if message := mapValue(root["message"]); message != nil && usage.UpstreamID == "" {
		usage.UpstreamID = stringValue(message["id"])
	}
	return usage
}

/**
 * parseUsageCandidate 兼容 OpenAI、Anthropic 和代理层使用的字段别名，并生成严格一致的 Token 明细。
 * 别名同时出现但为不同的非零值、缓存子项超过总输入，或缓存创建总数与分项不一致时，整组输入维度作废；
 * 最终结算宁可把该维度标记为缺失并按零处理，也不能依据矛盾数据重复收费。
 * @param candidate 包含 usage 字段的单个 JSON 对象。
 * @param semantics 当前协议的输入总数与缓存维度关系。
 * @return 已归一化的 Token 使用量。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func parseUsageCandidate(candidate map[string]any, semantics usageSemantics) tokenUsage {
	promptTokens, hasPromptTokens := intField(candidate, "prompt_tokens")
	inputTokens, hasInputTokens := intField(candidate, "input_tokens")
	completionTokens, hasCompletionTokens := intField(candidate, "completion_tokens")
	outputTokens, hasOutputTokens := intField(candidate, "output_tokens")
	inputFieldPresent := mapHasAny(candidate, "prompt_tokens", "input_tokens")
	outputFieldPresent := mapHasAny(candidate, "completion_tokens", "output_tokens")
	invalidInputTotal := hasInvalidIntField(candidate, "prompt_tokens") || hasInvalidIntField(candidate, "input_tokens") ||
		conflictingNonzeroAliases(promptTokens, hasPromptTokens, inputTokens, hasInputTokens)
	invalidOutput := hasInvalidIntField(candidate, "completion_tokens") || hasInvalidIntField(candidate, "output_tokens") ||
		conflictingNonzeroAliases(completionTokens, hasCompletionTokens, outputTokens, hasOutputTokens)
	// 允许“一个别名为 0、另一个提供真实值”的兼容响应；两个非零别名不一致已在上面判为无效。
	promptTokens = max(promptTokens, inputTokens)
	outputTokens = max(completionTokens, outputTokens)
	details := mapValue(candidate["prompt_tokens_details"])
	if details == nil {
		details = mapValue(candidate["input_tokens_details"])
	}
	cacheWriteTop, hasCacheWriteTop := intField(candidate, "cache_creation_input_tokens")
	cacheWriteDetails, hasCacheWriteDetails := intField(details, "cache_creation_input_tokens")
	invalidCacheField := hasInvalidMapField(candidate, "prompt_tokens_details") ||
		hasInvalidMapField(candidate, "input_tokens_details") ||
		hasInvalidMapField(candidate, "cache_creation") ||
		hasInvalidIntField(candidate, "prompt_cache_hit_tokens") ||
		hasInvalidIntField(candidate, "cached_tokens") ||
		hasInvalidIntField(details, "cached_tokens") ||
		hasInvalidIntField(candidate, "cache_read_input_tokens") ||
		hasInvalidIntField(candidate, "prompt_cache_miss_tokens") ||
		hasInvalidIntField(candidate, "cache_creation_input_tokens") ||
		hasInvalidIntField(details, "cache_creation_input_tokens") ||
		hasInvalidIntField(candidate, "cache_creation_1h_input_tokens") ||
		hasInvalidIntField(candidate, "cache_creation_5m_input_tokens") ||
		hasInvalidIntField(mapValue(candidate["cache_creation"]), "ephemeral_1h_input_tokens") ||
		hasInvalidIntField(mapValue(candidate["cache_creation"]), "ephemeral_5m_input_tokens") ||
		conflictingNonzeroAliases(cacheWriteTop, hasCacheWriteTop, cacheWriteDetails, hasCacheWriteDetails)
	cacheRead := max(intValue(candidate["prompt_cache_hit_tokens"]), intValue(candidate["cached_tokens"]), intValue(details["cached_tokens"]), intValue(candidate["cache_read_input_tokens"]))
	cacheMiss := intValue(candidate["prompt_cache_miss_tokens"])
	cacheWriteTotal := max(cacheWriteTop, cacheWriteDetails)
	hasCacheWriteTotal := hasCacheWriteTop || hasCacheWriteDetails
	cacheCreation := mapValue(candidate["cache_creation"])
	cacheWrite1h := max(intValue(candidate["cache_creation_1h_input_tokens"]), intValue(cacheCreation["ephemeral_1h_input_tokens"]))
	cacheWrite5m := max(intValue(candidate["cache_creation_5m_input_tokens"]), intValue(cacheCreation["ephemeral_5m_input_tokens"]))
	cacheWrite := cacheWriteTotal
	hasCacheWriteBreakdown := mapHasAny(candidate, "cache_creation_1h_input_tokens", "cache_creation_5m_input_tokens") || mapHasAny(cacheCreation, "ephemeral_1h_input_tokens", "ephemeral_5m_input_tokens")
	if hasCacheWriteBreakdown {
		// 缓存创建总数包含 5 分钟和 1 小时两种创建；计费记录把 5 分钟存入 CacheWrite，1 小时单列。
		if hasCacheWriteTotal && cacheWriteTotal != cacheWrite5m+cacheWrite1h {
			invalidCacheField = true
		}
		cacheWrite = cacheWrite5m
	}

	// OpenAI Chat/Responses 的缓存维度是总 prompt/input 的子集，必须用 total - categorized 得到未缓存输入。
	// Anthropic 的 input_tokens 本身是未缓存输入，缓存维度为额外且互斥的 Token，必须相加。
	uncached := 0
	inputTotal := 0
	inputReported := false
	inputInvalid := invalidInputTotal || invalidCacheField
	if semantics == usageSemanticsAnthropicAdditional && hasInputTokens && !inputInvalid {
		uncached = inputTokens
		inputTotal = uncached + cacheRead + cacheWrite + cacheWrite1h
		inputReported = true
	} else if semantics == usageSemanticsOpenAITotal && !inputInvalid {
		total := 0
		switch {
		case hasPromptTokens:
			total = promptTokens
		case hasInputTokens:
			total = inputTokens
		default:
			inputInvalid = inputFieldPresent
		}
		categorized := cacheRead + cacheWrite + cacheWrite1h
		if categorized > total || cacheMiss > total-categorized {
			inputInvalid = true
		} else if hasPromptTokens || hasInputTokens {
			uncached = total - categorized
			inputTotal = total
			inputReported = true
		}
	}
	if inputInvalid {
		// 归零整个输入组而不是保留局部缓存数，确保 Input == 各输入维度之和始终成立。
		inputTotal, uncached, cacheRead, cacheWrite, cacheWrite1h, inputReported = 0, 0, 0, 0, 0, false
	}
	return tokenUsage{
		Input: inputTotal, UncachedInput: uncached, CacheRead: cacheRead, CacheWrite: cacheWrite, CacheWrite1h: cacheWrite1h, Output: outputTokens,
		InputReported: inputReported, OutputReported: (hasCompletionTokens || hasOutputTokens) && !invalidOutput,
		InputInvalid: inputInvalid && inputFieldPresent, OutputInvalid: invalidOutput && outputFieldPresent,
	}
}

/**
 * parseNewAPIBillingUsage 解析 NewAPI 在 billing_usage 下封装的原始提供商 usage。
 * source 与 semantic 必须成对匹配；只信任对应原始协议的字段，避免代理层错误映射导致按错误语义结算。
 * @param candidate 含有可选 billing_usage 的 usage 对象。
 * @return 解析结果及该对象是否确实使用了 NewAPI 格式。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func parseNewAPIBillingUsage(candidate map[string]any) (tokenUsage, bool) {
	billingUsage := mapValue(candidate["billing_usage"])
	if billingUsage == nil {
		return tokenUsage{}, false
	}

	source := strings.TrimSpace(stringValue(billingUsage["source"]))
	semantic := strings.TrimSpace(stringValue(billingUsage["semantic"]))
	estimated := false
	if value, exists := billingUsage["estimated"]; exists {
		var ok bool
		estimated, ok = value.(bool)
		if !ok {
			return tokenUsage{}, false
		}
	}
	var usage tokenUsage
	var ok bool
	switch {
	case (strings.EqualFold(source, "oai_chat") || strings.EqualFold(source, "oai_responses")) && strings.EqualFold(semantic, "openai"):
		candidate := mapValue(billingUsage["openai_usage"])
		if candidate == nil {
			return tokenUsage{}, false
		}
		usage, ok = parseUsageCandidate(candidate, usageSemanticsOpenAITotal), true
	case strings.EqualFold(source, "claude_messages") && strings.EqualFold(semantic, "anthropic"):
		candidate := mapValue(billingUsage["claude_usage"])
		if candidate == nil {
			return tokenUsage{}, false
		}
		usage, ok = parseUsageCandidate(candidate, usageSemanticsAnthropicAdditional), true
	case strings.EqualFold(source, "gemini_chat") && strings.EqualFold(semantic, "gemini"):
		metadata := mapValue(billingUsage["gemini_usage_metadata"])
		if metadata == nil {
			return tokenUsage{}, false
		}
		usage, ok = parseNewAPIGeminiBillingUsage(metadata)
	default:
		return tokenUsage{}, false
	}
	usage.Estimated = usage.Estimated || estimated
	return usage, ok
}

/**
 * parseNewAPIGeminiBillingUsage 将 Gemini usageMetadata 映射到统一 Token 明细。
 * prompt 与 toolUsePrompt 都属于输入，candidates 与 thoughts 都属于输出；cachedContent 是输入的子集，
 * 因此未缓存输入为 input - cacheRead，缓存数超过输入总数时整组输入无效。
 * @param metadata Gemini usageMetadata 对象。
 * @return 解析后的 usage 和是否读取成功。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func parseNewAPIGeminiBillingUsage(metadata map[string]any) (tokenUsage, bool) {
	fields := []string{"promptTokenCount", "toolUsePromptTokenCount", "candidatesTokenCount", "thoughtsTokenCount", "cachedContentTokenCount", "totalTokenCount"}
	for _, field := range fields {
		if hasInvalidIntField(metadata, field) {
			return tokenUsage{}, false
		}
	}
	inputReported := mapHasAny(metadata, "promptTokenCount", "toolUsePromptTokenCount")
	outputReported := mapHasAny(metadata, "candidatesTokenCount", "thoughtsTokenCount")
	input, ok := addTokenCounts(intValue(metadata["promptTokenCount"]), intValue(metadata["toolUsePromptTokenCount"]))
	if !ok {
		return tokenUsage{}, false
	}
	output, ok := addTokenCounts(intValue(metadata["candidatesTokenCount"]), intValue(metadata["thoughtsTokenCount"]))
	if !ok {
		return tokenUsage{}, false
	}
	cacheRead := intValue(metadata["cachedContentTokenCount"])
	if cacheRead > input {
		return tokenUsage{InputInvalid: inputReported}, true
	}
	return tokenUsage{
		Input: input, UncachedInput: input - cacheRead, CacheRead: cacheRead, Output: output,
		InputReported: inputReported, OutputReported: outputReported,
	}, true
}

/**
 * conflictingNonzeroAliases 执行该名称对应的业务处理逻辑。
 * @param first 本次操作需要使用的输入参数。
 * @param hasFirst 本次操作需要使用的输入参数。
 * @param second 本次操作需要使用的输入参数。
 * @param hasSecond 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func conflictingNonzeroAliases(first int, hasFirst bool, second int, hasSecond bool) bool {
	return hasFirst && hasSecond && first > 0 && second > 0 && first != second
}

/**
 * addTokenCounts 相加两个非负 Token 字段，并在转换为 Go int 前拒绝溢出。
 * @param first 第一个 Token 计数。
 * @param second 第二个 Token 计数。
 * @return 相加结果和是否安全表示。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func addTokenCounts(first, second int) (int, bool) {
	if first > maxInt()-second {
		return 0, false
	}
	return first + second, true
}

/**
 * breakdown 执行该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (u tokenUsage) breakdown() billing.TokenBreakdown {
	return billing.TokenBreakdown{UncachedInput: u.UncachedInput, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, CacheWrite1h: u.CacheWrite1h, Output: u.Output}
}

/**
 * applyUsageFallback 将缺失的上游 usage 维度显式设为零并标记估算。
 * 预占阶段的请求字节数和最大输出只用于余额校验，绝不能在最终费用缺失时作为实际 Token 代替品。
 * @param usage 已解析的上游 usage。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func applyUsageFallback(usage tokenUsage, _, _ int, _ billing.RateCard) tokenUsage {
	if !usage.InputReported {
		usage.Input = 0
		usage.UncachedInput = 0
		usage.CacheRead = 0
		usage.CacheWrite = 0
		usage.CacheWrite1h = 0
		usage.Estimated = true
	}
	if !usage.OutputReported {
		usage.Output = 0
		usage.Estimated = true
	}
	return usage
}

/**
 * estimateInputTokens 为余额预占估算请求输入规模，而非最终计费。
 * 公式为 ceil(JSON字节数 / 4) + 64：四字节近似提供商无关的文本 Token 密度，64 覆盖协议字段和少量结构开销。
 * @param body 发给上游前的 JSON 请求正文。
 * @return 至少为 1 的预估输入 Token 数。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func estimateInputTokens(body []byte) int {
	// (len(body)+3)/4 是对非负整数除以 4 的向上取整；最终金额始终使用上游确认 usage。
	const protocolOverheadTokens = 64
	return max(1, (len(body)+3)/4+protocolOverheadTokens)
}

/**
 * sha256String 执行该名称对应的业务处理逻辑。
 * @param value 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func sha256String(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest)
}

/**
 * gatewayRequestHash 执行该名称对应的业务处理逻辑。
 * @param endpoint 本次操作需要使用的输入参数。
 * @param body 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func gatewayRequestHash(endpoint string, body []byte) string {
	digest := sha256.New()
	_, _ = io.WriteString(digest, endpoint)
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(body)
	return fmt.Sprintf("%x", digest.Sum(nil))
}

/**
 * writeOperationReplay 执行该名称对应的业务处理逻辑。
 * @param w 本次操作需要使用的输入参数。
 * @param operation 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func writeOperationReplay(w http.ResponseWriter, operation billing.Operation) {
	w.Header().Set(requestid.Header, operation.RequestID.String())
	switch operation.Status {
	case billing.OperationCompleted:
		writeJSON(w, http.StatusOK, map[string]any{"id": operation.RequestID.String(), "object": "novro.operation", "status": "completed"})
	case billing.OperationFailed:
		writeError(w, http.StatusBadGateway, "operation_failed", "该幂等请求此前已失败，不会重复调用上游")
	case billing.OperationPendingUnknown:
		writeError(w, http.StatusConflict, "operation_result_unknown", "该幂等请求的上游结果仍无法确认，不会重复调用上游")
	default:
		w.Header().Set("Retry-After", "2")
		writeError(w, http.StatusConflict, "operation_in_progress", "该幂等请求正在处理或等待结算，不会重复调用上游")
	}
}

/**
 * rateCardFor 优先读取本次请求已固定的价格快照，并兼容旧路由价格字段回退。
 * @param route 已解析提供商和模型价格的路由。
 * @return 网关计费计算使用的六维费率卡。
 * @author Gao Hongshun
 * @date 2026-08-14
 */
func rateCardFor(route modelroute.Resolved) billing.RateCard {
	prices := route.UpstreamModel.Prices
	if route.ResolvedPrices != nil {
		prices = *route.ResolvedPrices
	}
	return billing.RateCard{InputMicros: prices.InputMicros, OutputMicros: prices.OutputMicros, CacheReadMicros: prices.CacheReadMicros, CacheWriteMicros: prices.CacheWriteMicros, CacheWrite1hMicros: prices.CacheWrite1hMicros, RequestMicros: prices.RequestMicros}
}

/**
 * readMaxOutput 读取不同 OpenAI 兼容 endpoint 的最大输出字段，并在缺失时写入默认值。
 * chat_completions 优先新字段 max_completion_tokens 再兼容 max_tokens；Responses 仅使用 max_output_tokens。
 * @param payload 将发往上游且可能被补入默认字段的 JSON 请求对象。
 * @param endpoint 当前网关 endpoint。
 * @return 最大输出 Token 及字段是否为正整数。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func readMaxOutput(payload map[string]any, endpoint string) (int, bool) {
	keys := []string{"max_tokens"}
	if endpoint == "chat_completions" {
		keys = []string{"max_completion_tokens", "max_tokens"}
	}
	if endpoint == "responses" {
		keys = []string{"max_output_tokens"}
	}
	for _, key := range keys {
		if raw, exists := payload[key]; exists {
			value, valid := parseIntValue(raw)
			return value, valid && value > 0
		}
	}
	key := "max_tokens"
	if endpoint == "responses" {
		key = "max_output_tokens"
	}
	payload[key] = defaultMaxOutputTokens
	return defaultMaxOutputTokens, true
}

/**
 * protocolSupports 执行该名称对应的业务处理逻辑。
 * @param protocol 本次操作需要使用的输入参数。
 * @param endpoint 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func protocolSupports(protocol provider.Protocol, endpoint string) bool {
	if endpoint == "messages" {
		return protocol == provider.ProtocolAnthropic
	}
	return protocol == provider.ProtocolOpenAI
}

/**
 * buildUpstreamURL 执行该名称对应的业务处理逻辑。
 * @param base 本次操作需要使用的输入参数。
 * @param protocol 本次操作需要使用的输入参数。
 * @param endpoint 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func buildUpstreamURL(base string, protocol provider.Protocol, endpoint string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("invalid upstream URL")
	}
	path := "/" + strings.ReplaceAll(endpoint, "_", "/")
	if endpoint == "responses" {
		path = "/responses"
	}
	if protocol == provider.ProtocolAnthropic {
		path = "/v1/messages"
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if protocol == provider.ProtocolAnthropic && strings.HasSuffix(basePath, "/v1") {
		path = strings.TrimPrefix(path, "/v1")
	}
	parsed.Path = basePath + path
	parsed.RawPath, parsed.RawQuery, parsed.Fragment = "", "", ""
	return parsed.String(), nil
}

/**
 * selfReferentialUpstream 执行该名称对应的业务处理逻辑。
 * @param request 本次操作需要使用的输入参数。
 * @param base 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func selfReferentialUpstream(request *http.Request, base string) bool {
	if request == nil || strings.TrimSpace(request.Host) == "" {
		return false
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	requestHost := request.Host
	if host, _, splitErr := net.SplitHostPort(requestHost); splitErr == nil {
		requestHost = host
	} else {
		requestHost = strings.Trim(requestHost, "[]")
	}
	return strings.EqualFold(parsed.Hostname(), requestHost)
}

/**
 * newOutboundClient 执行该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func newOutboundClient() *http.Client {
	return upstreamhttp.NewClient()
}

/**
 * unsafeUpstreamIP 执行该名称对应的业务处理逻辑。
 * @param ip 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func unsafeUpstreamIP(ip net.IP) bool {
	return upstreamhttp.UnsafeIP(ip)
}

/**
 * readLimited 执行该名称对应的业务处理逻辑。
 * @param reader 本次操作需要使用的输入参数。
 * @param limit 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return io.ReadAll(reader)
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, fmt.Errorf("body exceeds limit")
	}
	return body, nil
}

/**
 * copyResponseHeaders 执行该名称对应的业务处理逻辑。
 * @param target 本次操作需要使用的输入参数。
 * @param source 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func copyResponseHeaders(target, source http.Header) {
	for _, key := range []string{"Content-Type", "Content-Encoding", "OpenAI-Processing-Ms", "X-Request-ID", "Request-ID"} {
		if value := source.Get(key); value != "" {
			target.Set(key, value)
		}
	}
	target.Set("Cache-Control", "no-store")
}

/**
 * mapValue 执行该名称对应的业务处理逻辑。
 * @param value 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func mapValue(value any) map[string]any { result, _ := value.(map[string]any); return result }

/**
 * stringValue 执行该名称对应的业务处理逻辑。
 * @param value 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func stringValue(value any) string { result, _ := value.(string); return result }

/**
 * intValue 执行该名称对应的业务处理逻辑。
 * @param value 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func intValue(value any) int {
	number, ok := parseIntValue(value)
	if !ok {
		return 0
	}
	return number
}

/**
 * intField 执行该名称对应的业务处理逻辑。
 * @param values 本次操作需要使用的输入参数。
 * @param key 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func intField(values map[string]any, key string) (int, bool) {
	value, exists := values[key]
	if !exists {
		return 0, false
	}
	return parseIntValue(value)
}

/**
 * parseIntValue 执行该名称对应的业务处理逻辑。
 * @param value 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func parseIntValue(value any) (int, bool) {
	if encoded, ok := value.(json.Number); ok {
		if integer, err := strconv.ParseInt(encoded.String(), 10, 0); err == nil {
			if integer < 0 {
				return 0, false
			}
			return int(integer), true
		}
		parsed, err := strconv.ParseFloat(encoded.String(), 64)
		if err != nil {
			return 0, false
		}
		value = parsed
	}
	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number || number < 0 || number >= float64(maxInt()) {
		return 0, false
	}
	return int(number), true
}

/**
 * maxInt 执行该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func maxInt() int {
	return int(^uint(0) >> 1)
}

/**
 * hasInvalidIntField 执行该名称对应的业务处理逻辑。
 * @param values 本次操作需要使用的输入参数。
 * @param key 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func hasInvalidIntField(values map[string]any, key string) bool {
	value, exists := values[key]
	if !exists {
		return false
	}
	_, valid := parseIntValue(value)
	return !valid
}

/**
 * hasInvalidMapField 执行该名称对应的业务处理逻辑。
 * @param values 本次操作需要使用的输入参数。
 * @param key 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func hasInvalidMapField(values map[string]any, key string) bool {
	value, exists := values[key]
	if !exists || value == nil {
		return false
	}
	_, valid := value.(map[string]any)
	return !valid
}

/**
 * mapHasAny 执行该名称对应的业务处理逻辑。
 * @param values 本次操作需要使用的输入参数。
 * @param keys 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func mapHasAny(values map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, exists := values[key]; exists {
			return true
		}
	}
	return false
}

/**
 * writeJSON 执行该名称对应的业务处理逻辑。
 * @param w 本次操作需要使用的输入参数。
 * @param status 本次操作需要使用的输入参数。
 * @param value 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

/**
 * writeError 执行该名称对应的业务处理逻辑。
 * @param w 本次操作需要使用的输入参数。
 * @param status 本次操作需要使用的输入参数。
 * @param code 本次操作需要使用的输入参数。
 * @param message 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func writeError(w http.ResponseWriter, status int, code, message string) {
	value := map[string]any{"error": map[string]any{"message": message, "type": "novro_error", "code": code}}
	if id := requestid.ResponseID(w); id != uuid.Nil {
		value["request_id"] = id.String()
	}
	writeJSON(w, status, value)
}
