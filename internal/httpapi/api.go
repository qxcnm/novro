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
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/novro-gateway/novro/internal/apikey"
	"github.com/novro-gateway/novro/internal/auth"
	"github.com/novro-gateway/novro/internal/billing"
	"github.com/novro-gateway/novro/internal/billinggroup"
	"github.com/novro-gateway/novro/internal/email"
	"github.com/novro-gateway/novro/internal/gatewaysettings"
	"github.com/novro-gateway/novro/internal/modelroute"
	"github.com/novro-gateway/novro/internal/payment"
	"github.com/novro-gateway/novro/internal/provider"
	"github.com/novro-gateway/novro/internal/providersync"
	"github.com/novro-gateway/novro/internal/referral"
	"github.com/novro-gateway/novro/internal/requestid"
	"github.com/novro-gateway/novro/internal/upstreammodel"
	"github.com/novro-gateway/novro/internal/user"
)

type AuthService interface {
	Login(context.Context, string, string) (auth.LoginResult, error)
	LoginOIDC(context.Context, auth.OIDCUser, bool) (auth.LoginResult, error)
	Authenticate(context.Context, string) (user.Record, error)
	Logout(context.Context, string) error
}

type UserService interface {
	Create(context.Context, user.CreateInput) (user.Record, error)
	EmailAvailable(context.Context, string) (bool, error)
	Register(context.Context, user.RegisterInput) (user.Record, error)
	InitializeAdmin(context.Context, user.RegisterInput) (user.Record, error)
	SetupRequired(context.Context) (bool, error)
	List(context.Context, user.ListFilter) (user.Page, error)
	Update(context.Context, uuid.UUID, user.UpdateInput) (user.Record, error)
	SetStatus(context.Context, uuid.UUID, user.Status) (user.Record, error)
	ResetPassword(context.Context, uuid.UUID, string) error
}

type APIKeyService interface {
	Create(context.Context, uuid.UUID, uuid.UUID, string) (apikey.CreateResult, error)
	ListForUser(context.Context, uuid.UUID) ([]apikey.Record, error)
	RevealForUser(context.Context, uuid.UUID, uuid.UUID) (string, error)
	RevokeForUser(context.Context, uuid.UUID, uuid.UUID) error
	ListAll(context.Context, apikey.ListFilter) (apikey.Page, error)
	Revoke(context.Context, uuid.UUID) error
}

type ProviderService interface {
	Create(context.Context, provider.CreateInput) (provider.Record, error)
	List(context.Context, provider.ListFilter) ([]provider.Record, error)
	Update(context.Context, uuid.UUID, provider.UpdateInput) (provider.Record, error)
	SetStatus(context.Context, uuid.UUID, provider.Status) (provider.Record, error)
	Delete(context.Context, uuid.UUID) error
}

type BillingService interface {
	Summary(context.Context, uuid.UUID) (billing.Summary, error)
	SummaryPage(context.Context, uuid.UUID, billing.EntryFilter) (billing.Summary, error)
	Usage(context.Context, uuid.UUID, billing.UsageFilter) (billing.UsagePage, error)
	UsageRate(context.Context, uuid.UUID) (billing.UsageRate, error)
	Adjust(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64, string) (billing.Summary, error)
}

type PaymentService interface {
	Config(context.Context) (payment.PublicConfig, error)
	AdminConfig(context.Context) (payment.AdminConfig, error)
	UpdateConfig(context.Context, payment.ConfigInput) (payment.AdminConfig, error)
	Create(context.Context, uuid.UUID, int64, string) (payment.CreateResult, error)
	List(context.Context, uuid.UUID, payment.ListFilter) (payment.Page, error)
	ReconcileForUser(context.Context, uuid.UUID, string) (payment.Order, error)
	ListAll(context.Context, payment.AdminListFilter) (payment.AdminPage, error)
	HandleNotification(context.Context, url.Values) error
}

type ReferralService interface {
	Summary(context.Context, uuid.UUID) (referral.Summary, error)
	AdminConfig(context.Context) (referral.AdminConfig, error)
	UpdateRewardBPS(context.Context, int64) (referral.AdminConfig, error)
}

type GatewaySettingsService interface {
	Config(context.Context) (gatewaysettings.Config, error)
	Update(context.Context, gatewaysettings.Config) (gatewaysettings.Config, error)
}

type ModelRouteService interface {
	Create(context.Context, modelroute.CreateInput) (modelroute.Record, error)
	List(context.Context, modelroute.ListFilter) ([]modelroute.Record, error)
	ListActive(context.Context, uuid.UUID) ([]modelroute.Record, error)
	Update(context.Context, uuid.UUID, modelroute.UpdateInput) (modelroute.Record, error)
	SetStatus(context.Context, uuid.UUID, modelroute.Status) (modelroute.Record, error)
	Delete(context.Context, uuid.UUID) error
}

type UpstreamModelService interface {
	Create(context.Context, upstreammodel.CreateInput) (upstreammodel.Record, error)
	List(context.Context, upstreammodel.ListFilter) ([]upstreammodel.Record, error)
	Update(context.Context, uuid.UUID, upstreammodel.UpdateInput) (upstreammodel.Record, error)
	SetStatus(context.Context, uuid.UUID, upstreammodel.Status) (upstreammodel.Record, error)
	Delete(context.Context, uuid.UUID) error
}

type ProviderModelService interface {
	Sync(context.Context, uuid.UUID) ([]providersync.CatalogModel, error)
	Link(context.Context, uuid.UUID, []uuid.UUID) (providersync.LinkResult, error)
}

type BillingGroupService interface {
	Create(context.Context, billinggroup.CreateInput) (billinggroup.Record, error)
	List(context.Context, billinggroup.ListFilter) ([]billinggroup.Record, error)
	Update(context.Context, uuid.UUID, billinggroup.UpdateInput) (billinggroup.Record, error)
	SetStatus(context.Context, uuid.UUID, billinggroup.Status) (billinggroup.Record, error)
	Delete(context.Context, uuid.UUID) error
}

type OIDCService interface {
	Start() (auth.OIDCFlow, error)
	Complete(context.Context, string, string, string) (auth.OIDCUser, bool, error)
}

type EmailVerificationService interface {
	Send(context.Context, string) error
	Verify(context.Context, string, string) error
}

type EmailConfigService interface {
	AdminConfig(context.Context) (email.AdminConfig, error)
	UpdateConfig(context.Context, email.ConfigInput) (email.AdminConfig, error)
	Test(context.Context, string) error
}

type Dependencies struct {
	Database            databasePinger
	Auth                AuthService
	Users               UserService
	APIKeys             APIKeyService
	Providers           ProviderService
	Billing             BillingService
	Payments            PaymentService
	Referrals           ReferralService
	GatewaySettings     GatewaySettingsService
	ModelRoutes         ModelRouteService
	UpstreamModels      UpstreamModelService
	ProviderModels      ProviderModelService
	BillingGroups       BillingGroupService
	Gateway             http.Handler
	Logger              *slog.Logger
	CookieName          string
	CookieSecure        bool
	AllowedOrigins      []string
	SetupToken          string
	RegistrationEnabled bool
	OIDC                OIDCService
	OIDCDisplayName     string
	EmailVerification   EmailVerificationService
	EmailConfig         EmailConfigService
}

type apiHandler struct {
	auth                AuthService
	users               UserService
	apiKeys             APIKeyService
	providers           ProviderService
	billing             BillingService
	payments            PaymentService
	referrals           ReferralService
	gatewaySettings     GatewaySettingsService
	modelRoutes         ModelRouteService
	upstreamModels      UpstreamModelService
	providerModels      ProviderModelService
	billingGroups       BillingGroupService
	logger              *slog.Logger
	cookieName          string
	cookieSecure        bool
	allowedOrigins      map[string]struct{}
	setupToken          string
	registrationEnabled bool
	oidc                OIDCService
	oidcDisplayName     string
	emailVerification   EmailVerificationService
	emailConfig         EmailConfigService
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
		payments:            deps.Payments,
		referrals:           deps.Referrals,
		gatewaySettings:     deps.GatewaySettings,
		modelRoutes:         deps.ModelRoutes,
		upstreamModels:      deps.UpstreamModels,
		providerModels:      deps.ProviderModels,
		billingGroups:       deps.BillingGroups,
		logger:              logger,
		cookieName:          deps.CookieName,
		cookieSecure:        deps.CookieSecure,
		allowedOrigins:      make(map[string]struct{}, len(deps.AllowedOrigins)),
		setupToken:          deps.SetupToken,
		registrationEnabled: deps.RegistrationEnabled,
		oidc:                deps.OIDC,
		oidcDisplayName:     deps.OIDCDisplayName,
		emailVerification:   deps.EmailVerification,
		emailConfig:         deps.EmailConfig,
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
	mux.HandleFunc("POST /api/auth/register/send-code", h.sendRegistrationCode)
	mux.HandleFunc("GET /api/auth/oidc/start", h.oidcStart)
	mux.HandleFunc("GET /api/auth/oidc/callback", h.oidcCallback)
	mux.HandleFunc("POST /api/auth/logout", h.logout)
	mux.HandleFunc("GET /api/auth/me", h.me)
	mux.HandleFunc("PATCH /api/account/profile", h.updateProfile)
	mux.HandleFunc("GET /api/account/referral", h.myReferral)
	mux.HandleFunc("GET /api/admin/referral", h.getReferralConfig)
	mux.HandleFunc("PUT /api/admin/referral", h.updateReferralConfig)
	mux.HandleFunc("GET /api/admin/gateway-settings", h.getGatewaySettings)
	mux.HandleFunc("PUT /api/admin/gateway-settings", h.updateGatewaySettings)
	mux.HandleFunc("GET /api/account/api-keys", h.listMyAPIKeys)
	mux.HandleFunc("POST /api/account/api-keys", h.createMyAPIKey)
	mux.HandleFunc("GET /api/account/api-keys/{id}/secret", h.revealMyAPIKeySecret)
	mux.HandleFunc("DELETE /api/account/api-keys/{id}", h.revokeMyAPIKey)
	mux.HandleFunc("GET /api/account/billing-groups", h.listMyBillingGroups)
	mux.HandleFunc("GET /api/account/models", h.listAvailableModels)
	mux.HandleFunc("GET /api/account/balance", h.myBalance)
	mux.HandleFunc("GET /api/account/usage", h.myUsage)
	mux.HandleFunc("GET /api/account/usage/rate", h.myUsageRate)
	mux.HandleFunc("GET /api/account/top-ups/config", h.topUpConfig)
	mux.HandleFunc("GET /api/account/top-ups", h.listMyTopUps)
	mux.HandleFunc("POST /api/account/top-ups", h.createMyTopUp)
	mux.HandleFunc("POST /api/account/top-ups/{out_trade_no}/reconcile", h.reconcileMyTopUp)
	mux.HandleFunc("GET /api/payments/epay/notify", h.epayNotification)
	mux.HandleFunc("POST /api/payments/epay/notify", h.epayNotification)
	mux.HandleFunc("GET /api/payments/epay/return", h.epayReturn)
	mux.HandleFunc("GET /api/admin/payments", h.getPaymentConfig)
	mux.HandleFunc("PUT /api/admin/payments", h.updatePaymentConfig)
	mux.HandleFunc("GET /api/admin/email", h.getEmailConfig)
	mux.HandleFunc("PUT /api/admin/email", h.updateEmailConfig)
	mux.HandleFunc("POST /api/admin/email/test", h.testEmailConfig)
	mux.HandleFunc("GET /api/admin/top-ups", h.listAllTopUps)
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
	mux.HandleFunc("DELETE /api/admin/providers/{id}", h.deleteProvider)
	mux.HandleFunc("PATCH /api/admin/providers/{id}/status", h.setProviderStatus)
	mux.HandleFunc("POST /api/admin/providers/{id}/models/sync", h.syncProviderModels)
	mux.HandleFunc("POST /api/admin/providers/{id}/models", h.linkProviderModels)
	mux.HandleFunc("GET /api/admin/upstream-models", h.listUpstreamModels)
	mux.HandleFunc("POST /api/admin/upstream-models", h.createUpstreamModel)
	mux.HandleFunc("PATCH /api/admin/upstream-models/{id}", h.updateUpstreamModel)
	mux.HandleFunc("DELETE /api/admin/upstream-models/{id}", h.deleteUpstreamModel)
	mux.HandleFunc("PATCH /api/admin/upstream-models/{id}/status", h.setUpstreamModelStatus)
	mux.HandleFunc("GET /api/admin/billing-groups", h.listBillingGroups)
	mux.HandleFunc("POST /api/admin/billing-groups", h.createBillingGroup)
	mux.HandleFunc("PATCH /api/admin/billing-groups/{id}", h.updateBillingGroup)
	mux.HandleFunc("DELETE /api/admin/billing-groups/{id}", h.deleteBillingGroup)
	mux.HandleFunc("PATCH /api/admin/billing-groups/{id}/status", h.setBillingGroupStatus)
	mux.HandleFunc("GET /api/admin/model-routes", h.listModelRoutes)
	mux.HandleFunc("POST /api/admin/model-routes", h.createModelRoute)
	mux.HandleFunc("PATCH /api/admin/model-routes/{id}", h.updateModelRoute)
	mux.HandleFunc("DELETE /api/admin/model-routes/{id}", h.deleteModelRoute)
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
		Email       string `json:"email"`
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
		Username: request.Username, Email: request.Email, DisplayName: request.DisplayName, Password: request.Password,
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
	var request struct {
		Username         string `json:"username"`
		Email            string `json:"email"`
		DisplayName      string `json:"display_name"`
		Password         string `json:"password"`
		VerificationCode string `json:"verification_code"`
		ReferralCode     string `json:"referral_code"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "注册信息格式无效")
		return
	}
	if h.emailVerification == nil {
		writeError(w, http.StatusServiceUnavailable, "email_unavailable", "邮箱验证服务暂不可用")
		return
	}
	if err := h.emailVerification.Verify(r.Context(), request.Email, request.VerificationCode); err != nil {
		h.writeVerificationError(w, err)
		return
	}
	input = user.RegisterInput{
		Username: request.Username, Email: request.Email, DisplayName: request.DisplayName,
		Password: request.Password, ReferralCode: request.ReferralCode,
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

func (h *apiHandler) sendRegistrationCode(w http.ResponseWriter, r *http.Request) {
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
	if h.emailVerification == nil {
		writeError(w, http.StatusServiceUnavailable, "email_unavailable", "邮箱验证服务暂不可用")
		return
	}
	var request struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "邮箱格式无效")
		return
	}
	available, err := h.users.EmailAvailable(r.Context(), request.Email)
	if err != nil {
		if errors.Is(err, user.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, "invalid_email", "请输入有效的邮箱地址")
			return
		}
		h.internalError(w, "check registration email", err)
		return
	}
	if !available {
		writeError(w, http.StatusConflict, "email_taken", "该邮箱已经注册，请更换其他邮箱")
		return
	}
	if err := h.emailVerification.Send(r.Context(), request.Email); err != nil {
		h.writeVerificationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true})
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
		writeError(w, http.StatusBadRequest, "invalid_request", "用户名或邮箱与密码不能为空")
		return
	}
	result, err := h.auth.Login(r.Context(), request.Username, request.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "用户名、邮箱或密码错误")
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
	h.clearSessionCookie(w)
	if err := h.auth.Logout(r.Context(), token); err != nil {
		h.internalError(w, "logout failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *apiHandler) me(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": record})
}

func (h *apiHandler) updateProfile(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var request struct {
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "资料格式无效")
		return
	}
	displayName := strings.TrimSpace(request.DisplayName)
	updated, err := h.users.Update(r.Context(), record.ID, user.UpdateInput{DisplayName: &displayName})
	if err != nil {
		h.writeUserError(w, "update profile", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": updated})
}

func (h *apiHandler) myReferral(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	if h.referrals == nil {
		writeError(w, http.StatusServiceUnavailable, "referral_unavailable", "邀请计划暂不可用")
		return
	}
	summary, err := h.referrals.Summary(r.Context(), record.ID)
	if err != nil {
		h.internalError(w, "read referral summary", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"referral": summary})
}

func (h *apiHandler) getReferralConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if h.referrals == nil {
		writeError(w, http.StatusServiceUnavailable, "referral_unavailable", "推荐计划暂不可用")
		return
	}
	config, err := h.referrals.AdminConfig(r.Context())
	if err != nil {
		h.writeReferralConfigError(w, "read referral configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"referral_config": config})
}

func (h *apiHandler) updateReferralConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if h.referrals == nil {
		writeError(w, http.StatusServiceUnavailable, "referral_unavailable", "推荐计划暂不可用")
		return
	}
	var request struct {
		RewardBPS int64 `json:"reward_bps"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "返现比例格式无效")
		return
	}
	config, err := h.referrals.UpdateRewardBPS(r.Context(), request.RewardBPS)
	if err != nil {
		h.writeReferralConfigError(w, "update referral configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"referral_config": config})
}

func (h *apiHandler) getGatewaySettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if h.gatewaySettings == nil {
		writeError(w, http.StatusServiceUnavailable, "gateway_settings_unavailable", "请求设置服务暂不可用")
		return
	}
	config, err := h.gatewaySettings.Config(r.Context())
	if err != nil {
		h.writeGatewaySettingsError(w, "read gateway request settings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"gateway_settings": config})
}

func (h *apiHandler) updateGatewaySettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if h.gatewaySettings == nil {
		writeError(w, http.StatusServiceUnavailable, "gateway_settings_unavailable", "请求设置服务暂不可用")
		return
	}
	var request gatewaysettings.Config
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求设置格式无效")
		return
	}
	config, err := h.gatewaySettings.Update(r.Context(), request)
	if err != nil {
		h.writeGatewaySettingsError(w, "update gateway request settings", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"gateway_settings": config})
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
		Name           string    `json:"name"`
		BillingGroupID uuid.UUID `json:"billing_group_id"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "API Key 信息格式无效")
		return
	}
	created, err := h.apiKeys.Create(r.Context(), record.ID, request.BillingGroupID, request.Name)
	if err != nil {
		h.writeAPIKeyError(w, "create API key", err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *apiHandler) revealMyAPIKeySecret(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "API Key ID 无效")
		return
	}
	key, err := h.apiKeys.RevealForUser(r.Context(), record.ID, id)
	if err != nil {
		h.writeAPIKeyError(w, "reveal user API key secret", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key})
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

func (h *apiHandler) deleteProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "提供商 ID 无效")
		return
	}
	if err := h.providers.Delete(r.Context(), id); err != nil {
		h.writeProviderError(w, "delete provider", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *apiHandler) syncProviderModels(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "提供商 ID 无效")
		return
	}
	models, err := h.providerModels.Sync(r.Context(), id)
	if err != nil {
		h.writeProviderModelError(w, "sync provider models", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (h *apiHandler) linkProviderModels(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "提供商 ID 无效")
		return
	}
	var request struct {
		ModelIDs []uuid.UUID `json:"model_ids"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "关联模型信息格式无效")
		return
	}
	result, err := h.providerModels.Link(r.Context(), id, request.ModelIDs)
	if err != nil {
		h.writeProviderModelError(w, "link provider models", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *apiHandler) listUpstreamModels(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	records, err := h.upstreamModels.List(r.Context(), upstreammodel.ListFilter{Search: r.URL.Query().Get("search"), Status: upstreammodel.Status(r.URL.Query().Get("status"))})
	if err != nil {
		h.writeUpstreamModelError(w, "list upstream models", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"upstream_models": records})
}

func (h *apiHandler) createUpstreamModel(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	var input upstreammodel.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "上游模型信息格式无效")
		return
	}
	created, err := h.upstreamModels.Create(r.Context(), input)
	if err != nil {
		h.writeUpstreamModelError(w, "create upstream model", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"upstream_model": created})
}

func (h *apiHandler) updateUpstreamModel(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "上游模型 ID 无效")
		return
	}
	var input upstreammodel.UpdateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "上游模型信息格式无效")
		return
	}
	updated, err := h.upstreamModels.Update(r.Context(), id, input)
	if err != nil {
		h.writeUpstreamModelError(w, "update upstream model", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"upstream_model": updated})
}

func (h *apiHandler) setUpstreamModelStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "上游模型 ID 无效")
		return
	}
	var request struct {
		Status upstreammodel.Status `json:"status"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "上游模型状态格式无效")
		return
	}
	updated, err := h.upstreamModels.SetStatus(r.Context(), id, request.Status)
	if err != nil {
		h.writeUpstreamModelError(w, "set upstream model status", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"upstream_model": updated})
}

func (h *apiHandler) deleteUpstreamModel(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "上游模型 ID 无效")
		return
	}
	if err := h.upstreamModels.Delete(r.Context(), id); err != nil {
		h.writeUpstreamModelError(w, "delete upstream model", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *apiHandler) listBillingGroups(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	records, err := h.billingGroups.List(r.Context(), billinggroup.ListFilter{Search: r.URL.Query().Get("search"), Status: billinggroup.Status(r.URL.Query().Get("status")), IncludeHidden: true})
	if err != nil {
		h.writeBillingGroupError(w, "list billing groups", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"billing_groups": records})
}

func (h *apiHandler) createBillingGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	var input billinggroup.CreateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "计费分组信息格式无效")
		return
	}
	created, err := h.billingGroups.Create(r.Context(), input)
	if err != nil {
		h.writeBillingGroupError(w, "create billing group", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"billing_group": created})
}

func (h *apiHandler) updateBillingGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "计费分组 ID 无效")
		return
	}
	var input billinggroup.UpdateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "计费分组信息格式无效")
		return
	}
	updated, err := h.billingGroups.Update(r.Context(), id, input)
	if err != nil {
		h.writeBillingGroupError(w, "update billing group", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"billing_group": updated})
}

func (h *apiHandler) setBillingGroupStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "计费分组 ID 无效")
		return
	}
	var request struct {
		Status billinggroup.Status `json:"status"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "计费分组状态格式无效")
		return
	}
	updated, err := h.billingGroups.SetStatus(r.Context(), id, request.Status)
	if err != nil {
		h.writeBillingGroupError(w, "set billing group status", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"billing_group": updated})
}

func (h *apiHandler) deleteBillingGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "计费分组 ID 无效")
		return
	}
	if err := h.billingGroups.Delete(r.Context(), id); err != nil {
		h.writeBillingGroupError(w, "delete billing group", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *apiHandler) myBalance(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	offset, err := parseQueryInt(r, "offset", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "分页参数无效")
		return
	}
	limit, err := parseQueryInt(r, "limit", 20)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "分页参数无效")
		return
	}
	summary, err := h.billing.SummaryPage(r.Context(), record.ID, billing.EntryFilter{Offset: offset, Limit: limit})
	if err != nil {
		h.writeBillingError(w, "read account balance", err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

type availableModel struct {
	ID           string            `json:"id"`
	DisplayName  string            `json:"display_name"`
	ProviderName string            `json:"provider_name"`
	Protocol     provider.Protocol `json:"protocol"`
	ChannelCount int               `json:"channel_count"`
	Prices       billing.RateCard  `json:"prices"`
}

func (h *apiHandler) listAvailableModels(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	groups, err := h.billingGroups.List(r.Context(), billingGroupFilterForUser(record))
	if err != nil {
		h.writeBillingGroupError(w, "list account billing groups", err)
		return
	}
	var selected billinggroup.Record
	requestedID := strings.TrimSpace(r.URL.Query().Get("billing_group_id"))
	if requestedID != "" {
		id, parseErr := uuid.Parse(requestedID)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "计费分组 ID 无效")
			return
		}
		for _, group := range groups {
			if group.ID == id {
				selected = group
				break
			}
		}
	} else {
		for _, group := range groups {
			if group.IsDefault {
				selected = group
				break
			}
		}
	}
	if selected.ID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "billing_group_unavailable", "计费分组不存在或已停用")
		return
	}
	routes, err := h.modelRoutes.ListActive(r.Context(), selected.ID)
	if err != nil {
		h.writeModelRouteError(w, "list available models", err)
		return
	}
	models := make([]availableModel, 0, len(routes))
	modelIndexes := make(map[string]int, len(routes))
	for _, route := range routes {
		if route.UpstreamModel == nil {
			continue
		}
		prices := route.UpstreamModel.Prices
		providerName := route.UpstreamModel.ProviderName
		if strings.TrimSpace(providerName) == "" {
			providerName = route.Provider.DisplayName
		}
		candidate := availableModel{
			ID: route.PublicName, DisplayName: route.DisplayName,
			ProviderName: providerName, Protocol: route.Provider.Protocol, ChannelCount: 1,
			Prices: billing.RateCard{
				InputMicros:        priceWithMultiplier(prices.InputMicros, selected.MultiplierBPS),
				OutputMicros:       priceWithMultiplier(prices.OutputMicros, selected.MultiplierBPS),
				CacheReadMicros:    priceWithMultiplier(prices.CacheReadMicros, selected.MultiplierBPS),
				CacheWriteMicros:   priceWithMultiplier(prices.CacheWriteMicros, selected.MultiplierBPS),
				CacheWrite1hMicros: priceWithMultiplier(prices.CacheWrite1hMicros, selected.MultiplierBPS),
				RequestMicros:      priceWithMultiplier(prices.RequestMicros, selected.MultiplierBPS),
			},
		}
		key := candidate.ID + "\x00" + string(candidate.Protocol)
		if index, exists := modelIndexes[key]; exists {
			models[index].ChannelCount++
			models[index].Prices = maximumRateCard(models[index].Prices, candidate.Prices)
			continue
		}
		modelIndexes[key] = len(models)
		models = append(models, candidate)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"models": models,
		"billing_group": map[string]any{
			"id":             selected.ID,
			"code":           selected.Code,
			"display_name":   selected.DisplayName,
			"multiplier_bps": selected.MultiplierBPS,
		},
	})
}

func (h *apiHandler) listMyBillingGroups(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	records, err := h.billingGroups.List(r.Context(), billingGroupFilterForUser(record))
	if err != nil {
		h.writeBillingGroupError(w, "list account billing groups", err)
		return
	}
	groups := make([]accountBillingGroup, 0, len(records))
	for _, record := range records {
		groups = append(groups, accountBillingGroup{
			ID:            record.ID,
			Code:          record.Code,
			DisplayName:   record.DisplayName,
			MultiplierBPS: record.MultiplierBPS,
			IsDefault:     record.IsDefault,
			Status:        record.Status,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"billing_groups": groups})
}

type accountBillingGroup struct {
	ID            uuid.UUID           `json:"id"`
	Code          string              `json:"code"`
	DisplayName   string              `json:"display_name"`
	MultiplierBPS int64               `json:"multiplier_bps"`
	IsDefault     bool                `json:"is_default"`
	Status        billinggroup.Status `json:"status"`
}

func maximumRateCard(first, second billing.RateCard) billing.RateCard {
	return billing.RateCard{
		InputMicros:        max(first.InputMicros, second.InputMicros),
		OutputMicros:       max(first.OutputMicros, second.OutputMicros),
		CacheReadMicros:    max(first.CacheReadMicros, second.CacheReadMicros),
		CacheWriteMicros:   max(first.CacheWriteMicros, second.CacheWriteMicros),
		CacheWrite1hMicros: max(first.CacheWrite1hMicros, second.CacheWrite1hMicros),
		RequestMicros:      max(first.RequestMicros, second.RequestMicros),
	}
}

func priceWithMultiplier(priceMicros, multiplierBPS int64) int64 {
	if priceMicros == 0 {
		return 0
	}
	return (priceMicros*multiplierBPS + billing.BasisPointsUnit - 1) / billing.BasisPointsUnit
}

func (h *apiHandler) myUsage(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	offset, err := parseQueryInt(r, "offset", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "分页参数无效")
		return
	}
	limit, err := parseQueryInt(r, "limit", 20)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "分页参数无效")
		return
	}
	var apiKeyID uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("api_key_id")); raw != "" {
		apiKeyID, err = uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "API Key 筛选无效")
			return
		}
	}
	var from *time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		value, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "时间筛选无效")
			return
		}
		from = &value
	}
	usage, err := h.billing.Usage(r.Context(), record.ID, billing.UsageFilter{
		Search: r.URL.Query().Get("search"), APIKeyID: apiKeyID, Model: r.URL.Query().Get("model"),
		Status: billing.UsageStatus(r.URL.Query().Get("status")), From: from, Offset: offset, Limit: limit,
	})
	if err != nil {
		h.writeBillingError(w, "list account usage", err)
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

func (h *apiHandler) myUsageRate(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	rate, err := h.billing.UsageRate(r.Context(), record.ID)
	if err != nil {
		h.writeBillingError(w, "read account usage rate", err)
		return
	}
	writeJSON(w, http.StatusOK, rate)
}

func (h *apiHandler) topUpConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	if h.payments == nil {
		writeJSON(w, http.StatusOK, payment.PublicConfig{
			Channels: []string{}, Methods: []payment.PaymentMethod{},
			MinMicros: payment.MinTopUpMicros, MaxMicros: payment.MaxTopUpMicros,
			PresetAmountMicros: []int64{10_000_000, 50_000_000, 100_000_000, 500_000_000}, BonusTiers: []payment.BonusTier{},
		})
		return
	}
	config, err := h.payments.Config(r.Context())
	if err != nil {
		h.internalError(w, "read payment configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (h *apiHandler) getPaymentConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if h.payments == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"payment_config": payment.AdminConfig{
				Provider: payment.ProviderEPay, Channels: []string{}, Methods: []payment.PaymentMethod{},
				MinMicros: payment.MinTopUpMicros, MaxMicros: payment.MaxTopUpMicros,
				PresetAmountMicros: []int64{10_000_000, 50_000_000, 100_000_000, 500_000_000}, BonusTiers: []payment.BonusTier{},
			},
		})
		return
	}
	config, err := h.payments.AdminConfig(r.Context())
	if err != nil {
		h.writePaymentConfigError(w, "read payment configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payment_config": config})
}

func (h *apiHandler) updatePaymentConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if h.payments == nil {
		writeError(w, http.StatusServiceUnavailable, "payments_unavailable", "支付服务暂不可用")
		return
	}
	var request struct {
		Enabled            bool                    `json:"enabled"`
		APIURL             string                  `json:"api_url"`
		MerchantID         string                  `json:"merchant_id"`
		MerchantKey        *string                 `json:"merchant_key"`
		SiteName           string                  `json:"site_name"`
		Channels           []string                `json:"channels"`
		Methods            []payment.PaymentMethod `json:"methods"`
		MinMicros          int64                   `json:"min_micros"`
		MaxMicros          int64                   `json:"max_micros"`
		PresetAmountMicros []int64                 `json:"preset_amounts_micros"`
		BonusTiers         []payment.BonusTier     `json:"bonus_tiers"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "支付配置格式无效")
		return
	}
	config, err := h.payments.UpdateConfig(r.Context(), payment.ConfigInput{
		Enabled: request.Enabled, APIURL: request.APIURL, MerchantID: request.MerchantID,
		MerchantKey: request.MerchantKey, SiteName: request.SiteName, Channels: request.Channels,
		Methods: request.Methods, MinMicros: request.MinMicros, MaxMicros: request.MaxMicros,
		PresetAmountMicros: request.PresetAmountMicros, BonusTiers: request.BonusTiers,
	})
	if err != nil {
		h.writePaymentConfigError(w, "update payment configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"payment_config": config})
}

func (h *apiHandler) getEmailConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if h.emailConfig == nil {
		writeJSON(w, http.StatusOK, map[string]any{"email_config": email.AdminConfig{Port: 587, Security: email.SecuritySTARTTLS}})
		return
	}
	config, err := h.emailConfig.AdminConfig(r.Context())
	if err != nil {
		h.writeEmailConfigError(w, "read email configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email_config": config})
}

func (h *apiHandler) updateEmailConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if h.emailConfig == nil {
		writeError(w, http.StatusServiceUnavailable, "email_unavailable", "邮件服务暂不可用")
		return
	}
	var request struct {
		Enabled     bool    `json:"enabled"`
		Host        string  `json:"host"`
		Port        int     `json:"port"`
		Username    string  `json:"username"`
		Password    *string `json:"password"`
		FromAddress string  `json:"from_address"`
		Security    string  `json:"security"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "邮件配置格式无效")
		return
	}
	config, err := h.emailConfig.UpdateConfig(r.Context(), email.ConfigInput{
		Enabled: request.Enabled, Host: request.Host, Port: request.Port, Username: request.Username,
		Password: request.Password, FromAddress: request.FromAddress, Security: request.Security,
	})
	if err != nil {
		h.writeEmailConfigError(w, "update email configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email_config": config})
}

func (h *apiHandler) testEmailConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if h.emailConfig == nil {
		writeError(w, http.StatusServiceUnavailable, "email_unavailable", "邮件服务暂不可用")
		return
	}
	var request struct {
		Recipient string `json:"recipient"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "测试收件地址格式无效")
		return
	}
	if err := h.emailConfig.Test(r.Context(), request.Recipient); err != nil {
		h.writeEmailConfigError(w, "send SMTP test message", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
}

func (h *apiHandler) listAllTopUps(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if h.payments == nil {
		writeJSON(w, http.StatusOK, payment.AdminPage{Orders: []payment.AdminOrder{}, Limit: 50})
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
	page, err := h.payments.ListAll(r.Context(), payment.AdminListFilter{
		Search: r.URL.Query().Get("search"), Status: payment.Status(r.URL.Query().Get("status")),
		Channel: r.URL.Query().Get("channel"), Offset: offset, Limit: limit,
	})
	if errors.Is(err, payment.ErrInvalidInput) {
		writeError(w, http.StatusBadRequest, "invalid_request", "充值记录筛选参数无效")
		return
	}
	if err != nil {
		h.internalError(w, "list all top-up orders", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *apiHandler) listMyTopUps(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	if h.payments == nil {
		writeError(w, http.StatusServiceUnavailable, "payments_disabled", "充值暂未开放")
		return
	}
	offset, err := parseQueryInt(r, "offset", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "分页参数无效")
		return
	}
	limit, err := parseQueryInt(r, "limit", 20)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "分页参数无效")
		return
	}
	page, err := h.payments.List(r.Context(), record.ID, payment.ListFilter{Offset: offset, Limit: limit})
	if errors.Is(err, payment.ErrInvalidInput) {
		writeError(w, http.StatusBadRequest, "invalid_request", "分页参数无效")
		return
	}
	if err != nil {
		h.writePaymentError(w, "list top-up orders", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *apiHandler) createMyTopUp(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	if h.payments == nil {
		writeError(w, http.StatusServiceUnavailable, "payments_disabled", "充值暂未开放")
		return
	}
	var request struct {
		AmountMicros int64  `json:"amount_micros"`
		Channel      string `json:"channel"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求内容无效")
		return
	}
	result, err := h.payments.Create(r.Context(), record.ID, request.AmountMicros, request.Channel)
	if err != nil {
		h.writePaymentError(w, "create top-up order", err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *apiHandler) reconcileMyTopUp(w http.ResponseWriter, r *http.Request) {
	record, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	if h.payments == nil {
		writeError(w, http.StatusServiceUnavailable, "payments_disabled", "充值服务暂不可用")
		return
	}
	order, err := h.payments.ReconcileForUser(r.Context(), record.ID, r.PathValue("out_trade_no"))
	if err != nil {
		h.writePaymentError(w, "reconcile top-up order", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"order": order})
}

func (h *apiHandler) epayNotification(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if h.payments == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("fail"))
		return
	}
	values, err := readFormValues(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("fail"))
		return
	}
	if err := h.payments.HandleNotification(r.Context(), values); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, payment.ErrDisabled) {
			status = http.StatusServiceUnavailable
		} else if !errors.Is(err, payment.ErrInvalidNotice) && !errors.Is(err, payment.ErrOrderNotFound) && !errors.Is(err, payment.ErrOrderConflict) {
			status = http.StatusInternalServerError
			h.logger.Error("complete EPay notification", "request_id", requestid.ResponseID(w), "error", err)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte("fail"))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("success"))
}

// epayReturn handles the browser redirect from EPay. It uses the exact same
// signed notification path as the server-to-server callback, so a successful
// payment is credited even when the callback cannot reach a local URL. The
// payment store keeps this operation transactional and idempotent.
func (h *apiHandler) epayReturn(w http.ResponseWriter, r *http.Request) {
	if h.payments == nil {
		http.Redirect(w, r, "/console/billing?payment=unavailable", http.StatusSeeOther)
		return
	}
	values := r.URL.Query()
	// Older return URLs included this UI-only query parameter. It is not part
	// of the gateway signature and must not affect verification.
	values.Del("payment")
	if err := h.payments.HandleNotification(r.Context(), values); err != nil {
		if !errors.Is(err, payment.ErrOrderNotFound) && !errors.Is(err, payment.ErrInvalidNotice) && !errors.Is(err, payment.ErrOrderConflict) {
			h.logger.Error("complete EPay return", "request_id", requestid.ResponseID(w), "error", err)
		}
		http.Redirect(w, r, "/console/billing?payment=failed", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/console/billing?payment=returned", http.StatusSeeOther)
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
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 255 {
		writeError(w, http.StatusBadRequest, "invalid_idempotency_key", "余额调整必须提供不超过 255 个字符的 Idempotency-Key")
		return
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "余额调整信息无效")
		return
	}
	summary, err := h.billing.Adjust(r.Context(), id, admin.ID, stableAdjustmentReference(admin.ID, id, idempotencyKey), request.AmountMicros, request.Note)
	if err != nil {
		h.writeBillingError(w, "adjust user balance", err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func stableAdjustmentReference(actorID, userID uuid.UUID, key string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(actorID.String()+"\x00"+userID.String()+"\x00"+key))
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

func (h *apiHandler) deleteModelRoute(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "模型路由 ID 无效")
		return
	}
	if err := h.modelRoutes.Delete(r.Context(), id); err != nil {
		h.writeModelRouteError(w, "delete model route", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func billingGroupFilterForUser(record user.Record) billinggroup.ListFilter {
	filter := billinggroup.ListFilter{Status: billinggroup.StatusActive}
	if record.Role == user.RoleAdmin {
		filter.IncludeHidden = true
	} else {
		filter.AuthorizedUserID = record.ID
	}
	return filter
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
	case errors.Is(err, user.ErrEmailTaken):
		writeError(w, http.StatusConflict, "email_taken", "邮箱已被使用")
	case errors.Is(err, user.ErrInvalidReferralCode):
		writeError(w, http.StatusBadRequest, "invalid_referral_code", "邀请码无效或邀请账号不可用")
	case errors.Is(err, user.ErrLastActiveAdmin):
		writeError(w, http.StatusConflict, "last_active_admin", "不能停用或降级最后一个启用的管理员")
	case errors.Is(err, user.ErrProtectedAdmin):
		writeError(w, http.StatusConflict, "protected_admin", "系统管理员账号不可停用或降级")
	case errors.Is(err, user.ErrAlreadyInitialized):
		writeError(w, http.StatusConflict, "already_initialized", "管理员账号已经初始化")
	default:
		h.internalError(w, operation, err)
	}
}

func (h *apiHandler) writeVerificationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, user.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_email", "请输入有效的邮箱地址")
	case errors.Is(err, email.ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, "verification_rate_limited", "验证码发送过于频繁，请稍后再试")
	case errors.Is(err, email.ErrExpired):
		writeError(w, http.StatusBadRequest, "verification_expired", "验证码已过期，请重新获取")
	case errors.Is(err, email.ErrInvalidCode):
		writeError(w, http.StatusBadRequest, "verification_invalid", "验证码错误或已使用")
	case errors.Is(err, email.ErrNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "email_not_configured", "注册邮件暂不可用，请联系管理员")
	default:
		h.internalError(w, "email verification", err)
	}
}

func (h *apiHandler) writeEmailConfigError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, email.ErrInvalidConfig):
		writeError(w, http.StatusBadRequest, "invalid_email_config", "邮件配置无效，请检查主机、端口、发件地址和凭据")
	case errors.Is(err, email.ErrNotConfigured):
		writeError(w, http.StatusConflict, "email_not_configured", "请先保存并启用完整的 SMTP 配置")
	default:
		h.internalError(w, operation, err)
	}
}

func (h *apiHandler) writeReferralConfigError(w http.ResponseWriter, operation string, err error) {
	if errors.Is(err, referral.ErrInvalidInput) {
		writeError(w, http.StatusBadRequest, "invalid_referral_config", "返现比例必须在 0% 到 100% 之间")
		return
	}
	h.internalError(w, operation, err)
}

func (h *apiHandler) writeGatewaySettingsError(w http.ResponseWriter, operation string, err error) {
	if errors.Is(err, gatewaysettings.ErrInvalidConfig) {
		writeError(w, http.StatusBadRequest, "invalid_gateway_settings", "请求设置无效，请检查时间范围")
		return
	}
	h.internalError(w, operation, err)
}

func (h *apiHandler) writeAPIKeyError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, apikey.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "API Key 信息无效")
	case errors.Is(err, apikey.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "API Key 不存在")
	case errors.Is(err, apikey.ErrLimitReached):
		writeError(w, http.StatusConflict, "api_key_limit_reached", "启用的 API Key 已达到上限")
	case errors.Is(err, apikey.ErrGroupUnavailable):
		writeError(w, http.StatusBadRequest, "billing_group_unavailable", "计费分组不存在或已停用")
	case errors.Is(err, apikey.ErrSecretUnavailable):
		writeError(w, http.StatusGone, "api_key_secret_unavailable", "该 API Key 没有可重新复制的副本，请重新创建")
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
	case errors.Is(err, provider.ErrGroupUnavailable):
		writeError(w, http.StatusBadRequest, "billing_group_unavailable", "计费分组不存在或已停用")
	default:
		h.internalError(w, operation, err)
	}
}

func (h *apiHandler) writeProviderModelError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, providersync.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "关联模型信息无效")
	case errors.Is(err, providersync.ErrProviderNotFound):
		writeError(w, http.StatusNotFound, "not_found", "提供商不存在")
	case errors.Is(err, providersync.ErrModelsUnavailable):
		writeError(w, http.StatusUnprocessableEntity, "models_unavailable", "上游没有返回可用的模型列表")
	case errors.Is(err, providersync.ErrDiscoveryFailed):
		var discoveryError *providersync.DiscoveryError
		if errors.As(err, &discoveryError) {
			writeError(w, http.StatusBadGateway, "model_sync_failed", "模型同步失败："+discoveryError.Error())
			return
		}
		writeError(w, http.StatusBadGateway, "model_sync_failed", "无法从该提供商同步模型，请检查模型列表路径和 API 密钥")
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
	case errors.Is(err, billing.ErrRequestConflict):
		writeError(w, http.StatusConflict, "idempotency_conflict", "同一 Idempotency-Key 已用于不同的余额调整")
	default:
		h.internalError(w, operation, err)
	}
}

func (h *apiHandler) writePaymentError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, payment.ErrDisabled):
		writeError(w, http.StatusServiceUnavailable, "payments_disabled", "充值暂未开放")
	case errors.Is(err, payment.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_top_up", "充值金额或支付方式无效")
	case errors.Is(err, payment.ErrOrderNotFound):
		writeError(w, http.StatusNotFound, "top_up_not_found", "充值订单不存在")
	case errors.Is(err, payment.ErrOrderUnpaid):
		writeError(w, http.StatusConflict, "top_up_unpaid", "支付平台尚未确认该订单到账")
	case errors.Is(err, payment.ErrOrderConflict):
		writeError(w, http.StatusConflict, "top_up_conflict", "支付平台记录与充值订单不一致，请联系管理员核对")
	case errors.Is(err, payment.ErrGatewayQuery):
		writeError(w, http.StatusBadGateway, "payment_query_failed", "暂时无法查询支付结果，请稍后重试")
	default:
		h.internalError(w, operation, err)
	}
}

func (h *apiHandler) writePaymentConfigError(w http.ResponseWriter, operation string, err error) {
	if errors.Is(err, payment.ErrInvalidInput) {
		writeError(w, http.StatusBadRequest, "invalid_payment_config", "支付配置无效，请检查地址、商户信息和支付渠道")
		return
	}
	h.internalError(w, operation, err)
}

func (h *apiHandler) writeModelRouteError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, modelroute.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "模型路由信息无效")
	case errors.Is(err, modelroute.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "模型路由不存在")
	case errors.Is(err, modelroute.ErrNameTaken):
		writeError(w, http.StatusConflict, "model_route_taken", "该对外模型已关联相同的提供商和目录模型")
	case errors.Is(err, modelroute.ErrPricingRequired):
		writeError(w, http.StatusConflict, "model_pricing_required", "请先在模型目录完成定价并启用模型")
	default:
		h.internalError(w, operation, err)
	}
}

func (h *apiHandler) writeUpstreamModelError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, upstreammodel.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "上游模型信息无效")
	case errors.Is(err, upstreammodel.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "上游模型不存在")
	case errors.Is(err, upstreammodel.ErrNameTaken):
		writeError(w, http.StatusConflict, "upstream_model_taken", "该模型 ID 已存在于全局目录")
	case errors.Is(err, upstreammodel.ErrPricingRequired):
		writeError(w, http.StatusConflict, "model_pricing_required", "请先完成模型定价，再启用模型")
	default:
		h.internalError(w, operation, err)
	}
}

func (h *apiHandler) writeBillingGroupError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, billinggroup.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "计费分组信息无效")
	case errors.Is(err, billinggroup.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "计费分组不存在")
	case errors.Is(err, billinggroup.ErrCodeTaken):
		writeError(w, http.StatusConflict, "billing_group_code_taken", "计费分组标识已存在")
	case errors.Is(err, billinggroup.ErrProtected):
		writeError(w, http.StatusConflict, "default_billing_group", "默认计费分组不能隐藏、停用或删除")
	case errors.Is(err, billinggroup.ErrInUse):
		writeError(w, http.StatusConflict, "billing_group_in_use", "计费分组仍被 API Key 或提供商使用，请先迁移关联配置")
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
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/payments/epay/notify" && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
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

func decodeJSON(_ http.ResponseWriter, r *http.Request, target any) error {
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

func readFormValues(r *http.Request) (url.Values, error) {
	queryValues := r.URL.Query()
	if r.Body == nil {
		return queryValues, nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	postValues, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	values := make(url.Values, len(postValues)+len(queryValues))
	for key, entries := range postValues {
		values[key] = append([]string(nil), entries...)
	}
	for key, entries := range queryValues {
		values[key] = append(values[key], entries...)
	}
	return values, nil
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
