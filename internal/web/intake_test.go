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
	if strings.Contains(created.Body.String(), "data-copy-target=\"intake-link\"") {
		t.Fatal("unavailable link must not offer copying")
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
	disabled := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "action": {"disable"}, "confirmed": {"1"}}, env.cookies, nil)
	if disabled.Code != http.StatusSeeOther {
		t.Fatalf("disable: %d", disabled.Code)
	}
}

func TestIntakeDestructiveActionsRequireConfirmation(t *testing.T) {
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
	path := "/workspaces/" + env.workspaceID.String() + "/intake"
	for _, action := range []string{"rotate", "disable"} {
		values := url.Values{"csrf": {env.csrf}, "action": {action}, "consent": {"Updated & consent"}}
		page := request(env.routes, http.MethodPost, path, values, env.cookies, nil)
		for _, want := range []string{"Existing links and QR codes will stop working immediately", `name="confirmed" value="1"`, `name="csrf"`, `name="action" value="` + action + `"`, `href="` + path + `"`, "Cancel"} {
			if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), want) {
				t.Fatalf("%s confirmation: status %d, missing %q", action, page.Code, want)
			}
		}
		if action == "rotate" && !strings.Contains(page.Body.String(), `name="consent" value="Updated &amp; consent"`) {
			t.Fatal("rotation confirmation must preserve consent")
		}
		if got, err := svc.GetToken(ctx, owner, env.workspaceID); err != nil || got != token {
			t.Fatalf("%s before confirmation changed link: %v", action, err)
		}
		cancel := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
		if cancel.Code != http.StatusOK {
			t.Fatalf("%s cancel: %d", action, cancel.Code)
		}
		spanish := append(append([]*http.Cookie(nil), env.cookies...), &http.Cookie{Name: "locale", Value: "es"})
		page = request(env.routes, http.MethodPost, path, values, spanish, nil)
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "dejarán de funcionar de inmediato") || !strings.Contains(page.Body.String(), "Cancelar") {
			t.Fatalf("%s Spanish confirmation: %d", action, page.Code)
		}
		values.Set("confirmed", "1")
		values.Del("csrf")
		if got := request(env.routes, http.MethodPost, path, values, env.cookies, nil); got.Code != http.StatusForbidden {
			t.Fatalf("%s without CSRF: %d", action, got.Code)
		}
		values.Set("csrf", env.viewerCSRF)
		if got := request(env.routes, http.MethodPost, path, values, env.viewerCookies, nil); got.Code != http.StatusForbidden {
			t.Fatalf("%s as viewer: %d", action, got.Code)
		}
	}
	rotated := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "action": {"rotate"}, "confirmed": {"1"}, "consent": {"Updated & consent"}}, env.cookies, nil)
	if rotated.Code != http.StatusOK {
		t.Fatalf("confirmed rotation: %d", rotated.Code)
	}
	newToken, err := svc.GetToken(ctx, owner, env.workspaceID)
	if err != nil || newToken == token {
		t.Fatalf("confirmed rotation did not replace link: %v", err)
	}
	settings, err := svc.GetSettings(ctx, owner, env.workspaceID)
	if err != nil || settings.ConsentText != "Updated & consent" {
		t.Fatalf("rotation did not preserve consent: %+v, %v", settings, err)
	}
	disabled := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "action": {"disable"}, "confirmed": {"1"}}, env.cookies, nil)
	if disabled.Code != http.StatusSeeOther {
		t.Fatalf("confirmed disable: %d", disabled.Code)
	}
	settings, err = svc.GetSettings(ctx, owner, env.workspaceID)
	if err != nil || settings.Active {
		t.Fatalf("confirmed disable did not disable link: %+v, %v", settings, err)
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
	response := request(handler.Routes(), http.MethodPost, path, url.Values{"csrf": {env.csrf}, "action": {"rotate"}, "confirmed": {"1"}, "consent": {"Unsaved consent"}}, env.cookies, map[string]string{"Accept-Language": "es"})
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
	if !strings.Contains(page.Body.String(), `class="min-w-0 wrap-anywhere"`) {
		t.Fatal("public consent must wrap long unbroken text")
	}
	for _, marker := range []string{`data-entry-field`, `for="name"`, `for="phone"`, `for="email"`, `for="notes"`, `name="website"`, `x-data="pageBehaviors"`} {
		if !strings.Contains(page.Body.String(), marker) {
			t.Errorf("public form missing %s", marker)
		}
	}
	if strings.Count(page.Body.String(), `data-entry-required-group="contact"`) != 2 {
		t.Error("public form must require at least one contact method")
	}
	invalid := request(env.routes, http.MethodPost, "/intake/"+token, url.Values{"name": {"Visitor"}, "phone": {"123"}, "notes": {"Private notes"}}, nil, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `value="Visitor"`) ||
		!strings.Contains(invalid.Body.String(), `value="123"`) || !strings.Contains(invalid.Body.String(), ">Private notes</textarea>") {
		t.Fatalf("invalid form must preserve the draft: %d, %s", invalid.Code, invalid.Body.String())
	}
	missingContact := request(env.routes, http.MethodPost, "/intake/"+token, url.Values{"name": {"Visitor"}, "consent": {"on"}}, nil, nil)
	if missingContact.Code != http.StatusUnprocessableEntity || !strings.Contains(missingContact.Body.String(), "Enter a phone number or email address to continue.") {
		t.Fatalf("missing contact: %d", missingContact.Code)
	}
	for _, tc := range []struct {
		name   string
		values url.Values
		lang   string
		want   []string
	}{
		{
			name:   "missing name contact and consent",
			values: url.Values{"name": {" "}, "notes": {"Private <notes>"}},
			want:   []string{`role="alert"`, `tabindex="-1"`, `aria-describedby="name-error"`, `aria-describedby="intake-contact-help phone-error"`, `aria-describedby="intake-contact-help email-error"`, `aria-describedby="consent-error"`, `Private &lt;notes&gt;`},
		},
		{
			name:   "invalid fields preserve consent and values",
			values: url.Values{"name": {strings.Repeat("a", 201)}, "phone": {strings.Repeat("1", 51)}, "email": {"bad email"}, "notes": {strings.Repeat("n", 2001)}, "consent": {"on"}},
			want:   []string{`aria-describedby="name-error"`, `aria-describedby="intake-contact-help phone-error"`, `aria-describedby="intake-contact-help email-error"`, `aria-describedby="notes-error"`, `name="consent" required checked`, `value="bad email"`},
		},
		{
			name:   "Spanish consent error",
			values: url.Values{"name": {"Visitante"}, "email": {"visitor@example.com"}},
			lang:   "es",
			want:   []string{`aria-describedby="consent-error"`, `Debes dar tu consentimiento antes de enviar.`, `value="Visitante"`, `value="visitor@example.com"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := request(env.routes, http.MethodPost, "/intake/"+token, tc.values, nil, map[string]string{"Accept-Language": tc.lang})
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", response.Code)
			}
			for _, marker := range tc.want {
				if !strings.Contains(response.Body.String(), marker) {
					t.Errorf("response missing %q", marker)
				}
			}
			if !strings.Contains(response.Body.String(), `aria-invalid="true"`) {
				t.Error("invalid fields must expose aria-invalid")
			}
			if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" {
				t.Error("validation response must retain privacy headers")
			}
		})
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
