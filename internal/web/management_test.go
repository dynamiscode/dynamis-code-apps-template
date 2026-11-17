package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/portability"
	"example.com/dynamis-code/apps-template/internal/sharing"
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
	itemService := items.NewService(db, cfg.Database.Driver, auth, cfg.Data.ItemsMaxPerWorkspace)
	sharingService := sharing.NewService(db, cfg.Database.Driver, auth)
	portabilityService := portability.NewService(db, cfg.Database.Driver, auth, cfg.Data.ExportMaxRecords, cfg.Data.ExportMaxBytes, cfg.Data.ImportMaxRecords, cfg.Data.ImportMaxBytes, itemService, nil)

	instance1, err := NewHandlerWithServices(auth, itemService, sharingService, portabilityService, nil, cfg.HTTP, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	instance2, err := NewHandlerWithServices(auth, itemService, sharingService, portabilityService, nil, cfg.HTTP, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return instance1.Routes(), instance2.Routes(), auth, owner.WorkspaceID, owner
}

func TestMultiInstanceImportFlashStateless(t *testing.T) {
	instance1, instance2, auth, workspaceID, owner := testMultiInstanceWeb(t)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	root := "/workspaces/" + workspaceID.String() + "/settings/import"

	// 1. Preview on instance 1, then confirm on instance 2.
	csvContent := "title,status\nItem One,active\nItem Two,complete\n"
	previewResp := multipartRequest(t, instance1, root, url.Values{"csrf": {session.CSRFSecret}, "action": {"preview"}}, "items.csv", csvContent, cookies, nil)
	if previewResp.Code != http.StatusOK || !strings.Contains(previewResp.Body.String(), "Records found") {
		t.Fatalf("expected 200 preview from instance1, got %d: %s", previewResp.Code, previewResp.Body.String())
	}
	postResp := multipartRequest(t, instance2, root, url.Values{
		"csrf": {session.CSRFSecret}, "action": {"confirm"}, "confirm": {"yes"},
		"preview_digest": {importDigest(csvContent)}, "preview_format": {"text/csv"},
	}, "items.csv", csvContent, cookies, nil)
	expectedRedirect := root + "?imported=" + url.QueryEscape(fmt.Sprintf("%d", 2))
	if postResp.Code != http.StatusSeeOther || postResp.Header().Get("Location") != expectedRedirect {
		t.Fatalf("expected 303 redirect to %q, got code %d, location %q", expectedRedirect, postResp.Code, postResp.Header().Get("Location"))
	}

	// 2. Follow redirect to instance 2 (verifies statelessness across multi-instance deployment)
	getResp := request(instance2, http.MethodGet, postResp.Header().Get("Location"), nil, cookies, nil)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from instance2, got %d", getResp.Code)
	}
	body := getResp.Body.String()
	if !strings.Contains(body, "Items imported:") || !strings.Contains(body, "2") {
		t.Fatalf("expected instance2 to render import confirmation with 2 items, got:\n%s", body)
	}
	if !strings.Contains(body, `class="success" role="status"`) {
		t.Fatalf("expected instance2 to render success role=status element, got:\n%s", body)
	}

	// 3. Requesting clean import page on instance 2 does not show flash
	cleanResp := request(instance2, http.MethodGet, root, nil, cookies, nil)
	if cleanResp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", cleanResp.Code)
	}
	if strings.Contains(cleanResp.Body.String(), "Items imported:") {
		t.Fatalf("expected instance2 clean page not to show import confirmation, got:\n%s", cleanResp.Body.String())
	}

	// 4. Preview JSON on instance 2, confirm on instance 1, then follow redirect.
	jsonExport, err := json.Marshal(portability.Export{
		FormatVersion: portability.FormatVersion,
		Items:         []items.Item{{ID: 1, Title: "Imported JSON", Status: items.Active}},
	})
	if err != nil {
		t.Fatal(err)
	}
	jsonContent := string(jsonExport)
	jsonPreview := multipartRequest(t, instance2, root, url.Values{"csrf": {session.CSRFSecret}, "action": {"preview"}}, "items.json", jsonContent, cookies, nil)
	if jsonPreview.Code != http.StatusOK || !strings.Contains(jsonPreview.Body.String(), "Imported JSON") {
		t.Fatalf("expected JSON preview from instance2, got %d: %s", jsonPreview.Code, jsonPreview.Body.String())
	}
	jsonPostResp := multipartRequest(t, instance1, root, url.Values{
		"csrf": {session.CSRFSecret}, "action": {"confirm"}, "confirm": {"yes"},
		"preview_digest": {importDigest(jsonContent)}, "preview_format": {"application/json"},
	}, "items.json", jsonContent, cookies, nil)
	expectedJSONRedirect := root + "?imported=" + url.QueryEscape(fmt.Sprintf("%d", 1))
	if jsonPostResp.Code != http.StatusSeeOther || jsonPostResp.Header().Get("Location") != expectedJSONRedirect {
		t.Fatalf("expected 303 redirect to %q, got code %d, location %q", expectedJSONRedirect, jsonPostResp.Code, jsonPostResp.Header().Get("Location"))
	}
	instance1Get := request(instance1, http.MethodGet, jsonPostResp.Header().Get("Location"), nil, cookies, nil)
	if instance1Get.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from instance1, got %d", instance1Get.Code)
	}
	if !strings.Contains(instance1Get.Body.String(), "Items imported:") || !strings.Contains(instance1Get.Body.String(), "1") {
		t.Fatalf("expected instance1 to render import confirmation with 1 item, got:\n%s", instance1Get.Body.String())
	}
}

func TestImportPageQueryParamHandling(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	root := "/workspaces/" + workspaceID.String() + "/settings/import"

	tests := []struct {
		name         string
		target       string
		wantConfirm  bool
		wantCountStr string
	}{
		{
			name:         "valid positive count",
			target:       root + "?imported=5",
			wantConfirm:  true,
			wantCountStr: "5",
		},
		{
			name:         "valid zero count",
			target:       root + "?imported=0",
			wantConfirm:  true,
			wantCountStr: "0",
		},
		{
			name:        "no query param",
			target:      root,
			wantConfirm: false,
		},
		{
			name:        "non-numeric param",
			target:      root + "?imported=abc",
			wantConfirm: false,
		},
		{
			name:        "negative count",
			target:      root + "?imported=-1",
			wantConfirm: false,
		},
		{
			name:        "empty param",
			target:      root + "?imported=",
			wantConfirm: false,
		},
		{
			name:        "whitespace param",
			target:      root + "?imported=%20%20",
			wantConfirm: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := request(handler, http.MethodGet, tc.target, nil, cookies, nil)
			if resp.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", resp.Code)
			}
			body := resp.Body.String()
			hasConfirm := strings.Contains(body, "Items imported:")
			if hasConfirm != tc.wantConfirm {
				t.Fatalf("target %s: wantConfirm=%v, but hasConfirm=%v\nbody: %s", tc.target, tc.wantConfirm, hasConfirm, body)
			}
			if tc.wantConfirm && !strings.Contains(body, tc.wantCountStr) {
				t.Fatalf("target %s: expected count %s in body\nbody: %s", tc.target, tc.wantCountStr, body)
			}
		})
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
		!strings.Contains(body, `data-webmcp-page="tokens"`) || !strings.Contains(body, "/assets/app.js") ||
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
