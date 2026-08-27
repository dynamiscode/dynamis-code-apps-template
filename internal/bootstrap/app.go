package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"example.com/dynamis-code/apps-template/internal/httpapi"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/mcpserver"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
	"example.com/dynamis-code/apps-template/internal/platform/telemetry"
	"example.com/dynamis-code/apps-template/internal/portability"
	"example.com/dynamis-code/apps-template/internal/web"
)

type App struct {
	DB          *sql.DB
	Identity    *identity.Service
	OIDC        *identity.OIDCRegistry
	Items       *items.Service
	Portability *portability.Service
	Handler     http.Handler
	Telemetry   *telemetry.Provider
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	telemetryProvider, err := telemetry.New(ctx, cfg.Telemetry)
	if err != nil {
		return nil, fmt.Errorf("initialize telemetry: %w", err)
	}
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	identityService, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("initialize identity: %w", err)
	}
	bootstrapped, err := identityService.IsBootstrapped(ctx)
	if err != nil {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("check bootstrap state: %w", err)
	}
	adminConfigured := cfg.Bootstrap.AdminEmail != "" ||
		cfg.Bootstrap.AdminWorkspace != "" || cfg.Bootstrap.AdminPassword != ""
	if !bootstrapped && adminConfigured && (cfg.Bootstrap.AdminEmail == "" ||
		cfg.Bootstrap.AdminWorkspace == "" || cfg.Bootstrap.AdminPassword == "") {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf(
			"BOOTSTRAP_ADMIN_EMAIL, BOOTSTRAP_ADMIN_WORKSPACE, and BOOTSTRAP_ADMIN_PASSWORD must be set together",
		)
	}
	if !bootstrapped && cfg.Bootstrap.AdminEmail != "" {
		if _, err := identityService.BootstrapFirstOwner(ctx, identity.BootstrapInput{
			Email: cfg.Bootstrap.AdminEmail, Password: cfg.Bootstrap.AdminPassword,
			WorkspaceName: cfg.Bootstrap.AdminWorkspace, WorkspaceLocale: cfg.Bootstrap.AdminWorkspaceLocale,
		}, identity.AuditContext{}); err != nil {
			db.Close()
			telemetryProvider.Shutdown(context.Background())
			return nil, fmt.Errorf("bootstrap first owner: %w", err)
		}
	}
	switch {
	case bootstrapped:
		slog.Info("bootstrap already complete; browser setup disabled")
	case cfg.Bootstrap.AdminEmail != "":
		slog.Info("environment bootstrap completed; browser setup disabled")
	case cfg.Bootstrap.SetupToken != "":
		slog.Info("remote browser setup enabled", "path", "/setup")
	default:
		slog.Info(
			"local browser setup enabled; remote setup requires configuration",
			"path", "/setup",
			"next", "set BOOTSTRAP_SETUP_TOKEN or all BOOTSTRAP_ADMIN_* variables for remote setup, or run bootstrap-admin",
		)
	}
	oidcRegistry, err := identity.NewOIDCRegistry(ctx, cfg.OIDC)
	if err != nil {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("initialize OIDC: %w", err)
	}
	itemService := items.NewService(
		db, cfg.Database.Driver, identityService, cfg.Data.ItemsMaxPerWorkspace,
	)
	portabilityService := portability.NewService(
		db, cfg.Database.Driver, identityService,
		cfg.Data.ExportMaxRecords, cfg.Data.ExportMaxBytes,
	)
	mailer, err := appmail.NewSMTP(cfg.Mail)
	if err != nil {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("initialize invitation mail: %w", err)
	}
	handler, err := httpapi.NewHandlerWithMail(
		db, identityService, itemService, portabilityService, oidcRegistry, cfg.HTTP, slog.Default(),
		cfg.PublicURL, mailer,
	)
	if err != nil {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("initialize HTTP handler: %w", err)
	}
	webHandler, err := web.NewHandlerWithServices(
		identityService, itemService, portabilityService, oidcRegistry, cfg.HTTP,
		cfg.Bootstrap.SetupToken, cfg.PublicURL, mailer,
	)
	if err != nil {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("initialize web handler: %w", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", handler)
	mux.Handle("/health/", handler)
	mux.Handle("/mcp", httpapi.Wrap(
		mcpserver.NewHandler(identityService, itemService, cfg.MCP, slog.Default()),
		cfg.HTTP, slog.Default(),
	))
	mux.Handle("/", httpapi.Wrap(webHandler.Routes(), cfg.HTTP, slog.Default()))

	return &App{
		DB: db, Identity: identityService, OIDC: oidcRegistry,
		Items: itemService, Portability: portabilityService,
		Handler: telemetry.HTTPHandler(mux), Telemetry: telemetryProvider,
	}, nil
}

func Run(ctx context.Context, cfg config.Config) error {
	app, err := New(ctx, cfg)
	if err != nil {
		return err
	}
	defer app.Close()
	server := httpapi.NewServer(cfg.HTTP, app.Handler)
	listener, err := net.Listen("tcp", cfg.HTTP.Address)
	if err != nil {
		return fmt.Errorf("listen HTTP: %w", err)
	}
	slog.Info("HTTP server listening", "address", listener.Addr().String())
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	select {
	case err := <-serveDone:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(), cfg.HTTP.ShutdownTimeout,
		)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown HTTP: %w", err)
		}
		err := <-serveDone
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	}
}

func (a *App) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return errors.Join(a.DB.Close(), a.Telemetry.Shutdown(ctx))
}
