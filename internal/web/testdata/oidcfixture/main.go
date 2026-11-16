//go:build oidcfixture

package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/web"
)

func main() {
	ctx := context.Background()
	address := os.Getenv("OIDC_FIXTURE_ADDRESS")
	databasePath := os.Getenv("OIDC_FIXTURE_DATABASE")
	values := map[string]string{
		"DATABASE_DRIVER": "sqlite", "SQLITE_PATH": databasePath,
		"HTTP_ADDRESS": address, "MULTI_WORKSPACE_ENABLED": "true",
		"HTTP_REQUESTS_PER_MINUTE": "10000", "HTTP_AUTH_REQUESTS_PER_MINUTE": "1000",
	}
	cfg, err := config.LoadFrom(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	if err != nil {
		log.Fatal("load OIDC fixture configuration")
	}
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		log.Fatal("open OIDC fixture database")
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		log.Fatal("migrate OIDC fixture database")
	}
	identityService, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		log.Fatal("initialize OIDC fixture identity")
	}
	if _, err := identityService.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "oidc-fixture@example.com", Password: "oidc-fixture-password-123", WorkspaceName: "OIDC Fixture",
	}, identity.AuditContext{}); err != nil {
		log.Fatal("bootstrap OIDC fixture account")
	}
	provider := newProviderFixture()
	defer provider.Close()
	issuer := provider.URL
	oidcRegistry, err := identity.NewOIDCRegistryForLoopbackTestFixture(ctx, config.OIDC{
		Enabled: true, ProviderID: "company", ProviderName: "Company SSO",
		IssuerURL: issuer, ClientID: "fixture-client", ClientSecret: "fixture-secret",
		RedirectURL: "http://" + address + "/auth/oidc/company/callback",
	}, provider.Client())
	if err != nil {
		log.Fatal("initialize local OIDC provider fixture")
	}
	itemService := items.NewService(db, cfg.Database.Driver, identityService, cfg.Data.ItemsMaxPerWorkspace)
	handler, err := web.NewHandlerWithServices(
		identityService, itemService, nil, nil, oidcRegistry, cfg.HTTP, "", "", nil,
	)
	if err != nil {
		log.Fatal("initialize OIDC fixture web handler")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/ready", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/", handler.Routes())
	server := &http.Server{Addr: address, Handler: mux}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("serve OIDC fixture")
	}
}

func newProviderFixture() *httptest.Server {
	var issuer string
	provider := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
				"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys",
				"response_types_supported":              []string{"code"},
				"subject_types_supported":               []string{"public"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/keys":
			_, _ = writer.Write([]byte(`{"keys":[]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	issuer = provider.URL
	return provider
}
