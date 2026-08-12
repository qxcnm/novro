package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
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
	"github.com/novro-gateway/novro/internal/upstreammodel"
	"github.com/novro-gateway/novro/internal/user"
)

type fakeAPIAuth struct {
	current         user.Record
	login           auth.LoginResult
	authErr         error
	logoutErr       error
	logoutToken     string
	loginIdentifier string
}

func (f *fakeAPIAuth) Login(_ context.Context, identifier, _ string) (auth.LoginResult, error) {
	f.loginIdentifier = identifier
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

func (f *fakeAPIAuth) Logout(_ context.Context, token string) error {
	f.logoutToken = token
	return f.logoutErr
}

type fakeAPIUsers struct {
	createInput   user.CreateInput
	updateInput   user.UpdateInput
	registerInput user.RegisterInput
	emailTaken    bool
	emailCheckErr error
	setupRequired bool
	initializeErr error
	statusErr     error
}

type fakeAPIEmailVerification struct {
	sentEmail, verifiedEmail, verifiedCode string
	sendErr, verifyErr                     error
}

type fakeAPIEmailConfig struct {
	config    email.AdminConfig
	input     email.ConfigInput
	recipient string
	err       error
}

func (f *fakeAPIEmailConfig) AdminConfig(context.Context) (email.AdminConfig, error) {
	return f.config, f.err
}

func (f *fakeAPIEmailConfig) UpdateConfig(_ context.Context, input email.ConfigInput) (email.AdminConfig, error) {
	f.input = input
	return f.config, f.err
}

func (f *fakeAPIEmailConfig) Test(_ context.Context, recipient string) error {
	f.recipient = recipient
	return f.err
}

func (f *fakeAPIEmailVerification) Send(_ context.Context, email string) error {
	f.sentEmail = email
	return f.sendErr
}

func (f *fakeAPIEmailVerification) Verify(_ context.Context, email, code string) error {
	f.verifiedEmail, f.verifiedCode = email, code
	return f.verifyErr
}

type fakeAPIKeys struct {
	createdUserID  uuid.UUID
	createdGroupID uuid.UUID
	createdName    string
	secretUserID   uuid.UUID
	secretID       uuid.UUID
	revokedUserID  uuid.UUID
	revokedID      uuid.UUID
	listFilter     apikey.ListFilter
	err            error
}

type fakeProviders struct {
	createInput provider.CreateInput
	updateInput provider.UpdateInput
	status      provider.Status
	deletedID   uuid.UUID
	err         error
}

type fakeProviderModels struct {
	syncProviderID uuid.UUID
	linkProviderID uuid.UUID
	modelIDs       []uuid.UUID
	syncModels     []providersync.CatalogModel
	linkResult     providersync.LinkResult
	err            error
}

type fakePayments struct {
	notification     url.Values
	listFilter       payment.AdminListFilter
	userListFilter   payment.ListFilter
	reconcileUserID  uuid.UUID
	reconcileOrderNo string
	reconciled       payment.Order
	err              error
}

type fakeReferrals struct {
	summary     referral.Summary
	config      referral.AdminConfig
	userID      uuid.UUID
	updatedRate int64
	err         error
}

type fakeGatewaySettings struct {
	config  gatewaysettings.Config
	updated gatewaysettings.Config
	err     error
}

type fakeAPIBilling struct {
	rate            billing.UsageRate
	rateUserID      uuid.UUID
	adjustReference uuid.UUID
	adjustAmount    int64
	err             error
}

func (f *fakeAPIBilling) Summary(context.Context, uuid.UUID) (billing.Summary, error) {
	return billing.Summary{}, f.err
}

func (f *fakeAPIBilling) SummaryPage(context.Context, uuid.UUID, billing.EntryFilter) (billing.Summary, error) {
	return billing.Summary{}, f.err
}

func (f *fakeAPIBilling) Usage(context.Context, uuid.UUID, billing.UsageFilter) (billing.UsagePage, error) {
	return billing.UsagePage{}, f.err
}

func (f *fakeAPIBilling) UsageRate(_ context.Context, userID uuid.UUID) (billing.UsageRate, error) {
	f.rateUserID = userID
	return f.rate, f.err
}

func (f *fakeAPIBilling) Adjust(_ context.Context, _, _, referenceID uuid.UUID, amount int64, _ string) (billing.Summary, error) {
	f.adjustReference, f.adjustAmount = referenceID, amount
	return billing.Summary{}, f.err
}

func (f *fakeGatewaySettings) Config(context.Context) (gatewaysettings.Config, error) {
	return f.config, f.err
}

func (f *fakeGatewaySettings) Update(_ context.Context, config gatewaysettings.Config) (gatewaysettings.Config, error) {
	f.updated = config
	if f.err != nil {
		return gatewaysettings.Config{}, f.err
	}
	f.config = config
	return config, nil
}

func (f *fakeReferrals) Summary(_ context.Context, userID uuid.UUID) (referral.Summary, error) {
	f.userID = userID
	return f.summary, f.err
}

func (f *fakeReferrals) AdminConfig(context.Context) (referral.AdminConfig, error) {
	return f.config, f.err
}

func (f *fakeReferrals) UpdateRewardBPS(_ context.Context, rewardBPS int64) (referral.AdminConfig, error) {
	f.updatedRate = rewardBPS
	if f.err != nil {
		return referral.AdminConfig{}, f.err
	}
	f.config.RewardBPS = rewardBPS
	return f.config, nil
}

type fakeModelRoutes struct {
	active            []modelroute.Record
	listActiveGroupID uuid.UUID
	err               error
}

type fakeBillingGroups struct {
	records     []billinggroup.Record
	createInput billinggroup.CreateInput
	updateInput billinggroup.UpdateInput
	status      billinggroup.Status
	deletedID   uuid.UUID
	listFilter  billinggroup.ListFilter
	createErr   error
	listErr     error
	updateErr   error
	statusErr   error
	deleteErr   error
}

func (f *fakeModelRoutes) Create(context.Context, modelroute.CreateInput) (modelroute.Record, error) {
	return modelroute.Record{}, f.err
}
func (f *fakeModelRoutes) List(context.Context, modelroute.ListFilter) ([]modelroute.Record, error) {
	return []modelroute.Record{}, f.err
}
func (f *fakeModelRoutes) ListActive(_ context.Context, billingGroupID uuid.UUID) ([]modelroute.Record, error) {
	f.listActiveGroupID = billingGroupID
	return f.active, f.err
}
func (f *fakeModelRoutes) Update(context.Context, uuid.UUID, modelroute.UpdateInput) (modelroute.Record, error) {
	return modelroute.Record{}, f.err
}
func (f *fakeModelRoutes) SetStatus(context.Context, uuid.UUID, modelroute.Status) (modelroute.Record, error) {
	return modelroute.Record{}, f.err
}
func (f *fakeModelRoutes) Delete(context.Context, uuid.UUID) error { return f.err }

func activeBillingGroup(id uuid.UUID, code, displayName string, multiplierBPS int64, isDefault bool) billinggroup.Record {
	return billinggroup.Record{
		ID: id, Code: code, DisplayName: displayName, MultiplierBPS: multiplierBPS,
		IsDefault: isDefault, Status: billinggroup.StatusActive,
	}
}

func defaultBillingGroups() *fakeBillingGroups {
	return &fakeBillingGroups{records: []billinggroup.Record{activeBillingGroup(uuid.New(), billinggroup.DefaultCode, "默认", billinggroup.DefaultMultiplierBPS, true)}}
}

func (f *fakeBillingGroups) Create(_ context.Context, input billinggroup.CreateInput) (billinggroup.Record, error) {
	f.createInput = input
	if f.createErr != nil {
		return billinggroup.Record{}, f.createErr
	}
	return activeBillingGroup(uuid.New(), input.Code, input.DisplayName, input.MultiplierBPS, false), nil
}

func (f *fakeBillingGroups) List(_ context.Context, filter billinggroup.ListFilter) ([]billinggroup.Record, error) {
	f.listFilter = filter
	if f.listErr != nil {
		return nil, f.listErr
	}
	records := f.records
	if records == nil {
		records = defaultBillingGroups().records
	}
	if filter.Status == "" {
		return records, nil
	}
	filtered := make([]billinggroup.Record, 0, len(records))
	for _, record := range records {
		authorized := false
		for _, authorizedUser := range record.AuthorizedUsers {
			if authorizedUser.ID == filter.AuthorizedUserID {
				authorized = true
				break
			}
		}
		if record.Status == filter.Status && (filter.IncludeHidden || !record.IsHidden || authorized) {
			filtered = append(filtered, record)
		}
	}
	return filtered, nil
}

func (f *fakeBillingGroups) Update(_ context.Context, id uuid.UUID, input billinggroup.UpdateInput) (billinggroup.Record, error) {
	f.updateInput = input
	if f.updateErr != nil {
		return billinggroup.Record{}, f.updateErr
	}
	record := activeBillingGroup(id, "default", "默认", billinggroup.DefaultMultiplierBPS, true)
	if input.DisplayName != nil {
		record.DisplayName = *input.DisplayName
	}
	if input.MultiplierBPS != nil {
		record.MultiplierBPS = *input.MultiplierBPS
	}
	return record, nil
}

func (f *fakeBillingGroups) SetStatus(_ context.Context, id uuid.UUID, status billinggroup.Status) (billinggroup.Record, error) {
	f.status = status
	if f.statusErr != nil {
		return billinggroup.Record{}, f.statusErr
	}
	record := activeBillingGroup(id, "default", "默认", billinggroup.DefaultMultiplierBPS, true)
	record.Status = status
	return record, nil
}

func (f *fakeBillingGroups) Delete(_ context.Context, id uuid.UUID) error {
	f.deletedID = id
	return f.deleteErr
}

func (f *fakePayments) Config(context.Context) (payment.PublicConfig, error) {
	return payment.PublicConfig{Enabled: true, Provider: "epay", Channels: []string{"alipay"}, MinMicros: payment.MinTopUpMicros, MaxMicros: payment.MaxTopUpMicros}, nil
}

func (f *fakePayments) AdminConfig(context.Context) (payment.AdminConfig, error) {
	return payment.AdminConfig{Provider: payment.ProviderEPay, Enabled: true, Configured: true, MerchantID: "1000", SiteName: "Novro", Channels: []string{"alipay"}, HasMerchantKey: true}, nil
}

func (f *fakePayments) UpdateConfig(_ context.Context, _ payment.ConfigInput) (payment.AdminConfig, error) {
	return f.AdminConfig(context.Background())
}

func (f *fakePayments) Create(_ context.Context, userID uuid.UUID, amount int64, channel string) (payment.CreateResult, error) {
	return payment.CreateResult{Order: payment.Order{ID: uuid.New(), UserID: userID, AmountMicros: amount, Channel: channel, Status: payment.StatusPending}}, f.err
}

func (f *fakePayments) List(_ context.Context, _ uuid.UUID, filter payment.ListFilter) (payment.Page, error) {
	f.userListFilter = filter
	return payment.Page{Orders: []payment.Order{}, Total: 0, Offset: filter.Offset, Limit: filter.Limit}, f.err
}

func (f *fakePayments) ReconcileForUser(_ context.Context, userID uuid.UUID, outTradeNo string) (payment.Order, error) {
	f.reconcileUserID = userID
	f.reconcileOrderNo = outTradeNo
	return f.reconciled, f.err
}

func (f *fakePayments) ListAll(_ context.Context, filter payment.AdminListFilter) (payment.AdminPage, error) {
	f.listFilter = filter
	return payment.AdminPage{Orders: []payment.AdminOrder{{Order: payment.Order{ID: uuid.New(), OutTradeNo: "NVR1", Status: payment.StatusPaid}, Owner: payment.TopUpOwner{Username: "alice"}}}, Total: 1, Limit: filter.Limit}, f.err
}

func (f *fakePayments) HandleNotification(_ context.Context, values url.Values) error {
	f.notification = values
	return f.err
}

func (f *fakeProviderModels) Sync(_ context.Context, providerID uuid.UUID) ([]providersync.CatalogModel, error) {
	f.syncProviderID = providerID
	return f.syncModels, f.err
}

func (f *fakeProviderModels) Link(_ context.Context, providerID uuid.UUID, modelIDs []uuid.UUID) (providersync.LinkResult, error) {
	f.linkProviderID = providerID
	f.modelIDs = modelIDs
	return f.linkResult, f.err
}

func (f *fakeProviders) Create(_ context.Context, input provider.CreateInput) (provider.Record, error) {
	f.createInput = input
	return provider.Record{ID: uuid.New(), BillingGroupID: input.BillingGroupID, BillingGroup: billinggroup.Summary{ID: input.BillingGroupID, DisplayName: "默认", MultiplierBPS: billinggroup.DefaultMultiplierBPS}, Code: input.Code, DisplayName: input.DisplayName, Protocol: input.Protocol, BaseURL: input.BaseURL, APIKeyHint: "1234", HasAPIKey: true, Status: provider.StatusActive}, f.err
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
func (f *fakeProviders) Delete(_ context.Context, id uuid.UUID) error {
	f.deletedID = id
	return f.err
}

func (f *fakeAPIKeys) Create(_ context.Context, userID, billingGroupID uuid.UUID, name string) (apikey.CreateResult, error) {
	f.createdUserID, f.createdGroupID, f.createdName = userID, billingGroupID, name
	if f.err != nil {
		return apikey.CreateResult{}, f.err
	}
	return apikey.CreateResult{
		APIKey: apikey.Record{ID: uuid.New(), UserID: userID, BillingGroupID: billingGroupID, BillingGroup: billinggroup.Summary{ID: billingGroupID, DisplayName: "默认", MultiplierBPS: billinggroup.DefaultMultiplierBPS}, Name: name, KeyPrefix: "nvr_example", CanCopySecret: true, Status: apikey.StatusActive},
		Key:    "nvr_full-secret-returned-once",
	}, nil
}

func (f *fakeAPIKeys) ListForUser(context.Context, uuid.UUID) ([]apikey.Record, error) {
	return []apikey.Record{}, f.err
}

func (f *fakeAPIKeys) RevealForUser(_ context.Context, userID, id uuid.UUID) (string, error) {
	f.secretUserID, f.secretID = userID, id
	if f.err != nil {
		return "", f.err
	}
	return "nvr_full-secret-returned-once", nil
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

func (f *fakeAPIUsers) EmailAvailable(context.Context, string) (bool, error) {
	if f.emailCheckErr != nil {
		return false, f.emailCheckErr
	}
	return !f.emailTaken, nil
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
func (f *fakeAPIUsers) FindByUsername(context.Context, string) (user.Record, error) {
	return user.Record{}, user.ErrNotFound
}

func testAPI(authService *fakeAPIAuth, users *fakeAPIUsers) http.Handler {
	return testAPIWithKeys(authService, users, &fakeAPIKeys{})
}

func testAPIWithKeys(authService *fakeAPIAuth, users *fakeAPIUsers, apiKeys *fakeAPIKeys) http.Handler {
	emailVerification := &fakeAPIEmailVerification{}
	inner := New(Dependencies{
		Auth:                authService,
		Users:               users,
		APIKeys:             apiKeys,
		Providers:           &fakeProviders{},
		BillingGroups:       defaultBillingGroups(),
		Logger:              slog.New(slog.NewTextHandler(io.Discard, nil)),
		CookieName:          "novro_session",
		CookieSecure:        true,
		AllowedOrigins:      []string{"http://localhost:3000"},
		RegistrationEnabled: true,
		EmailVerification:   emailVerification,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && r.Header.Get("Origin") == "" {
			r.Header.Set("Origin", "http://localhost:3000")
		}
		inner.ServeHTTP(w, r)
	})
}

func TestRegistrationCreatesMemberAndSetsSession(t *testing.T) {
	users := &fakeAPIUsers{}
	authService := &fakeAPIAuth{login: auth.LoginResult{
		Token: "nvs_test-token", ExpiresAt: time.Now().Add(time.Hour),
		User: user.Record{ID: uuid.New(), Username: "member.one", Role: user.RoleMember, Status: user.StatusActive},
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"member.one","email":"member@example.com","display_name":"Member","password":"long-test-password","verification_code":"123456","referral_code":"ABCD1234EF56"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	testAPI(authService, users).ServeHTTP(response, request)
	if response.Code != http.StatusCreated || users.registerInput.Username != "member.one" || users.registerInput.Email != "member@example.com" || users.registerInput.ReferralCode != "ABCD1234EF56" {
		t.Fatalf("status=%d body=%s input=%+v", response.Code, response.Body.String(), users.registerInput)
	}
}

func TestReferralSummaryUsesAuthenticatedUser(t *testing.T) {
	userID := uuid.New()
	referrals := &fakeReferrals{summary: referral.Summary{
		InviteCode: "ABCD1234EF56", InviteURL: "https://app.example.invalid/register?ref=ABCD1234EF56",
		InvitedCount: 4, PendingRewardMicros: 1_000_000, TotalRewardMicros: 2_000_000, RewardBPS: 1_000,
		Invitations: []referral.Invitation{{Username: "member.one", DisplayName: "Member One"}},
		Rewards:     []referral.Reward{{Username: "member.one", DisplayName: "Member One", PaidAmountMicros: 10_000_000, RewardMicros: 1_000_000}},
	}}
	handler := New(Dependencies{
		Auth:  &fakeAPIAuth{current: user.Record{ID: userID, Role: user.RoleMember, Status: user.StatusActive}},
		Users: &fakeAPIUsers{}, Referrals: referrals, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	request := httptest.NewRequest(http.MethodGet, "/api/account/referral", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || referrals.userID != userID || !strings.Contains(response.Body.String(), "ABCD1234EF56") ||
		!strings.Contains(response.Body.String(), "\"display_name\":\"Member One\"") || strings.Contains(response.Body.String(), "\"email\"") {
		t.Fatalf("status=%d user=%s body=%s", response.Code, referrals.userID, response.Body.String())
	}
}

func TestUsageRateUsesAuthenticatedUser(t *testing.T) {
	userID := uuid.New()
	billingService := &fakeAPIBilling{rate: billing.UsageRate{
		WindowSeconds: 60, Requests: 4, InputTokens: 800, OutputTokens: 200,
		TotalTokens: 1000, RPM: 4, TPM: 1000, CalculatedAt: time.Date(2026, time.August, 12, 7, 2, 0, 0, time.UTC),
	}}
	handler := New(Dependencies{
		Auth:    &fakeAPIAuth{current: user.Record{ID: userID, Role: user.RoleMember, Status: user.StatusActive}},
		Users:   &fakeAPIUsers{},
		Billing: billingService,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/account/usage/rate", nil))

	if response.Code != http.StatusOK || billingService.rateUserID != userID || !strings.Contains(response.Body.String(), `"rpm":4`) || !strings.Contains(response.Body.String(), `"tpm":1000`) || !strings.Contains(response.Body.String(), `"window_seconds":60`) {
		t.Fatalf("status=%d user=%s body=%s", response.Code, billingService.rateUserID, response.Body.String())
	}
}

func TestAdminReferralConfigRequiresAdminAndUpdates(t *testing.T) {
	referrals := &fakeReferrals{config: referral.AdminConfig{RewardBPS: 1_000}}
	handler := New(Dependencies{
		Auth:  &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}},
		Users: &fakeAPIUsers{}, Referrals: referrals, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AllowedOrigins: []string{"http://localhost:3000"},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/admin/referral", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"reward_bps":1000`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/api/admin/referral", strings.NewReader(`{"reward_bps":625}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || referrals.updatedRate != 625 || !strings.Contains(response.Body.String(), `"reward_bps":625`) {
		t.Fatalf("status=%d rate=%d body=%s", response.Code, referrals.updatedRate, response.Body.String())
	}

	memberHandler := New(Dependencies{
		Auth:  &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleMember, Status: user.StatusActive}},
		Users: &fakeAPIUsers{}, Referrals: referrals, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	request = httptest.NewRequest(http.MethodGet, "/api/admin/referral", nil)
	response = httptest.NewRecorder()
	memberHandler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("member status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminReferralConfigRejectsInvalidRate(t *testing.T) {
	referrals := &fakeReferrals{err: referral.ErrInvalidInput}
	handler := New(Dependencies{
		Auth:  &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}},
		Users: &fakeAPIUsers{}, Referrals: referrals, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodPut, "/api/admin/referral", strings.NewReader(`{"reward_bps":10001}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_referral_config") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminGatewaySettingsRequiresAdminAndUpdates(t *testing.T) {
	settings := &fakeGatewaySettings{config: gatewaysettings.DefaultConfig()}
	handler := New(Dependencies{
		Auth: &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}, Users: &fakeAPIUsers{},
		GatewaySettings: settings, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/admin/gateway-settings", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"sse_heartbeat_interval_ms":15000`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	body := `{"sse_heartbeat_enabled":false,"sse_heartbeat_interval_ms":30000,"upstream_timeout_ms":120000,"upstream_stream_idle_timeout_ms":45000,"reservation_input_token_cap":32768,"reservation_output_token_cap":2048}`
	request = httptest.NewRequest(http.MethodPut, "/api/admin/gateway-settings", strings.NewReader(body))
	request.Header.Set("Origin", "http://localhost:3000")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || settings.updated.SSEHeartbeatEnabled || settings.updated.UpstreamTimeoutMS != 120_000 || settings.updated.UpstreamStreamIdleTimeoutMS != 45_000 || settings.updated.ReservationInputTokenCap != 32_768 || settings.updated.ReservationOutputTokenCap != 2048 {
		t.Fatalf("status=%d settings=%+v body=%s", response.Code, settings.updated, response.Body.String())
	}

	memberHandler := New(Dependencies{
		Auth: &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleMember, Status: user.StatusActive}}, Users: &fakeAPIUsers{},
		GatewaySettings: settings, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	request = httptest.NewRequest(http.MethodGet, "/api/admin/gateway-settings", nil)
	response = httptest.NewRecorder()
	memberHandler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("member status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminGatewaySettingsRejectsInvalidValues(t *testing.T) {
	settings := &fakeGatewaySettings{err: gatewaysettings.ErrInvalidConfig}
	handler := New(Dependencies{
		Auth: &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}, Users: &fakeAPIUsers{},
		GatewaySettings: settings, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodPut, "/api/admin/gateway-settings", strings.NewReader(`{"sse_heartbeat_enabled":true,"sse_heartbeat_interval_ms":0,"upstream_timeout_ms":0,"upstream_stream_idle_timeout_ms":0}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_gateway_settings") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminBalanceAdjustmentRequiresAndStabilizesIdempotencyKey(t *testing.T) {
	admin := user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}
	billingService := &fakeAPIBilling{}
	handler := New(Dependencies{Auth: &fakeAPIAuth{current: admin}, Users: &fakeAPIUsers{}, Billing: billingService, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session", AllowedOrigins: []string{"http://localhost:3000"}})
	userID := uuid.New()
	makeRequest := func(key string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+userID.String()+"/balance-adjustments", strings.NewReader(`{"amount_micros":1000000,"note":"test"}`))
		request.Header.Set("Origin", "http://localhost:3000")
		if key != "" {
			request.Header.Set("Idempotency-Key", key)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := makeRequest(""); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_idempotency_key") {
		t.Fatalf("missing key status=%d body=%s", response.Code, response.Body.String())
	}
	first := makeRequest("adjust-once")
	firstReference := billingService.adjustReference
	second := makeRequest("adjust-once")
	if first.Code != http.StatusOK || second.Code != http.StatusOK || firstReference == uuid.Nil || billingService.adjustReference != firstReference || billingService.adjustAmount != 1_000_000 {
		t.Fatalf("first=%d second=%d reference=%s amount=%d", first.Code, second.Code, billingService.adjustReference, billingService.adjustAmount)
	}
}

func TestAdminBalanceAdjustmentReturnsConflictForChangedIdempotentRequest(t *testing.T) {
	admin := user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}
	billingService := &fakeAPIBilling{err: billing.ErrRequestConflict}
	handler := New(Dependencies{Auth: &fakeAPIAuth{current: admin}, Users: &fakeAPIUsers{}, Billing: billingService, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"http://localhost:3000"}})
	request := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+uuid.NewString()+"/balance-adjustments", strings.NewReader(`{"amount_micros":1000000,"note":"test"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Idempotency-Key", "reused-adjustment")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"idempotency_conflict"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLogoutClearsCookieEvenWhenRevocationFails(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		logoutErr  error
		wantStatus int
	}{
		{name: "success", wantStatus: http.StatusNoContent},
		{name: "storage error", logoutErr: errors.New("storage unavailable"), wantStatus: http.StatusInternalServerError},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			authService := &fakeAPIAuth{logoutErr: testCase.logoutErr}
			handler := New(Dependencies{
				Auth: authService, Users: &fakeAPIUsers{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
				CookieName: "novro_session", AllowedOrigins: []string{"http://localhost:3000"},
			})
			request := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
			request.Header.Set("Origin", "http://localhost:3000")
			request.AddCookie(&http.Cookie{Name: "novro_session", Value: "nvs_logout-token"})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			cookies := response.Result().Cookies()
			if response.Code != testCase.wantStatus || authService.logoutToken != "nvs_logout-token" || len(cookies) != 1 || cookies[0].Name != "novro_session" || cookies[0].MaxAge != -1 {
				t.Fatalf("status=%d token=%q cookies=%#v body=%s", response.Code, authService.logoutToken, cookies, response.Body.String())
			}
		})
	}
}

func TestEPayNotificationBypassesBrowserOriginCheck(t *testing.T) {
	payments := &fakePayments{}
	handler := New(Dependencies{
		Payments: payments, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/payments/epay/notify", strings.NewReader("pid=1000&out_trade_no=NVR1"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "success" {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if payments.notification.Get("out_trade_no") != "NVR1" {
		t.Fatalf("unexpected notification: %#v", payments.notification)
	}
}

func TestEPayNotificationAcceptsBodyAboveFormerLimit(t *testing.T) {
	payments := &fakePayments{}
	handler := New(Dependencies{Payments: payments, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	padding := strings.Repeat("x", (64<<10)+1024)
	request := httptest.NewRequest(http.MethodPost, "/api/payments/epay/notify", strings.NewReader("pid=1000&out_trade_no=NVR1&padding="+padding))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(payments.notification.Get("padding")) != len(padding) {
		t.Fatalf("status=%d parsed_padding=%d want=%d", response.Code, len(payments.notification.Get("padding")), len(padding))
	}
}

func TestEPayNotificationPrefersSignedFormBodyOverQueryValues(t *testing.T) {
	payments := &fakePayments{}
	handler := New(Dependencies{Payments: payments, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	request := httptest.NewRequest(http.MethodPost, "/api/payments/epay/notify?out_trade_no=QUERY", strings.NewReader("pid=1000&out_trade_no=NVR1"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || payments.notification.Get("out_trade_no") != "NVR1" {
		t.Fatalf("status=%d out_trade_no=%q", response.Code, payments.notification.Get("out_trade_no"))
	}
}

func TestEPayReturnCompletesSignedResultAndRemovesUIParameter(t *testing.T) {
	payments := &fakePayments{}
	handler := New(Dependencies{
		Payments: payments, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/payments/epay/return?payment=returned&pid=1000&out_trade_no=NVR1&sign=signed", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/console/billing?payment=returned" {
		t.Fatalf("status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
	if payments.notification.Get("out_trade_no") != "NVR1" || payments.notification.Has("payment") {
		t.Fatalf("unexpected return values: %#v", payments.notification)
	}
}

func TestUserCanReconcileOnlyThroughAuthenticatedOrderFlow(t *testing.T) {
	userID := uuid.New()
	payments := &fakePayments{reconciled: payment.Order{ID: uuid.New(), UserID: userID, OutTradeNo: "NVR1", Status: payment.StatusPaid}}
	handler := New(Dependencies{
		Auth:  &fakeAPIAuth{current: user.Record{ID: userID, Role: user.RoleMember, Status: user.StatusActive}},
		Users: &fakeAPIUsers{}, Payments: payments, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		CookieName: "novro_session", AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/account/top-ups/NVR1/reconcile", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	request.AddCookie(&http.Cookie{Name: "novro_session", Value: "nvs_member-token"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || payments.reconcileUserID != userID || payments.reconcileOrderNo != "NVR1" || !strings.Contains(response.Body.String(), `"status":"paid"`) {
		t.Fatalf("status=%d body=%s user=%s order=%q", response.Code, response.Body.String(), payments.reconcileUserID, payments.reconcileOrderNo)
	}
}

func TestUserListsTopUpsWithPagination(t *testing.T) {
	userID := uuid.New()
	payments := &fakePayments{}
	handler := New(Dependencies{
		Auth:  &fakeAPIAuth{current: user.Record{ID: userID, Role: user.RoleMember, Status: user.StatusActive}},
		Users: &fakeAPIUsers{}, Payments: payments, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		CookieName: "novro_session",
	})
	request := httptest.NewRequest(http.MethodGet, "/api/account/top-ups?offset=20&limit=50", nil)
	request.AddCookie(&http.Cookie{Name: "novro_session", Value: "nvs_member-token"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || payments.userListFilter.Offset != 20 || payments.userListFilter.Limit != 50 || !strings.Contains(response.Body.String(), `"total":0`) {
		t.Fatalf("status=%d body=%s filter=%+v", response.Code, response.Body.String(), payments.userListFilter)
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
		request.Header.Set("Origin", "http://localhost:3000")
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

func TestRegistrationEmailVerificationEndpoints(t *testing.T) {
	users := &fakeAPIUsers{}
	authService := &fakeAPIAuth{login: auth.LoginResult{Token: "nvs_registration-session", ExpiresAt: time.Now().Add(time.Hour)}}
	verification := &fakeAPIEmailVerification{}
	handler := New(Dependencies{
		Auth: authService, Users: users, EmailVerification: verification,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
		AllowedOrigins: []string{"http://localhost:3000"}, RegistrationEnabled: true,
	})
	send := httptest.NewRequest(http.MethodPost, "/api/auth/register/send-code", strings.NewReader(`{"email":"member@example.com"}`))
	send.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, send)
	if response.Code != http.StatusOK || verification.sentEmail != "member@example.com" {
		t.Fatalf("send status=%d body=%s email=%q", response.Code, response.Body.String(), verification.sentEmail)
	}

	verification.verifyErr = email.ErrInvalidCode
	register := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"member.one","email":"member@example.com","password":"long-test-password","verification_code":"000000"}`))
	register.Header.Set("Origin", "http://localhost:3000")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, register)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "verification_invalid") {
		t.Fatalf("invalid verification status=%d body=%s", response.Code, response.Body.String())
	}

	verification.verifyErr = nil
	register = httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"member.one","email":"member@example.com","password":"long-test-password","verification_code":"123456"}`))
	register.Header.Set("Origin", "http://localhost:3000")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, register)
	if response.Code != http.StatusCreated || verification.verifiedCode != "123456" {
		t.Fatalf("register status=%d body=%s code=%q", response.Code, response.Body.String(), verification.verifiedCode)
	}
}

func TestRegistrationEmailAvailabilityRejectsTakenEmailBeforeSendingCode(t *testing.T) {
	users := &fakeAPIUsers{emailTaken: true}
	verification := &fakeAPIEmailVerification{}
	handler := New(Dependencies{
		Auth: &fakeAPIAuth{}, Users: users, EmailVerification: verification,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
		AllowedOrigins: []string{"http://localhost:3000"}, RegistrationEnabled: true,
	})
	send := httptest.NewRequest(http.MethodPost, "/api/auth/register/send-code", strings.NewReader(`{"email":"member@example.com"}`))
	send.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, send)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "email_taken") || verification.sentEmail != "" {
		t.Fatalf("send status=%d body=%s sent=%q", response.Code, response.Body.String(), verification.sentEmail)
	}
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
	request := httptest.NewRequest(http.MethodPost, "/api/auth/setup", strings.NewReader(`{"setup_token":"a-setup-token-that-is-long-enough","username":"admin","email":"admin@example.com","display_name":"Administrator","password":"long-test-password"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || users.registerInput.Username != "admin" || users.registerInput.Email != "admin@example.com" {
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

func TestLoginAcceptsEmailIdentifier(t *testing.T) {
	authService := &fakeAPIAuth{login: auth.LoginResult{
		Token: "nvs_test-token", ExpiresAt: time.Now().Add(time.Hour),
		User: user.Record{ID: uuid.New(), Username: "alice", Email: "alice@example.com", Role: user.RoleMember, Status: user.StatusActive},
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"alice@example.com","password":"correct-password"}`))
	response := httptest.NewRecorder()
	testAPI(authService, &fakeAPIUsers{}).ServeHTTP(response, request)
	if response.Code != http.StatusOK || authService.loginIdentifier != "alice@example.com" {
		t.Fatalf("status=%d body=%s identifier=%q", response.Code, response.Body.String(), authService.loginIdentifier)
	}
}

func TestEmailConflictUsesStableSafeError(t *testing.T) {
	response := httptest.NewRecorder()
	(&apiHandler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}).writeUserError(response, "create user", fmt.Errorf("%w: internal constraint", user.ErrEmailTaken))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "email_taken") || strings.Contains(response.Body.String(), "internal constraint") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
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

func TestUnsafeAPIRequestRequiresAllowedOrigin(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{}`))
	response := httptest.NewRecorder()
	New(Dependencies{
		Auth: &fakeAPIAuth{}, Users: &fakeAPIUsers{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		CookieName: "novro_session", AllowedOrigins: []string{"http://localhost:3000"},
	}).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "invalid_origin") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestGatewayRequestDoesNotUseBrowserOriginValidation(t *testing.T) {
	called := false
	handler := New(Dependencies{
		Auth: &fakeAPIAuth{}, Users: &fakeAPIUsers{},
		Gateway: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}),
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !called || response.Code != http.StatusNoContent {
		t.Fatalf("gateway called=%v status=%d body=%s", called, response.Code, response.Body.String())
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
		httptest.NewRequest(http.MethodGet, "/api/admin/payments", nil),
		httptest.NewRequest(http.MethodGet, "/api/admin/email", nil),
		httptest.NewRequest(http.MethodPut, "/api/admin/email", strings.NewReader(`{"enabled":false,"port":587,"security":"starttls"}`)),
		httptest.NewRequest(http.MethodPost, "/api/admin/email/test", strings.NewReader(`{"recipient":"admin@example.com"}`)),
		httptest.NewRequest(http.MethodGet, "/api/admin/top-ups", nil),
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

func TestAdminListsTopUpsWithFilters(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	payments := &fakePayments{}
	handler := New(Dependencies{
		Auth: authService, Users: &fakeAPIUsers{}, Payments: payments,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
	})
	request := httptest.NewRequest(http.MethodGet, "/api/admin/top-ups?search=alice&status=paid&channel=alipay&offset=20&limit=20", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"username":"alice"`) || payments.listFilter.Search != "alice" || payments.listFilter.Status != payment.StatusPaid || payments.listFilter.Channel != "alipay" || payments.listFilter.Offset != 20 || payments.listFilter.Limit != 20 {
		t.Fatalf("status=%d body=%s filter=%+v", response.Code, response.Body.String(), payments.listFilter)
	}
}

func TestAdminPaymentConfigDoesNotReturnMerchantKey(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	handler := New(Dependencies{
		Auth: authService, Users: &fakeAPIUsers{}, Payments: &fakePayments{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
		AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/admin/payments", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"has_merchant_key":true`) || strings.Contains(response.Body.String(), "merchant-secret") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminPaymentConfigWithoutServiceReturnsWrappedEmptyChannels(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	handler := New(Dependencies{
		Auth: authService, Users: &fakeAPIUsers{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
		AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/admin/payments", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	request.AddCookie(&http.Cookie{Name: "novro_session", Value: "nvs_admin-token"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	var body struct {
		PaymentConfig payment.AdminConfig `json:"payment_config"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
	if response.Code != http.StatusOK || body.PaymentConfig.Provider != payment.ProviderEPay || body.PaymentConfig.Channels == nil || len(body.PaymentConfig.Channels) != 0 {
		t.Fatalf("status=%d payment_config=%+v body=%s", response.Code, body.PaymentConfig, response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"channels":null`) {
		t.Fatalf("empty channels encoded as null: %s", response.Body.String())
	}
}

func TestAdminEmailConfigIsSecretSafeAndSupportsUpdateAndTest(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	emailConfig := &fakeAPIEmailConfig{config: email.AdminConfig{
		Enabled: true, Configured: true, Host: "smtp.example.com", Port: 587,
		Username: "verify@example.com", FromAddress: "verify@example.com",
		Security: email.SecuritySTARTTLS, HasPassword: true,
	}}
	handler := New(Dependencies{
		Auth: authService, Users: &fakeAPIUsers{}, EmailConfig: emailConfig,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
		AllowedOrigins: []string{"http://localhost:3000"},
	})

	request := httptest.NewRequest(http.MethodGet, "/api/admin/email", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"has_password":true`) || strings.Contains(response.Body.String(), "smtp-secret") || strings.Contains(response.Body.String(), `"password"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/api/admin/email", strings.NewReader(`{"enabled":true,"host":"smtp.example.com","port":465,"username":"verify@example.com","password":"smtp-secret","from_address":"verify@example.com","security":"ssl"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || emailConfig.input.Password == nil || *emailConfig.input.Password != "smtp-secret" || emailConfig.input.Security != email.SecuritySSL || strings.Contains(response.Body.String(), "smtp-secret") {
		t.Fatalf("status=%d input=%+v body=%s", response.Code, emailConfig.input, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/admin/email/test", strings.NewReader(`{"recipient":"admin@example.com"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || emailConfig.recipient != "admin@example.com" || !strings.Contains(response.Body.String(), `"sent":true`) {
		t.Fatalf("status=%d recipient=%q body=%s", response.Code, emailConfig.recipient, response.Body.String())
	}
}

func TestAdminEmailConfigRejectsInvalidInputWithoutLeakingDetails(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	handler := New(Dependencies{
		Auth: authService, Users: &fakeAPIUsers{}, EmailConfig: &fakeAPIEmailConfig{err: email.ErrInvalidConfig},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
		AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodPut, "/api/admin/email", strings.NewReader(`{"enabled":true,"host":"bad"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_email_config") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestUserCreatesAndRevokesOnlyOwnAPIKeys(t *testing.T) {
	currentID := uuid.New()
	groupID := uuid.New()
	authService := &fakeAPIAuth{current: user.Record{ID: currentID, Role: user.RoleMember, Status: user.StatusActive}}
	keys := &fakeAPIKeys{}
	handler := testAPIWithKeys(authService, &fakeAPIUsers{}, keys)

	request := httptest.NewRequest(http.MethodPost, "/api/account/api-keys", strings.NewReader(`{"name":"Production","billing_group_id":"`+groupID.String()+`"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || keys.createdUserID != currentID || keys.createdGroupID != groupID || keys.createdName != "Production" || !strings.Contains(response.Body.String(), "nvr_full-secret-returned-once") {
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

func TestUserCanReCopyCreatedAPIKeySecret(t *testing.T) {
	currentID := uuid.New()
	keyID := uuid.New()
	authService := &fakeAPIAuth{current: user.Record{ID: currentID, Role: user.RoleMember, Status: user.StatusActive}}
	keys := &fakeAPIKeys{}
	handler := testAPIWithKeys(authService, &fakeAPIUsers{}, keys)

	request := httptest.NewRequest(http.MethodGet, "/api/account/api-keys/"+keyID.String()+"/secret", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || keys.secretUserID != currentID || keys.secretID != keyID || !strings.Contains(response.Body.String(), "nvr_full-secret-returned-once") {
		t.Fatalf("status=%d body=%s keys=%+v", response.Code, response.Body.String(), keys)
	}
}

func TestUserListsOnlyActiveModelsAtTheirBillingGroupPrices(t *testing.T) {
	currentID := uuid.New()
	groupID := uuid.New()
	authService := &fakeAPIAuth{current: user.Record{
		ID: currentID, Role: user.RoleMember, Status: user.StatusActive,
	}}
	groups := &fakeBillingGroups{records: []billinggroup.Record{activeBillingGroup(groupID, "personal", "个人版", 12_500, false)}}
	routes := &fakeModelRoutes{active: []modelroute.Record{{
		PublicName: "deepseek-chat", DisplayName: "DeepSeek Chat",
		Provider: modelroute.ProviderSummary{DisplayName: "DeepSeek", Protocol: provider.ProtocolOpenAI},
		UpstreamModel: &upstreammodel.Record{Prices: upstreammodel.Prices{
			InputMicros: 2_000_000, OutputMicros: 8_000_000, CacheReadMicros: 200_000, RequestMicros: 1_000,
		}},
	}}}
	handler := New(Dependencies{
		Auth: authService, Users: &fakeAPIUsers{}, APIKeys: &fakeAPIKeys{}, BillingGroups: groups, ModelRoutes: routes,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
	})
	request := httptest.NewRequest(http.MethodGet, "/api/account/models?billing_group_id="+groupID.String(), nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	var body struct {
		Models []struct {
			ID           string            `json:"id"`
			ProviderName string            `json:"provider_name"`
			Protocol     provider.Protocol `json:"protocol"`
			Prices       billing.RateCard  `json:"prices"`
		} `json:"models"`
		BillingGroup struct {
			DisplayName   string `json:"display_name"`
			MultiplierBPS int64  `json:"multiplier_bps"`
		} `json:"billing_group"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
	if response.Code != http.StatusOK || len(body.Models) != 1 || routes.listActiveGroupID != groupID || groups.listFilter.Status != billinggroup.StatusActive {
		t.Fatalf("status=%d models=%+v routeGroup=%s groupFilter=%+v body=%s", response.Code, body.Models, routes.listActiveGroupID, groups.listFilter, response.Body.String())
	}
	model := body.Models[0]
	if model.ID != "deepseek-chat" || model.ProviderName != "DeepSeek" || model.Protocol != provider.ProtocolOpenAI {
		t.Fatalf("unexpected model metadata: %+v", model)
	}
	if body.BillingGroup.DisplayName != "个人版" || body.BillingGroup.MultiplierBPS != 12_500 {
		t.Fatalf("unexpected billing group: %+v", body.BillingGroup)
	}
	if model.Prices.InputMicros != 2_500_000 || model.Prices.OutputMicros != 10_000_000 || model.Prices.CacheReadMicros != 250_000 || model.Prices.RequestMicros != 1_250 {
		t.Fatalf("unexpected customer prices: %+v", model.Prices)
	}
}

func TestUserListsActiveBillingGroups(t *testing.T) {
	currentID := uuid.New()
	activeID := uuid.New()
	disabledID := uuid.New()
	active := activeBillingGroup(activeID, "personal", "个人版", 12_500, false)
	active.APIKeyCount = 7
	active.ProviderCount = 3
	active.CreatedAt = time.Unix(1_700_000_000, 0).UTC()
	hiddenID := uuid.New()
	hidden := activeBillingGroup(hiddenID, "partner", "代理折扣", 3_000, false)
	hidden.IsHidden = true
	groups := &fakeBillingGroups{records: []billinggroup.Record{
		active,
		hidden,
		{ID: disabledID, Code: "disabled", DisplayName: "停用组", MultiplierBPS: 20_000, Status: billinggroup.StatusDisabled},
	}}
	handler := New(Dependencies{
		Auth:  &fakeAPIAuth{current: user.Record{ID: currentID, Role: user.RoleMember, Status: user.StatusActive}},
		Users: &fakeAPIUsers{}, BillingGroups: groups,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/account/billing-groups", nil))
	body := response.Body.String()
	if response.Code != http.StatusOK || groups.listFilter.Status != billinggroup.StatusActive || groups.listFilter.IncludeHidden || !strings.Contains(body, activeID.String()) || strings.Contains(body, hiddenID.String()) || strings.Contains(body, disabledID.String()) || strings.Contains(body, "api_key_count") || strings.Contains(body, "provider_count") || strings.Contains(body, "created_at") {
		t.Fatalf("status=%d filter=%+v body=%s", response.Code, groups.listFilter, response.Body.String())
	}
}

func TestAuthorizedUserListsHiddenBillingGroups(t *testing.T) {
	userID := uuid.New()
	partnerID := uuid.New()
	partner := activeBillingGroup(partnerID, "partner", "代理折扣", 3_000, false)
	partner.IsHidden = true
	partner.AuthorizedUsers = []billinggroup.AuthorizedUser{{ID: userID, Username: "partner-user"}}
	businessID := uuid.New()
	business := activeBillingGroup(businessID, "business", "B 端专用", 8_000, false)
	business.IsHidden = true
	groups := &fakeBillingGroups{records: []billinggroup.Record{partner, business}}
	handler := New(Dependencies{
		Auth:  &fakeAPIAuth{current: user.Record{ID: userID, Role: user.RoleMember, Status: user.StatusActive}},
		Users: &fakeAPIUsers{}, BillingGroups: groups,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/account/billing-groups", nil))
	if response.Code != http.StatusOK || groups.listFilter.IncludeHidden || groups.listFilter.AuthorizedUserID != userID || !strings.Contains(response.Body.String(), partnerID.String()) || strings.Contains(response.Body.String(), businessID.String()) {
		t.Fatalf("status=%d filter=%+v body=%s", response.Code, groups.listFilter, response.Body.String())
	}
}

func TestUserModelListRejectsHiddenGroupWithoutPermission(t *testing.T) {
	hiddenID := uuid.New()
	hidden := activeBillingGroup(hiddenID, "partner", "代理折扣", 3_000, false)
	hidden.IsHidden = true
	groups := &fakeBillingGroups{records: []billinggroup.Record{hidden}}
	routes := &fakeModelRoutes{}
	handler := New(Dependencies{
		Auth:  &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleMember, Status: user.StatusActive}},
		Users: &fakeAPIUsers{}, BillingGroups: groups, ModelRoutes: routes,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/account/models?billing_group_id="+hiddenID.String(), nil))
	if response.Code != http.StatusBadRequest || groups.listFilter.IncludeHidden || routes.listActiveGroupID != uuid.Nil || !strings.Contains(response.Body.String(), "billing_group_unavailable") {
		t.Fatalf("status=%d filter=%+v routeGroup=%s body=%s", response.Code, groups.listFilter, routes.listActiveGroupID, response.Body.String())
	}
}

func TestUserModelListRejectsUnavailableBillingGroup(t *testing.T) {
	currentID := uuid.New()
	activeID := uuid.New()
	requestedID := uuid.New()
	groups := &fakeBillingGroups{records: []billinggroup.Record{activeBillingGroup(activeID, "personal", "个人版", 12_500, false)}}
	routes := &fakeModelRoutes{}
	handler := New(Dependencies{
		Auth:  &fakeAPIAuth{current: user.Record{ID: currentID, Role: user.RoleMember, Status: user.StatusActive}},
		Users: &fakeAPIUsers{}, BillingGroups: groups, ModelRoutes: routes,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/account/models?billing_group_id="+requestedID.String(), nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "billing_group_unavailable") || routes.listActiveGroupID != uuid.Nil {
		t.Fatalf("status=%d routeGroup=%s body=%s", response.Code, routes.listActiveGroupID, response.Body.String())
	}
}

func TestUserModelListAggregatesFailoverChannelsAtMaximumPrice(t *testing.T) {
	currentID := uuid.New()
	groupID := uuid.New()
	authService := &fakeAPIAuth{current: user.Record{
		ID: currentID, Role: user.RoleMember, Status: user.StatusActive,
	}}
	groups := &fakeBillingGroups{records: []billinggroup.Record{activeBillingGroup(groupID, "personal", "个人版", 10_000, true)}}
	routes := &fakeModelRoutes{active: []modelroute.Record{
		{PublicName: "shared-chat", DisplayName: "Shared Chat", Provider: modelroute.ProviderSummary{DisplayName: "First", Protocol: provider.ProtocolOpenAI}, UpstreamModel: &upstreammodel.Record{Prices: upstreammodel.Prices{InputMicros: 2_000_000, OutputMicros: 8_000_000, CacheReadMicros: 500_000}}},
		{PublicName: "shared-chat", DisplayName: "Shared Chat", Provider: modelroute.ProviderSummary{DisplayName: "Second", Protocol: provider.ProtocolOpenAI}, UpstreamModel: &upstreammodel.Record{Prices: upstreammodel.Prices{InputMicros: 3_000_000, OutputMicros: 7_000_000, CacheReadMicros: 600_000}}},
	}}
	handler := New(Dependencies{
		Auth: authService, Users: &fakeAPIUsers{}, APIKeys: &fakeAPIKeys{}, BillingGroups: groups, ModelRoutes: routes,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/account/models", nil))
	var body struct {
		Models []struct {
			ID           string           `json:"id"`
			ProviderName string           `json:"provider_name"`
			ChannelCount int              `json:"channel_count"`
			Prices       billing.RateCard `json:"prices"`
		} `json:"models"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != http.StatusOK || len(body.Models) != 1 {
		t.Fatalf("status=%d models=%+v body=%s", response.Code, body.Models, response.Body.String())
	}
	model := body.Models[0]
	if model.ID != "shared-chat" || model.ChannelCount != 2 || model.ProviderName != "First" || model.Prices.InputMicros != 3_000_000 || model.Prices.OutputMicros != 8_000_000 || model.Prices.CacheReadMicros != 600_000 {
		t.Fatalf("unexpected aggregate model: %+v", model)
	}
}

func TestAPIKeyErrorsAreStable(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleMember, Status: user.StatusActive}}
	keys := &fakeAPIKeys{err: apikey.ErrLimitReached}
	request := httptest.NewRequest(http.MethodPost, "/api/account/api-keys", strings.NewReader(`{"name":"Overflow","billing_group_id":"`+uuid.NewString()+`"}`))
	response := httptest.NewRecorder()
	testAPIWithKeys(authService, &fakeAPIUsers{}, keys).ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "api_key_limit_reached") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminCreatesAndUpdatesProviderWithoutCredentialLeak(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	groupID := uuid.New()
	providers := &fakeProviders{}
	handler := New(Dependencies{
		Auth: authService, Users: &fakeAPIUsers{}, APIKeys: &fakeAPIKeys{}, Providers: providers,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), CookieName: "novro_session",
		AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/admin/providers", strings.NewReader(`{"code":"deepseek","display_name":"DeepSeek","protocol":"openai","base_url":"https://api.deepseek.com","model_list_path":"/catalog/models","weight":250,"api_key":"upstream-secret","billing_group_id":"`+groupID.String()+`"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || providers.createInput.BillingGroupID != groupID || providers.createInput.APIKey != "upstream-secret" || providers.createInput.ModelListPath != "/catalog/models" || providers.createInput.Weight != 250 || strings.Contains(response.Body.String(), "upstream-secret") {
		t.Fatalf("status=%d body=%s input=%+v", response.Code, response.Body.String(), providers.createInput)
	}

	id := uuid.New()
	request = httptest.NewRequest(http.MethodPatch, "/api/admin/providers/"+id.String(), strings.NewReader(`{"display_name":"DeepSeek API","model_list_path":"/v1/model/list"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || providers.updateInput.DisplayName == nil || *providers.updateInput.DisplayName != "DeepSeek API" || providers.updateInput.ModelListPath == nil || *providers.updateInput.ModelListPath != "/v1/model/list" {
		t.Fatalf("status=%d body=%s input=%+v", response.Code, response.Body.String(), providers.updateInput)
	}
}

func TestAdminDeletesProvider(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	providers := &fakeProviders{}
	handler := New(Dependencies{
		Auth: authService, Users: &fakeAPIUsers{}, APIKeys: &fakeAPIKeys{}, Providers: providers,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"http://localhost:3000"},
	})
	id := uuid.New()
	request := httptest.NewRequest(http.MethodDelete, "/api/admin/providers/"+id.String(), nil)
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || providers.deletedID != id {
		t.Fatalf("status=%d body=%s deleted=%s", response.Code, response.Body.String(), providers.deletedID)
	}
}

func TestBillingGroupDeleteConflictUsesSafeMessage(t *testing.T) {
	response := httptest.NewRecorder()
	(&apiHandler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}).writeBillingGroupError(response, "delete billing group", fmt.Errorf("%w: internal constraint", billinggroup.ErrInUse))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "billing_group_in_use") || strings.Contains(response.Body.String(), "internal constraint") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminCreatesHiddenBillingGroupWithAuthorizedUsers(t *testing.T) {
	firstUserID := uuid.New()
	secondUserID := uuid.New()
	groups := &fakeBillingGroups{}
	handler := New(Dependencies{
		Auth:           &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}},
		Users:          &fakeAPIUsers{},
		BillingGroups:  groups,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		CookieName:     "novro_session",
		AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/admin/billing-groups", strings.NewReader(fmt.Sprintf(`{"code":"partner","display_name":"代理端","multiplier_bps":5000,"is_hidden":true,"authorized_user_ids":["%s","%s"]}`, firstUserID, secondUserID)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !groups.createInput.IsHidden || len(groups.createInput.AuthorizedUserIDs) != 2 || groups.createInput.AuthorizedUserIDs[0] != firstUserID || groups.createInput.AuthorizedUserIDs[1] != secondUserID {
		t.Fatalf("unexpected create input: %+v", groups.createInput)
	}
}

func TestProviderValidationErrorIsStable(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	providers := &fakeProviders{err: provider.ErrCodeTaken}
	handler := New(Dependencies{Auth: authService, Users: &fakeAPIUsers{}, APIKeys: &fakeAPIKeys{}, Providers: providers, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"http://localhost:3000"}})
	request := httptest.NewRequest(http.MethodPost, "/api/admin/providers", strings.NewReader(`{"code":"deepseek"}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "provider_code_taken") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminSyncsAndLinksProviderModels(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	providerID := uuid.New()
	modelID := uuid.New()
	providerModels := &fakeProviderModels{
		syncModels: []providersync.CatalogModel{{ID: modelID, ProviderName: "DeepSeek", UpstreamName: "deepseek-chat", Added: true}},
		linkResult: providersync.LinkResult{Created: 1},
	}
	handler := New(Dependencies{
		Auth: authService, Users: &fakeAPIUsers{}, APIKeys: &fakeAPIKeys{}, Providers: &fakeProviders{}, ProviderModels: providerModels,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"http://localhost:3000"},
	})

	request := httptest.NewRequest(http.MethodPost, "/api/admin/providers/"+providerID.String()+"/models/sync", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || providerModels.syncProviderID != providerID || !strings.Contains(response.Body.String(), "deepseek-chat") {
		t.Fatalf("sync status=%d body=%s provider=%s", response.Code, response.Body.String(), providerModels.syncProviderID)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/admin/providers/"+providerID.String()+"/models", strings.NewReader(`{"model_ids":["`+modelID.String()+`"]}`))
	request.Header.Set("Origin", "http://localhost:3000")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || providerModels.linkProviderID != providerID || len(providerModels.modelIDs) != 1 || providerModels.modelIDs[0] != modelID || !strings.Contains(response.Body.String(), `"created":1`) {
		t.Fatalf("link status=%d body=%s provider=%s models=%v", response.Code, response.Body.String(), providerModels.linkProviderID, providerModels.modelIDs)
	}
}

func TestProviderModelSyncFailureUsesSafeError(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	providerModels := &fakeProviderModels{err: fmt.Errorf("%w: secret upstream detail", providersync.ErrDiscoveryFailed)}
	handler := New(Dependencies{
		Auth: authService, Users: &fakeAPIUsers{}, APIKeys: &fakeAPIKeys{}, Providers: &fakeProviders{}, ProviderModels: providerModels,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/admin/providers/"+uuid.NewString()+"/models/sync", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "model_sync_failed") || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestProviderModelSyncFailureShowsUpstreamStatus(t *testing.T) {
	authService := &fakeAPIAuth{current: user.Record{ID: uuid.New(), Role: user.RoleAdmin, Status: user.StatusActive}}
	providerModels := &fakeProviderModels{err: &providersync.DiscoveryError{StatusCode: http.StatusUnauthorized, Reason: "请检查模型列表路径和 API 密钥"}}
	handler := New(Dependencies{
		Auth: authService, Users: &fakeAPIUsers{}, APIKeys: &fakeAPIKeys{}, Providers: &fakeProviders{}, ProviderModels: providerModels,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), AllowedOrigins: []string{"http://localhost:3000"},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/admin/providers/"+uuid.NewString()+"/models/sync", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "HTTP 401") || !strings.Contains(response.Body.String(), "API 密钥") {
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

func TestDecodeJSONAcceptsLargeBodyAndKeepsSchemaValidation(t *testing.T) {
	type requestBody struct {
		Value string `json:"value"`
	}
	largeValue := strings.Repeat("x", (1<<20)+1024)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"value":"`+largeValue+`"}`))
	var decoded requestBody
	if err := decodeJSON(httptest.NewRecorder(), request, &decoded); err != nil || decoded.Value != largeValue {
		t.Fatalf("decode large JSON: bytes=%d err=%v", len(decoded.Value), err)
	}

	request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"value":"ok","unknown":true}`))
	if err := decodeJSON(httptest.NewRecorder(), request, &requestBody{}); err == nil {
		t.Fatal("unknown JSON field was accepted")
	}
	request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"value":"first"}{"value":"second"}`))
	if err := decodeJSON(httptest.NewRecorder(), request, &requestBody{}); err == nil {
		t.Fatal("multiple JSON values were accepted")
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

func TestResponsesIncludeServerGeneratedRequestID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	request.Header.Set("X-Novro-Request-ID", "00000000-0000-0000-0000-000000000001")
	response := httptest.NewRecorder()
	testAPI(&fakeAPIAuth{authErr: auth.ErrUnauthenticated}, &fakeAPIUsers{}).ServeHTTP(response, request)
	requestID := response.Header().Get("X-Novro-Request-ID")
	if _, err := uuid.Parse(requestID); err != nil {
		t.Fatalf("invalid request id header %q: %v", requestID, err)
	}
	if requestID == request.Header.Get("X-Novro-Request-ID") {
		t.Fatal("server accepted client request id")
	}
	var body struct {
		RequestID string `json:"request_id"`
		Error     struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.RequestID != requestID || body.Error.Type != "novro_error" {
		t.Fatalf("request id/type mismatch header=%q body=%+v", requestID, body)
	}
}
