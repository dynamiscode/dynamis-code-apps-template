package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectCompletionClient(t *testing.T, handler http.Handler, token, origin string) (*mcp.ClientSession, *authTransport) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	transport := &authTransport{token: token, origin: origin}
	client := mcp.NewClient(&mcp.Implementation{Name: "completion-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: server.URL, HTTPClient: &http.Client{Transport: transport},
		DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session, transport
}

func complete(t *testing.T, session *mcp.ClientSession, argument, value string, scope map[string]string) *mcp.CompleteResult {
	t.Helper()
	params := &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: "ref/prompt", Name: itemReviewPromptName},
		Argument: mcp.CompleteParamsArgument{Name: argument, Value: value},
	}
	if scope != nil {
		params.Context = &mcp.CompleteContext{Arguments: scope}
	}
	result, err := session.Complete(context.Background(), params)
	if err != nil {
		t.Fatalf("Complete(%s=%q) error = %v", argument, value, err)
	}
	if result.Completion.Values == nil {
		t.Fatalf("Complete(%s=%q) returned null values", argument, value)
	}
	return result
}

func TestMCPCompletionAndPromptAreWorkspaceScoped(t *testing.T) {
	state := newTestState(t, config.MCP{})
	session, transport := connectCompletionClient(t, state.handler, state.token.Secret, "")

	initialized := session.InitializeResult()
	if initialized.Capabilities == nil || initialized.Capabilities.Completions == nil ||
		initialized.Capabilities.Prompts == nil {
		t.Fatalf("capabilities = %+v", initialized.Capabilities)
	}
	if transport.sawSession.Load() {
		t.Fatal("stateless endpoint returned Mcp-Session-Id")
	}

	listed, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListPrompts() error = %v", err)
	}
	var review *mcp.Prompt
	for _, prompt := range listed.Prompts {
		if prompt.Name == itemReviewPromptName {
			review = prompt
		}
	}
	if review == nil {
		t.Fatalf("prompts = %+v", listed.Prompts)
	}
	for _, argument := range review.Arguments {
		if !argument.Required {
			t.Fatalf("prompt argument %q is optional", argument.Name)
		}
	}

	created := decodeOutput[items.Item](t, callTool(t, session, "items_create_v1", map[string]any{
		"workspaceId": state.workspaceID, "title": "completion secret-shaped title",
		"idempotencyKey": "completion-key-1",
	}))

	statuses := complete(t, session, "status", "", nil)
	if fmt.Sprint(statuses.Completion.Values) != "[active complete]" {
		t.Fatalf("status values = %v", statuses.Completion.Values)
	}
	filtered := complete(t, session, "status", "com", nil)
	if fmt.Sprint(filtered.Completion.Values) != "[complete]" {
		t.Fatalf("filtered status values = %v", filtered.Completion.Values)
	}

	ids := complete(t, session, "itemId", created.ID.String(), map[string]string{"workspaceId": state.workspaceID})
	if fmt.Sprint(ids.Completion.Values) != fmt.Sprintf("[%s]", created.ID.String()) {
		t.Fatalf("item id values = %v, want [%s]", ids.Completion.Values, created.ID.String())
	}
	statusScoped := complete(t, session, "itemId", "", map[string]string{
		"workspaceId": state.workspaceID, "status": "complete",
	})
	if len(statusScoped.Completion.Values) != 0 {
		t.Fatalf("complete-status item ids = %v, want none", statusScoped.Completion.Values)
	}
	withoutWorkspace := complete(t, session, "itemId", "", nil)
	if len(withoutWorkspace.Completion.Values) != 0 {
		t.Fatalf("item ids without workspace = %v", withoutWorkspace.Completion.Values)
	}
	foreignWorkspace := complete(t, session, "itemId", "", map[string]string{
		"workspaceId": "00000000000000000000000000000000",
	})
	if len(foreignWorkspace.Completion.Values) != 0 || foreignWorkspace.Completion.HasMore {
		t.Fatalf("foreign workspace completion = %+v", foreignWorkspace.Completion)
	}
	malformedWorkspace := complete(t, session, "itemId", "", map[string]string{"workspaceId": "not-an-id"})
	if len(malformedWorkspace.Completion.Values) != 0 {
		t.Fatalf("malformed workspace completion = %v", malformedWorkspace.Completion.Values)
	}
	unknownArgument := complete(t, session, "title", "", map[string]string{"workspaceId": state.workspaceID})
	if len(unknownArgument.Completion.Values) != 0 {
		t.Fatalf("unknown argument completion = %v", unknownArgument.Completion.Values)
	}
	unknownPrompt, err := session.Complete(context.Background(), &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: "ref/prompt", Name: "missing_v1"},
		Argument: mcp.CompleteParamsArgument{Name: "status", Value: ""},
	})
	if err != nil || len(unknownPrompt.Completion.Values) != 0 {
		t.Fatalf("unknown prompt completion = %+v, error = %v", unknownPrompt, err)
	}

	prompt, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name: itemReviewPromptName, Arguments: map[string]string{
			"workspaceId": state.workspaceID, "itemId": created.ID.String(), "status": "complete",
		},
	})
	if err != nil || len(prompt.Messages) != 1 {
		t.Fatalf("GetPrompt() = %+v, error = %v", prompt, err)
	}
	text := fmt.Sprint(prompt.Messages[0].Content)
	if !strings.Contains(text, created.ID.String()) || !strings.Contains(text, "complete") ||
		strings.Contains(text, "secret-shaped") {
		t.Fatalf("prompt text = %q", text)
	}
	for _, arguments := range []map[string]string{
		{"workspaceId": state.workspaceID, "itemId": created.ID.String(), "status": "archived"},
		{"workspaceId": "00000000000000000000000000000000", "itemId": created.ID.String(), "status": "complete"},
		{"workspaceId": state.workspaceID, "itemId": "00000000000000000000000000000000", "status": "complete"},
		{"workspaceId": state.workspaceID, "status": "complete"},
	} {
		if _, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
			Name: itemReviewPromptName, Arguments: arguments,
		}); err == nil {
			t.Fatalf("GetPrompt(%v) succeeded", arguments)
		}
	}
}

func TestMCPCompletionIsBoundedToOnePage(t *testing.T) {
	state := newTestState(t, config.MCP{})
	session, _ := connectCompletionClient(t, state.handler, state.token.Secret, "")
	for index := range 101 {
		callTool(t, session, "items_create_v1", map[string]any{
			"workspaceId": state.workspaceID, "title": fmt.Sprintf("item %d", index),
			"idempotencyKey": fmt.Sprintf("bound-key-%d", index),
		})
	}
	bounded := complete(t, session, "itemId", "", map[string]string{"workspaceId": state.workspaceID})
	if len(bounded.Completion.Values) != 100 || !bounded.Completion.HasMore {
		t.Fatalf("bounded completion = %d values, hasMore=%v", len(bounded.Completion.Values), bounded.Completion.HasMore)
	}
}

func TestMCPCompletionRequiresReadScope(t *testing.T) {
	state := newTestState(t, config.MCP{})
	writeOnly, err := state.auth.CreateAPIToken(context.Background(), state.actor, "write-only",
		[]identity.Permission{identity.ResourcesWrite}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"completion/complete","params":{"ref":{"type":"ref/prompt","name":"item_review_v1"},"argument":{"name":"status","value":""}}}`
	denied := postMCP(state.handler, writeOnly.Secret, "", body)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("write-only completion status = %d, body = %s", denied.Code, denied.Body.String())
	}
}

func TestMCPCompletionAllowsReadOnlyTokens(t *testing.T) {
	state := newTestState(t, config.MCP{AllowedOrigins: []string{"https://client.example"}})
	readToken, err := state.auth.CreateAPIToken(context.Background(), state.actor, "read-only",
		[]identity.Permission{identity.ResourcesRead}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	session, _ := connectCompletionClient(t, state.handler, readToken.Secret, "https://client.example")
	statuses := complete(t, session, "status", "", nil)
	if fmt.Sprint(statuses.Completion.Values) != "[active complete]" {
		t.Fatalf("read-only status values = %v", statuses.Completion.Values)
	}
}
