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
)

const (
	maxGatewayBodyBytes          = 10 << 20
	maxUpstreamBodyBytes         = 32 << 20
	defaultMaxOutputTokens       = 4096
	maxOutputTokens              = 1_000_000
	priceUnitTokens        int64 = 1_000_000
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
	apiKeys KeyAuthenticator
	routes  RouteService
	billing BillingService
	client  *http.Client
	logger  *slog.Logger
	now     func() time.Time
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
	return &Handler{apiKeys: deps.APIKeys, routes: deps.Routes, billing: deps.Billing, client: client, logger: logger, now: func() time.Time { return time.Now().UTC() }}
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
	reserved := priceForTokens(inputEstimate, route.InputPriceMicros) + priceForTokens(maximum, route.OutputPriceMicros)
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
	if usage.Input == 0 && usage.Output == 0 {
		usage = tokenUsage{Input: inputEstimate, Output: min(len(body)+128, outputMaximum), Estimated: true}
	}
	cost := priceForTokens(usage.Input, route.InputPriceMicros) + priceForTokens(usage.Output, route.OutputPriceMicros)
	if cost > reserved {
		cost = reserved
		usage.Estimated = true
	}
	if err := h.finalize(r.Context(), actor, route, requestID, endpoint, reserved, cost, usage, startedAt); err != nil {
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
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		line := append(append([]byte(nil), scanner.Bytes()...), '\n')
		if bytes.HasPrefix(scanner.Bytes(), []byte("data:")) {
			data := bytes.TrimSpace(bytes.TrimPrefix(scanner.Bytes(), []byte("data:")))
			if !bytes.Equal(data, []byte("[DONE]")) {
				usage.merge(parseUsage(data))
			}
		}
		if _, err := w.Write(line); err != nil {
			break
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	if usage.Input == 0 && usage.Output == 0 {
		usage = tokenUsage{Input: inputEstimate, Output: outputMaximum, Estimated: true}
	}
	cost := priceForTokens(usage.Input, route.InputPriceMicros) + priceForTokens(usage.Output, route.OutputPriceMicros)
	if cost > reserved {
		cost = reserved
		usage.Estimated = true
	}
	if err := h.finalize(r.Context(), actor, route, requestID, endpoint, reserved, cost, usage, startedAt); err != nil {
		h.logger.Error("finalize streamed gateway usage", "request_id", requestID, "error", err)
	}
}

func (h *Handler) finalize(ctx context.Context, actor apikey.Actor, route modelroute.Resolved, requestID uuid.UUID, endpoint string, reserved, cost int64, usage tokenUsage, startedAt time.Time) error {
	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err := h.billing.Finalize(finalizeCtx, billing.UsageInput{UserID: actor.User.ID, APIKeyID: actor.APIKey.ID, ModelRouteID: route.ID, RequestID: requestID, Endpoint: endpoint, InputTokens: usage.Input, OutputTokens: usage.Output, CostMicros: cost, ReservedMicros: reserved, Estimated: usage.Estimated, UpstreamRequestID: usage.UpstreamID, CreatedAt: startedAt, FinishedAt: h.now()})
	if err != nil {
		h.logger.Error("finalize gateway usage", "request_id", requestID, "error", err)
	}
	return err
}

func (h *Handler) refund(userID, requestID uuid.UUID, amount int64, description string) {
	if amount <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.billing.Refund(ctx, userID, requestID, amount, description); err != nil {
		h.logger.Error("refund gateway reservation", "request_id", requestID, "error", err)
	}
}

type tokenUsage struct {
	Input, Output int
	Estimated     bool
	UpstreamID    string
}

func (u *tokenUsage) merge(other tokenUsage) {
	if other.Input > u.Input {
		u.Input = other.Input
	}
	if other.Output > u.Output {
		u.Output = other.Output
	}
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
		usage.Input = max(usage.Input, intValue(candidate["prompt_tokens"]), intValue(candidate["input_tokens"]))
		usage.Output = max(usage.Output, intValue(candidate["completion_tokens"]), intValue(candidate["output_tokens"]))
	}
	if response := mapValue(root["response"]); response != nil && usage.UpstreamID == "" {
		usage.UpstreamID = stringValue(response["id"])
	}
	if message := mapValue(root["message"]); message != nil && usage.UpstreamID == "" {
		usage.UpstreamID = stringValue(message["id"])
	}
	return usage
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
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("parse upstream address: %w", err)
			}
			addresses, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolve upstream host: %w", err)
			}
			for _, ip := range addresses {
				if unsafeUpstreamIP(ip) {
					continue
				}
				connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return connection, nil
				}
				err = dialErr
			}
			if err != nil {
				return nil, fmt.Errorf("dial upstream host: %w", err)
			}
			return nil, fmt.Errorf("upstream host does not resolve to a public address")
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	return &http.Client{Transport: transport}
}

func unsafeUpstreamIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

func priceForTokens(tokens int, price int64) int64 {
	if tokens <= 0 || price <= 0 {
		return 0
	}
	count := int64(tokens)
	whole := (count / priceUnitTokens) * price
	remainder := count % priceUnitTokens
	return whole + (remainder*price+priceUnitTokens-1)/priceUnitTokens
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
