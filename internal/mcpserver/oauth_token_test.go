package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

const testPKCEVerifier = "verifier-with-enough-entropy-0123456789abcdef"

func s256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type oauthSetup struct {
	state   *testState
	client  identity.OAuthClient
	session identity.NewSession
}

func newOAuthSetup(t *testing.T) oauthSetup {
	t.Helper()
	state := newTestState(t, testOAuthConfig())
	ctx := context.Background()
	client, err := state.auth.RegisterOAuthClient(ctx, "Test MCP client",
		[]string{"https://client.example/callback"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := state.auth.CreateSession(ctx, state.actor.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return oauthSetup{state: state, client: client, session: session}
}

func (setup oauthSetup) code(t *testing.T, scopes []identity.Permission) string {
	t.Helper()
	code, err := setup.state.auth.ApproveOAuthConsent(context.Background(), setup.session.Session,
		setup.state.workspaceKey, setup.client.ClientID, scopes, "https://client.example/callback",
		s256(testPKCEVerifier), testOAuthResourceURL, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func postForm(handler http.Handler, path string, values url.Values) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

func TestOAuthTokenEndpointIssuesRefreshesAndServesMCP(t *testing.T) {
	t.Parallel()
	setup := newOAuthSetup(t)
	tokenEndpoint := NewOAuthTokenEndpoint(setup.state.auth)
	code := setup.code(t, []identity.Permission{identity.ResourcesRead})

	first := postForm(tokenEndpoint, "/oauth/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {setup.client.ClientID},
		"redirect_uri": {"https://client.example/callback"}, "code_verifier": {testPKCEVerifier},
		"resource": {testOAuthResourceURL},
	})
	var issued tokenResponse
	if first.Code != http.StatusOK || json.Unmarshal(first.Body.Bytes(), &issued) != nil {
		t.Fatalf("authorization_code exchange = %d %s", first.Code, first.Body.String())
	}
	if issued.TokenType != "Bearer" || issued.ExpiresIn != 900 || issued.Scope != "resources:read" ||
		issued.AccessToken == "" || issued.RefreshToken == "" {
		t.Fatalf("token response = %#v", issued)
	}
	if first.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("token response Cache-Control = %q", first.Header().Get("Cache-Control"))
	}

	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"oauth","version":"1"}}}`
	served := postMCP(setup.state.handler, issued.AccessToken, "", initialize)
	if served.Code != http.StatusOK || served.Header().Get("Mcp-Session-Id") != "" {
		t.Fatalf("MCP with OAuth access token = %d headers=%v", served.Code, served.Header())
	}

	refreshed := postForm(tokenEndpoint, "/oauth/token", url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {issued.RefreshToken}, "client_id": {setup.client.ClientID},
	})
	if refreshed.Code != http.StatusOK {
		t.Fatalf("refresh = %d %s", refreshed.Code, refreshed.Body.String())
	}
	reused := postForm(tokenEndpoint, "/oauth/token", url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {issued.RefreshToken}, "client_id": {setup.client.ClientID},
	})
	if reused.Code != http.StatusBadRequest || !strings.Contains(reused.Body.String(), `"invalid_grant"`) ||
		strings.Contains(reused.Body.String(), issued.RefreshToken) {
		t.Fatalf("reused refresh = %d %s", reused.Code, reused.Body.String())
	}
}

func TestOAuthTokenEndpointErrorsAreOAuthShaped(t *testing.T) {
	t.Parallel()
	setup := newOAuthSetup(t)
	tokenEndpoint := NewOAuthTokenEndpoint(setup.state.auth)
	code := setup.code(t, []identity.Permission{identity.ResourcesRead})
	const submitted = "submitted-code-value-that-is-not-real"

	bad := postForm(tokenEndpoint, "/oauth/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {submitted}, "client_id": {setup.client.ClientID},
		"redirect_uri": {"https://client.example/callback"}, "code_verifier": {testPKCEVerifier},
		"resource": {testOAuthResourceURL},
	})
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), `"error":"invalid_grant"`) ||
		strings.Contains(bad.Body.String(), submitted) {
		t.Fatalf("unknown code = %d %s", bad.Code, bad.Body.String())
	}
	wrongClient := postForm(tokenEndpoint, "/oauth/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "client_id": {"someone-else"},
		"redirect_uri": {"https://client.example/callback"}, "code_verifier": {testPKCEVerifier},
		"resource": {testOAuthResourceURL},
	})
	if wrongClient.Code != http.StatusUnauthorized || !strings.Contains(wrongClient.Body.String(), `"invalid_client"`) {
		t.Fatalf("wrong client = %d %s", wrongClient.Code, wrongClient.Body.String())
	}
	unsupported := postForm(tokenEndpoint, "/oauth/token", url.Values{"grant_type": {"password"}})
	if unsupported.Code != http.StatusBadRequest || !strings.Contains(unsupported.Body.String(), `"unsupported_grant_type"`) {
		t.Fatalf("unsupported grant = %d %s", unsupported.Code, unsupported.Body.String())
	}
	get := httptest.NewRecorder()
	tokenEndpoint.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/oauth/token", nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET token = %d", get.Code)
	}
}

func TestOAuthAccessTokenScopesChallengesAndRevocation(t *testing.T) {
	t.Parallel()
	setup := newOAuthSetup(t)
	tokenEndpoint := NewOAuthTokenEndpoint(setup.state.auth)
	revokeEndpoint := NewOAuthRevocationEndpoint(setup.state.auth)
	issued := postForm(tokenEndpoint, "/oauth/token", url.Values{
		"grant_type": {"authorization_code"}, "code": {setup.code(t, []identity.Permission{identity.ResourcesRead})},
		"client_id": {setup.client.ClientID}, "redirect_uri": {"https://client.example/callback"},
		"code_verifier": {testPKCEVerifier}, "resource": {testOAuthResourceURL},
	})
	var tokens tokenResponse
	if err := json.Unmarshal(issued.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"items_create_v1","arguments":{}}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	request.Header.Set("Mcp-Name", "items_create_v1")
	forbidden := httptest.NewRecorder()
	setup.state.handler.ServeHTTP(forbidden, request)
	if forbidden.Code != http.StatusForbidden || !strings.Contains(forbidden.Header().Get("WWW-Authenticate"), `error="insufficient_scope"`) {
		t.Fatalf("read-only OAuth token write call = %d headers=%v", forbidden.Code, forbidden.Header())
	}

	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"oauth","version":"1"}}}`
	revoked := postForm(revokeEndpoint, "/oauth/revoke", url.Values{"token": {tokens.AccessToken}})
	if revoked.Code != http.StatusOK || revoked.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("revoke = %d headers=%v", revoked.Code, revoked.Header())
	}
	after := postMCP(setup.state.handler, tokens.AccessToken, "", initialize)
	if after.Code != http.StatusUnauthorized ||
		after.Header().Get("WWW-Authenticate") != `Bearer error="invalid_token", resource_metadata="`+testOAuthMetadataURL+`"` ||
		strings.Contains(after.Body.String(), tokens.AccessToken) {
		t.Fatalf("revoked access token = %d WWW-Authenticate=%q", after.Code, after.Header().Get("WWW-Authenticate"))
	}
	unknown := postForm(revokeEndpoint, "/oauth/revoke", url.Values{"token": {"unknown-token-value"}})
	if unknown.Code != http.StatusOK {
		t.Fatalf("unknown revoke = %d, want 200", unknown.Code)
	}
}
