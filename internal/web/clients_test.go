package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/portability"
)

type clientsTestEnv struct {
	routes        http.Handler
	auth          *identity.Service
	workspaceID   identity.WorkspaceID
	ownerID       identity.UserID
	cookies       []*http.Cookie
	csrf          string
	viewerCookies []*http.Cookie
	viewerCSRF    string
}

// newClientsTestEnv wires the web handler with the Clients service, signs in the
// workspace owner, and signs in a viewer invited to the same workspace.
func newClientsTestEnv(t *testing.T) clientsTestEnv {
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
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Clients",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	itemService := items.NewService(db, cfg.Database.Driver, auth, cfg.Data.ItemsMaxPerWorkspace)
	handler, err := NewHandlerWithServices(auth, itemService,
		nil, portability.NewService(db, cfg.Database.Driver, auth, cfg.Data.ExportMaxRecords, cfg.Data.ExportMaxBytes, cfg.Data.ImportMaxRecords, cfg.Data.ImportMaxBytes, itemService),
		nil, cfg.HTTP, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler.EnableClients(clients.NewService(db, cfg.Database.Driver, auth))
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	invitations, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(ctx, invitations, "viewer@example.com", identity.Viewer, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	viewerID, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "viewer-long-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	viewerSession, err := auth.CreateSession(ctx, viewerID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return clientsTestEnv{
		routes: handler.Routes(), auth: auth, workspaceID: owner.WorkspaceID, ownerID: owner.UserID,
		cookies:       []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}},
		csrf:          session.CSRFSecret,
		viewerCookies: []*http.Cookie{{Name: "session", Value: viewerSession.Secret}, {Name: "csrf", Value: viewerSession.CSRFSecret}},
		viewerCSRF:    viewerSession.CSRFSecret,
	}
}

func (env clientsTestEnv) listPath() string {
	return "/workspaces/" + env.workspaceID.String() + "/clients"
}

func TestClientsListShowsCreateFormOnlyToWriters(t *testing.T) {
	env := newClientsTestEnv(t)
	owner := request(env.routes, http.MethodGet, env.listPath(), nil, env.cookies, nil)
	if owner.Code != http.StatusOK || !strings.Contains(owner.Body.String(), `name="display_name"`) ||
		!strings.Contains(owner.Body.String(), `action="`+env.listPath()+`"`) {
		t.Fatalf("owner list = %d, want the create form: %s", owner.Code, owner.Body.String())
	}
	viewer := request(env.routes, http.MethodGet, env.listPath(), nil, env.viewerCookies, nil)
	body := viewer.Body.String()
	if viewer.Code != http.StatusOK || strings.Contains(body, `name="display_name"`) || strings.Contains(body, `action="`+env.listPath()+`"`) {
		t.Fatalf("viewer list = %d, want read-only without create controls: %s", viewer.Code, body)
	}
}

func TestClientCreateRedirectsToShortIDDetailAndShowsValues(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada Lovelace"}, "email": {"ada@example.com"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d, want 303: %s", created.Code, created.Body.String())
	}
	location := created.Header().Get("Location")
	if !strings.HasPrefix(location, env.listPath()+"/") {
		t.Fatalf("create redirect = %q, want the client detail path", location)
	}
	detailID := strings.TrimPrefix(location, env.listPath()+"/")
	if _, err := clients.ParseClientID(detailID); err != nil {
		t.Fatalf("detail path segment %q is not a client short ID: %v", detailID, err)
	}
	detail := request(env.routes, http.MethodGet, location, nil, env.viewerCookies, nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "Ada Lovelace") ||
		!strings.Contains(detail.Body.String(), "ada@example.com") {
		t.Fatalf("viewer detail = %d: %s", detail.Code, detail.Body.String())
	}
	if strings.Contains(detail.Body.String(), `name="display_name"`) {
		t.Fatal("viewer detail renders an edit form")
	}
}

func TestClientCreateRefusesBlankNameWithMessageAndRefusesViewer(t *testing.T) {
	env := newClientsTestEnv(t)
	blank := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"   "},
	}, env.cookies, nil)
	if blank.Code != http.StatusUnprocessableEntity || !strings.Contains(blank.Body.String(), "The display name is required.") {
		t.Fatalf("blank create = %d: %s", blank.Code, blank.Body.String())
	}
	badEmail := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada"}, "email": {"Ada <ada@example.com>"},
	}, env.cookies, nil)
	if badEmail.Code != http.StatusUnprocessableEntity || !strings.Contains(badEmail.Body.String(), "The email address is invalid.") {
		t.Fatalf("bad email create = %d: %s", badEmail.Code, badEmail.Body.String())
	}
	viewerWrite := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.viewerCSRF}, "display_name": {"Blocked"},
	}, env.viewerCookies, nil)
	if viewerWrite.Code != http.StatusForbidden {
		t.Fatalf("viewer create = %d, want 403", viewerWrite.Code)
	}
}

func TestClientDetailRefusesMalformedAndIntegerIDs(t *testing.T) {
	env := newClientsTestEnv(t)
	for _, probe := range []string{"1", "0", "not-a-short-id", env.workspaceID.String()} {
		response := request(env.routes, http.MethodGet, env.listPath()+"/"+probe, nil, env.cookies, nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("probe %q = %d, want 404", probe, response.Code)
		}
	}
}

func TestClientUpdateChecksVersionAndShowsMessage(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Before"},
	}, env.cookies, nil)
	detailPath := created.Header().Get("Location")
	saved := request(env.routes, http.MethodPost, detailPath, url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "display_name": {"After"}, "phone": {"555"},
	}, env.cookies, nil)
	if saved.Code != http.StatusSeeOther {
		t.Fatalf("update = %d: %s", saved.Code, saved.Body.String())
	}
	stale := request(env.routes, http.MethodPost, detailPath, url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "display_name": {"Stale"},
	}, env.cookies, nil)
	if stale.Code != http.StatusPreconditionFailed || !strings.Contains(stale.Body.String(), "This client changed.") {
		t.Fatalf("stale update = %d: %s", stale.Code, stale.Body.String())
	}
	viewerEdit := request(env.routes, http.MethodPost, detailPath, url.Values{
		"csrf": {env.viewerCSRF}, "version": {"2"}, "display_name": {"Viewer"},
	}, env.viewerCookies, nil)
	if viewerEdit.Code != http.StatusForbidden {
		t.Fatalf("viewer update = %d, want 403", viewerEdit.Code)
	}
}

func TestClientLabelsFollowWorkspaceLocale(t *testing.T) {
	env := newClientsTestEnv(t)
	if err := env.auth.UpdateWorkspaceLocale(context.Background(), mustOwnerPrincipal(t, env), "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	response := request(env.routes, http.MethodGet, env.listPath(), nil, env.cookies, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Nombre para mostrar") {
		t.Fatalf("Spanish list = %d, want Spanish labels: %s", response.Code, response.Body.String())
	}
}

func mustOwnerPrincipal(t *testing.T, env clientsTestEnv) identity.Principal {
	t.Helper()
	principal, err := env.auth.Authorize(context.Background(), env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	return principal
}
