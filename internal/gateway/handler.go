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
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/apikey"
	"github.com/novro-gateway/novro/internal/billing"
	"github.com/novro-gateway/novro/internal/modelroute"
	"github.com/novro-gateway/novro/internal/provider"
	"github.com/novro-gateway/novro/internal/requestid"
	"github.com/novro-gateway/novro/internal/upstreamhttp"
)

const (
	maxGatewayBodyBytes    = 10 << 20
	maxUpstreamBodyBytes   = 32 << 20
	defaultMaxOutputTokens = 4096
	maxOutputTokens        = 1_000_000
)

type KeyAuthenticator interface {
	Authenticate(context.Context, string) (apikey.Actor, error)
}
type RouteService interface {
	Resolve(context.Context, string) (modelroute.Resolved, error)
	ListActive(context.Context) ([]modelroute.Record, error)
}
type BillingService interface {
	Reserve(context.Context, uuid.UUID, uuid.UUID, int64, string) error
	Refund(context.Context, uuid.UUID, uuid.UUID, int64, string) error
	Finalize(context.Context, billing.UsageInput) error
}

type Dependencies struct {
	APIKeys KeyAuthenticator
	Routes  RouteService
	Billing BillingService
	Client  *http.Client
	Logger  *slog.Logger
}

type Handler struct {
	apiKeys               KeyAuthenticator
	routes                RouteService
	billing               BillingService
	client                *http.Client
	logger                *slog.Logger
	now                   func() time.Time
	settlementRetryDelays []time.Duration
}

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
		apiKeys: deps.APIKeys, routes: deps.Routes, billing: deps.Billing, client: client, logger: logger,
		now: func() time.Time { return time.Now().UTC() }, settlementRetryDelays: []time.Duration{100 * time.Millisecond, 300 * time.Millisecond},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, id := requestid.Ensure(r.Context())
	r = r.WithContext(ctx)
	w.Header().Set(requestid.Header, id.String())
	actor, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
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

func (h *Handler) listModels(w http.ResponseWriter, r *http.Request, _ apikey.Actor) {
	routes, err := h.routes.ListActive(r.Context())
	if err != nil {
		h.logger.Error("list gateway models", "request_id", requestid.FromContext(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
	}
	data := make([]map[string]any, 0, len(routes))
	for _, route := range routes {
		data = append(data, map[string]any{"id": route.PublicName, "object": "model", "created": route.CreatedAt.Unix(), "owned_by": route.Provider.Code})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (h *Handler) proxy(w http.ResponseWriter, r *http.Request, actor apikey.Actor, endpoint string) {
	startedAt := h.now()
	body, err := readLimited(r.Body, maxGatewayBodyBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求体无效或过大")
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求体必须是 JSON 对象")
		return
	}
	model, ok := payload["model"].(string)
	if !ok || strings.TrimSpace(model) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "model 不能为空")
		return
	}
	route, err := h.routes.Resolve(r.Context(), model)
	if err != nil {
		if errors.Is(err, modelroute.ErrNotFound) {
			writeError(w, http.StatusNotFound, "model_not_found", "模型不存在或当前不可用")
			return
		}
		h.logger.Error("resolve gateway model", "request_id", requestid.FromContext(r.Context()), "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
		return
	}
	if !protocolSupports(route.Provider.Protocol, endpoint) {
		writeError(w, http.StatusBadRequest, "unsupported_endpoint", "该模型不支持当前 API 协议")
		return
	}
	if route.UpstreamModel == nil || route.UpstreamModelID == nil || actor.User.BillingGroup == nil || actor.User.BillingGroupID == nil {
		h.logger.Error("resolve gateway billing context", "request_id", requestid.FromContext(r.Context()), "model", route.PublicName)
		writeError(w, http.StatusInternalServerError, "billing_configuration_error", "模型计费配置暂时不可用")
		return
	}
	payload["model"] = route.UpstreamName
	stream, _ := payload["stream"].(bool)
	maximum, ok := readMaxOutput(payload, endpoint)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "最大输出 token 参数无效")
		return
	}
	if endpoint == "chat_completions" && stream {
		options, _ := payload["stream_options"].(map[string]any)
		if options == nil {
			options = make(map[string]any)
		}
		options["include_usage"] = true
		payload["stream_options"] = options
	}
	upstreamBody, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求体无效")
		return
	}
	inputEstimate := len(upstreamBody) + 256
	rates := rateCardFor(route)
	reservation, err := billing.EstimateReservation(inputEstimate, maximum, rates, actor.User.BillingGroup.MultiplierBPS)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "billing_configuration_error", "模型计费配置暂时不可用")
		return
	}
	reserved := reservation.CostMicros
	requestID := requestid.FromContext(r.Context())
	if requestID == uuid.Nil {
		requestID = requestid.New()
	}
	if reserved > 0 {
		if err := h.billing.Reserve(r.Context(), actor.User.ID, requestID, reserved, "调用 "+route.PublicName); err != nil {
			if errors.Is(err, billing.ErrInsufficientBalance) {
				writeError(w, http.StatusPaymentRequired, "insufficient_balance", "余额不足")
				return
			}
			h.logger.Error("reserve gateway balance", "request_id", requestID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
			return
		}
	}
	upstreamURL, err := buildUpstreamURL(route.BaseURL, route.Provider.Protocol, endpoint)
	if err != nil {
		h.refund(actor.User.ID, requestID, reserved, "上游地址无效")
		writeError(w, http.StatusBadGateway, "upstream_unavailable", "上游服务暂时不可用")
		return
	}
	upstreamRequest, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstreamURL, bytes.NewReader(upstreamBody))
	if err != nil {
		h.refund(actor.User.ID, requestID, reserved, "创建上游请求失败")
		writeError(w, http.StatusBadGateway, "upstream_unavailable", "上游服务暂时不可用")
		return
	}
	upstreamRequest.Header.Set("Content-Type", "application/json")
	upstreamRequest.Header.Set("Accept", "application/json, text/event-stream")
	upstreamRequest.Header.Set("User-Agent", "Novro-Gateway/1")
	if route.Provider.Protocol == provider.ProtocolAnthropic {
		upstreamRequest.Header.Set("X-API-Key", route.APIKey)
		version := strings.TrimSpace(r.Header.Get("Anthropic-Version"))
		if version == "" {
			version = "2023-06-01"
		}
		upstreamRequest.Header.Set("Anthropic-Version", version)
		if beta := strings.TrimSpace(r.Header.Get("Anthropic-Beta")); beta != "" {
			upstreamRequest.Header.Set("Anthropic-Beta", beta)
		}
	} else {
		upstreamRequest.Header.Set("Authorization", "Bearer "+route.APIKey)
	}
	response, err := h.client.Do(upstreamRequest)
	if err != nil {
		h.refund(actor.User.ID, requestID, reserved, "上游请求失败")
		h.logger.Warn("gateway upstream request failed", "request_id", requestID, "provider", route.Provider.Code, "error", err)
		writeError(w, http.StatusBadGateway, "upstream_unavailable", "上游服务暂时不可用")
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		h.refund(actor.User.ID, requestID, reserved, "上游拒绝请求")
		h.logger.Warn("gateway upstream rejected request", "request_id", requestID, "provider", route.Provider.Code, "status", response.StatusCode)
		writeError(w, http.StatusBadGateway, "upstream_error", "上游服务未能完成请求")
		return
	}
	if stream || strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		h.streamResponse(w, r, response, actor, route, requestID, endpoint, reserved, inputEstimate, maximum, startedAt)
		return
	}
	h.bufferedResponse(w, r, response, actor, route, requestID, endpoint, reserved, inputEstimate, maximum, startedAt)
}

func (h *Handler) bufferedResponse(w http.ResponseWriter, r *http.Request, response *http.Response, actor apikey.Actor, route modelroute.Resolved, requestID uuid.UUID, endpoint string, reserved int64, inputEstimate, outputMaximum int, startedAt time.Time) {
	body, err := readLimited(response.Body, maxUpstreamBodyBytes)
	if err != nil {
		h.refund(actor.User.ID, requestID, reserved, "上游响应无效")
		writeError(w, http.StatusBadGateway, "upstream_error", "上游响应无效")
		return
	}
	usage := parseUsage(body)
	rates := rateCardFor(route)
	usage = applyUsageFallback(usage, inputEstimate, min(len(body)+128, outputMaximum), rates)
	quote, err := billing.CalculateCost(usage.breakdown(), rates, actor.User.BillingGroup.MultiplierBPS)
	if err != nil {
		h.refund(actor.User.ID, requestID, reserved, "计费配置无效")
		writeError(w, http.StatusInternalServerError, "billing_error", "调用已完成但计费记录失败")
		return
	}
	if err := h.finalize(r.Context(), actor, route, requestID, endpoint, reserved, quote, usage, startedAt); err != nil {
		writeError(w, http.StatusInternalServerError, "billing_error", "调用已完成但计费记录失败")
		return
	}
	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Set("X-Novro-Request-ID", requestID.String())
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(body)
}

func (h *Handler) streamResponse(w http.ResponseWriter, r *http.Request, response *http.Response, actor apikey.Actor, route modelroute.Resolved, requestID uuid.UUID, endpoint string, reserved int64, inputEstimate, outputMaximum int, startedAt time.Time) {
	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Novro-Request-ID", requestID.String())
	w.WriteHeader(response.StatusCode)
	flusher, _ := w.(http.Flusher)
	usage := tokenUsage{}
	completed := false
	reader := bufio.NewReaderSize(response.Body, 64<<10)
	line := make([]byte, 0, 64<<10)
	lineTooLarge := false
	var relayErr error
	for {
		fragment, isPrefix, readErr := reader.ReadLine()
		if len(fragment) == 0 && readErr != nil && len(line) == 0 && !lineTooLarge {
			if readErr != io.EOF {
				relayErr = readErr
			}
			break
		}
		if !lineTooLarge {
			if len(line)+len(fragment) <= maxUpstreamBodyBytes {
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
			if streamCompleted(endpoint, data) {
				completed = true
			}
			if !bytes.Equal(data, []byte("[DONE]")) {
				usage.merge(parseUsage(data))
			}
		}
		if len(fragment) > 0 {
			written, writeErr := w.Write(fragment)
			if writeErr == nil && written != len(fragment) {
				writeErr = io.ErrShortWrite
			}
			if writeErr != nil {
				relayErr = writeErr
				break
			}
		}
		if !isPrefix {
			if _, writeErr := w.Write([]byte{'\n'}); writeErr != nil {
				relayErr = writeErr
				break
			}
			line = line[:0]
			lineTooLarge = false
		}
		if flusher != nil {
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
	usage = applyUsageFallback(usage, inputEstimate, outputMaximum, rateCardFor(route))
	if !completed {
		usage.Estimated = true
	}
	quote, err := billing.CalculateCost(usage.breakdown(), rateCardFor(route), actor.User.BillingGroup.MultiplierBPS)
	if err != nil {
		h.logger.Error("calculate streamed usage", "request_id", requestID, "error", err)
		return
	}
	_ = h.finalize(r.Context(), actor, route, requestID, endpoint, reserved, quote, usage, startedAt)
}

func streamCompleted(endpoint string, data []byte) bool {
	if bytes.Equal(data, []byte("[DONE]")) {
		return true
	}
	var event struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(data, &event) != nil {
		return false
	}
	switch endpoint {
	case "responses":
		return event.Type == "response.completed"
	case "messages":
		return event.Type == "message_stop"
	default:
		return false
	}
}

func (h *Handler) finalize(ctx context.Context, actor apikey.Actor, route modelroute.Resolved, requestID uuid.UUID, endpoint string, reserved int64, quote billing.Quote, usage tokenUsage, startedAt time.Time) error {
	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	input := billing.UsageInput{UserID: actor.User.ID, APIKeyID: actor.APIKey.ID, ModelRouteID: route.ID, UpstreamModelID: route.UpstreamModelID, BillingGroupID: actor.User.BillingGroupID, RequestID: requestID, Endpoint: endpoint,
		InputTokens: usage.Input, Tokens: usage.breakdown(), OutputTokens: usage.Output, Rates: rateCardFor(route), BaseCostMicros: quote.BaseCostMicros, MultiplierBPS: actor.User.BillingGroup.MultiplierBPS, CostMicros: quote.CostMicros, ReservedMicros: reserved,
		Estimated: usage.Estimated, UpstreamRequestID: usage.UpstreamID, ModelName: route.PublicName, UpstreamModelName: route.UpstreamModel.UpstreamName, BillingGroupCode: actor.User.BillingGroup.Code, BillingGroupName: actor.User.BillingGroup.DisplayName, CalculationVersion: billing.CalculationVersion, CreatedAt: startedAt, FinishedAt: h.now()}
	err, attempts := h.retrySettlement(finalizeCtx, func() error { return h.billing.Finalize(finalizeCtx, input) })
	if err != nil {
		h.logger.Error("finalize gateway usage", "request_id", requestID, "attempts", attempts, "error", err)
	}
	return err
}

func (h *Handler) refund(userID, requestID uuid.UUID, amount int64, description string) {
	if amount <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err, attempts := h.retrySettlement(ctx, func() error { return h.billing.Refund(ctx, userID, requestID, amount, description) }); err != nil {
		h.logger.Error("refund gateway reservation", "request_id", requestID, "attempts", attempts, "error", err)
	}
}

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

func retryableSettlementError(err error) bool {
	return err != nil &&
		!errors.Is(err, billing.ErrInvalidInput) && !errors.Is(err, billing.ErrWalletNotFound) &&
		!errors.Is(err, billing.ErrInsufficientBalance) && !errors.Is(err, billing.ErrRequestConflict) &&
		!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

type tokenUsage struct {
	Input, UncachedInput, CacheRead, CacheWrite, CacheWrite1h, Output int
	InputReported, OutputReported                                     bool
	Estimated                                                         bool
	UpstreamID                                                        string
}

func (u *tokenUsage) merge(other tokenUsage) {
	if other.Input > u.Input {
		u.Input = other.Input
	}
	if other.UncachedInput > u.UncachedInput {
		u.UncachedInput = other.UncachedInput
	}
	if other.CacheRead > u.CacheRead {
		u.CacheRead = other.CacheRead
	}
	if other.CacheWrite > u.CacheWrite {
		u.CacheWrite = other.CacheWrite
	}
	if other.CacheWrite1h > u.CacheWrite1h {
		u.CacheWrite1h = other.CacheWrite1h
	}
	if other.Output > u.Output {
		u.Output = other.Output
	}
	u.InputReported = u.InputReported || other.InputReported
	u.OutputReported = u.OutputReported || other.OutputReported
	if other.UpstreamID != "" {
		u.UpstreamID = other.UpstreamID
	}
	u.Estimated = u.Estimated || other.Estimated
}

func parseUsage(body []byte) tokenUsage {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return tokenUsage{}
	}
	usage := tokenUsage{UpstreamID: stringValue(root["id"])}
	for _, candidate := range []map[string]any{mapValue(root["usage"]), mapValue(mapValue(root["response"])["usage"]), mapValue(mapValue(root["message"])["usage"])} {
		if candidate == nil {
			continue
		}
		usage.merge(parseUsageCandidate(candidate))
	}
	if response := mapValue(root["response"]); response != nil && usage.UpstreamID == "" {
		usage.UpstreamID = stringValue(response["id"])
	}
	if message := mapValue(root["message"]); message != nil && usage.UpstreamID == "" {
		usage.UpstreamID = stringValue(message["id"])
	}
	return usage
}

func parseUsageCandidate(candidate map[string]any) tokenUsage {
	promptTokens, hasPromptTokens := intField(candidate, "prompt_tokens")
	inputTokens, hasInputTokens := intField(candidate, "input_tokens")
	completionTokens, hasCompletionTokens := intField(candidate, "completion_tokens")
	outputTokens, hasOutputTokens := intField(candidate, "output_tokens")
	outputTokens = max(completionTokens, outputTokens)
	details := mapValue(candidate["prompt_tokens_details"])
	if details == nil {
		details = mapValue(candidate["input_tokens_details"])
	}
	cacheRead := max(intValue(candidate["prompt_cache_hit_tokens"]), intValue(candidate["cached_tokens"]), intValue(details["cached_tokens"]), intValue(candidate["cache_read_input_tokens"]))
	cacheMiss := intValue(candidate["prompt_cache_miss_tokens"])
	cacheWriteTotal := intValue(candidate["cache_creation_input_tokens"])
	cacheCreation := mapValue(candidate["cache_creation"])
	cacheWrite1h := max(intValue(candidate["cache_creation_1h_input_tokens"]), intValue(cacheCreation["ephemeral_1h_input_tokens"]))
	cacheWrite5m := max(intValue(candidate["cache_creation_5m_input_tokens"]), intValue(cacheCreation["ephemeral_5m_input_tokens"]))
	cacheWrite := cacheWriteTotal
	if cacheWrite5m+cacheWrite1h > 0 {
		cacheWrite = max(cacheWrite5m, cacheWriteTotal-cacheWrite1h)
	}

	// OpenAI-compatible providers use prompt_tokens as a total and expose cached
	// tokens as a subset. Anthropic uses input_tokens for regular input and reports
	// cache reads and cache creation as additional, mutually exclusive dimensions.
	uncached := 0
	inputTotal := 0
	if hasPromptTokens {
		if cacheMiss > 0 {
			uncached = cacheMiss
		} else {
			uncached = max(0, promptTokens-cacheRead)
		}
		inputTotal = uncached + cacheRead
		if inputTotal < promptTokens {
			uncached += promptTokens - inputTotal
			inputTotal = promptTokens
		}
		if inputTotal > promptTokens {
			promptTokens = inputTotal
		}
		// Cache creation is not part of any currently supported OpenAI-compatible
		// prompt total. Preserve it if a provider returns the fields explicitly.
		inputTotal = promptTokens + cacheWrite + cacheWrite1h
	} else if hasInputTokens {
		uncached = inputTokens
		inputTotal = uncached + cacheRead + cacheWrite + cacheWrite1h
	} else {
		uncached = cacheMiss
		inputTotal = uncached + cacheRead + cacheWrite + cacheWrite1h
	}
	return tokenUsage{Input: inputTotal, UncachedInput: uncached, CacheRead: cacheRead, CacheWrite: cacheWrite, CacheWrite1h: cacheWrite1h, Output: outputTokens, InputReported: hasPromptTokens || hasInputTokens, OutputReported: hasCompletionTokens || hasOutputTokens}
}

func (u tokenUsage) breakdown() billing.TokenBreakdown {
	return billing.TokenBreakdown{UncachedInput: u.UncachedInput, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, CacheWrite1h: u.CacheWrite1h, Output: u.Output}
}

func applyUsageFallback(usage tokenUsage, input, output int, rates billing.RateCard) tokenUsage {
	if !usage.InputReported {
		estimated := estimatedUsage(input, 0, rates)
		usage.Input = estimated.Input
		usage.UncachedInput = estimated.UncachedInput
		usage.CacheRead = estimated.CacheRead
		usage.CacheWrite = estimated.CacheWrite
		usage.CacheWrite1h = estimated.CacheWrite1h
		usage.Estimated = true
	}
	if !usage.OutputReported {
		usage.Output = output
		usage.Estimated = true
	}
	return usage
}

func estimatedUsage(input, output int, rates billing.RateCard) tokenUsage {
	usage := tokenUsage{Input: input, UncachedInput: input, Output: output, Estimated: true}
	highest := rates.InputMicros
	if rates.CacheReadMicros > highest {
		highest = rates.CacheReadMicros
		usage.UncachedInput, usage.CacheRead = 0, input
	}
	if rates.CacheWriteMicros > highest {
		highest = rates.CacheWriteMicros
		usage.UncachedInput, usage.CacheRead, usage.CacheWrite = 0, 0, input
	}
	if rates.CacheWrite1hMicros > highest {
		usage.UncachedInput, usage.CacheRead, usage.CacheWrite, usage.CacheWrite1h = 0, 0, 0, input
	}
	return usage
}

func rateCardFor(route modelroute.Resolved) billing.RateCard {
	prices := route.UpstreamModel.Prices
	return billing.RateCard{InputMicros: prices.InputMicros, OutputMicros: prices.OutputMicros, CacheReadMicros: prices.CacheReadMicros, CacheWriteMicros: prices.CacheWriteMicros, CacheWrite1hMicros: prices.CacheWrite1hMicros, RequestMicros: prices.RequestMicros}
}

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
			value := intValue(raw)
			return value, value > 0 && value <= maxOutputTokens
		}
	}
	key := "max_tokens"
	if endpoint == "responses" {
		key = "max_output_tokens"
	}
	payload[key] = defaultMaxOutputTokens
	return defaultMaxOutputTokens, true
}

func protocolSupports(protocol provider.Protocol, endpoint string) bool {
	if endpoint == "messages" {
		return protocol == provider.ProtocolAnthropic
	}
	return protocol == provider.ProtocolOpenAI
}

func buildUpstreamURL(base string, protocol provider.Protocol, endpoint string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
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

func newOutboundClient() *http.Client {
	return upstreamhttp.NewClient()
}

func unsafeUpstreamIP(ip net.IP) bool {
	return upstreamhttp.UnsafeIP(ip)
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, fmt.Errorf("body exceeds limit")
	}
	return body, nil
}
func copyResponseHeaders(target, source http.Header) {
	for _, key := range []string{"Content-Type", "Content-Encoding", "OpenAI-Processing-Ms", "X-Request-ID", "Request-ID"} {
		if value := source.Get(key); value != "" {
			target.Set(key, value)
		}
	}
	target.Set("Cache-Control", "no-store")
}
func mapValue(value any) map[string]any { result, _ := value.(map[string]any); return result }
func stringValue(value any) string      { result, _ := value.(string); return result }
func intValue(value any) int {
	number, ok := value.(float64)
	if !ok || number < 0 || number > maxOutputTokens*100 {
		return 0
	}
	return int(number)
}

func intField(values map[string]any, key string) (int, bool) {
	value, exists := values[key]
	if !exists {
		return 0, false
	}
	return intValue(value), true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	value := map[string]any{"error": map[string]any{"message": message, "type": "novro_error", "code": code}}
	if id := requestid.ResponseID(w); id != uuid.Nil {
		value["request_id"] = id.String()
	}
	writeJSON(w, status, value)
}
