package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/httpapi"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

type App struct {
	DB       *sql.DB
	Identity *identity.Service
	OIDC     *identity.OIDCRegistry
	Items    *items.Service
	Handler  http.Handler
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	identityService, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize identity: %w", err)
	}
	bootstrapped, err := identityService.IsBootstrapped(ctx)
	if err != nil {
		db.Close()
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
			WorkspaceName: cfg.Bootstrap.AdminWorkspace,
		}, identity.AuditContext{}); err != nil {
			db.Close()
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
		return nil, fmt.Errorf("initialize OIDC: %w", err)
	}
	itemService := items.NewService(db, cfg.Database.Driver, identityService)
	handler, err := httpapi.NewHandler(
		db, identityService, itemService, oidcRegistry, cfg.HTTP, slog.Default(),
	)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize HTTP handler: %w", err)
	}

	return &App{
		DB: db, Identity: identityService, OIDC: oidcRegistry,
		Items: itemService, Handler: handler,
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
	return a.DB.Close()
}
