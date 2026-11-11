package bootstrap

import (
	"log/slog"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/httpapi"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/mcpserver"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func registerAgent(
	mux *http.ServeMux,
	identityService *identity.Service,
	itemService *items.Service,
	cfg config.Config,
) {
	mux.Handle("/mcp", httpapi.Wrap(
		mcpserver.NewHandler(identityService, itemService, cfg.MCP, slog.Default()),
		cfg.HTTP, slog.Default(),
	))
	if metadata := mcpserver.NewProtectedResourceMetadataHandler(cfg.MCP); metadata != nil {
		mux.Handle(mcpserver.ProtectedResourceMetadataPath, httpapi.Wrap(metadata, cfg.HTTP, slog.Default()))
	}
	if cfg.MCP.OAuthBuiltIn {
		mux.Handle("/oauth/token", httpapi.Wrap(
			mcpserver.NewOAuthTokenEndpoint(identityService), cfg.HTTP, slog.Default(),
		))
		mux.Handle("/oauth/revoke", httpapi.Wrap(
			mcpserver.NewOAuthRevocationEndpoint(identityService), cfg.HTTP, slog.Default(),
		))
		mux.Handle("/oauth/register", httpapi.Wrap(
			mcpserver.NewOAuthRegistrationEndpoint(identityService), cfg.HTTP, slog.Default(),
		))
		if metadata := mcpserver.NewAuthorizationServerMetadataHandler(cfg.MCP); metadata != nil {
			mux.Handle("/.well-known/oauth-authorization-server", httpapi.Wrap(metadata, cfg.HTTP, slog.Default()))
		}
	}
}
