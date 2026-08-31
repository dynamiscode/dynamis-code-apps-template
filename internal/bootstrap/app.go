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

	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/httpapi"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/jobs"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
	"example.com/dynamis-code/apps-template/internal/platform/telemetry"
	"example.com/dynamis-code/apps-template/internal/portability"
	"example.com/dynamis-code/apps-template/internal/sharing"
	"example.com/dynamis-code/apps-template/internal/web"
	"example.com/dynamis-code/apps-template/internal/webhooks"
)

type App struct {
	DB          *sql.DB
	Identity    *identity.Service
	OIDC        *identity.OIDCRegistry
	Items       *items.Service
	Sharing     *sharing.Service
	Portability *portability.Service
	Webhooks    *webhooks.Service
	Files       *appfiles.Service
	Jobs        *jobs.Queue
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
	identityService, err := identity.NewServiceWithMFA(db, cfg.Database.Driver, identity.MFAConfig{
		Enabled: cfg.MFA.Enabled, EncryptionKey: cfg.MFA.EncryptionKey,
		RelyingPartyID: cfg.MFA.RelyingPartyID, Origins: cfg.MFA.Origins,
		DisplayName: cfg.MFA.DisplayName, RequireForAdmins: cfg.MFA.RequireForAdmins,
	})
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
	jobQueue := jobs.NewQueue(db, cfg.Database.Driver, slog.Default())
	webhookService := webhooks.NewService(
		db, cfg.Database.Driver, identityService, cfg.Webhooks.SecretKey, jobQueue,
	)
	storageConfig := cfg.Storage
	if storageConfig.Driver == config.StorageLocal {
		storageConfig.S3Prefix = ""
	}
	objectStore, err := appfiles.NewStore(ctx, storageConfig)
	if err != nil {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("initialize file storage: %w", err)
	}
	fileService := appfiles.NewService(
		db, cfg.Database.Driver, identityService, objectStore,
		storageConfig.MaxObjectBytes, storageConfig.MaxWorkspaceBytes,
		storageConfig.SignedURLTTL, storageConfig.S3Prefix,
	)
	if err := jobQueue.Register(webhooks.JobKind, webhookService.HandleJob); err != nil {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("register webhook job handler: %w", err)
	}
	if err := jobQueue.RegisterExhausted(webhooks.JobKind, webhookService.HandleExhaustedJob); err != nil {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("register exhausted webhook job handler: %w", err)
	}
	itemService := items.NewService(
		db, cfg.Database.Driver, identityService, cfg.Data.ItemsMaxPerWorkspace, webhookService,
	)
	sharingService := sharing.NewService(db, cfg.Database.Driver, identityService)
	portabilityService := portability.NewService(
		db, cfg.Database.Driver, identityService,
		cfg.Data.ExportMaxRecords, cfg.Data.ExportMaxBytes,
		cfg.Data.ImportMaxRecords, cfg.Data.ImportMaxBytes, itemService,
	)
	mailer, err := appmail.NewSMTP(cfg.Mail)
	if err != nil {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("initialize invitation mail: %w", err)
	}
	handler, err := httpapi.NewHandlerWithWebhooksAndFiles(
		db, identityService, itemService, portabilityService, oidcRegistry, cfg.HTTP, slog.Default(),
		webhookService, fileService, cfg.PublicURL, mailer,
	)
	if err != nil {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("initialize HTTP handler: %w", err)
	}
	webHandler, err := web.NewHandlerWithServicesAndFilesAndSharing(
		identityService, itemService, sharingService, portabilityService, oidcRegistry, cfg.HTTP,
		fileService, cfg.Bootstrap.SetupToken, cfg.PublicURL, mailer, webhookService,
	)
	if err != nil {
		db.Close()
		telemetryProvider.Shutdown(context.Background())
		return nil, fmt.Errorf("initialize web handler: %w", err)
	}
	jobQueue.Start(ctx)
	mux := http.NewServeMux()
	mux.Handle("/api/", handler)
	mux.Handle("/health/", handler)
	registerAgent(mux, identityService, itemService, cfg)
	mux.Handle("/", httpapi.Wrap(webHandler.Routes(), cfg.HTTP, slog.Default()))

	return &App{
		DB: db, Identity: identityService, OIDC: oidcRegistry,
		Items: itemService, Sharing: sharingService, Portability: portabilityService, Webhooks: webhookService,
		Files: fileService, Jobs: jobQueue,
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
	if a.Jobs != nil {
		a.Jobs.Close()
	}
	return errors.Join(a.DB.Close(), a.Telemetry.Shutdown(ctx))
}
