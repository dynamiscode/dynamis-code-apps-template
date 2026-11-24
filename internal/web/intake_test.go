package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/intake"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/secretbox"
)

func TestIntakeManagementThroughWeb(t *testing.T) {
	env := newClientsTestEnv(t)
	path := "/workspaces/" + env.workspaceID.String() + "/intake"
	page := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Link disabled") || !strings.Contains(page.Body.String(), "public form is unavailable") || !strings.Contains(page.Body.String(), "/engagement-stages") {
		t.Fatalf("initial: %d", page.Code)
	}
	if !strings.Contains(page.Body.String(), `aria-current="page" href="`+path+`"`) {
		t.Fatal("intake settings navigation must mark the current page")
	}
	created := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "action": {"create"}, "consent": {"I agree"}}, env.cookies, nil)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), "/intake/") || !strings.Contains(created.Body.String(), "public form is unavailable") || strings.Contains(created.Body.String(), "Link active") {
		t.Fatalf("create: %d", created.Code)
	}
	if strings.Contains(created.Body.String(), "data-copy-target=\"intake-link\"") || !strings.Contains(created.Body.String(), "Existing links and QR codes will stop working") {
		t.Fatal("unavailable link must not offer copying and must warn before replacement")
	}
	owner, err := env.auth.Authorize(context.Background(), env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engagement.NewService(env.db, config.SQLite, env.auth).CreateStage(context.Background(), owner, env.workspaceID, engagement.StageInput{Name: "New", Kind: "open"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	page = request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	if !strings.Contains(page.Body.String(), `data-copy-target="intake-link"`) {
		t.Fatal("existing link must remain available on a subsequent page load")
	}
	if page.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("intake settings must not be cached")
	}
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), "public form is unavailable") {
		t.Fatalf("open stage: %d", page.Code)
	}
	denied := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("viewer page: %d", denied.Code)
	}
	denied = request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.viewerCSRF}, "action": {"disable"}}, env.viewerCookies, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("viewer disable: %d", denied.Code)
	}
	disabled := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "action": {"disable"}}, env.cookies, nil)
	if disabled.Code != http.StatusSeeOther {
		t.Fatalf("disable: %d", disabled.Code)
	}
}

func TestIntakeMissingPrimaryKey(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	owner, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	svc := intake.NewService(env.db, config.SQLite, env.auth)
	svc.ConfigureEncryption(secretbox.New(map[int][]byte{1: []byte("01234567890123456789012345678901")}, 1))
	token, err := svc.Create(ctx, owner, env.workspaceID, "Original consent", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandlerWithServices(env.auth, nil, nil, config.HTTP{}, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler.EnableIntake(intake.NewService(env.db, config.SQLite, env.auth))
	if err := env.auth.SetUserLocale(ctx, env.ownerID, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	path := "/workspaces/" + env.workspaceID.String() + "/intake"
	response := request(handler.Routes(), http.MethodPost, path, url.Values{"csrf": {env.csrf}, "action": {"rotate"}, "consent": {"Unsaved consent"}}, env.cookies, map[string]string{"Accept-Language": "es"})
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "ENCRYPTION_KEY") || !strings.Contains(response.Body.String(), "Unsaved consent") || !strings.Contains(response.Body.String(), "reinicia el servidor") {
		t.Fatalf("missing key: status %d, localized message or input missing", response.Code)
	}
	settings, err := svc.GetSettings(ctx, owner, env.workspaceID)
	if err != nil || !settings.Active || settings.ConsentText != "Original consent" {
		t.Fatalf("settings changed: %+v, %v", settings, err)
	}
	if got, err := svc.GetToken(ctx, owner, env.workspaceID); err != nil || got != token {
		t.Fatalf("existing link changed: %v", err)
	}
}

func TestIntakeRetrievalFailures(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	owner, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	svc := intake.NewService(env.db, config.SQLite, env.auth)
	svc.ConfigureEncryption(secretbox.New(map[int][]byte{1: []byte("01234567890123456789012345678901")}, 1))
	token, err := svc.Create(ctx, owner, env.workspaceID, "Saved consent", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	path := "/workspaces/" + env.workspaceID.String() + "/intake"
	handler, err := NewHandlerWithServices(env.auth, nil, nil, config.HTTP{}, "", "https://example.test/base/", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler.EnableIntake(svc)
	page := request(handler.Routes(), http.MethodGet, path, nil, env.cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "https://example.test/base/intake/"+token) {
		t.Fatal("retrieved link must use the configured public URL")
	}
	for _, tc := range []struct {
		name string
		keys secretbox.Keyring
	}{
		{"missing", secretbox.Keyring{}},
		{"wrong", secretbox.New(map[int][]byte{1: []byte("abcdefghijabcdefghijabcdefghijab")}, 1)},
		{"invalid length", secretbox.New(map[int][]byte{1: []byte("short")}, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := NewHandlerWithServices(env.auth, nil, nil, config.HTTP{}, "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			broken := intake.NewService(env.db, config.SQLite, env.auth)
			broken.ConfigureEncryption(tc.keys)
			handler.EnableIntake(broken)
			response := request(handler.Routes(), http.MethodGet, path, nil, env.cookies, nil)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Restore the original encryption key") || strings.Contains(response.Body.String(), "Rotate it to get") || strings.Contains(response.Body.String(), `data-copy-target="intake-link"`) {
				t.Fatalf("key failure: status %d, incorrect recovery guidance", response.Code)
			}
		})
	}
	if _, err := env.db.ExecContext(ctx, `UPDATE public_intake_links SET encrypted_token = NULL, token_key_version = NULL WHERE workspace_id = ?`, env.workspaceID); err != nil {
		t.Fatal(err)
	}
	response := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Rotate it to get a shareable link") || strings.Contains(response.Body.String(), "Restore the original encryption key") {
		t.Fatalf("legacy link: status %d, incorrect recovery guidance", response.Code)
	}
}

func TestPublicIntakeForm(t *testing.T) {
	env := newClientsTestEnv(t)
	owner, err := env.auth.Authorize(context.Background(), env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engagement.NewService(env.db, config.SQLite, env.auth).CreateStage(context.Background(), owner, env.workspaceID, engagement.StageInput{Name: "New", Kind: "open"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	svc := intake.NewService(env.db, config.SQLite, env.auth)
	svc.ConfigureEncryption(secretbox.New(map[int][]byte{1: []byte("01234567890123456789012345678901")}, 1))
	token, err := svc.Create(context.Background(), owner, env.workspaceID, "Custom consent", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	page := request(env.routes, http.MethodGet, "/intake/"+token, nil, nil, map[string]string{"Accept-Language": "es"})
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Custom consent") || !strings.Contains(page.Body.String(), "Nombre") {
		t.Fatalf("public form: status %d, body %s", page.Code, page.Body.String())
	}
	invalid := request(env.routes, http.MethodPost, "/intake/"+token, url.Values{"name": {"Visitor"}}, nil, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "Visitor") {
		t.Fatalf("invalid form: %d", invalid.Code)
	}
	missingContact := request(env.routes, http.MethodPost, "/intake/"+token, url.Values{"name": {"Visitor"}, "consent": {"on"}}, nil, nil)
	if missingContact.Code != http.StatusUnprocessableEntity || !strings.Contains(missingContact.Body.String(), "Enter a phone number or email address to continue.") {
		t.Fatalf("missing contact: %d", missingContact.Code)
	}
	result := request(env.routes, http.MethodPost, "/intake/"+token, url.Values{"name": {"Visitor"}, "email": {"visitor@example.com"}, "consent": {"on"}}, nil, nil)
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), "Thank you") || strings.Contains(result.Body.String(), "Visitor") || strings.Contains(result.Body.String(), "/workspaces/") {
		t.Fatalf("confirmation: %d, body %s", result.Code, result.Body.String())
	}
	bot := request(env.routes, http.MethodPost, "/intake/"+token, url.Values{"website": {"spam"}}, nil, nil)
	var count int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM clients`).Scan(&count); err != nil || bot.Code != http.StatusOK || bot.Body.String() != result.Body.String() || count != 1 {
		t.Fatalf("honeypot: status %d, clients %d, error %v", bot.Code, count, err)
	}
	missing := request(env.routes, http.MethodGet, "/intake/unknown", nil, nil, nil)
	if missing.Code != http.StatusNotFound || strings.Contains(missing.Body.String(), "Custom consent") {
		t.Fatalf("unknown link: %d", missing.Code)
	}
}
