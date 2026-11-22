package web

import (
	"context"
	"database/sql"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/secretbox"
	"example.com/dynamis-code/apps-template/internal/workspaceemail"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceEmailSettingsAreAdminOnlyAndLoginTested(t *testing.T) {
	ctx := context.Background()
	handler, db, auth, _, workspaceID, owner := emailWebTest(t)
	base := "/workspaces/" + workspaceID.String() + "/settings/email"

	invitation, err := auth.CreateInvitation(ctx, mustPrincipal(t, auth, owner.UserID, workspaceID, identity.InvitationsManage), "member@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	memberID, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "member-long-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	ownerSession, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	memberSession, err := auth.CreateSession(ctx, memberID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	ownerCookies := []*http.Cookie{{Name: "session", Value: ownerSession.Secret}, {Name: "csrf", Value: ownerSession.CSRFSecret}}
	memberCookies := []*http.Cookie{{Name: "session", Value: memberSession.Secret}, {Name: "csrf", Value: memberSession.CSRFSecret}}
	form := func(csrf string, password string) url.Values {
		return url.Values{
			"csrf": {csrf}, "sender_name": {"Dynamis"}, "sender_address": {"mailer@example.com"},
			"host": {"smtp.example.com"}, "port": {"587"}, "username": {"mailer"}, "password": {password},
		}
	}

	// A member can neither read nor write the settings.
	memberPage := request(handler, http.MethodGet, base, nil, memberCookies, nil)
	if memberPage.Code == http.StatusOK || strings.Contains(memberPage.Body.String(), `id="email-password"`) {
		t.Fatalf("member saw email settings: %d", memberPage.Code)
	}
	generalBase := "/workspaces/" + workspaceID.String() + "/settings/general"
	memberGeneral := request(handler, http.MethodGet, generalBase, nil, memberCookies, nil)
	if memberGeneral.Code != http.StatusOK || !strings.Contains(memberGeneral.Body.String(), `id="workspace-locale"`) ||
		!strings.Contains(memberGeneral.Body.String(), "disabled") || strings.Contains(memberGeneral.Body.String(), "data-requires-change") ||
		strings.Contains(memberGeneral.Body.String(), "/settings/email") {
		t.Fatalf("member general settings = %d, body=%s", memberGeneral.Code, memberGeneral.Body.String())
	}
	ownerGeneral := request(handler, http.MethodGet, generalBase+"?saved=1", nil, ownerCookies, nil)
	if ownerGeneral.Code != http.StatusOK || ownerGeneral.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(ownerGeneral.Body.String(), `role="status"`) || !strings.Contains(ownerGeneral.Body.String(), "Workspace settings saved.") ||
		!strings.Contains(ownerGeneral.Body.String(), "No workspace notification preferences are set.") {
		t.Fatalf("owner general settings saved state = %d, body=%s", ownerGeneral.Code, ownerGeneral.Body.String())
	}
	invalidLocale := request(handler, http.MethodPost, generalBase, url.Values{
		"csrf": {ownerSession.CSRFSecret}, "locale": {"invalid"},
	}, ownerCookies, nil)
	if invalidLocale.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidLocale.Body.String(), "The selected language is invalid.") {
		t.Fatalf("invalid workspace locale = %d, body=%s", invalidLocale.Code, invalidLocale.Body.String())
	}
	invalidPreference := request(handler, http.MethodPost, "/workspaces/"+workspaceID.String()+"/settings/notifications", url.Values{
		"csrf": {ownerSession.CSRFSecret}, "notification_type": {""}, "enabled": {"true"},
	}, ownerCookies, nil)
	if invalidPreference.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidPreference.Body.String(), "The notification preference is invalid.") {
		t.Fatalf("invalid notification preference = %d, body=%s", invalidPreference.Code, invalidPreference.Body.String())
	}
	localizedCookies := append(append([]*http.Cookie(nil), ownerCookies...),
		&http.Cookie{Name: "locale", Value: "es"}, &http.Cookie{Name: themeCookie, Value: "dark"})
	if err := auth.UpdateUserProfile(ctx, owner.UserID, identity.ProfileUpdateInput{Timezone: "UTC", Theme: "dark"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	localizedEmail := request(handler, http.MethodGet, base, nil, localizedCookies, nil)
	if localizedEmail.Code != http.StatusOK || localizedEmail.Header().Get("Content-Language") != "es" ||
		!hasHTMLRootLang(localizedEmail.Body.String(), "es") || !strings.Contains(localizedEmail.Body.String(), `data-theme="dark"`) {
		t.Fatalf("localized email settings = %d, language=%q, body=%s", localizedEmail.Code, localizedEmail.Header().Get("Content-Language"), localizedEmail.Body.String())
	}
	localizedGeneral := request(handler, http.MethodGet, generalBase, nil, localizedCookies, nil)
	if localizedGeneral.Code != http.StatusOK || localizedGeneral.Header().Get("Content-Language") != "es" ||
		!strings.Contains(localizedGeneral.Body.String(), `<option value="workspace">Espacio de trabajo</option>`) {
		t.Fatalf("localized general settings = %d, language=%q, body=%s", localizedGeneral.Code, localizedGeneral.Header().Get("Content-Language"), localizedGeneral.Body.String())
	}
	memberSave := request(handler, http.MethodPost, base, form(memberSession.CSRFSecret, "member-smtp-pass"), memberCookies, nil)
	if memberSave.Code == http.StatusFound || memberSave.Code == http.StatusOK {
		t.Fatalf("member saved email settings: %d", memberSave.Code)
	}
	if count := countWorkspaceEmail(t, db); count != 0 {
		t.Fatalf("member write left %d settings rows", count)
	}

	// A failed login test blocks saving. Port 1 on loopback refuses the connection, so the login test fails.
	failedForm := form(ownerSession.CSRFSecret, "wrong-pass")
	failedForm.Set("host", "127.0.0.1")
	failedForm.Set("port", "1")
	failed := request(handler, http.MethodPost, base, failedForm, ownerCookies, nil)
	if failed.Code != http.StatusUnprocessableEntity || !strings.Contains(failed.Body.String(), "SMTP login test failed") ||
		!strings.Contains(failed.Body.String(), `role="alert"`) || !strings.Contains(failed.Body.String(), `tabindex="-1"`) {
		t.Fatalf("failed login save = %d, body=%s", failed.Code, failed.Body.String())
	}
	if count := countWorkspaceEmail(t, db); count != 0 {
		t.Fatalf("failed login stored %d settings rows", count)
	}

	// The admin page renders the form and never echoes a password.
	page := request(handler, http.MethodGet, base, nil, ownerCookies, nil)
	if page.Code != http.StatusOK || page.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(page.Body.String(), `class="dashboard-workspace"`) ||
		!strings.Contains(page.Body.String(), `class="input`) || !strings.Contains(page.Body.String(), `id="email-password"`) ||
		!strings.Contains(page.Body.String(), "No workspace email provider is set.") || strings.Contains(page.Body.String(), "wrong-pass") {
		t.Fatalf("admin page = %d", page.Code)
	}
	emailSaved := request(handler, http.MethodGet, base+"?saved=1", nil, ownerCookies, nil)
	if emailSaved.Code != http.StatusOK || !strings.Contains(emailSaved.Body.String(), `role="status"`) ||
		!strings.Contains(emailSaved.Body.String(), "Email settings saved.") {
		t.Fatalf("email saved state = %d, body=%s", emailSaved.Code, emailSaved.Body.String())
	}
}

func countWorkspaceEmail(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM workspace_email_settings").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func emailWebTest(t *testing.T) (http.Handler, *sql.DB, *identity.Service, *workspaceemail.Service, identity.WorkspaceID, identity.BootstrapResult) {
	t.Helper()
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns = 1, 1
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
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
	keys := secretbox.New(map[int][]byte{config.GeneralKeyVersion: []byte("01234567890123456789012345678901")}, config.GeneralKeyVersion)
	email := workspaceemail.NewService(db, cfg.Database.Driver, auth, keys, nil)
	handler, err := NewHandlerWithServices(auth, nil, nil, cfg.HTTP, "", "", email)
	if err != nil {
		t.Fatal(err)
	}
	return handler.Routes(), db, auth, email, owner.WorkspaceID, owner
}
