package mcpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testRequestID = "0123456789abcdef0123456789abcdef"

type testState struct {
	db          *sql.DB
	auth        *identity.Service
	actor       identity.Principal
	workspaceID string
	token       identity.NewAPIToken
	handler     http.Handler
}

type authTransport struct {
	token      string
	origin     string
	approval   atomic.Bool
	sawSession atomic.Bool
}

func (transport *authTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+transport.token)
	clone.Header.Set("X-Request-ID", testRequestID)
	if transport.origin != "" {
		clone.Header.Set("Origin", transport.origin)
	}
	if transport.approval.Load() {
		clone.Header.Set("Mcp-Human-Approval", "true")
	}
	response, err := http.DefaultTransport.RoundTrip(clone)
	if err == nil && response.Header.Get("Mcp-Session-Id") != "" {
		transport.sawSession.Store(true)
	}
	return response, err
}

func TestMCPToolsUseSharedScopedItemService(t *testing.T) {
	state := newTestState(t, config.MCP{})
	server := httptest.NewServer(state.handler)
	t.Cleanup(server.Close)
	transport := &authTransport{token: state.token.Secret}
	client := mcp.NewClient(&mcp.Implementation{Name: "component-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: server.URL, HTTPClient: &http.Client{Transport: transport},
		DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { session.Close() })

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	wantNames := []string{"items_create_v1", "items_delete_v1", "items_get_v1", "items_list_v1", "items_update_v1"}
	wantReadOnly := map[string]bool{"items_get_v1": true, "items_list_v1": true}
	wantDestructive := map[string]bool{"items_delete_v1": true, "items_update_v1": true}
	if len(listed.Tools) != len(wantNames) {
		t.Fatalf("tools = %d, want %d", len(listed.Tools), len(wantNames))
	}
	for index, tool := range listed.Tools {
		if tool.Name != wantNames[index] || tool.InputSchema == nil || tool.OutputSchema == nil ||
			tool.Annotations == nil || tool.Annotations.ReadOnlyHint != wantReadOnly[tool.Name] ||
			!tool.Annotations.IdempotentHint || tool.Annotations.DestructiveHint == nil ||
			*tool.Annotations.DestructiveHint != wantDestructive[tool.Name] ||
			tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("tool[%d] = %+v", index, tool)
		}
	}
	encodedTools, err := json.Marshal(listed.Tools)
	if err != nil || !bytes.Contains(encodedTools, []byte(`"maximum":100`)) ||
		!bytes.Contains(encodedTools, []byte(`"additionalProperties":false`)) {
		t.Fatalf("bounded tool schemas = %s, error = %v", encodedTools, err)
	}
	if transport.sawSession.Load() {
		t.Fatal("stateless endpoint returned Mcp-Session-Id")
	}

	createdResult := callTool(t, session, "items_create_v1", map[string]any{
		"workspaceId": state.workspaceID, "title": "MCP secret-shaped title",
		"idempotencyKey": "mcp-key-1",
	})
	if createdResult.IsError {
		t.Fatalf("create result = %+v", createdResult)
	}
	created := decodeOutput[items.Item](t, createdResult)
	if created.Title != "MCP secret-shaped title" || created.WorkspaceID != state.workspaceID {
		t.Fatalf("created = %+v", created)
	}

	page := decodeOutput[items.Page](t, callTool(t, session, "items_list_v1", map[string]any{
		"workspaceId": state.workspaceID, "limit": 1, "sort": "-created_at",
	}))
	if len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("page = %+v", page)
	}
	got := decodeOutput[items.Item](t, callTool(t, session, "items_get_v1", map[string]any{
		"workspaceId": state.workspaceID, "itemId": created.ID,
	}))
	if got.ID != created.ID {
		t.Fatalf("got = %+v", got)
	}
	updateArgs := map[string]any{
		"workspaceId": state.workspaceID, "itemId": created.ID, "version": 1, "status": "complete",
	}
	withoutUpdateApproval := callTool(t, session, "items_update_v1", updateArgs)
	if !withoutUpdateApproval.IsError || !strings.Contains(contentText(withoutUpdateApproval), "human approval") {
		t.Fatalf("update without approval = %+v", withoutUpdateApproval)
	}
	transport.approval.Store(true)
	updated := decodeOutput[items.Item](t, callTool(t, session, "items_update_v1", updateArgs))
	if updated.Status != items.Complete || updated.Version != 2 {
		t.Fatalf("updated = %+v", updated)
	}

	transport.approval.Store(false)
	withoutApproval := callTool(t, session, "items_delete_v1", map[string]any{
		"workspaceId": state.workspaceID, "itemId": created.ID, "version": 2,
	})
	if !withoutApproval.IsError || !strings.Contains(contentText(withoutApproval), "human approval") {
		t.Fatalf("delete without approval = %+v", withoutApproval)
	}
	transport.approval.Store(true)
	deleted := decodeOutput[deleteOutput](t, callTool(t, session, "items_delete_v1", map[string]any{
		"workspaceId": state.workspaceID, "itemId": created.ID, "version": 2,
	}))
	if !deleted.Deleted || deleted.ItemID != created.ID {
		t.Fatalf("deleted = %+v", deleted)
	}

	wrongWorkspace := callTool(t, session, "items_list_v1", map[string]any{
		"workspaceId": "00000000000000000000000000000000", "limit": 1, "sort": "created_at",
	})
	if !wrongWorkspace.IsError || contentText(wrongWorkspace) != "access is denied" {
		t.Fatalf("wrong workspace = %+v", wrongWorkspace)
	}
	invalid := callTool(t, session, "items_create_v1", map[string]any{
		"workspaceId": state.workspaceID, "title": "x", "idempotencyKey": "key", "unknown": true,
	})
	if !invalid.IsError || contentText(invalid) != "parameters are invalid" {
		t.Fatalf("invalid result = %+v", invalid)
	}
	overLimit := callTool(t, session, "items_list_v1", map[string]any{
		"workspaceId": state.workspaceID, "limit": 101, "sort": "created_at",
	})
	if !overLimit.IsError || contentText(overLimit) != "parameters are invalid" {
		t.Fatalf("over-limit result = %+v", overLimit)
	}

	rows, err := state.db.Query(`SELECT metadata, request_id FROM audit_events WHERE event_type = 'mcp.tool.called'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var metadata, requestID string
		if err := rows.Scan(&metadata, &requestID); err != nil {
			t.Fatal(err)
		}
		count++
		if requestID != testRequestID || strings.Contains(metadata, state.token.Secret) ||
			strings.Contains(metadata, "MCP secret-shaped title") || !strings.Contains(metadata, `"tool_version":"v1"`) {
			t.Fatalf("unsafe/incomplete audit metadata = %s, request = %s", metadata, requestID)
		}
	}
	if count != 10 {
		t.Fatalf("MCP audit events = %d, want 10", count)
	}
}

func TestMCPHTTPBoundariesAndLegacyInitialize(t *testing.T) {
	state := newTestState(t, config.MCP{AllowedOrigins: []string{"https://client.example"}})

	denied := postMCP(state.handler, "", "https://other.example", `{}`)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("denied Origin status = %d", denied.Code)
	}
	missing := postMCP(state.handler, "", "https://client.example", `{}`)
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d", missing.Code)
	}
	oversized := postMCP(state.handler, state.token.Secret, "https://client.example", strings.Repeat("x", maxResultBytes+1))
	if oversized.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized request status = %d", oversized.Code)
	}
	legacyBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"legacy-test","version":"1"}}}`
	legacy := postMCP(state.handler, state.token.Secret, "https://client.example", legacyBody)
	if legacy.Code != http.StatusOK || !strings.Contains(legacy.Body.String(), `"protocolVersion":"2025-11-25"`) ||
		legacy.Header().Get("Mcp-Session-Id") != "" {
		t.Fatalf("legacy initialize = %d %s headers=%v", legacy.Code, legacy.Body.String(), legacy.Header())
	}

	readToken, err := state.auth.CreateAPIToken(context.Background(), state.actor, "read-only",
		[]identity.Permission{identity.ResourcesRead}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(state.handler)
	t.Cleanup(server.Close)
	transport := &authTransport{token: readToken.Secret, origin: "https://client.example"}
	client := mcp.NewClient(&mcp.Implementation{Name: "scope-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: server.URL, HTTPClient: &http.Client{Transport: transport},
		DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "items_create_v1", Arguments: map[string]any{
			"workspaceId": state.workspaceID, "title": "blocked", "idempotencyKey": "blocked-key",
		},
	})
	if err == nil {
		t.Fatal("read-only token created an item")
	}
	_ = session.Close()

	if err := state.auth.RevokeAPIToken(context.Background(), state.actor, readToken.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	revoked := postMCP(state.handler, readToken.Secret, "https://client.example", legacyBody)
	if revoked.Code != http.StatusUnauthorized || strings.Contains(revoked.Body.String(), readToken.Secret) {
		t.Fatalf("revoked token response = %d %s", revoked.Code, revoked.Body.String())
	}
}

func newTestState(t *testing.T, mcpConfig config.MCP) *testState {
	t.Helper()
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns = 1, 1
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		t.Fatal(err)
	}
	authService, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := authService.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "MCP test",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := authService.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	token, err := authService.CreateAPIToken(ctx, actor, "MCP test",
		[]identity.Permission{identity.ResourcesRead, identity.ResourcesWrite}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &testState{
		db: db, auth: authService, actor: actor, workspaceID: owner.WorkspaceID, token: token,
		handler: NewHandler(authService, items.NewService(db, cfg.Database.Driver, authService), mcpConfig, logger),
	}
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, arguments any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("CallTool(%s) error = %v", name, err)
	}
	return result
}

func decodeOutput[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	var output T
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil || json.Unmarshal(encoded, &output) != nil {
		t.Fatalf("decode structured output %T: %v", result.StructuredContent, err)
	}
	return output
}

func contentText(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil {
		return ""
	}
	return text.Text
}

func postMCP(handler http.Handler, token, origin, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("X-Request-ID", testRequestID)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
