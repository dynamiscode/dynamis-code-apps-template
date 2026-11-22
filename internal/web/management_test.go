package web

import (
	"context"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/portability"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testMultiInstanceWeb(t *testing.T) (http.Handler, http.Handler, *identity.Service, identity.WorkspaceID, identity.BootstrapResult) {
	t.Helper()
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns = 1
	cfg.Database.MaxIdleConns = 1
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	portabilityService := portability.NewService(db, cfg.Database.Driver, auth, cfg.Data.ExportMaxRecords, cfg.Data.ExportMaxBytes, nil)

	instance1, err := NewHandlerWithServices(auth, portabilityService, nil, cfg.HTTP, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	instance2, err := NewHandlerWithServices(auth, portabilityService, nil, cfg.HTTP, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return instance1.Routes(), instance2.Routes(), auth, owner.WorkspaceID, owner
}

func TestItemImportRouteRemoved(t *testing.T) {
	instance1, instance2, auth, workspaceID, owner := testMultiInstanceWeb(t)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/settings/import"
	for _, handler := range []http.Handler{instance1, instance2} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			response := request(handler, method, path, nil, cookies, nil)
			if response.Code != http.StatusNotFound {
				t.Fatalf("%s %s = %d, want 404", method, path, response.Code)
			}
		}
	}
}

func TestAPITokenSettingsTemplLifecycle(t *testing.T) {
	handler, _, auth, workspaceID, owner := testMultiInstanceWeb(t)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/settings/tokens"

	page := request(handler, http.MethodGet, path, nil, cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || page.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(body, `data-webmcp-page="tokens"`) || !strings.Contains(body, "/assets/alpine-components.js") ||
		!strings.Contains(body, "/assets/components.css") || !strings.Contains(body, "No API tokens yet.") {
		t.Fatalf("API token settings page = %d, cache-control=%q, required view markers present=%v", page.Code, page.Header().Get("Cache-Control"), strings.Contains(body, "No API tokens yet."))
	}

	form := url.Values{"action": {"create"}, "name": {"automation"}, "scope": {"workspace:read"}}
	withoutCSRF := request(handler, http.MethodPost, path, form, cookies, nil)
	if withoutCSRF.Code != http.StatusForbidden {
		t.Fatalf("API token create without CSRF = %d", withoutCSRF.Code)
	}
	invalidExpiry := url.Values{
		"action": {"create"}, "name": {"automation"}, "scope": {"workspace:read"},
		"expires_at": {"not-a-date"}, "csrf": {session.CSRFSecret},
	}
	validation := request(handler, http.MethodPost, path, invalidExpiry, cookies, nil)
	if validation.Code != http.StatusUnprocessableEntity || validation.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(validation.Body.String(), `role="alert"`) || !strings.Contains(validation.Body.String(), "The token expiration is invalid.") {
		t.Fatalf("API token validation state = %d, cache-control=%q, alert present=%v", validation.Code, validation.Header().Get("Cache-Control"), strings.Contains(validation.Body.String(), `role="alert"`))
	}
	deniedScope := request(handler, http.MethodPost, path, url.Values{
		"action": {"create"}, "name": {"automation"}, "scope": {"unknown:permission"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if deniedScope.Code != http.StatusUnprocessableEntity || !strings.Contains(deniedScope.Body.String(), "The token could not be created.") {
		t.Fatalf("API token denied-scope state = %d, alert present=%v", deniedScope.Code, strings.Contains(deniedScope.Body.String(), `role="alert"`))
	}

	form.Set("csrf", session.CSRFSecret)
	created := request(handler, http.MethodPost, path, form, cookies, nil)
	createdBody := created.Body.String()
	if created.Code != http.StatusOK || created.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(createdBody, "Copy this token now") || !strings.Contains(createdBody, `role="status"`) {
		t.Fatalf("API token create = %d, cache-control=%q, one-time alert present=%v", created.Code, created.Header().Get("Cache-Control"), strings.Contains(createdBody, "Copy this token now"))
	}
	secret := extractAPITokenSecret(t, createdBody)
	if strings.Count(createdBody, secret) != 1 {
		t.Fatal("API token secret was not rendered exactly once")
	}

	principal, err := auth.Authorize(context.Background(), owner.UserID, workspaceID, identity.WorkspaceRead)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.ListAPITokens(context.Background(), principal)
	if err != nil || len(tokens) != 1 || tokens[0].Name != "automation" {
		t.Fatalf("created API token list length=%d, err=%v", len(tokens), err)
	}
	page = request(handler, http.MethodGet, path, nil, cookies, nil)
	if page.Code != http.StatusOK || page.Header().Get("Cache-Control") != "no-store" || strings.Contains(page.Body.String(), secret) {
		t.Fatalf("API token readback = %d, cache-control=%q, one-time secret absent=%v", page.Code, page.Header().Get("Cache-Control"), !strings.Contains(page.Body.String(), secret))
	}
	if !strings.Contains(page.Body.String(), `data-confirm=`) || !strings.Contains(page.Body.String(), `data-webmcp-id="`+tokens[0].ID.String()+`"`) {
		t.Fatal("active API token lacks a confirmed revoke form")
	}

	revoked := request(handler, http.MethodPost, path+"/"+tokens[0].ID.String(), url.Values{
		"action": {"revoke"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if revoked.Code != http.StatusSeeOther || revoked.Header().Get("Location") != path {
		t.Fatalf("API token revoke = %d, location=%q", revoked.Code, revoked.Header().Get("Location"))
	}
	page = request(handler, http.MethodGet, path, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "revoked") || strings.Contains(page.Body.String(), `data-webmcp-id="`+tokens[0].ID.String()+`"`) {
		t.Fatalf("revoked API token state = %d, revoked label present=%v, revoke form absent=%v", page.Code, strings.Contains(page.Body.String(), "revoked"), !strings.Contains(page.Body.String(), `data-webmcp-id="`+tokens[0].ID.String()+`"`))
	}
}

func extractAPITokenSecret(t *testing.T, body string) string {
	t.Helper()
	warningStart := strings.Index(body, "Copy this token now")
	if warningStart < 0 {
		t.Fatal("API token one-time warning is missing")
	}
	start := strings.Index(body[warningStart:], "<code")
	if start < 0 {
		t.Fatal("API token secret code element is missing")
	}
	start += warningStart
	contentStart := strings.Index(body[start:], ">")
	if contentStart < 0 {
		t.Fatal("API token secret code element is malformed")
	}
	start += contentStart + 1
	end := strings.Index(body[start:], "</code>")
	if end < 0 {
		t.Fatal("API token secret code element is not closed")
	}
	secret := body[start : start+end]
	if secret == "" {
		t.Fatal("API token secret is empty")
	}
	return secret
}
