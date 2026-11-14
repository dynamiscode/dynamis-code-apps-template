package web

import (
	"context"
	"database/sql"
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
	db            *sql.DB
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
	handler.EnableClients(clients.NewService(db, cfg.Database.Driver, auth, cfg.Data.ClientFieldsMaxPerWorkspace))
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
		db: db, routes: handler.Routes(), auth: auth, workspaceID: owner.WorkspaceID, ownerID: owner.UserID,
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

// The list filters and sorts from an ordinary GET form, for every role that
// can read. An invalid choice gets a message, not a server error.
func TestClientsListFilterAndSortFromTheForm(t *testing.T) {
	env := newClientsTestEnv(t)
	for _, name := range []string{"Ada", "Bea", "Cy"} {
		created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
			"csrf": {env.csrf}, "display_name": {name},
		}, env.cookies, nil)
		if created.Code != http.StatusSeeOther {
			t.Fatalf("create %s = %d", name, created.Code)
		}
	}
	list := func(t *testing.T, query string) (int, string) {
		t.Helper()
		response := request(env.routes, http.MethodGet, env.listPath()+query, nil, env.viewerCookies, nil)
		return response.Code, response.Body.String()
	}
	code, body := list(t, "?sort=displayName&order=desc")
	if code != http.StatusOK || strings.Index(body, ">Cy<") > strings.Index(body, ">Bea<") ||
		strings.Index(body, ">Bea<") > strings.Index(body, ">Ada<") {
		t.Fatalf("sorted list = %d: %s", code, body)
	}
	code, body = list(t, "?filter_field=displayName&filter_op=contains&filter_value=A")
	if code != http.StatusOK || !strings.Contains(body, ">Ada<") || !strings.Contains(body, ">Bea<") ||
		strings.Contains(body, ">Cy<") {
		t.Fatalf("filtered list = %d: %s", code, body)
	}
	if !strings.Contains(body, `name="filter_op"`) || !strings.Contains(body, `name="sort"`) {
		t.Fatalf("list lacks the query form: %s", body)
	}
	code, body = list(t, "?filter_field=displayName&filter_op=before&filter_value=x")
	if code != http.StatusBadRequest || !strings.Contains(body, "Choose a valid sort or filter.") {
		t.Fatalf("invalid filter = %d: %s", code, body)
	}

	// A choice field's form offers its options, and filtering by one works.
	service := clients.NewService(env.db, config.SQLite, env.auth, 50)
	owner := mustOwnerPrincipal(t, env)
	tier, err := service.DefineField(context.Background(), owner, env.workspaceID,
		clients.FieldInput{Label: "Tier", Type: clients.FieldTypeSingleChoice}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	gold, err := service.AddOption(context.Background(), owner, env.workspaceID, tier.ID, "Gold", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	code, body = list(t, "?filter_field="+tier.ID.String()+"&filter_op=equals")
	if code != http.StatusOK || !strings.Contains(body, `value="`+gold.ID.String()+`"`) || !strings.Contains(body, "Gold") {
		t.Fatalf("choice form = %d: %s", code, body)
	}
}

// TestClientEditRefusalKeepsTheTypedValues re-renders the edit form with what
// the user typed, so a refused save does not discard it.
func TestClientEditRefusalKeepsTheTypedValues(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Before"},
	}, env.cookies, nil)
	detailPath := created.Header().Get("Location")
	refused := request(env.routes, http.MethodPost, detailPath, url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "display_name": {"   "}, "phone": {"555-0100"},
	}, env.cookies, nil)
	if refused.Code != http.StatusUnprocessableEntity || !strings.Contains(refused.Body.String(), "The display name is required.") {
		t.Fatalf("blank edit = %d: %s", refused.Code, refused.Body.String())
	}
	if !strings.Contains(refused.Body.String(), `value="555-0100"`) {
		t.Fatalf("refused edit lost the typed phone: %s", refused.Body.String())
	}
}
