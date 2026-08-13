package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent/migrate"
	"github.com/novro-gateway/novro/internal/announcement"
	"github.com/novro-gateway/novro/internal/apikey"
	"github.com/novro-gateway/novro/internal/app"
	"github.com/novro-gateway/novro/internal/auth"
	"github.com/novro-gateway/novro/internal/auth/password"
	"github.com/novro-gateway/novro/internal/billing"
	"github.com/novro-gateway/novro/internal/billinggroup"
	"github.com/novro-gateway/novro/internal/config"
	"github.com/novro-gateway/novro/internal/database"
	"github.com/novro-gateway/novro/internal/email"
	"github.com/novro-gateway/novro/internal/gateway"
	"github.com/novro-gateway/novro/internal/gatewaysettings"
	"github.com/novro-gateway/novro/internal/httpapi"
	"github.com/novro-gateway/novro/internal/modelroute"
	"github.com/novro-gateway/novro/internal/payment"
	"github.com/novro-gateway/novro/internal/provider"
	"github.com/novro-gateway/novro/internal/providersync"
	"github.com/novro-gateway/novro/internal/referral"
	"github.com/novro-gateway/novro/internal/upstreammodel"
	"github.com/novro-gateway/novro/internal/user"
)

const (
	defaultBootstrapUsername    = "novro"
	defaultBootstrapEmail       = "novro@example.invalid"
	defaultBootstrapDisplayName = "Novro"
)

/**
 * main 初始化并启动 Novro 应用程序。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) > 1 && os.Args[1] == "init-db" {
		if err := database.EnsureDatabase(ctx, cfg.Database); err != nil {
			logger.Error("database initialization failed", "error", err)
			os.Exit(1)
		}
		logger.Info("database initialized")
		return
	}

	application, err := app.New(ctx, cfg)
	if err != nil {
		logger.Error("application initialization failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := application.Close(); err != nil {
			logger.Error("application shutdown failed", "error", err)
		}
	}()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "migrate":
			if err := applyPendingMigrations(ctx, application.DB, migrate.Apply); err != nil {
				logger.Error("database migration failed", "error", err)
				os.Exit(1)
			}
			logger.Info("database migrations applied")
			return
		case "check-db":
			logger.Info("database connection is ready")
			return
		}
	}
	passwordHasher := password.Hasher{}
	userService := user.NewService(user.NewEntStore(application.Ent), passwordHasher)
	providerCipher, err := provider.NewCipher(cfg.Provider.EncryptionSecret)
	if err != nil {
		logger.Error("provider encryption initialization failed", "error", err)
		os.Exit(1)
	}
	apiKeyService := apikey.NewService(apikey.NewEntStore(application.Ent), providerCipher)
	providerService := provider.NewService(provider.NewEntStore(application.Ent), providerCipher)
	billingService := billing.NewService(billing.NewEntStore(application.Ent))
	modelRouteService := modelroute.NewService(modelroute.NewEntStore(application.Ent), providerCipher)
	upstreamModelService := upstreammodel.NewService(upstreammodel.NewEntStore(application.Ent))
	providerModelService := providersync.NewService(application.Ent, providerCipher, nil)
	billingGroupService := billinggroup.NewService(billinggroup.NewEntStore(application.Ent))
	referralService := referral.NewService(referral.NewEntStore(application.Ent), cfg.Referral.RewardBPS, cfg.Auth.PublicURL)
	gatewaySettingsService := gatewaysettings.NewService(gatewaysettings.NewEntStore(application.Ent))
	announcementService := announcement.NewService(announcement.NewEntStore(application.Ent))
	paymentService := payment.NewService(
		payment.NewEntStore(application.Ent, cfg.Referral.RewardBPS), payment.NewConfigEntStore(application.Ent), providerCipher,
		payment.EPayConfig{
			APIURL: cfg.Payment.EPay.APIURL, MerchantID: cfg.Payment.EPay.MerchantID, MerchantKey: cfg.Payment.EPay.MerchantKey,
			SiteName: cfg.Payment.EPay.SiteName, Channels: cfg.Payment.EPay.Channels,
			NotifyURL: cfg.Auth.PublicURL + "/api/payments/epay/notify",
			ReturnURL: cfg.Auth.PublicURL + "/api/payments/epay/return",
		},
	)
	if len(os.Args) > 1 && os.Args[1] == "bootstrap-admin" {
		username := envOrDefault("NOVRO_BOOTSTRAP_USERNAME", defaultBootstrapUsername)
		email := envOrDefault("NOVRO_BOOTSTRAP_EMAIL", defaultBootstrapEmail)
		displayName := envOrDefault("NOVRO_BOOTSTRAP_DISPLAY_NAME", defaultBootstrapDisplayName)
		plainTextPassword := os.Getenv("NOVRO_BOOTSTRAP_PASSWORD")
		if plainTextPassword == "" {
			logger.Error("bootstrap requires NOVRO_BOOTSTRAP_PASSWORD")
			os.Exit(1)
		}
		created, err := userService.InitializeAdmin(ctx, user.RegisterInput{
			Username:    username,
			Email:       email,
			DisplayName: displayName,
			Password:    plainTextPassword,
		})
		if err != nil {
			if errors.Is(err, user.ErrAlreadyInitialized) {
				logger.Info("administrator already initialized")
				return
			}
			logger.Error("administrator bootstrap failed", "error", err)
			os.Exit(1)
		}
		logger.Info("administrator created", "user_id", created.ID, "username", created.Username)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "reset-admin" {
		username := envOrDefault("NOVRO_BOOTSTRAP_USERNAME", defaultBootstrapUsername)
		plainTextPassword := os.Getenv("NOVRO_ADMIN_PASSWORD")
		if plainTextPassword == "" {
			logger.Error("reset-admin requires NOVRO_ADMIN_PASSWORD")
			os.Exit(1)
		}
		admin, err := userService.FindByUsername(ctx, username)
		if err != nil {
			logger.Error("find administrator for password reset", "error", err)
			os.Exit(1)
		}
		if admin.Role != user.RoleAdmin || !admin.IsSystemAdmin {
			logger.Error("refusing to reset a non-system administrator", "username", username)
			os.Exit(1)
		}
		if err := userService.ResetPassword(ctx, admin.ID, plainTextPassword); err != nil {
			logger.Error("administrator password reset failed", "error", err)
			os.Exit(1)
		}
		logger.Info("administrator password reset", "username", admin.Username)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "reconcile-top-up" {
		if len(os.Args) != 3 {
			logger.Error("reconcile-top-up requires exactly one Novro order number")
			os.Exit(2)
		}
		if err := paymentService.Bootstrap(ctx); err != nil {
			logger.Error("payment configuration bootstrap failed", "error", err)
			os.Exit(1)
		}
		order, err := paymentService.Reconcile(ctx, os.Args[2])
		if err != nil {
			logger.Error("top-up reconciliation failed", "out_trade_no", os.Args[2], "error", err)
			os.Exit(1)
		}
		logger.Info("top-up reconciliation complete", "out_trade_no", order.OutTradeNo, "status", order.Status)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "compensate-usage" {
		if len(os.Args) != 4 {
			logger.Error("compensate-usage requires a legacy usage request ID and actor user ID")
			os.Exit(2)
		}
		requestID, requestErr := uuid.Parse(os.Args[2])
		actorID, actorErr := uuid.Parse(os.Args[3])
		if requestErr != nil || actorErr != nil {
			logger.Error("compensate-usage IDs must be valid UUIDs")
			os.Exit(2)
		}
		_, amount, err := billingService.CompensateLegacyUsage(ctx, requestID, actorID)
		if err != nil {
			logger.Error("legacy usage compensation failed", "request_id", requestID, "error", err)
			os.Exit(1)
		}
		logger.Info("legacy usage compensation complete", "request_id", requestID, "amount_micros", amount)
		return
	}
	if len(os.Args) > 1 {
		logger.Error("unknown command", "command", os.Args[1])
		os.Exit(2)
	}
	if err := paymentService.Bootstrap(ctx); err != nil {
		logger.Error("payment configuration bootstrap failed", "error", err)
		os.Exit(1)
	}
	authService, err := auth.NewService(
		auth.NewEntStore(application.Ent),
		passwordHasher,
		cfg.Session.Secret,
		cfg.Session.TTL,
	)
	if err != nil {
		logger.Error("authentication initialization failed", "error", err)
		os.Exit(1)
	}
	oidcClient, err := auth.NewOIDCClient(ctx, cfg.Auth.OIDC, cfg.Auth.PublicURL, cfg.Session.Secret)
	if err != nil {
		logger.Error("OIDC initialization failed", "error", err)
		os.Exit(1)
	}
	oidcService := optionalOIDCService(oidcClient)
	var developmentMailer email.Mailer
	if cfg.Environment != "production" {
		developmentMailer = email.NewLogMailer(logger)
	}
	emailConfigService := email.NewService(email.NewEntStore(application.Ent), providerCipher, cfg.Email, developmentMailer, cfg.Environment == "production")
	emailVerificationService, err := email.NewVerificationService(email.NewSQLStore(application.DB), emailConfigService, cfg.Session.Secret)
	if err != nil {
		logger.Error("email verification initialization failed", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.New(httpapi.Dependencies{
			Database:            application.DB,
			Auth:                authService,
			Users:               userService,
			APIKeys:             apiKeyService,
			Providers:           providerService,
			Billing:             billingService,
			Payments:            paymentService,
			Referrals:           referralService,
			ModelRoutes:         modelRouteService,
			UpstreamModels:      upstreamModelService,
			ProviderModels:      providerModelService,
			BillingGroups:       billingGroupService,
			GatewaySettings:     gatewaySettingsService,
			Announcements:       announcementService,
			Gateway:             gateway.New(gateway.Dependencies{APIKeys: apiKeyService, Routes: modelRouteService, Billing: billingService, Settings: gatewaySettingsService, Logger: logger}),
			Logger:              logger,
			CookieName:          cfg.Session.CookieName,
			CookieSecure:        cfg.Session.CookieSecure,
			AllowedOrigins:      cfg.AllowedOrigin,
			SetupToken:          cfg.Auth.SetupToken,
			RegistrationEnabled: cfg.Auth.RegistrationEnabled,
			OIDC:                oidcService,
			OIDCDisplayName:     cfg.Auth.OIDC.DisplayName,
			EmailVerification:   emailVerificationService,
			EmailConfig:         emailConfigService,
		}),
		ReadHeaderTimeout: 0,
		ReadTimeout:       0,
		WriteTimeout:      0,
		IdleTimeout:       0,
	}
	go recoverPendingBilling(ctx, billingService, logger)

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("Novro server listening", "address", cfg.HTTPAddr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP server shutdown failed", "error", err)
		}
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}
}

type pendingBillingRecoverer interface {
	/**
	 * RecoverPendingSettlements 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @param arg2 类型为 int 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	RecoverPendingSettlements(context.Context, int) (int, error)
}

/**
 * recoverPendingBilling 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param recoverer 本次操作需要使用的输入参数。
 * @param logger 用于记录结构化运行日志的日志器。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func recoverPendingBilling(ctx context.Context, recoverer pendingBillingRecoverer, logger *slog.Logger) {
	recoverOnce := func() {
		recoveryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		recovered, err := recoverer.RecoverPendingSettlements(recoveryCtx, 100)
		if err != nil {
			logger.Error("recover pending gateway settlements", "recovered", recovered, "error", err)
			return
		}
		if recovered > 0 {
			logger.Info("recovered pending gateway settlements", "count", recovered)
		}
	}
	recoverOnce()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			recoverOnce()
		}
	}
}

/**
 * envOrDefault 封装该名称对应的业务处理逻辑。
 * @param key 本次操作需要使用的输入参数。
 * @param fallback 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

/**
 * optionalOIDCService 封装该名称对应的业务处理逻辑。
 * @param client 用于访问外部或底层服务的客户端。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func optionalOIDCService(client *auth.OIDCClient) httpapi.OIDCService {
	if client == nil {
		return nil
	}
	return client
}

type migrationApplier func(context.Context, *sql.DB) error

/**
 * applyPendingMigrations 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param db 本次操作需要使用的输入参数。
 * @param apply 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func applyPendingMigrations(ctx context.Context, db *sql.DB, apply migrationApplier) error {
	if err := apply(ctx, db); err != nil {
		return fmt.Errorf("apply pending migrations: %w", err)
	}
	return nil
}
