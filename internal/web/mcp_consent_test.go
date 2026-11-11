package web

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

const (
	consentResource = "https://app.example.com/mcp"
	consentVerifier = "consent-verifier-with-enough-entropy-0123456789"
	consentRedirect = "https://client.example/callback"
)

func consentChallenge() string {
	sum := sha256.Sum256([]byte(consentVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func consentQuery(clientID string) url.Values {
	return url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {consentRedirect},
		"code_challenge": {consentChallenge()}, "code_challenge_method": {"S256"},
		"resource": {consentResource}, "scope": {"resources:read"}, "state": {"state-value-123"},
	}
}

func TestMCPConsentRequiresSessionAndApprovesOnce(t *testing.T) {
	ctx := context.Background()
	handler, auth, _, workspaceID, _ := testWebConfigured(t, 10, false, func(h *Handler) {
		h.EnableMCPOAuth(consentResource)
	})
	client, err := auth.RegisterOAuthClient(ctx, "<script>alert(1)</script>Desk", []string{consentRedirect}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	query := consentQuery(client.ClientID).Encode()

	anonymous := request(handler, http.MethodGet, "/oauth/authorize?"+query, nil, nil, nil)
	if anonymous.Code != http.StatusSeeOther || !strings.HasPrefix(anonymous.Header().Get("Location"), "/login?return_to=") {
		t.Fatalf("anonymous consent = %d Location=%q", anonymous.Code, anonymous.Header().Get("Location"))
	}

	loginPage := request(handler, http.MethodGet, "/login", nil, nil, nil)
	loginCSRF := responseCookie(loginPage, "login_csrf")
	loggedIn := request(handler, http.MethodPost, "/login", url.Values{
		"email": {"owner@example.com"}, "password": {"long-enough-password"}, "csrf": {loginCSRF.Value},
	}, []*http.Cookie{loginCSRF}, nil)
	cookies := []*http.Cookie{responseCookie(loggedIn, "session"), responseCookie(loggedIn, "csrf")}
	csrf := cookies[1].Value

	page := request(handler, http.MethodGet, "/oauth/authorize?"+query, nil, cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `name="workspace_id" value="`+workspaceID+`"`) ||
		strings.Contains(body, "<script>alert(1)") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("consent page = %d %s", page.Code, body)
	}

	assertAccessiblePage(t, body, "Authorize MCP client")
	badRedirect := consentQuery(client.ClientID)
	badRedirect.Set("redirect_uri", "https://evil.example/callback")
	if rejected := request(handler, http.MethodGet, "/oauth/authorize?"+badRedirect.Encode(), nil, cookies, nil); rejected.Code != http.StatusBadRequest {
		t.Fatalf("unregistered redirect consent = %d", rejected.Code)
	}
	wrongResource := consentQuery(client.ClientID)
	wrongResource.Set("resource", "https://other.example/mcp")
	if rejected := request(handler, http.MethodGet, "/oauth/authorize?"+wrongResource.Encode(), nil, cookies, nil); rejected.Code != http.StatusBadRequest {
		t.Fatalf("wrong resource consent = %d", rejected.Code)
	}

	form := url.Values{
		"decision": {"approve"}, "workspace_id": {workspaceID}, "csrf": {csrf},
	}
	for key, values := range consentQuery(client.ClientID) {
		form[key] = values
	}
	withoutCSRF := form.Encode()
	forged := request(handler, http.MethodPost, "/oauth/authorize", parseForm(t, withoutCSRF, "csrf"), cookies, nil)
	if forged.Code != http.StatusForbidden {
		t.Fatalf("POST without CSRF = %d", forged.Code)
	}

	approved := request(handler, http.MethodPost, "/oauth/authorize", form, cookies, nil)
	location, err := url.Parse(approved.Header().Get("Location"))
	if approved.Code != http.StatusSeeOther || err != nil || location.Host != "client.example" ||
		location.Query().Get("code") == "" || location.Query().Get("state") != "state-value-123" {
		t.Fatalf("approve = %d Location=%q", approved.Code, approved.Header().Get("Location"))
	}
	tokens, err := auth.ExchangeOAuthCode(ctx, location.Query().Get("code"), client.ClientID, consentRedirect,
		consentVerifier, consentResource, identity.AuditContext{})
	if err != nil || tokens.AccessToken == "" {
		t.Fatalf("exchange approved code = %v", err)
	}

	denied := request(handler, http.MethodPost, "/oauth/authorize", parseForm(t, withoutCSRFDeny(form), ""), cookies, nil)
	deniedLocation, _ := url.Parse(denied.Header().Get("Location"))
	if denied.Code != http.StatusSeeOther || deniedLocation.Query().Get("error") != "access_denied" ||
		deniedLocation.Query().Get("code") != "" {
		t.Fatalf("deny = %d Location=%q", denied.Code, denied.Header().Get("Location"))
	}
}

func parseForm(t *testing.T, encoded, drop string) url.Values {
	t.Helper()
	values, err := url.ParseQuery(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if drop != "" {
		values.Del(drop)
	}
	return values
}

func withoutCSRFDeny(form url.Values) string {
	copied := url.Values{}
	for key, values := range form {
		copied[key] = values
	}
	copied.Set("decision", "deny")
	return copied.Encode()
}
