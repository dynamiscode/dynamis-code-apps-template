package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const otherItemID = "00000000000000000000000000000001"

func connectResourceSession(t *testing.T, endpoint, token string) (*mcp.ClientSession, *authTransport) {
	t.Helper()
	transport := &authTransport{token: token}
	client := mcp.NewClient(&mcp.Implementation{Name: "resource-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: endpoint, HTTPClient: &http.Client{Transport: transport},
		DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session, transport
}

func resourceCode(err error) int64 {
	var rpcErr *jsonrpc.Error
	if errors.As(err, &rpcErr) {
		return rpcErr.Code
	}
	return 0
}

func TestMCPResourceTemplateReadsOnlyScopedItems(t *testing.T) {
	ctx := context.Background()
	state := newTestState(t, config.MCP{})
	server := httptest.NewServer(state.handler)
	t.Cleanup(server.Close)
	session, transport := connectResourceSession(t, server.URL, state.token.Secret)

	templates, err := session.ListResourceTemplates(ctx, nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates() error = %v", err)
	}
	const template = "dynamis://workspaces/{workspaceId}/items/{itemId}"
	if len(templates.ResourceTemplates) != 1 || templates.ResourceTemplates[0].URITemplate != template {
		t.Fatalf("templates = %#v, want only %s", templates.ResourceTemplates, template)
	}

	created := callTool(t, session, "items_create_v1", map[string]any{
		"workspaceId": state.workspaceID, "title": "Alpha", "idempotencyKey": "resource-alpha",
	})
	item := decodeOutput[struct {
		ID string `json:"id"`
	}](t, created)
	uri := "dynamis://workspaces/" + state.workspaceID + "/items/" + item.ID

	read, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource() error = %v", err)
	}
	if len(read.Contents) != 1 || read.Contents[0].URI != uri || read.Contents[0].MIMEType != "application/json" {
		t.Fatalf("contents = %#v", read.Contents)
	}
	var got struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspaceId"`
		Title       string `json:"title"`
	}
	if err := json.Unmarshal([]byte(read.Contents[0].Text), &got); err != nil {
		t.Fatalf("resource JSON error = %v", err)
	}
	if got.ID != item.ID || got.WorkspaceID != state.workspaceID || got.Title != "Alpha" {
		t.Fatalf("resource = %+v", got)
	}

	missing := "dynamis://workspaces/" + state.workspaceID + "/items/" + otherItemID
	if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: missing}); resourceCode(err) != mcp.CodeResourceNotFound {
		t.Fatalf("missing item error = %v, want resource not found", err)
	}
	if transport.sawSession.Load() {
		t.Fatal("stateless resource reads issued Mcp-Session-Id")
	}
}

func TestMCPResourcesIsolateWorkspacesAndRejectMalformedURIs(t *testing.T) {
	ctx := context.Background()
	state := newTestState(t, config.MCP{})
	server := httptest.NewServer(state.handler)
	t.Cleanup(server.Close)
	session, _ := connectResourceSession(t, server.URL, state.token.Secret)

	otherWorkspace, err := state.auth.CreateWorkspace(ctx, state.actor,
		identity.WorkspaceCreateInput{Name: "Other", Locale: "en"}, identity.AuditContext{})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	otherActor, err := state.auth.Authorize(ctx, state.actor.UserID, otherWorkspace, identity.ResourcesWrite)
	if err != nil {
		t.Fatalf("Authorize(other) error = %v", err)
	}
	other, err := items.NewService(state.db, config.SQLite, state.auth, 100).Create(
		ctx, otherActor, otherWorkspace, "Other workspace secret", "resource-other", identity.AuditContext{})
	if err != nil {
		t.Fatalf("Create(other) error = %v", err)
	}

	own := callTool(t, session, "items_create_v1", map[string]any{
		"workspaceId": state.workspaceID, "title": "Own", "idempotencyKey": "resource-own",
	})
	ownID := decodeOutput[struct {
		ID string `json:"id"`
	}](t, own).ID
	if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "dynamis://workspaces/" + state.workspaceID + "/items/" + ownID,
	}); err != nil {
		t.Fatalf("own-workspace positive control error = %v", err)
	}

	crossWorkspace := "dynamis://workspaces/" + otherWorkspace.String() + "/items/" + other.Item.ID.String()
	if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: crossWorkspace}); err == nil ||
		strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "access is denied") {
		t.Fatalf("cross-workspace read error = %v, want access denied without item data", err)
	}
	wrongPath := "dynamis://workspaces/" + state.workspaceID + "/items/" + other.Item.ID.String()
	if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: wrongPath}); resourceCode(err) != mcp.CodeResourceNotFound {
		t.Fatalf("item in other workspace error = %v, want resource not found", err)
	}
	for _, uri := range []string{
		"dynamis://workspaces/not-an-id/items/" + otherItemID,
		"dynamis://workspaces/" + state.workspaceID + "/items/" + strings.ToUpper(otherItemID),
	} {
		if _, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri}); resourceCode(err) != mcp.CodeResourceNotFound {
			t.Fatalf("malformed URI %q error = %v, want resource not found", uri, err)
		}
	}
}

func TestMCPResourcesRequireResourcesReadScope(t *testing.T) {
	ctx := context.Background()
	state := newTestState(t, config.MCP{})
	server := httptest.NewServer(state.handler)
	t.Cleanup(server.Close)

	writeOnly, err := state.auth.CreateAPIToken(ctx, state.actor, "write only",
		[]identity.Permission{identity.ResourcesWrite}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatalf("CreateAPIToken(write only) error = %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "write-only", Version: "1.0.0"}, nil)
	if _, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: server.URL, HTTPClient: &http.Client{Transport: &authTransport{token: writeOnly.Secret}},
		DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil); err == nil {
		t.Fatal("write-only token connected to resource surface")
	}

	readOnly, err := state.auth.CreateAPIToken(ctx, state.actor, "read only",
		[]identity.Permission{identity.ResourcesRead}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatalf("CreateAPIToken(read only) error = %v", err)
	}
	session, _ := connectResourceSession(t, server.URL, readOnly.Secret)
	seeded, err := items.NewService(state.db, config.SQLite, state.auth, 100).Create(
		ctx, state.actor, state.workspaceKey, "Readable", "resource-readable", identity.AuditContext{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	uri := "dynamis://workspaces/" + state.workspaceID + "/items/" + seeded.Item.ID.String()
	read, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if err != nil || len(read.Contents) != 1 || !strings.Contains(read.Contents[0].Text, "Readable") {
		t.Fatalf("read-only token read = %#v, %v; want the item", read, err)
	}
}

func TestMCPResourcesRequireAuthenticatedBearerToken(t *testing.T) {
	ctx := context.Background()
	state := newTestState(t, config.MCP{})
	server := httptest.NewServer(state.handler)
	t.Cleanup(server.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "anonymous", Version: "1.0.0"}, nil)
	if _, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: server.URL, HTTPClient: &http.Client{Transport: &authTransport{token: "invalid"}},
		DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil); err == nil {
		t.Fatal("invalid bearer token connected to resource surface")
	}
}
