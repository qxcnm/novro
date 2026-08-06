package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/novro-gateway/novro/internal/apikey"
	"github.com/novro-gateway/novro/internal/auth"
	"github.com/novro-gateway/novro/internal/provider"
	"github.com/novro-gateway/novro/internal/user"
)

type fakeAPIAuth struct {
	current user.Record
	login   auth.LoginResult
	authErr error
}

func (f *fakeAPIAuth) Login(context.Context, string, string) (auth.LoginResult, error) {
	return f.login, f.authErr
}

func (f *fakeAPIAuth) LoginOIDC(context.Context, auth.OIDCUser, bool) (auth.LoginResult, error) {
	return f.login, f.authErr
}

func (f *fakeAPIAuth) Authenticate(context.Context, string) (user.Record, error) {
	if f.authErr != nil {
		return user.Record{}, f.authErr
	}
	return f.current, nil
}

func (f *fakeAPIAuth) Logout(context.Context, string) error { return nil }

type fakeAPIUsers struct {
	createInput   user.CreateInput
	updateInput   user.UpdateInput
	registerInput user.RegisterInput
	setupRequired bool
	initializeErr error
	statusErr     error
}

type fakeAPIKeys struct {
	createdUserID uuid.UUID
	createdName   string
	revokedUserID uuid.UUID
	revokedID     uuid.UUID
	listFilter    apikey.ListFilter
	err           error
}

type fakeProviders struct {
	createInput provider.CreateInput
	updateInput provider.UpdateInput
	status      provider.Status
	err         error
}

func (f *fakeProviders) Create(_ context.Context, input provider.CreateInput) (provider.Record, error) {
	f.createInput = input
	return provider.Record{ID: uuid.New(), Code: input.Code, DisplayName: input.DisplayName, Protocol: input.Protocol, BaseURL: input.BaseURL, APIKeyHint: "1234", HasAPIKey: true, Status: provider.StatusActive}, f.err
}
func (f *fakeProviders) List(context.Context, provider.ListFilter) ([]provider.Record, error) {
	return []provider.Record{}, f.err
}
func (f *fakeProviders) Update(_ context.Context, id uuid.UUID, input provider.UpdateInput) (provider.Record, error) {
	f.updateInput = input
	return provider.Record{ID: id}, f.err
}
func (f *fakeProviders) SetStatus(_ context.Context, id uuid.UUID, status provider.Status) (provider.Record, error) {
	f.status = status
	return provider.Record{ID: id, Status: status}, f.err
}

func (f *fakeAPIKeys) Create(_ context.Context, userID uuid.UUID, name string) (apikey.CreateResult, error) {
	f.createdUserID, f.createdName = userID, name
	if f.err != nil {
		return apikey.CreateResult{}, f.err
	}
	return apikey.CreateResult{
		APIKey: apikey.Record{ID: uuid.New(), UserID: userID, Name: name, KeyPrefix: "nvr_example", Status: apikey.StatusActive},
		Key:    "nvr_full-secret-returned-once",
	}, nil
}

func (f *fakeAPIKeys) ListForUser(context.Context, uuid.UUID) ([]apikey.Record, error) {
	return []apikey.Record{}, f.err
}

func (f *fakeAPIKeys) RevokeForUser(_ context.Context, userID, id uuid.UUID) error {
	f.revokedUserID, f.revokedID = userID, id
	return f.err
}

func (f *fakeAPIKeys) ListAll(_ context.Context, filter apikey.ListFilter) (apikey.Page, error) {
	f.listFilter = filter
	return apikey.Page{APIKeys: []apikey.AdminRecord{}, Limit: filter.Limit}, f.err
}

func (f *fakeAPIKeys) Revoke(_ context.Context, id uuid.UUID) error {
	f.revokedID = id
	return f.err
}

type fakeOIDCService struct {
	flow         auth.OIDCFlow
	identity     auth.OIDCUser
	autoRegister bool
	startErr     error
	completeErr  error
}

func (f *fakeOIDCService) Start() (auth.OIDCFlow, error) {
	return f.flow, f.startErr
}

func (f *fakeOIDCService) Complete(context.Context, string, string, string) (auth.OIDCUser, bool, error) {
	return f.identity, f.autoRegister, f.completeErr
}

func (f *fakeAPIUsers) Create(_ context.Context, input user.CreateInput) (user.Record, error) {
	f.createInput = input
	return user.Record{ID: uuid.New(), Username: input.Username, Role: input.Role, Status: user.StatusActive}, nil
}

func (f *fakeAPIUsers) Register(_ context.Context, input user.RegisterInput) (user.Record, error) {
	f.registerInput = input
	return user.Record{ID: uuid.New(), Username: input.Username, Role: user.RoleMember, Status: user.StatusActive}, nil
}

func (f *fakeAPIUsers) InitializeAdmin(_ context.Context, input user.RegisterInput) (user.Record, error) {
	if f.initializeErr != nil {
		return user.Record{}, f.initializeErr
	}
	f.registerInput = input
	return user.Record{ID: uuid.New(), Username: input.Username, Role: user.RoleAdmin, Status: user.StatusActive}, nil
}

func (f *fakeAPIUsers) SetupRequired(context.Context) (bool, error) { return f.setupRequired, nil }

func (f *fakeAPIUsers) List(context.Context, user.ListFilter) (user.Page, error) {
	return user.Page{Users: []user.Record{}, Total: 0, Limit: 50}, nil
}

func (f *fakeAPIUsers) SetStatus(_ context.Context, id uuid.UUID, status user.Status) (user.Record, error) {
	if f.statusErr != nil {
		return user.Record{}, f.statusErr
	}
	return user.Record{ID: id, Status: status}, nil
}

func (f *fakeAPIUsers) Update(_ context.Context, id uuid.UUID, input user.UpdateInput) (user.Record, error) {
	f.updateInput = input
	if f.statusErr != nil {
		return user.Record{}, f.statusErr
	}
	record := user.Record{ID: id, Status: user.StatusActive}
	if input.DisplayName != nil {
		record.DisplayName = *input.DisplayName
	}
	if input.Role != nil {
		record.Role = *input.Role
	}
	return record, nil
}

func (f *fakeAPIUsers) ResetPassword(context.Context, uuid.UUID, string) error { return nil }

func testAPI(authService *fakeAPIAuth, users *fakeAPIUsers) http.Handler {
	return testAPIWithKeys(authService, users, &fakeAPIKeys{})
}

func testAPIWithKeys(authService *fakeAPIAuth, users *fakeAPIUsers, apiKeys *fakeAPIKeys) http.Handler {
	return New(Dependencies{
		Auth:                authService,
		Users:               users,
		APIKeys:             apiKeys,
		Providers:           &fakeProviders{},
		Logger:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		CookieName:          "novro_session",
		CookieSecure:        true,
		AllowedOrigins:      []string{"http://localhost:3000"},
		RegistrationEnabled: true,
	})
}

func TestRegistrationCreatesMemberAndSetsSession(t *testing.T) {
	users := &fakeAPIUsers{}
	authService := &fakeAPIAuth{login: auth.LoginResult{
		Token: "nvs_test-token", ExpiresAt: time.Now().Add(time.Hour),
		User: user.Record{ID: uuid.New(), Username: "member.one", Role: user.RoleMember, Status: user.StatusActive},
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"member.one","display_name":"Member","password":"long-test-password"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	testAPI(authService, users).ServeHTTP(response, request)
	if response.Code != http.StatusCreated || users.registerInput.Username != "member.one" {
		t.Fatalf("status=%d body=%s input=%+v", response.Code, response.Body.String(), users.registerInput)
	}
}

func TestRegistrationHonorsConfigurationAndSetupState(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		handler := New(Dependencies{
			Auth: &fakeAPIAuth{}, Users: &fakeAPIUsers{},
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
			AllowedOrigins: []string{"http://localhost:3000"}, RegistrationEnabled: false,
		})
		request := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"member.one","password":"long-test-password"}`))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "registration_disabled") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("before administrator setup", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"member.one","password":"long-test-password"}`))
		response := httptest.NewRecorder()
		testAPI(&fakeAPIAuth{}, &fakeAPIUsers{setupRequired: true}).ServeHTTP(response, request)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "setup_required") {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
}

func TestAuthOptionsExposeOnlyPublicAuthenticationState(t *testing.T) {
	users := &fakeAPIUsers{setupRequired: true}
	handler := New(Dependencies{
		Auth: &fakeAPIAuth{}, Users: users,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
		AllowedOrigins: []string{"http://localhost:3000"}, SetupToken: "server-only-setup-token-value",
		RegistrationEnabled: true, OIDC: &fakeOIDCService{}, OIDCDisplayName: "公司账号",
	})
	request := httptest.NewRequest(http.MethodGet, "/api/auth/options", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"setup_required":true`) ||
		!strings.Contains(body, `"setup_enabled":true`) || !strings.Contains(body, `"registration_enabled":false`) ||
		!strings.Contains(body, `"oidc_enabled":false`) || !strings.Contains(body, `"oidc_display_name":"公司账号"`) {
		t.Fatalf("status=%d body=%s", response.Code, body)
	}
	if strings.Contains(body, "server-only-setup-token-value") {
		t.Fatal("authentication options leaked the setup token")
	}

	users.setupRequired = false
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	body = response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"registration_enabled":true`) ||
		!strings.Contains(body, `"oidc_enabled":true`) || !strings.Contains(body, `"setup_enabled":false`) {
		t.Fatalf("initialized status=%d body=%s", response.Code, body)
	}
}

func TestSetupRequiresServerTokenAndRejectsRepeat(t *testing.T) {
	users := &fakeAPIUsers{setupRequired: true}
	authService := &fakeAPIAuth{}
	handler := New(Dependencies{
		Auth: authService, Users: users, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		CookieName: "novro_session", AllowedOrigins: []string{"http://localhost:3000"},
		SetupToken: "a-setup-token-that-is-long-enough", RegistrationEnabled: true,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(`{"setup_token":"wrong","username":"admin","password":"long-test-password"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "invalid_setup_token") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	users.setupRequired = false
	request = httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(`{"setup_token":"a-setup-token-that-is-long-enough","username":"admin","password":"long-test-password"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "already_initialized") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSetupCreatesAdministratorAndSetsSession(t *testing.T) {
	created := user.Record{ID: uuid.New(), Username: "admin", Role: user.RoleAdmin, Status: user.StatusActive}
	authService := &fakeAPIAuth{login: auth.LoginResult{
		Token: "nvs_initialized-session", ExpiresAt: time.Now().Add(time.Hour), User: created,
	}}
	users := &fakeAPIUsers{setupRequired: true}
	handler := New(Dependencies{
		Auth: authService, Users: users, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		CookieName: "novro_session", CookieSecure: true, AllowedOrigins: []string{"http://localhost:3000"},
		SetupToken: "a-setup-token-that-is-long-enough", RegistrationEnabled: true,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(`{"setup_token":"a-setup-token-that-is-long-enough","username":"admin","display_name":"Administrator","password":"long-test-password"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || users.registerInput.Username != "admin" {
		t.Fatalf("status=%d body=%s input=%+v", response.Code, response.Body.String(), users.registerInput)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "novro_session" || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatalf("unexpected session cookie: %#v", cookies)
	}
	if strings.Contains(response.Body.String(), "long-test-password") || strings.Contains(response.Body.String(), "nvs_initialized-session") {
		t.Fatal("setup response leaked authentication material")
	}
}

func TestOIDCRoutesFailClosed(t *testing.T) {
	t.Run("disabled route", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/start", nil)
		response := httptest.NewRecorder()
		testAPI(&fakeAPIAuth{}, &fakeAPIUsers{}).ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("before setup", func(t *testing.T) {
		oidcService := &fakeOIDCService{flow: auth.OIDCFlow{AuthorizationURL: "https://id.example.com/authorize"}}
		handler := New(Dependencies{
			Auth: &fakeAPIAuth{}, Users: &fakeAPIUsers{setupRequired: true}, OIDC: oidcService,
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
			AllowedOrigins: []string{"http://localhost:3000"},
		})
		request := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/start", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusFound || response.Header().Get("Location") != "/login?error=setup_required" {
			t.Fatalf("status=%d location=%q", response.Code, response.Header().Get("Location"))
		}
	})

	t.Run("failed callback", func(t *testing.T) {
		handler := New(Dependencies{
			Auth: &fakeAPIAuth{}, Users: &fakeAPIUsers{}, OIDC: &fakeOIDCService{completeErr: auth.ErrInvalidOIDCFlow},
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
			AllowedOrigins: []string{"http://localhost:3000"},
		})
		request := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?code=code&state=state", nil)
		request.AddCookie(&http.Cookie{Name: "novro_oidc_flow", Value: "encrypted-flow"})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusFound || response.Header().Get("Location") != "/login?error=oidc_failed" {
			t.Fatalf("status=%d location=%q", response.Code, response.Header().Get("Location"))
		}
		cookies := response.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Name != "novro_oidc_flow" || cookies[0].MaxAge != -1 {
			t.Fatalf("OIDC flow cookie was not cleared: %#v", cookies)
		}
	})
}

func TestLoginSetsProtectedSessionCookie(t *testing.T) {
	authService := &fakeAPIAuth{login: auth.LoginResult{
		Token:     "nvs_test-token",
		ExpiresAt: time.Now().Add(time.Hour),
		User:      user.Record{ID: uuid.New(), Username: "admin", Role: user.RoleAdmin, Status: user.StatusActive},
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"correct-password"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	testAPI(authService, &fakeAPIUsers{}).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected session cookie: %#v", cookies)
	}
	if strings.Contains(response.Body.String(), "correct-password") || strings.Contains(response.Body.String(), "nvs_test-token") {
		t.Fatal("response leaked authentication material")
	}
}

func TestUnsafeRequestRejectsUnknownOrigin(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{}`))
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	testAPI(&fakeAPIAuth{}, &fakeAPIUsers{}).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "invalid_origin") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminRoutesRequireAdmin(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleMember, Status: user.StatusActive}}
	id := uuid.New().String()
	requests := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/admin/users", nil),
		httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+id, strings.NewReader(`{"display_name":"Denied"}`)),
		httptest.NewRequest(http.MethodGet, "/api/admin/api-keys", nil),
		httptest.NewRequest(http.MethodGet, "/api/admin/providers", nil),
	}
	for _, request := range requests {
		request.AddCookie(&http.Cookie{Name: "novro_session", Value: "nvs_member-token"})
		response := httptest.NewRecorder()
		testAPI(authService, &fakeAPIUsers{}).ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "forbidden") {
			t.Fatalf("%s %s: status=%d body=%s", request.Method, request.URL.Path, response.Code, response.Body.String())
		}
	}
}

func TestUserCreatesAndRevokesOnlyOwnAPIKeys(t *testing.T) {
	currentID := uuid.New()
	authService := &fakeAPIAuth{current: user.Record{ID: currentID, Role: user.RoleMember, Status: user.StatusActive}}
	keys := &fakeAPIKeys{}
	handler := testAPIWithKeys(authService, &fakeAPIUsers{}, keys)

	request := httptest.NewRequest(http.MethodPost, "/api/account/api-keys", strings.NewReader(`{"name":"Production"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || keys.createdUserID != currentID || keys.createdName != "Production" || !strings.Contains(response.Body.String(), "nvr_full-secret-returned-once") {
		t.Fatalf("status=%d body=%s keys=%+v", response.Code, response.Body.String(), keys)
	}

	keyID := uuid.New()
	request = httptest.NewRequest(http.MethodDelete, "/api/account/api-keys/"+keyID.String(), nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || keys.revokedUserID != currentID || keys.revokedID != keyID {
		t.Fatalf("status=%d body=%s keys=%+v", response.Code, response.Body.String(), keys)
	}
}

func TestAPIKeyErrorsAreStable(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleMember, Status: user.StatusActive}}
	keys := &fakeAPIKeys{err: apikey.ErrLimitReached}
	request := httptest.NewRequest(http.MethodPost, "/api/account/api-keys", strings.NewReader(`{"name":"Overflow"}`))
	response := httptest.NewRecorder()
	testAPIWithKeys(authService, &fakeAPIUsers{}, keys).ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "api_key_limit_reached") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminCreatesAndUpdatesProviderWithoutCredentialLeak(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	providers := &fakeProviders{}
	handler := New(Dependencies{
		Auth: authService, Users: &fakeAPIUsers{}, APIKeys: &fakeAPIKeys{}, Providers: providers,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
		AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/admin/providers", strings.NewReader(`{"code":"deepseek","display_name":"DeepSeek","protocol":"openai","base_url":"https://api.deepseek.com","api_key":"upstream-secret"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || providers.createInput.APIKey != "upstream-secret" || strings.Contains(response.Body.String(), "upstream-secret") {
		t.Fatalf("status=%d body=%s input=%+v", response.Code, response.Body.String(), providers.createInput)
	}

	id := uuid.New()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/providers/"+id.String(), strings.NewReader(`{"display_name":"DeepSeek API"}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || providers.updateInput.DisplayName == nil || *providers.updateInput.DisplayName != "DeepSeek API" {
		t.Fatalf("status=%d body=%s input=%+v", response.Code, response.Body.String(), providers.updateInput)
	}
}

func TestProviderValidationErrorIsStable(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	providers := &fakeProviders{err: provider.ErrCodeTaken}
	handler := New(Dependencies{Auth: authService, Users: &fakeAPIUsers{}, APIKeys: &fakeAPIKeys{}, Providers: providers, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	request := httptest.NewRequest(http.MethodPost, "/api/admin/providers", strings.NewReader(`{"code":"deepseek"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "provider_code_taken") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminCreatesUserAndProtectsLastAdmin(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	users := &fakeAPIUsers{}
	request := httptest.NewRequest(http.MethodPost, "/api/admin/users", strings.NewReader(`{"username":"member.one","display_name":"Member One","password":"long-test-password","role":"member"}`))
	response := httptest.NewRecorder()
	testAPI(authService, users).ServeHTTP(response, request)
	if response.Code != http.StatusCreated || users.createInput.Username != "member.one" {
		t.Fatalf("status=%d input=%+v body=%s", response.Code, users.createInput, response.Body.String())
	}

	users.statusErr = user.ErrLastActiveAdmin
	id := uuid.New()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+id.String()+"/status", strings.NewReader(`{"status":"disabled"}`))
	response = httptest.NewRecorder()
	testAPI(authService, users).ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "last_active_admin") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminUpdatesEditableUserFields(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	users := &fakeAPIUsers{}
	id := uuid.New()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+id.String(), strings.NewReader(`{"display_name":"Member Renamed","role":"admin"}`))
	response := httptest.NewRecorder()
	testAPI(authService, users).ServeHTTP(response, request)
	if response.Code != http.StatusOK || users.updateInput.DisplayName == nil || *users.updateInput.DisplayName != "Member Renamed" || users.updateInput.Role == nil || *users.updateInput.Role != user.RoleAdmin {
		t.Fatalf("status=%d input=%+v body=%s", response.Code, users.updateInput, response.Body.String())
	}
}

func TestAdminUpdateProtectsLastActiveAdministrator(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	users := &fakeAPIUsers{statusErr: user.ErrLastActiveAdmin}
	id := uuid.New()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+id.String(), strings.NewReader(`{"role":"member"}`))
	response := httptest.NewRecorder()
	testAPI(authService, users).ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "last_active_admin") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAuthenticationErrorsAreStable(t *testing.T) {
	authService := &fakeAPIAuth{authErr: auth.ErrUnauthenticated}
	request := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	response := httptest.NewRecorder()
	testAPI(authService, &fakeAPIUsers{}).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "unauthenticated") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	authService.authErr = errors.New("internal database detail")
	response = httptest.NewRecorder()
	testAPI(authService, &fakeAPIUsers{}).ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "database") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
