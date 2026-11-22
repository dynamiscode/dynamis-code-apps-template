package mcpserver

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPApprovalUsesMRTRAndStaysStateless(t *testing.T) {
	state := newTestStateWith(t, config.MCP{}, true)
	server := httptest.NewServer(state.handler)
	t.Cleanup(server.Close)
	transport := &authTransport{token: state.token.Secret}
	ctx := context.Background()

	// Manual retry: the test drives every round trip so the server's own
	// input_required output is observable.
	connect := func() *mcp.ClientSession {
		t.Helper()
		client := mcp.NewClient(&mcp.Implementation{Name: "mrtr-test", Version: "1"}, &mcp.ClientOptions{
			MultiRoundTrip: &mcp.MultiRoundTripOptions{Disabled: true},
		})
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
			Endpoint: server.URL, HTTPClient: &http.Client{Transport: transport},
			DisableStandaloneSSE: true, MaxRetries: -1,
		}, nil)
		if err != nil {
			t.Fatalf("Connect() error = %v", err)
		}
		t.Cleanup(func() { session.Close() })
		return session
	}
	first := connect()
	if version := first.InitializeResult().ProtocolVersion; version != "2026-07-28" {
		t.Fatalf("negotiated protocol = %q, want 2026-07-28", version)
	}
	created := createClientTool(t, first, state.workspaceID, "approval title")
	args := map[string]any{
		"workspaceId": state.workspaceID, "clientId": created.ID,
	}
	requireClient := func() {
		t.Helper()
		result := callTool(t, first, "clients_get_v1", map[string]any{"workspaceId": state.workspaceID, "clientId": created.ID})
		if result.IsError {
			t.Fatalf("client missing before approval: %s", contentText(result))
		}
	}

	// Round one asks the client host for confirmation and mutates nothing.
	prompt, err := first.CallTool(ctx, &mcp.CallToolParams{Name: "clients_delete_v1", Arguments: args})
	if err != nil {
		t.Fatalf("prompt CallTool() error = %v", err)
	}
	if !prompt.NeedsInput() || prompt.RequestState == "" {
		t.Fatalf("prompt = %+v, want input_required with request state", prompt)
	}
	request, ok := prompt.InputRequests["human-approval"].(*mcp.ElicitParams)
	if !ok || !strings.Contains(request.Message, string(created.ID)) {
		t.Fatalf("input request = %#v, want elicitation naming the item", prompt.InputRequests)
	}
	requireClient()

	// Header-only approval is ignored on MRTR sessions.
	transport.approval.Store(true)
	headerOnly, err := first.CallTool(ctx, &mcp.CallToolParams{Name: "clients_delete_v1", Arguments: args})
	transport.approval.Store(false)
	if err != nil || !headerOnly.NeedsInput() {
		t.Fatalf("header-only result = %+v, error = %v; want input_required", headerOnly, err)
	}
	requireClient()

	accept := mcp.InputResponseMap{"human-approval": &mcp.ElicitResult{
		Action: "accept", Content: map[string]any{"confirm": true},
	}}
	decline := mcp.InputResponseMap{"human-approval": &mcp.ElicitResult{Action: "decline"}}

	// The retry arrives on a new connection, so the server must rely only on the echoed state.
	second := connect()
	forged, err := second.CallTool(ctx, &mcp.CallToolParams{
		Name: "clients_delete_v1", Arguments: args, InputResponses: accept, RequestState: "forged",
	})
	if err != nil || !forged.NeedsInput() {
		t.Fatalf("forged state result = %+v, error = %v; want input_required", forged, err)
	}
	changed := map[string]any{
		"workspaceId": state.workspaceID, "clientId": "23456789abcd",
	}
	mismatched, err := second.CallTool(ctx, &mcp.CallToolParams{
		Name: "clients_delete_v1", Arguments: changed, InputResponses: accept, RequestState: prompt.RequestState,
	})
	if err != nil || !mismatched.NeedsInput() {
		t.Fatalf("changed-arguments result = %+v, error = %v; want input_required", mismatched, err)
	}
	requireClient()

	declined, err := second.CallTool(ctx, &mcp.CallToolParams{
		Name: "clients_delete_v1", Arguments: args, InputResponses: decline, RequestState: prompt.RequestState,
	})
	if err != nil || !declined.IsError || contentText(declined) != "human approval was declined" {
		t.Fatalf("declined result = %+v, error = %v", declined, err)
	}
	requireClient()

	accepted, err := second.CallTool(ctx, &mcp.CallToolParams{
		Name: "clients_delete_v1", Arguments: args, InputResponses: accept, RequestState: prompt.RequestState,
	})
	if err != nil || accepted.IsError {
		t.Fatalf("accepted result = %+v, error = %v", accepted, err)
	}
	if got := callTool(t, first, "clients_get_v1", args); !got.IsError {
		t.Fatal("client still exists after approved deletion")
	}
	if transport.sawSession.Load() {
		t.Fatal("MRTR round trip issued Mcp-Session-Id on a stateless endpoint")
	}
}

func TestMCPLegacyProtocolCannotConfirmWithHeader(t *testing.T) {
	state := newTestStateWith(t, config.MCP{}, true)
	server := httptest.NewServer(state.handler)
	t.Cleanup(server.Close)
	ctx := context.Background()
	transport := &authTransport{token: state.token.Secret}
	client := mcp.NewClient(&mcp.Implementation{Name: "legacy-approval", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: server.URL, HTTPClient: &http.Client{Transport: transport},
		DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { session.Close() })
	created := createClientTool(t, session, state.workspaceID, "legacy title")

	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"clients_delete_v1","arguments":{"workspaceId":%q,"clientId":%q}}}`, state.workspaceID, created.ID)
	response := postToolCall(state.handler, state.token.Secret, "2025-11-25", true, body)
	if !strings.Contains(response.Body.String(), `"error"`) {
		t.Fatalf("legacy header call = %d %s, want protocol error", response.Code, response.Body.String())
	}
	if result := callTool(t, session, "clients_get_v1", map[string]any{"workspaceId": state.workspaceID, "clientId": created.ID}); result.IsError {
		t.Fatalf("client missing after legacy approval: %s", contentText(result))
	}
}

func postToolCall(handler http.Handler, token, protocolVersion string, approvalHeader bool, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("MCP-Protocol-Version", protocolVersion)
	request.Header.Set("X-Request-ID", testRequestID)
	if approvalHeader {
		request.Header.Set("Mcp-Human-Approval", "true")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
