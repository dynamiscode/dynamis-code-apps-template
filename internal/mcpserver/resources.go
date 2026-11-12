package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// itemResourceTemplate is read-only and workspace-scoped. It is enumerable
// only per workspace through items_list_v1, so no static resource list exists.
// ponytail: no resources/list enumeration; add static resources when a
// bounded per-workspace listing is needed.
const (
	itemResourcePrefix   = "dynamis://workspaces/"
	itemResourceInfix    = "/items/"
	itemResourceTemplate = itemResourcePrefix + "{workspaceId}" + itemResourceInfix + "{itemId}"
)

func (h *handler) addResources(server *mcp.Server) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: itemResourceTemplate,
		Name:        "item_v1",
		Title:       "Workspace item",
		Description: "One item in an authorized workspace as read-only JSON. Requires resources:read.",
		MIMEType:    "application/json",
	}, h.readItemResource)
}

func (h *handler) readItemResource(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	uri := request.Params.URI
	workspaceID, itemID, ok := parseItemURI(uri)
	if !ok {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	item, err := h.items.Get(ctx, resourcePrincipal(request), workspaceID, itemID)
	switch {
	case errors.Is(err, items.ErrNotFound):
		return nil, mcp.ResourceNotFoundError(uri)
	case errors.Is(err, identity.ErrForbidden):
		return nil, errors.New("access is denied")
	case err != nil:
		return nil, errors.New("internal failure")
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		return nil, errors.New("internal failure")
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
		URI: uri, MIMEType: "application/json", Text: string(encoded),
	}}}, nil
}

func parseItemURI(uri string) (identity.WorkspaceID, string, bool) {
	rest, ok := strings.CutPrefix(uri, itemResourcePrefix)
	if !ok {
		return 0, "", false
	}
	shortID, itemID, ok := strings.Cut(rest, itemResourceInfix)
	if !ok || !validID(itemID) {
		return 0, "", false
	}
	workspaceID, err := identity.ParseWorkspaceID(shortID)
	if err != nil {
		return 0, "", false
	}
	return workspaceID, itemID, true
}

func resourcePrincipal(request *mcp.ReadResourceRequest) identity.Principal {
	if request.Extra == nil || request.Extra.TokenInfo == nil {
		return identity.Principal{}
	}
	value, _ := request.Extra.TokenInfo.Extra["principal"].(identity.Principal)
	return value
}
