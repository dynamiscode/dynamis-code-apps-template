package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

const (
	testOAuthResourceURL    = "https://app.example.com/mcp"
	testOAuthAuthServer     = "https://idp.example.com/tenant"
	testOAuthMetadataURL    = "https://app.example.com/.well-known/oauth-protected-resource/mcp"
	testOAuthInitializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"oauth-test","version":"1"}}}`
)

func testOAuthConfig() config.MCP {
	return config.MCP{OAuthAuthorizationServer: testOAuthAuthServer, OAuthResourceURL: testOAuthResourceURL}
}

func TestProtectedResourceMetadataIsOffByDefault(t *testing.T) {
	t.Parallel()

	if got := protectedResourceMetadataURL(config.MCP{}); got != "" {
		t.Fatalf("metadata URL = %q, want empty", got)
	}
	if handler := NewProtectedResourceMetadataHandler(config.MCP{}); handler != nil {
		t.Fatal("metadata handler registered without an authorization server")
	}
	state := newTestState(t, config.MCP{})
	response := postMCP(state.handler, "", "", testOAuthInitializeBody)
	if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("default 401 = %d headers=%v", response.Code, response.Header())
	}
	readToken, err := state.auth.CreateAPIToken(context.Background(), state.actor, "default read-only",
		[]identity.Permission{identity.ResourcesRead}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"items_create_v1","arguments":{}}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Authorization", "Bearer "+readToken.Secret)
	request.Header.Set("Mcp-Name", "items_create_v1")
	forbidden := httptest.NewRecorder()
	state.handler.ServeHTTP(forbidden, request)
	if forbidden.Code != http.StatusForbidden || forbidden.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("default 403 = %d headers=%v", forbidden.Code, forbidden.Header())
	}
}

func TestProtectedResourceMetadataDocument(t *testing.T) {
	t.Parallel()

	handler := NewProtectedResourceMetadataHandler(testOAuthConfig())
	request := httptest.NewRequest(http.MethodGet, testOAuthMetadataURL, nil)
	request.Host = "attacker.example"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("GET metadata = %d headers=%v", response.Code, response.Header())
	}
	if response.Header().Get("Mcp-Session-Id") != "" {
		t.Fatal("metadata response issued a session identifier")
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode metadata fields: %v", err)
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	wantKeys := []string{"authorization_servers", "bearer_methods_supported", "resource", "scopes_supported"}
	if !sameSet(keys, wantKeys) {
		t.Fatalf("metadata keys = %v, want %v", keys, wantKeys)
	}

	var metadata oauthex.ProtectedResourceMetadata
	if err := json.Unmarshal(response.Body.Bytes(), &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata.Resource != testOAuthResourceURL ||
		!reflect.DeepEqual(metadata.AuthorizationServers, []string{testOAuthAuthServer}) ||
		!reflect.DeepEqual(metadata.ScopesSupported, []string{"resources:read", "resources:write"}) ||
		!reflect.DeepEqual(metadata.BearerMethodsSupported, []string{"header"}) {
		t.Fatalf("metadata = %#v", metadata)
	}
	if strings.Contains(response.Body.String(), "attacker.example") {
		t.Fatal("metadata reflected the request Host header")
	}

	options := httptest.NewRecorder()
	handler.ServeHTTP(options, httptest.NewRequest(http.MethodOptions, testOAuthMetadataURL, nil))
	if options.Code != http.StatusNoContent || options.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("OPTIONS metadata = %d headers=%v", options.Code, options.Header())
	}
	post := httptest.NewRecorder()
	handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, testOAuthMetadataURL, nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST metadata = %d", post.Code)
	}
}

func TestMCPChallengesAdvertiseProtectedResourceMetadata(t *testing.T) {
	t.Parallel()

	state := newTestState(t, testOAuthConfig())
	wantChallenge := `Bearer resource_metadata="` + testOAuthMetadataURL + `"`

	missing := postMCP(state.handler, "", "", testOAuthInitializeBody)
	if missing.Code != http.StatusUnauthorized || missing.Header().Get("WWW-Authenticate") != wantChallenge {
		t.Fatalf("missing token = %d WWW-Authenticate=%q", missing.Code, missing.Header().Get("WWW-Authenticate"))
	}

	const submitted = "not-a-real-token-value"
	wantInvalidChallenge := `Bearer error="invalid_token", resource_metadata="` + testOAuthMetadataURL + `"`
	invalid := postMCP(state.handler, submitted, "", testOAuthInitializeBody)
	if invalid.Code != http.StatusUnauthorized || invalid.Header().Get("WWW-Authenticate") != wantInvalidChallenge {
		t.Fatalf("invalid token = %d WWW-Authenticate=%q", invalid.Code, invalid.Header().Get("WWW-Authenticate"))
	}
	for name, value := range map[string]string{
		"body": invalid.Body.String(), "headers": strings.Join(invalid.Header().Values("WWW-Authenticate"), " "),
	} {
		if strings.Contains(value, submitted) {
			t.Fatalf("401 %s reflected the submitted token", name)
		}
	}

	valid := postMCP(state.handler, state.token.Secret, "", testOAuthInitializeBody)
	if valid.Code != http.StatusOK || !strings.Contains(valid.Body.String(), `"protocolVersion":"2025-11-25"`) ||
		valid.Header().Get("WWW-Authenticate") != "" || valid.Header().Get("Mcp-Session-Id") != "" {
		t.Fatalf("API token with OAuth metadata enabled = %d headers=%v body=%s",
			valid.Code, valid.Header(), valid.Body.String())
	}
}

func TestMCPInsufficientScopeChallengeAdvertisesProtectedResourceMetadata(t *testing.T) {
	t.Parallel()

	state := newTestState(t, testOAuthConfig())
	readToken, err := state.auth.CreateAPIToken(context.Background(), state.actor, "oauth read-only",
		[]identity.Permission{identity.ResourcesRead}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"items_create_v1","arguments":{}}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Authorization", "Bearer "+readToken.Secret)
	request.Header.Set("Mcp-Name", "items_create_v1")
	response := httptest.NewRecorder()
	state.handler.ServeHTTP(response, request)
	want := `Bearer error="insufficient_scope", resource_metadata="` + testOAuthMetadataURL + `"`
	if response.Code != http.StatusForbidden || response.Header().Get("WWW-Authenticate") != want {
		t.Fatalf("insufficient scope = %d WWW-Authenticate=%q", response.Code, response.Header().Get("WWW-Authenticate"))
	}
	if strings.Contains(response.Body.String(), readToken.Secret) {
		t.Fatal("403 reflected the bearer token")
	}
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]bool, len(got))
	for _, value := range got {
		seen[value] = true
	}
	for _, value := range want {
		if !seen[value] {
			return false
		}
	}
	return true
}
