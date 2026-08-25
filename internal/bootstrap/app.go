package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

type App struct {
	DB       *sql.DB
	Identity *identity.Service
	OIDC     *identity.OIDCRegistry
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

	return &App{DB: db, Identity: identityService, OIDC: oidcRegistry}, nil
}

func Run(ctx context.Context, cfg config.Config) error {
	app, err := New(ctx, cfg)
	if err != nil {
		return err
	}
	defer app.Close()

	<-ctx.Done()
	return nil
}

func (a *App) Close() error {
	return a.DB.Close()
}
