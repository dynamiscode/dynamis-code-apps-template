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
	if !deleted.Deleted || string(deleted.ItemID) != created.ID {
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
		handler: NewHandler(authService, items.NewService(db, cfg.Database.Driver, authService, cfg.Data.ItemsMaxPerWorkspace), mcpConfig, logger),
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

type auditRow struct {
	action, outcome, requestID, metadata string
}

func auditRows(t *testing.T, db *sql.DB, eventType string) []auditRow {
	t.Helper()
	rows, err := db.Query(`SELECT action, outcome, COALESCE(request_id, ''), metadata FROM audit_events
		WHERE event_type = ? ORDER BY created_at`, eventType)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var result []auditRow
	for rows.Next() {
		var row auditRow
		if err := rows.Scan(&row.action, &row.outcome, &row.requestID, &row.metadata); err != nil {
			t.Fatal(err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func findAudit(t *testing.T, rows []auditRow, action string) auditRow {
	t.Helper()
	for _, row := range rows {
		if row.action == action {
			return row
		}
	}
	t.Fatalf("no audit row for action %q in %+v", action, rows)
	return auditRow{}
}

func TestMCPReceivingMiddlewareAuditsEveryKnownMethod(t *testing.T) {
	state := newTestState(t, config.MCP{})
	server := httptest.NewServer(state.handler)
	t.Cleanup(server.Close)
	transport := &authTransport{token: state.token.Secret}
	client := mcp.NewClient(&mcp.Implementation{Name: "middleware-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: server.URL, HTTPClient: &http.Client{Transport: transport},
		DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { session.Close() })
	if _, err := session.ListTools(context.Background(), nil); err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	promptsBody := `{"jsonrpc":"2.0","id":2,"method":"prompts/list","params":{"_meta":{` + statelessMeta + `}}}`
	prompts := postMCPMethod(state.handler, state.token.Secret, "prompts/list", promptsBody)
	if prompts.Code != http.StatusOK || !strings.Contains(prompts.Body.String(), `"prompts":[{`) {
		t.Fatalf("prompts/list response = %d %s", prompts.Code, prompts.Body.String())
	}

	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "client-supplied-name-v9", Arguments: map[string]any{},
	}); err == nil {
		t.Fatal("unregistered tool call succeeded")
	}
	unknownAudited := false
	for _, row := range auditRows(t, state.db, "mcp.tool.called") {
		if strings.Contains(row.action, "client-supplied") {
			t.Fatalf("client-supplied tool name reached audit: %+v", row)
		}
		if row.action == "tools/call" && row.outcome == "failure" && strings.Contains(row.metadata, `"error_category":"protocol_error"`) {
			unknownAudited = true
		}
	}
	if !unknownAudited {
		t.Fatal("unregistered tool call was not audited as tools/call failure")
	}

	rows := auditRows(t, state.db, "mcp.request.called")
	listed := findAudit(t, rows, "tools/list")
	if listed.outcome != "success" || listed.requestID != testRequestID || !strings.Contains(listed.metadata, `"duration_ms"`) ||
		!strings.Contains(listed.metadata, `"method":"tools/list"`) || strings.Contains(listed.metadata, state.token.Secret) {
		t.Fatalf("tools/list audit = %+v", listed)
	}
	prompted := findAudit(t, rows, "prompts/list")
	if prompted.outcome != "success" || prompted.requestID != testRequestID ||
		!strings.Contains(prompted.metadata, `"method":"prompts/list"`) || !strings.Contains(prompted.metadata, `"duration_ms"`) {
		t.Fatalf("prompts/list audit = %+v", prompted)
	}
}

func TestMCPReceivingMiddlewareRecoversPanicsWithoutLeakingValues(t *testing.T) {
	state := newTestState(t, config.MCP{})
	var logs bytes.Buffer
	h := &handler{
		identity: state.auth, logger: slog.New(slog.NewTextHandler(&logs, nil)), tools: map[string]*mcp.Tool{},
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "panic-server", Version: "1"}, nil)
	server.AddReceivingMiddleware(h.observe)
	type none struct{}
	addTool(server, h, &mcp.Tool{
		Name: "panic_v1", Description: "Panics for middleware tests.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, nil, func(context.Context, *mcp.CallToolRequest, none) (none, toolRun) {
		panic("super-secret-panic-value")
	})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "panic-test", Version: "1"}, nil).Connect(
		context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })

	result := callTool(t, session, "panic_v1", map[string]any{})
	if !result.IsError || contentText(result) != "internal failure" {
		t.Fatalf("panic result = %+v", result)
	}
	if strings.Contains(logs.String(), "super-secret-panic-value") {
		t.Fatalf("panic value leaked into logs: %s", logs.String())
	}
	row := findAudit(t, auditRows(t, state.db, "mcp.tool.called"), "panic_v1")
	if row.outcome != "failure" || !strings.Contains(row.metadata, `"error_category":"internal"`) {
		t.Fatalf("panic audit = %+v", row)
	}
}

func TestMCPUnknownMethodIsRejectedBeforeMiddleware(t *testing.T) {
	state := newTestState(t, config.MCP{})
	body := `{"jsonrpc":"2.0","id":3,"method":"x/unknown","params":{"_meta":{` + statelessMeta + `}}}`
	response := postMCPMethod(state.handler, state.token.Secret, "x/unknown", body)
	if !strings.Contains(response.Body.String(), `"code":-32601`) {
		t.Fatalf("unknown method response = %d %s", response.Code, response.Body.String())
	}
	for _, row := range auditRows(t, state.db, "mcp.request.called") {
		if row.action == "x/unknown" {
			t.Fatalf("SDK rejects unknown methods before receiving middleware; unexpected audit %+v", row)
		}
	}
}

const statelessMeta = `"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
	`"io.modelcontextprotocol/clientInfo":{"name":"raw-test","version":"1"},` +
	`"io.modelcontextprotocol/clientCapabilities":{}`

func postMCPMethod(handler http.Handler, token, method, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("MCP-Protocol-Version", "2026-07-28")
	request.Header.Set("Mcp-Method", method)
	request.Header.Set("X-Request-ID", testRequestID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestMCPToolsListCacheHints(t *testing.T) {
	state := newTestState(t, config.MCP{})
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	response := postMCP(state.handler, state.token.Secret, "", body)
	if response.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Mcp-Session-Id") != "" {
		t.Fatalf("tools/list issued Mcp-Session-Id")
	}
	var envelope struct {
		Result struct {
			Tools      []struct{ Name string } `json:"tools"`
			TTLMs      int                     `json:"ttlMs"`
			CacheScope string                  `json:"cacheScope"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode tools/list: %v body = %s", err, response.Body.String())
	}
	if envelope.Result.TTLMs != 300_000 || envelope.Result.CacheScope != "public" {
		t.Fatalf("cache hints = ttlMs %d cacheScope %q", envelope.Result.TTLMs, envelope.Result.CacheScope)
	}
	if len(envelope.Result.Tools) != 5 {
		t.Fatalf("tools/list returned %d tools, want 5", len(envelope.Result.Tools))
	}

	// Discovery of other results is untouched: an unscoped method must not gain hints.
	ping := postMCP(state.handler, state.token.Secret, "", `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	if strings.Contains(ping.Body.String(), "ttlMs") || strings.Contains(ping.Body.String(), "cacheScope") {
		t.Fatalf("ping gained cache hints: %s", ping.Body.String())
	}
}
