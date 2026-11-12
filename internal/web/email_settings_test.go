package web

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/secretbox"
	"example.com/dynamis-code/apps-template/internal/workspaceemail"
)

func TestWorkspaceEmailSettingsAreAdminOnlyAndLoginTested(t *testing.T) {
	ctx := context.Background()
	handler, db, auth, _, workspaceID, owner := emailWebTest(t)
	base := "/workspaces/" + workspaceID + "/settings/email"

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
	if failed.Code != http.StatusUnprocessableEntity || !strings.Contains(failed.Body.String(), "SMTP login test failed") {
		t.Fatalf("failed login save = %d, body=%s", failed.Code, failed.Body.String())
	}
	if count := countWorkspaceEmail(t, db); count != 0 {
		t.Fatalf("failed login stored %d settings rows", count)
	}

	// The admin page renders the form and never echoes a password.
	page := request(handler, http.MethodGet, base, nil, ownerCookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `id="email-password"`) || strings.Contains(page.Body.String(), "wrong-pass") {
		t.Fatalf("admin page = %d", page.Code)
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

func emailWebTest(t *testing.T) (http.Handler, *sql.DB, *identity.Service, *workspaceemail.Service, string, identity.BootstrapResult) {
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
	itemService := items.NewService(db, cfg.Database.Driver, auth, cfg.Data.ItemsMaxPerWorkspace)
	handler, err := NewHandlerWithServices(auth, itemService, nil, nil, nil, cfg.HTTP, "", "", email, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler.Routes(), db, auth, email, owner.WorkspaceID, owner
}
