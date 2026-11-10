package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func builtInConfig() config.MCP {
	return config.MCP{
		OAuthAuthorizationServer: "https://app.example.com", OAuthResourceURL: testOAuthResourceURL, OAuthBuiltIn: true,
	}
}

func TestOAuthRegistrationEndpointRegistersPublicClients(t *testing.T) {
	t.Parallel()
	state := newTestState(t, builtInConfig())
	endpoint := NewOAuthRegistrationEndpoint(state.auth)

	body := `{"client_name":"Desktop MCP","redirect_uris":["https://client.example/callback","http://127.0.0.1:8765/cb"],"token_endpoint_auth_method":"none","grant_types":["authorization_code","refresh_token"],"response_types":["code"]}`
	request := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	var registered map[string]any
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &registered) != nil {
		t.Fatalf("register = %d %s", response.Code, response.Body.String())
	}
	if registered["client_id"] == "" || registered["client_name"] != "Desktop MCP" ||
		registered["token_endpoint_auth_method"] != "none" ||
		!reflect.DeepEqual(registered["response_types"], []any{"code"}) ||
		response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("registration response = %#v headers=%v", registered, response.Header())
	}
}

func TestOAuthRegistrationEndpointRejectsInvalidMetadata(t *testing.T) {
	t.Parallel()
	state := newTestState(t, builtInConfig())
	endpoint := NewOAuthRegistrationEndpoint(state.auth)
	for name, body := range map[string]string{
		"confidential client": `{"client_name":"x","redirect_uris":["https://client.example/cb"],"token_endpoint_auth_method":"client_secret_basic"}`,
		"remote plain http":   `{"client_name":"x","redirect_uris":["http://client.example/cb"]}`,
		"unknown grant":       `{"client_name":"x","redirect_uris":["https://client.example/cb"],"grant_types":["password"]}`,
		"malformed JSON":      `{"client_name":`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"invalid_client_metadata"`) {
			t.Fatalf("%s = %d %s", name, response.Code, response.Body.String())
		}
	}
	get := httptest.NewRecorder()
	endpoint.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/oauth/register", nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET register = %d", get.Code)
	}
}

func TestAuthorizationServerMetadataDocument(t *testing.T) {
	t.Parallel()
	handler := NewAuthorizationServerMetadataHandler(builtInConfig())
	if handler == nil {
		t.Fatal("metadata handler missing for built-in server")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	var metadata map[string]any
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &metadata) != nil {
		t.Fatalf("metadata = %d %s", response.Code, response.Body.String())
	}
	want := map[string]any{
		"issuer":                                "https://app.example.com",
		"authorization_endpoint":                "https://app.example.com/oauth/authorize",
		"token_endpoint":                        "https://app.example.com/oauth/token",
		"revocation_endpoint":                   "https://app.example.com/oauth/revoke",
		"registration_endpoint":                 "https://app.example.com/oauth/register",
		"code_challenge_methods_supported":      []any{"S256"},
		"token_endpoint_auth_methods_supported": []any{"none"},
	}
	for key, value := range want {
		if !reflect.DeepEqual(metadata[key], value) {
			t.Fatalf("metadata[%s] = %#v, want %#v", key, metadata[key], value)
		}
	}
	if NewAuthorizationServerMetadataHandler(config.MCP{}) != nil {
		t.Fatal("metadata served without built-in server")
	}
}
