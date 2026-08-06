package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/novro-gateway/novro/internal/apikey"
	"github.com/novro-gateway/novro/internal/auth"
	"github.com/novro-gateway/novro/internal/billing"
	"github.com/novro-gateway/novro/internal/modelroute"
	"github.com/novro-gateway/novro/internal/provider"
	"github.com/novro-gateway/novro/internal/requestid"
	"github.com/novro-gateway/novro/internal/user"
)

const maxJSONBodyBytes = 1 << 20

type AuthService interface {
	Login(context.Context, string, string) (auth.LoginResult, error)
	LoginOIDC(context.Context, auth.OIDCUser, bool) (auth.LoginResult, error)
	Authenticate(context.Context, string) (user.Record, error)
	Logout(context.Context, string) error
}

type UserService interface {
	Create(context.Context, user.CreateInput) (user.Record, error)
	Register(context.Context, user.RegisterInput) (user.Record, error)
	InitializeAdmin(context.Context, user.RegisterInput) (user.Record, error)
	SetupRequired(context.Context) (bool, error)
	List(context.Context, user.ListFilter) (user.Page, error)
	Update(context.Context, uuid.UUID, user.UpdateInput) (user.Record, error)
	SetStatus(context.Context, uuid.UUID, user.Status) (user.Record, error)
	ResetPassword(context.Context, uuid.UUID, string) error
}

type APIKeyService interface {
	Create(context.Context, uuid.UUID, string) (apikey.CreateResult, error)
	ListForUser(context.Context, uuid.UUID) ([]apikey.Record, error)
	RevokeForUser(context.Context, uuid.UUID, uuid.UUID) error
	ListAll(context.Context, apikey.ListFilter) (apikey.Page, error)
	Revoke(context.Context, uuid.UUID) error
}

type ProviderService interface {
	Create(context.Context, provider.CreateInput) (provider.Record, error)
	List(context.Context, provider.ListFilter) ([]provider.Record, error)
	Update(context.Context, uuid.UUID, provider.UpdateInput) (provider.Record, error)
	SetStatus(context.Context, uuid.UUID, provider.Status) (provider.Record, error)
}

type BillingService interface {
	Summary(context.Context, uuid.UUID) (billing.Summary, error)
	Usage(context.Context, uuid.UUID) ([]billing.Usage, error)
	Adjust(context.Context, uuid.UUID, uuid.UUID, int64, string) (billing.Summary, error)
}

type ModelRouteService interface {
	Create(context.Context, modelroute.CreateInput) (modelroute.Record, error)
	List(context.Context, modelroute.ListFilter) ([]modelroute.Record, error)
	Update(context.Context, uuid.UUID, modelroute.UpdateInput) (modelroute.Record, error)
	SetStatus(context.Context, uuid.UUID, modelroute.Status) (modelroute.Record, error)
}

type OIDCService interface {
	Start() (auth.OIDCFlow, error)
	Complete(context.Context, string, string, string) (auth.OIDCUser, bool, error)
}

type Dependencies struct {
	Database            databasePinger
	Auth                AuthService
	Users               UserService
	APIKeys             APIKeyService
	Providers           ProviderService
	Billing             BillingService
	ModelRoutes         ModelRouteService
	Gateway             http.Handler
	Logger              *slog.Logger
	CookieName          string
	CookieSecure        bool
	AllowedOrigins      []string
	SetupToken          string
	RegistrationEnabled bool
	OIDC                OIDCService
	OIDCDisplayName     string
}

type apiHandler struct {
	auth                AuthService
	users               UserService
	apiKeys             APIKeyService
	providers           ProviderService
	billing             BillingService
	modelRoutes         ModelRouteService
	logger              *slog.Logger
	cookieName          string
	cookieSecure        bool
	allowedOrigins      map[string]struct{}
	setupToken          string
	registrationEnabled bool
	oidc                OIDCService
	oidcDisplayName     string
}

func New(deps Dependencies) http.Handler {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	h := &apiHandler{
		auth:                deps.Auth,
		users:               deps.Users,
		apiKeys:             deps.APIKeys,
		providers:           deps.Providers,
		billing:             deps.Billing,
		modelRoutes:         deps.ModelRoutes,
		logger:              logger,
		cookieName:          deps.CookieName,
		cookieSecure:        deps.CookieSecure,
		allowedOrigins:      make(map[string]struct{}, len(deps.AllowedOrigins)),
		setupToken:          deps.SetupToken,
		registrationEnabled: deps.RegistrationEnabled,
		oidc:                deps.OIDC,
		oidcDisplayName:     deps.OIDCDisplayName,
	}
	for _, origin := range deps.AllowedOrigins {
		h.allowedOrigins[strings.TrimRight(origin, "/")] = struct{}{}
	}

	mux := http.NewServeMux()
	health := healthHandler{db: deps.Database}
	mux.HandleFunc("GET /healthz", health.live)
	mux.HandleFunc("GET /readyz", health.ready)
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("GET /api/auth/options", h.authOptions)
	mux.HandleFunc("POST /api/auth/setup", h.setup)
	mux.HandleFunc("POST /api/auth/register", h.register)
	mux.HandleFunc("GET /api/auth/oidc/start", h.oidcStart)
	mux.HandleFunc("GET /api/auth/oidc/callback", h.oidcCallback)
	mux.HandleFunc("POST /api/auth/logout", h.logout)
	mux.HandleFunc("GET /api/auth/me", h.me)
	mux.HandleFunc("GET /api/account/api-keys", h.listMyAPIKeys)
	mux.HandleFunc("POST /api/account/api-keys", h.createMyAPIKey)
	mux.HandleFunc("DELETE /api/account/api-keys/{id}", h.revokeMyAPIKey)
	mux.HandleFunc("GET /api/account/balance", h.myBalance)
	mux.HandleFunc("GET /api/account/usage", h.myUsage)
	mux.HandleFunc("GET /api/admin/users", h.listUsers)
	mux.HandleFunc("POST /api/admin/users", h.createUser)
	mux.HandleFunc("PATCH /api/admin/users/{id}", h.updateUser)
	mux.HandleFunc("PATCH /api/admin/users/{id}/status", h.setUserStatus)
	mux.HandleFunc("POST /api/admin/users/{id}/reset-password", h.resetUserPassword)
	mux.HandleFunc("GET /api/admin/users/{id}/balance", h.userBalance)
	mux.HandleFunc("POST /api/admin/users/{id}/balance-adjustments", h.adjustUserBalance)
	mux.HandleFunc("GET /api/admin/api-keys", h.listAPIKeys)
	mux.HandleFunc("POST /api/admin/api-keys/{id}/revoke", h.revokeAPIKey)
	mux.HandleFunc("GET /api/admin/providers", h.listProviders)
	mux.HandleFunc("POST /api/admin/providers", h.createProvider)
	mux.HandleFunc("PATCH /api/admin/providers/{id}", h.updateProvider)
	mux.HandleFunc("PATCH /api/admin/providers/{id}/status", h.setProviderStatus)
	mux.HandleFunc("GET /api/admin/model-routes", h.listModelRoutes)
	mux.HandleFunc("POST /api/admin/model-routes", h.createModelRoute)
	mux.HandleFunc("PATCH /api/admin/model-routes/{id}", h.updateModelRoute)
	mux.HandleFunc("PATCH /api/admin/model-routes/{id}/status", h.setModelRouteStatus)
	if deps.Gateway != nil {
		mux.Handle("/v1/", deps.Gateway)
	}
	return requestid.Middleware(h.securityHeaders(h.validateOrigin(mux)))
}

func (h *apiHandler) authOptions(w http.ResponseWriter, r *http.Request) {
	setupRequired, err := h.users.SetupRequired(r.Context())
	if err != nil {
		h.internalError(w, "read authentication options", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"setup_required":       setupRequired,
		"setup_enabled":        setupRequired && h.setupToken != "",
		"registration_enabled": h.registrationEnabled && !setupRequired,
		"oidc_enabled":         h.oidc != nil && !setupRequired,
		"oidc_display_name":    h.oidcDisplayName,
	})
}

func (h *apiHandler) setup(w http.ResponseWriter, r *http.Request) {
	setupRequired, err := h.users.SetupRequired(r.Context())
	if err != nil {
		h.internalError(w, "check administrator initialization", err)
		return
	}
	if !setupRequired {
		writeError(w, http.StatusConflict, "already_initialized", "管理员账号已经初始化")
		return
	}
	if h.setupToken == "" {
		writeError(w, http.StatusForbidden, "setup_disabled", "管理员初始化未启用")
		return
	}
	var request struct {
		SetupToken  string `json:"setup_token"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "初始化信息格式无效")
		return
	}
	provided := sha256.Sum256([]byte(request.SetupToken))
	expected := sha256.Sum256([]byte(h.setupToken))
	if subtle.ConstantTimeCompare(provided[:], expected[:]) != 1 {
		writeError(w, http.StatusForbidden, "invalid_setup_token", "初始化令牌无效")
		return
	}
	created, err := h.users.InitializeAdmin(r.Context(), user.RegisterInput{
		Username: request.Username, DisplayName: request.DisplayName, Password: request.Password,
	})
	if err != nil {
		h.writeUserError(w, "initialize administrator", err)
		return
	}
	result, err := h.auth.Login(r.Context(), created.Username, request.Password)
	if err != nil {
		h.internalError(w, "login initialized administrator", err)
		return
	}
	h.setSessionCookie(w, result.Token, result.ExpiresAt)
	writeJSON(w, http.StatusCreated, map[string]any{"user": result.User})
}

func (h *apiHandler) register(w http.ResponseWriter, r *http.Request) {
	if !h.registrationEnabled {
		writeError(w, http.StatusForbidden, "registration_disabled", "用户注册未开放")
		return
	}
	setupRequired, err := h.users.SetupRequired(r.Context())
	if err != nil {
		h.internalError(w, "check registration readiness", err)
		return
	}
	if setupRequired {
		writeError(w, http.StatusConflict, "setup_required", "请先初始化管理员账号")
		return
	}
	var input user.RegisterInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "注册信息格式无效")
		return
	}
	created, err := h.users.Register(r.Context(), input)
	if err != nil {
		h.writeUserError(w, "register user", err)
		return
	}
	result, err := h.auth.Login(r.Context(), created.Username, input.Password)
	if err != nil {
		h.internalError(w, "login registered user", err)
		return
	}
	h.setSessionCookie(w, result.Token, result.ExpiresAt)
	writeJSON(w, http.StatusCreated, map[string]any{"user": result.User})
}

func (h *apiHandler) oidcStart(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil {
		http.NotFound(w, r)
		return
	}
	setupRequired, err := h.users.SetupRequired(r.Context())
	if err != nil {
		h.internalError(w, "check OIDC login readiness", err)
		return
	}
	if setupRequired {
		http.Redirect(w, r, "/login?error=setup_required", http.StatusFound)
		return
	}
	flow, err := h.oidc.Start()
	if err != nil {
		h.internalError(w, "start OIDC login", err)
		return
	}
	h.setOIDCFlowCookie(w, flow.CookieValue, flow.ExpiresAt)
	http.Redirect(w, r, flow.AuthorizationURL, http.StatusFound)
}

func (h *apiHandler) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if h.oidc == nil {
		http.NotFound(w, r)
		return
	}
	setupRequired, err := h.users.SetupRequired(r.Context())
	if err != nil || setupRequired {
		h.clearOIDCFlowCookie(w)
		http.Redirect(w, r, "/login?error=setup_required", http.StatusFound)
		return
	}
	cookie, err := r.Cookie("novro_oidc_flow")
	if err != nil || r.URL.Query().Get("error") != "" {
		h.clearOIDCFlowCookie(w)
		http.Redirect(w, r, "/login?error=oidc_failed", http.StatusFound)
		return
	}
	identity, autoRegister, err := h.oidc.Complete(r.Context(), r.URL.Query().Get("code"), r.URL.Query().Get("state"), cookie.Value)
	h.clearOIDCFlowCookie(w)
	if err != nil {
		http.Redirect(w, r, "/login?error=oidc_failed", http.StatusFound)
		return
	}
	result, err := h.auth.LoginOIDC(r.Context(), identity, autoRegister)
	if err != nil {
		if errors.Is(err, auth.ErrOIDCNotProvisioned) {
			http.Redirect(w, r, "/login?error=oidc_not_provisioned", http.StatusFound)
			return
		}
		h.internalError(w, "complete OIDC login", err)
		return
	}
	h.setSessionCookie(w, result.Token, result.ExpiresAt)
	http.Redirect(w, r, "/console", http.StatusFound)
}

func (h *apiHandler) login(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &request); err != nil || request.Username == "" || request.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "用户名和密码不能为空")
		return
	}
	result, err := h.auth.Login(r.Context(), request.Username, request.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
			return
		}
		h.internalError(w, "login failed", err)
		return
	}
	h.setSessionCookie(w, result.Token, result.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{"user": result.User})
}

func (h *apiHandler) logout(w http.ResponseWriter, r *http.Request) {
	token := h.sessionToken(r)
	if err := h.auth.Logout(r.Context(), token); err != nil {
		h.internalError(w, "logout failed", err)
		return
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *apiHandler) me(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": record})
}

func (h *apiHandler) listMyAPIKeys(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	keys, err := h.apiKeys.ListForUser(r.Context(), record.ID)
	if err != nil {
		h.writeAPIKeyError(w, "list user API keys", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": keys})
}

func (h *apiHandler) createMyAPIKey(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "API Key 信息格式无效")
		return
	}
	created, err := h.apiKeys.Create(r.Context(), record.ID, request.Name)
	if err != nil {
		h.writeAPIKeyError(w, "create API key", err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *apiHandler) revokeMyAPIKey(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "API Key ID 无效")
		return
	}
	if err := h.apiKeys.RevokeForUser(r.Context(), record.ID, id); err != nil {
		h.writeAPIKeyError(w, "revoke user API key", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *apiHandler) listUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	offset, err := parseQueryInt(r, "offset", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "分页参数无效")
		return
	}
	limit, err := parseQueryInt(r, "limit", 50)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "分页参数无效")
		return
	}
	page, err := h.users.List(r.Context(), user.ListFilter{
		Search: r.URL.Query().Get("search"),
		Status: user.Status(r.URL.Query().Get("status")),
		Offset: offset,
		Limit:  limit,
	})
	if err != nil {
		h.writeUserError(w, "list users", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *apiHandler) createUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	var input user.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "用户信息格式无效")
		return
	}
	created, err := h.users.Create(r.Context(), input)
	if err != nil {
		h.writeUserError(w, "create user", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": created})
}

func (h *apiHandler) updateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "用户 ID 无效")
		return
	}
	var input user.UpdateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "用户信息格式无效")
		return
	}
	updated, err := h.users.Update(r.Context(), id, input)
	if err != nil {
		h.writeUserError(w, "update user", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": updated})
}

func (h *apiHandler) setUserStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "用户 ID 无效")
		return
	}
	var request struct {
		Status user.Status `json:"status"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "用户状态格式无效")
		return
	}
	updated, err := h.users.SetStatus(r.Context(), id, request.Status)
	if err != nil {
		h.writeUserError(w, "set user status", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": updated})
}

func (h *apiHandler) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "用户 ID 无效")
		return
	}
	var request struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &request); err != nil || request.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "新密码不能为空")
		return
	}
	if err := h.users.ResetPassword(r.Context(), id, request.Password); err != nil {
		h.writeUserError(w, "reset password", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *apiHandler) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	offset, err := parseQueryInt(r, "offset", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "分页参数无效")
		return
	}
	limit, err := parseQueryInt(r, "limit", 50)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "分页参数无效")
		return
	}
	page, err := h.apiKeys.ListAll(r.Context(), apikey.ListFilter{
		Search: r.URL.Query().Get("search"),
		Status: apikey.Status(r.URL.Query().Get("status")),
		Offset: offset,
		Limit:  limit,
	})
	if err != nil {
		h.writeAPIKeyError(w, "list API keys", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *apiHandler) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "API Key ID 无效")
		return
	}
	if err := h.apiKeys.Revoke(r.Context(), id); err != nil {
		h.writeAPIKeyError(w, "revoke API key", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *apiHandler) listProviders(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	records, err := h.providers.List(r.Context(), provider.ListFilter{
		Search: r.URL.Query().Get("search"),
		Status: provider.Status(r.URL.Query().Get("status")),
	})
	if err != nil {
		h.writeProviderError(w, "list providers", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": records})
}

func (h *apiHandler) createProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	var input provider.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "提供商信息格式无效")
		return
	}
	created, err := h.providers.Create(r.Context(), input)
	if err != nil {
		h.writeProviderError(w, "create provider", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"provider": created})
}

func (h *apiHandler) updateProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "提供商 ID 无效")
		return
	}
	var input provider.UpdateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "提供商信息格式无效")
		return
	}
	updated, err := h.providers.Update(r.Context(), id, input)
	if err != nil {
		h.writeProviderError(w, "update provider", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": updated})
}

func (h *apiHandler) setProviderStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "提供商 ID 无效")
		return
	}
	var request struct {
		Status provider.Status `json:"status"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "提供商状态格式无效")
		return
	}
	updated, err := h.providers.SetStatus(r.Context(), id, request.Status)
	if err != nil {
		h.writeProviderError(w, "set provider status", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": updated})
}

func (h *apiHandler) myBalance(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	summary, err := h.billing.Summary(r.Context(), record.ID)
	if err != nil {
		h.writeBillingError(w, "read account balance", err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *apiHandler) myUsage(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	usage, err := h.billing.Usage(r.Context(), record.ID)
	if err != nil {
		h.writeBillingError(w, "list account usage", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"usage": usage})
}

func (h *apiHandler) userBalance(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "用户 ID 无效")
		return
	}
	summary, err := h.billing.Summary(r.Context(), id)
	if err != nil {
		h.writeBillingError(w, "read user balance", err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *apiHandler) adjustUserBalance(w http.ResponseWriter, r *http.Request) {
	admin, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "用户 ID 无效")
		return
	}
	var request struct {
		AmountMicros int64  `json:"amount_micros"`
		Note         string `json:"note"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "余额调整信息无效")
		return
	}
	summary, err := h.billing.Adjust(r.Context(), id, admin.ID, request.AmountMicros, request.Note)
	if err != nil {
		h.writeBillingError(w, "adjust user balance", err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *apiHandler) listModelRoutes(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	records, err := h.modelRoutes.List(r.Context(), modelroute.ListFilter{Search: r.URL.Query().Get("search"), Status: modelroute.Status(r.URL.Query().Get("status"))})
	if err != nil {
		h.writeModelRouteError(w, "list model routes", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"model_routes": records})
}

func (h *apiHandler) createModelRoute(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	var input modelroute.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "模型路由信息无效")
		return
	}
	created, err := h.modelRoutes.Create(r.Context(), input)
	if err != nil {
		h.writeModelRouteError(w, "create model route", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"model_route": created})
}

func (h *apiHandler) updateModelRoute(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "模型路由 ID 无效")
		return
	}
	var input modelroute.UpdateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "模型路由信息无效")
		return
	}
	updated, err := h.modelRoutes.Update(r.Context(), id, input)
	if err != nil {
		h.writeModelRouteError(w, "update model route", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"model_route": updated})
}

func (h *apiHandler) setModelRouteStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "模型路由 ID 无效")
		return
	}
	var request struct {
		Status modelroute.Status `json:"status"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "模型路由状态无效")
		return
	}
	updated, err := h.modelRoutes.SetStatus(r.Context(), id, request.Status)
	if err != nil {
		h.writeModelRouteError(w, "set model route status", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"model_route": updated})
}

func (h *apiHandler) requireUser(w http.ResponseWriter, r *http.Request) (user.Record, bool) {
	record, err := h.auth.Authenticate(r.Context(), h.sessionToken(r))
	if err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "请先登录")
			return user.Record{}, false
		}
		h.internalError(w, "authenticate request", err)
		return user.Record{}, false
	}
	return record, true
}

func (h *apiHandler) requireAdmin(w http.ResponseWriter, r *http.Request) (user.Record, bool) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return user.Record{}, false
	}
	if record.Role != user.RoleAdmin {
		writeError(w, http.StatusForbidden, "forbidden", "需要管理员权限")
		return user.Record{}, false
	}
	return record, true
}

func (h *apiHandler) sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(h.cookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (h *apiHandler) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   max(1, int(time.Until(expires).Seconds())),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *apiHandler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *apiHandler) setOIDCFlowCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: "novro_oidc_flow", Value: value, Path: "/api/auth/oidc", Expires: expires,
		MaxAge: max(1, int(time.Until(expires).Seconds())), HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode})
}

func (h *apiHandler) clearOIDCFlowCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "novro_oidc_flow", Value: "", Path: "/api/auth/oidc", MaxAge: -1,
		HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode})
}

func (h *apiHandler) writeUserError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, user.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "用户信息无效")
	case errors.Is(err, user.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "用户不存在")
	case errors.Is(err, user.ErrUsernameTaken):
		writeError(w, http.StatusConflict, "username_taken", "用户名已存在")
	case errors.Is(err, user.ErrLastActiveAdmin):
		writeError(w, http.StatusConflict, "last_active_admin", "不能停用或降级最后一个启用的管理员")
	case errors.Is(err, user.ErrAlreadyInitialized):
		writeError(w, http.StatusConflict, "already_initialized", "管理员账号已经初始化")
	default:
		h.internalError(w, operation, err)
	}
}

func (h *apiHandler) writeAPIKeyError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, apikey.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "API Key 信息无效")
	case errors.Is(err, apikey.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "API Key 不存在")
	case errors.Is(err, apikey.ErrLimitReached):
		writeError(w, http.StatusConflict, "api_key_limit_reached", "启用的 API Key 已达到上限")
	default:
		h.internalError(w, operation, err)
	}
}

func (h *apiHandler) writeProviderError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, provider.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "提供商信息无效")
	case errors.Is(err, provider.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "提供商不存在")
	case errors.Is(err, provider.ErrCodeTaken):
		writeError(w, http.StatusConflict, "provider_code_taken", "提供商标识已存在")
	default:
		h.internalError(w, operation, err)
	}
}

func (h *apiHandler) writeBillingError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, billing.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "余额操作信息无效")
	case errors.Is(err, billing.ErrWalletNotFound):
		writeError(w, http.StatusNotFound, "not_found", "用户钱包不存在")
	case errors.Is(err, billing.ErrInsufficientBalance):
		writeError(w, http.StatusConflict, "insufficient_balance", "余额不足")
	default:
		h.internalError(w, operation, err)
	}
}

func (h *apiHandler) writeModelRouteError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, modelroute.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "模型路由信息无效")
	case errors.Is(err, modelroute.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "模型路由不存在")
	case errors.Is(err, modelroute.ErrNameTaken):
		writeError(w, http.StatusConflict, "model_name_taken", "对外模型名称已存在")
	default:
		h.internalError(w, operation, err)
	}
}

func (h *apiHandler) internalError(w http.ResponseWriter, operation string, err error) {
	h.logger.Error(operation, "request_id", requestid.ResponseID(w), "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
}

func (h *apiHandler) validateOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			origin := strings.TrimRight(r.Header.Get("Origin"), "/")
			if _, ok := h.allowedOrigins[origin]; !ok {
				writeError(w, http.StatusForbidden, "invalid_origin", "请求来源无效")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *apiHandler) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request must contain exactly one JSON value")
	}
	return nil
}

func parseQueryInt(r *http.Request, key string, fallback int) (int, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback, nil
	}
	return strconv.Atoi(raw)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	value := map[string]any{
		"error": map[string]string{"code": code, "message": message, "type": "novro_error"},
	}
	if id := requestid.ResponseID(w); id != uuid.Nil {
		value["request_id"] = id.String()
	}
	writeJSON(w, status, value)
}
