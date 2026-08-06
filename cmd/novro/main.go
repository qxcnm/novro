package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/novro-gateway/novro/ent/migrate"
	"github.com/novro-gateway/novro/internal/apikey"
	"github.com/novro-gateway/novro/internal/app"
	"github.com/novro-gateway/novro/internal/auth"
	"github.com/novro-gateway/novro/internal/auth/password"
	"github.com/novro-gateway/novro/internal/billing"
	"github.com/novro-gateway/novro/internal/config"
	"github.com/novro-gateway/novro/internal/database"
	"github.com/novro-gateway/novro/internal/gateway"
	"github.com/novro-gateway/novro/internal/httpapi"
	"github.com/novro-gateway/novro/internal/modelroute"
	"github.com/novro-gateway/novro/internal/provider"
	"github.com/novro-gateway/novro/internal/user"
)

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
			if err := migrate.Apply(ctx, application.DB); err != nil {
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
	apiKeyService := apikey.NewService(apikey.NewEntStore(application.Ent))
	providerCipher, err := provider.NewCipher(cfg.Provider.EncryptionSecret)
	if err != nil {
		logger.Error("provider encryption initialization failed", "error", err)
		os.Exit(1)
	}
	providerService := provider.NewService(provider.NewEntStore(application.Ent), providerCipher)
	billingService := billing.NewService(billing.NewEntStore(application.Ent))
	modelRouteService := modelroute.NewService(modelroute.NewEntStore(application.Ent), providerCipher)
	if len(os.Args) > 1 && os.Args[1] == "bootstrap-admin" {
		username := os.Getenv("NOVRO_BOOTSTRAP_USERNAME")
		displayName := os.Getenv("NOVRO_BOOTSTRAP_DISPLAY_NAME")
		plainTextPassword := os.Getenv("NOVRO_BOOTSTRAP_PASSWORD")
		if username == "" || plainTextPassword == "" {
			logger.Error("bootstrap requires NOVRO_BOOTSTRAP_USERNAME and NOVRO_BOOTSTRAP_PASSWORD")
			os.Exit(1)
		}
		created, err := userService.InitializeAdmin(ctx, user.RegisterInput{
			Username:    username,
			DisplayName: displayName,
			Password:    plainTextPassword,
		})
		if err != nil {
			logger.Error("administrator bootstrap failed", "error", err)
			os.Exit(1)
		}
		logger.Info("administrator created", "user_id", created.ID, "username", created.Username)
		return
	}
	if len(os.Args) > 1 {
		logger.Error("unknown command", "command", os.Args[1])
		os.Exit(2)
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

	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.New(httpapi.Dependencies{
			Database:            application.DB,
			Auth:                authService,
			Users:               userService,
			APIKeys:             apiKeyService,
			Providers:           providerService,
			Billing:             billingService,
			ModelRoutes:         modelRouteService,
			Gateway:             gateway.New(gateway.Dependencies{APIKeys: apiKeyService, Routes: modelRouteService, Billing: billingService, Logger: logger}),
			Logger:              logger,
			CookieName:          cfg.Session.CookieName,
			CookieSecure:        cfg.Session.CookieSecure,
			AllowedOrigins:      cfg.AllowedOrigin,
			SetupToken:          cfg.Auth.SetupToken,
			RegistrationEnabled: cfg.Auth.RegistrationEnabled,
			OIDC:                oidcService,
			OIDCDisplayName:     cfg.Auth.OIDC.DisplayName,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
	}

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

func optionalOIDCService(client *auth.OIDCClient) httpapi.OIDCService {
	if client == nil {
		return nil
	}
	return client
}
