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
}
