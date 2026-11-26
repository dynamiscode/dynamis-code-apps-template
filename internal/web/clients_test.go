package web

import (
	"context"
	"database/sql"
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/intake"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/listviews"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/secretbox"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
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
	handler, err := NewHandlerWithServices(auth, nil, nil, cfg.HTTP, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler.EnableClients(clients.NewService(db, cfg.Database.Driver, auth, cfg.Data.ClientFieldsMaxPerWorkspace),
		listprefs.NewService(db, cfg.Database.Driver, auth))
	handler.EnableEngagement(engagement.NewService(db, cfg.Database.Driver, auth))
	intakeService := intake.NewService(db, cfg.Database.Driver, auth)
	intakeService.ConfigureEncryption(secretbox.New(map[int][]byte{1: []byte("01234567890123456789012345678901")}, 1))
	handler.EnableIntake(intakeService)
	handler.EnableListViews(listviews.NewService(db, cfg.Database.Driver, auth))
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

// TestClientsListMatchAnyJoinsFilters checks that match=any returns the union of two filters
// on different fields, and match=all returns their intersection.
func TestClientsListMatchAnyJoinsFilters(t *testing.T) {
	env := newClientsTestEnv(t)
	for _, client := range []struct{ name, email string }{
		{"Ada", "ada@example.test"}, {"Bea", "bea@example.test"}, {"Cy", "cy@example.test"},
	} {
		created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
			"csrf": {env.csrf}, "display_name": {client.name}, "email": {client.email},
		}, env.cookies, nil)
		if created.Code != http.StatusSeeOther {
			t.Fatalf("create %s = %d", client.name, created.Code)
		}
	}
	list := func(t *testing.T, query string) (int, string) {
		t.Helper()
		response := request(env.routes, http.MethodGet, env.listPath()+withAll(query), nil, env.viewerCookies, nil)
		return response.Code, response.Body.String()
	}
	// Ada matches the name filter, Cy matches the email filter, and nobody matches both.
	code, body := list(t, "?filter=displayName:contains:Ada&filter=email:contains:cy@&match=any")
	if code != http.StatusOK || !strings.Contains(body, ">Ada<") || !strings.Contains(body, ">Cy<") || strings.Contains(body, ">Bea<") {
		t.Fatalf("match=any = %d, want Ada and Cy: %s", code, body)
	}
	code, body = list(t, "?filter=displayName:contains:Ada&filter=email:contains:cy@&match=all")
	if code != http.StatusOK || strings.Contains(body, ">Ada<") || strings.Contains(body, ">Cy<") {
		t.Fatalf("match=all = %d, want no clients: %s", code, body)
	}
	code, _ = list(t, "?filter=displayName:contains:Ada&match=sometimes")
	if code != http.StatusBadRequest {
		t.Fatalf("invalid match = %d, want 400", code)
	}
	// The rows-per-page dropdown sits on the toolbar, offers 10, 30, 50, and 100, and keeps the match mode.
	code, body = list(t, "?filter=displayName:contains:Ada&filter=email:contains:cy@&match=any")
	if code != http.StatusOK || !strings.Contains(body, `list-page-size`) ||
		!strings.Contains(body, `limit=10`) || !strings.Contains(body, `limit=30`) ||
		!strings.Contains(body, `limit=100`) || !strings.Contains(body, `match=any`) {
		t.Fatalf("rows-per-page dropdown missing or match not kept: %d %s", code, body)
	}
}

// TestClientsAddFilterStepsChooseFieldThenCondition checks the two-step add-filter menu: the first
// step links each field, and the second step shows that field's conditions as radio buttons.
func TestClientsAddFilterStepsChooseFieldThenCondition(t *testing.T) {
	env := newClientsTestEnv(t)
	list := func(t *testing.T, query string) (int, string) {
		t.Helper()
		response := request(env.routes, http.MethodGet, env.listPath()+withAll(query), nil, env.viewerCookies, nil)
		return response.Code, response.Body.String()
	}
	code, body := list(t, "?filter=displayName:contains:Ada&sort=displayName")
	if code != http.StatusOK || !strings.Contains(body, `class="client-field-choices"`) ||
		!strings.Contains(body, `filter_field=displayName`) || !strings.Contains(body, `sort=displayName`) {
		t.Fatalf("step one lacks field links that keep the sort: %d %s", code, body)
	}
	code, body = list(t, "?filter=displayName:contains:Ada&filter_field=displayName")
	if code != http.StatusOK || strings.Contains(body, `class="client-field-choices"`) ||
		!strings.Contains(body, `type="radio" name="filter_op"`) || !strings.Contains(body, `name="filter_field" value="displayName"`) {
		t.Fatalf("step two lacks the condition radios: %d %s", code, body)
	}
	// Only fields with a range operator get the "to" value: text has none, a date has one.
	if _, body = list(t, "?filter=displayName:contains:Ada&filter_field=displayName"); strings.Contains(body, `filter_value_to`) {
		t.Fatalf("text field shows the range value: %s", body)
	}
	if _, body = list(t, "?filter=displayName:contains:Ada&filter_field=createdAt"); !strings.Contains(body, `filter_value_to`) {
		t.Fatalf("date field lacks the range value: %s", body)
	}
}

func TestClientsSearchWithoutMatchesShowsFilteredEmptyState(t *testing.T) {
	env := newClientsTestEnv(t)
	env.seedClient(t, "Ada")
	response := request(env.routes, http.MethodGet, env.listPath()+"?all=1&q=unmatched", nil, env.cookies, nil)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, "No clients match") || strings.Contains(body, "Add your first client") || !strings.Contains(body, `href="`+env.listPath()+`?all=1"`) {
		t.Fatalf("search empty state = %d: %s", response.Code, body)
	}
}

func TestClientsListShowsCreateFormOnlyToWriters(t *testing.T) {
	env := newClientsTestEnv(t)
	owner := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil)
	if owner.Code != http.StatusOK || !strings.Contains(owner.Body.String(), `href="`+env.listPath()+`/new"`) {
		t.Fatalf("owner list = %d, want a link to the create page: %s", owner.Code, owner.Body.String())
	}
	body := owner.Body.String()
	// A pristine list keeps the create action in the toolbar.
	emptyCard := strings.Index(body, `id="client-empty-state"`)
	createLink := strings.Index(body, `href="`+env.listPath()+`/new"`)
	if emptyCard < 0 || createLink < 0 || createLink > emptyCard {
		t.Fatalf("pristine list must link to the create page from its toolbar: %s", body)
	}
	if !strings.Contains(body, `data-client-dialog`) || !strings.Contains(body, `name="display_name"`) ||
		!strings.Contains(body, `filter_field=archiveStatus`) {
		t.Fatalf("pristine list lacks the create dialog or archive filter: %s", body)
	}

	// With a row present the toolbar retains both controls before the results region.
	env.seedClient(t, "Ada")
	body = request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil).Body.String()
	results := strings.Index(body, `id="client-results"`)
	createControls := strings.Index(body, `class="btn btn-primary" href="`+env.listPath()+`/new"`)
	filterControls := strings.Index(body, `aria-label="Filter"`)
	listResults := strings.Index(body, `id="list-results"`)
	if results < 0 || listResults <= results || createControls <= results || createControls >= listResults ||
		filterControls <= results || filterControls >= listResults {
		t.Fatalf("create control and filter menu must sit in the toolbar, before the list results: %s", body)
	}
	viewer := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.viewerCookies, nil)
	body = viewer.Body.String()
	if viewer.Code != http.StatusOK || strings.Contains(body, `href="`+env.listPath()+`/new"`) ||
		strings.Contains(body, `data-client-dialog`) {
		t.Fatalf("viewer list = %d, want read-only without create controls: %s", viewer.Code, body)
	}
}

// seedClient creates one client through the form so a list is no longer pristine-empty.
func (e clientsTestEnv) seedClient(t *testing.T, name string) {
	t.Helper()
	created := request(e.routes, http.MethodPost, e.listPath(), url.Values{"csrf": {e.csrf}, "display_name": {name}}, e.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("seed client = %d: %s", created.Code, created.Body.String())
	}
}

// pageSizeIsLive reports whether the page-size form swaps the results region in place.
func pageSizeIsLive(body string) bool {
	start := strings.Index(body, "list-page-size")
	if start < 0 {
		return false
	}
	end := strings.Index(body[start:], "</details>")
	return end >= 0 && strings.Contains(body[start:start+end], "hx-get=")
}

func TestClientsListTemplShellLoadsRealtimeAssetsAndKeepsNativeForms(t *testing.T) {
	env := newClientsTestEnv(t)
	seeded := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Contoso"}, "email": {"contoso@example.test"},
	}, env.cookies, nil)
	if seeded.Code != http.StatusSeeOther {
		t.Fatalf("seed client = %d", seeded.Code)
	}
	if err := env.auth.UpdateWorkspaceLocale(context.Background(), mustOwnerPrincipal(t, env), "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	page := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil)
	body := page.Body.String()
	htmx := strings.Index(body, `<script defer src="/assets/htmx.min.js">`)
	app := strings.Index(body, `<script defer src="/assets/alpine-components.js">`)
	if page.Code != http.StatusOK || page.Header().Get("Content-Language") != "es" ||
		!strings.Contains(body, `href="/assets/clients.css"`) || htmx < 0 || app < 0 ||
		!strings.Contains(body, `method="get"`) ||
		!strings.Contains(body, `method="post"`) || !pageSizeIsLive(body) || !strings.Contains(body, `hx-target="#client-create-content"`) {
		t.Fatalf("Clients page contract: status=%d locale=%q css=%t htmx=%d app=%d get=%t post=%t hxGet=%t",
			page.Code, page.Header().Get("Content-Language"), strings.Contains(body, `href="/assets/clients.css"`), htmx, app,
			strings.Contains(body, `method="get"`), strings.Contains(body, `method="post"`),
			strings.Contains(body, `hx-get=`))
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

func TestClientDetailUsesSharedTemplComponents(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada Lovelace"},
	}, env.cookies, nil)
	detail := request(env.routes, http.MethodGet, created.Header().Get("Location"), nil, env.cookies, nil)
	body := detail.Body.String()
	if detail.Code != http.StatusOK || !strings.Contains(body, `id="inline-display_name"`) ||
		!strings.Contains(body, `name="field" value="display_name"`) ||
		!strings.Contains(body, `name="field" value="notes"`) {
		t.Fatalf("client detail rendering or script boundary is incorrect: %s", body)
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
	if !strings.Contains(blank.Body.String(), `id="client-new-page"`) ||
		!strings.Contains(blank.Body.String(), `class="create" action="`+env.listPath()+`/new"`) {
		t.Fatalf("blank create must return the form on the create page: %s", blank.Body.String())
	}
	badEmail := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada"}, "email": {"Ada <ada@example.com>"},
	}, env.cookies, nil)
	if badEmail.Code != http.StatusUnprocessableEntity || !strings.Contains(badEmail.Body.String(), "The email address is invalid.") {
		t.Fatalf("bad email create = %d: %s", badEmail.Code, badEmail.Body.String())
	}
	if !strings.Contains(badEmail.Body.String(), `data-dirty-on-load`) || !strings.Contains(badEmail.Body.String(), `value="Ada &lt;ada@example.com&gt;"`) {
		t.Fatalf("invalid draft was not returned as dirty: %s", badEmail.Body.String())
	}
	viewerWrite := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.viewerCSRF}, "display_name": {"Blocked"},
	}, env.viewerCookies, nil)
	if viewerWrite.Code != http.StatusForbidden {
		t.Fatalf("viewer create = %d, want 403", viewerWrite.Code)
	}
}

func TestClientModalCreateRetainsDraftAndRedirects(t *testing.T) {
	env := newClientsTestEnv(t)
	page := request(env.routes, http.MethodGet, env.listPath(), nil, env.cookies, nil)
	if !strings.Contains(page.Body.String(), `data-client-dialog`) || !strings.Contains(page.Body.String(), `data-client-create`) {
		t.Fatal("clients page lacks the create dialog or its link")
	}
	viewer := request(env.routes, http.MethodGet, env.listPath(), nil, env.viewerCookies, nil)
	if strings.Contains(viewer.Body.String(), `data-client-dialog`) || strings.Contains(viewer.Body.String(), `data-client-create`) {
		t.Fatal("viewer can access client creation")
	}
	headers := map[string]string{"HX-Request": "true"}
	invalid := request(env.routes, http.MethodPost, env.listPath()+"/new", url.Values{
		"csrf": {env.csrf}, "client_modal": {"1"}, "display_name": {"Ada"},
		"email": {"Ada <ada@example.com>"}, "phone": {"123"}, "notes": {"Remember this"},
	}, env.cookies, headers)
	body := invalid.Body.String()
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "The email address is invalid.") ||
		!strings.Contains(body, `value="Ada &lt;ada@example.com&gt;"`) || !strings.Contains(body, `value="123"`) ||
		!strings.Contains(body, "Remember this") || strings.Contains(body, `id="client-new-page"`) {
		t.Fatalf("modal validation lost draft or error: %d %s", invalid.Code, body)
	}
	created := request(env.routes, http.MethodPost, env.listPath()+"/new", url.Values{
		"csrf": {env.csrf}, "client_modal": {"1"}, "display_name": {"Ada"},
	}, env.cookies, headers)
	location := created.Header().Get("X-Client-Location")
	if created.Code != http.StatusNoContent || !strings.HasPrefix(location, env.listPath()+"/") {
		t.Fatalf("modal create = %d, location %q", created.Code, location)
	}
	detail := request(env.routes, http.MethodGet, location, nil, env.cookies, nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "Ada") {
		t.Fatalf("created client detail = %d", detail.Code)
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
	response := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "<title>Clientes ·") {
		t.Fatalf("Spanish list = %d, want Spanish title: %s", response.Code, response.Body.String())
	}
	form := request(env.routes, http.MethodGet, env.listPath()+"/new", nil, env.cookies, nil)
	if form.Code != http.StatusOK || !strings.Contains(form.Body.String(), "Nombre para mostrar") ||
		!strings.Contains(form.Body.String(), "<title>Clientes ·") {
		t.Fatalf("Spanish create page = %d, want Spanish labels: %s", form.Code, form.Body.String())
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
		response := request(env.routes, http.MethodGet, env.listPath()+withAll(query), nil, env.viewerCookies, nil)
		return response.Code, response.Body.String()
	}
	code, body := list(t, "?sort=displayName&order=desc")
	if code != http.StatusOK || strings.Index(body, ">Cy<") > strings.Index(body, ">Bea<") ||
		strings.Index(body, ">Bea<") > strings.Index(body, ">Ada<") ||
		!strings.Contains(body, `aria-label="Filter"`) {
		t.Fatalf("sorted list = %d: %s", code, body)
	}
	code, body = list(t, "?filter_field=displayName&filter_op=contains&filter_value=A")
	if code != http.StatusOK || !strings.Contains(body, ">Ada<") || !strings.Contains(body, ">Bea<") ||
		strings.Contains(body, ">Cy<") || !strings.Contains(body, `aria-label="Filter"`) {
		t.Fatalf("filtered list = %d: %s", code, body)
	}
	// After a filter is applied the menu starts again at the field list, including archive status.
	if !strings.Contains(body, `class="client-field-choices"`) || !strings.Contains(body, `filter_field=archiveStatus`) {
		t.Fatalf("list lacks the field choices or archive filter: %s", body)
	}
	code, body = list(t, "?filter_field=displayName&filter_op=before&filter_value=x")
	if code != http.StatusBadRequest || !strings.Contains(body, "Choose a valid sort or filter.") {
		t.Fatalf("invalid filter = %d: %s", code, body)
	}
	// A page that is not a number shows the first page, so an old bookmark still opens.
	code, body = list(t, "?page=not-a-number")
	if code != http.StatusOK {
		t.Fatalf("invalid page = %d: %s", code, body)
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
	if code != http.StatusOK || !strings.Contains(body, `value="`+gold.ID.String()+`"`) ||
		!strings.Contains(body, "Gold") || !strings.Contains(body, `<details class="dropdown list-menu" open>`) {
		t.Fatalf("choice form = %d: %s", code, body)
	}
}

func TestClientsListUsesResponsiveTableAndSavedColumns(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada Lovelace"}, "email": {"ada@example.com"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create client = %d", created.Code)
	}
	response := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.viewerCookies, nil)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `class="list-view-table"`) ||
		!strings.Contains(body, `class="list-view-cards"`) ||
		!strings.Contains(body, `aria-label="Columns"`) ||
		!strings.Contains(body, `name="visible" value="1"`) {
		t.Fatalf("Clients list should render the responsive table and default column controls: %d %s", response.Code, body)
	}
	service := clients.NewService(env.db, config.SQLite, env.auth, 50)
	owner := mustOwnerPrincipal(t, env)
	field, err := service.DefineField(context.Background(), owner, env.workspaceID,
		clients.FieldInput{Label: "Account tier", Type: clients.FieldTypeText}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	clientID, err := clients.ParseClientID(strings.TrimPrefix(created.Header().Get("Location"), env.listPath()+"/"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := service.Get(context.Background(), owner, env.workspaceID, clientID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetValue(context.Background(), owner, env.workspaceID, clientID, client.Version, field.ID, "A-17", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	showCustom := request(env.routes, http.MethodPost, env.listPath()+"/preferences/columns/"+field.ID.String(), url.Values{
		"csrf": {env.viewerCSRF}, "visible": {"true"},
	}, env.viewerCookies, nil)
	if showCustom.Code != http.StatusSeeOther {
		t.Fatalf("show custom column = %d %s", showCustom.Code, showCustom.Body.String())
	}
	response = request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.viewerCookies, nil)
	if !strings.Contains(response.Body.String(), `data-column="`+field.ID.String()+`">A-17`) {
		t.Fatalf("visible custom field value missing from table: %s", response.Body.String())
	}
	hideName := request(env.routes, http.MethodPost, env.listPath()+"/preferences/columns/displayName", url.Values{
		"csrf": {env.viewerCSRF}, "visible": {"false"},
	}, env.viewerCookies, nil)
	if hideName.Code != http.StatusSeeOther {
		t.Fatalf("hide default column = %d %s", hideName.Code, hideName.Body.String())
	}
	response = request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.viewerCookies, nil)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `data-column="displayName"`) ||
		!strings.Contains(response.Body.String(), `class="list-row-link"`) ||
		strings.Contains(response.Body.String(), `>Ada Lovelace<`) {
		t.Fatalf("hidden column remains visible: %d %s", response.Code, response.Body.String())
	}
	ownerList := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil)
	if !strings.Contains(ownerList.Body.String(), `data-column="displayName"`) {
		t.Fatalf("viewer column choice leaked to workspace owner: %s", ownerList.Body.String())
	}
	reset := request(env.routes, http.MethodPost, env.listPath()+"/preferences/columns/reset", url.Values{
		"csrf": {env.viewerCSRF},
	}, env.viewerCookies, nil)
	if reset.Code != http.StatusSeeOther {
		t.Fatalf("reset columns = %d %s", reset.Code, reset.Body.String())
	}
	response = request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.viewerCookies, nil)
	if !strings.Contains(response.Body.String(), `data-column="displayName"`) {
		t.Fatalf("reset did not restore the default columns: %s", response.Body.String())
	}
}

func TestClientsListCombinesFiltersAndPreservesPageQuery(t *testing.T) {
	env := newClientsTestEnv(t)
	for _, name := range []string{"Ada", "Bea", "Cara"} {
		created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
			"csrf": {env.csrf}, "display_name": {name},
		}, env.cookies, nil)
		if created.Code != http.StatusSeeOther {
			t.Fatalf("create %s = %d", name, created.Code)
		}
	}
	response := request(env.routes, http.MethodGet,
		env.listPath()+"?all=1&filter=phone:is_empty:&filter=displayName:contains:a&sort=displayName&order=asc&limit=1",
		nil, env.viewerCookies, nil)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `>Ada<`) ||

		!strings.Contains(body, `filter=phone%3Ais_empty%3A`) {
		t.Fatalf("combined filtered page = %d: %s", response.Code, body)
	}
	cursorAt := strings.Index(body, "page=2")
	if cursorAt < 0 {
		t.Fatalf("filtered page lacks a link to page 2: %s", body)
	}
	hrefStart := strings.LastIndex(body[:cursorAt], `href="`)
	hrefEnd := strings.Index(body[cursorAt:], `"`)
	if hrefStart < 0 || hrefEnd < 0 {
		t.Fatalf("filtered page lacks a next-page link: %s", body)
	}
	link := body[hrefStart+len(`href="`) : cursorAt+hrefEnd]
	if !strings.Contains(link, "page=2") || !strings.Contains(link, "filter=") ||
		!strings.Contains(link, "sort=displayName") || !strings.Contains(link, "limit=1") {
		t.Fatalf("next link lost query state: %s", link)
	}
	// A field can carry more than one condition, so a repeated field is a valid query.
	if response := request(env.routes, http.MethodGet,
		env.listPath()+"?all=1&filter=displayName:contains:a&filter=displayName:contains:b", nil, env.viewerCookies, nil); response.Code != http.StatusOK {
		t.Fatalf("repeated filter field = %d, want 200", response.Code)
	}
}

// TestClientsListSameFieldConditionsAndConnectors checks two conditions on one field under both
// modes, the connector text between chips, the edit link, and the 20-filter limit.
func TestClientsListSameFieldConditionsAndConnectors(t *testing.T) {
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
		response := request(env.routes, http.MethodGet, env.listPath()+withAll(query), nil, env.viewerCookies, nil)
		return response.Code, response.Body.String()
	}
	// Names containing "a" and "d": only Ada. Containing "a" or "d": Ada and Bea.
	code, body := list(t, "?filter=displayName:contains:a&filter=displayName:contains:d&match=all")
	if code != http.StatusOK || !strings.Contains(body, ">Ada<") || strings.Contains(body, ">Bea<") {
		t.Fatalf("same-field all = %d, want only Ada: %s", code, body)
	}
	code, body = list(t, "?filter=displayName:contains:a&filter=displayName:contains:d&match=any")
	if code != http.StatusOK || !strings.Contains(body, ">Ada<") || !strings.Contains(body, ">Bea<") || strings.Contains(body, ">Cy<") {
		t.Fatalf("same-field any = %d, want Ada and Bea: %s", code, body)
	}
	if !strings.Contains(body, `class="list-filter-connector"`) || !strings.Contains(body, "or") {
		t.Fatalf("any mode lacks the or connector: %s", body)
	}
	// The edit link takes the condition out of the list and reopens its field, operator, and value.
	if !strings.Contains(body, `filter_field=displayName`) || !strings.Contains(body, `filter_value=d`) {
		t.Fatalf("edit link does not reopen the condition: %s", body)
	}
	// Twenty conditions reach the limit: the add menu shows the notice and no field list.
	var query strings.Builder
	query.WriteString("?")
	for i := 0; i < 20; i++ {
		query.WriteString("filter=displayName:contains:x" + strconv.Itoa(i) + "&")
	}
	code, body = list(t, query.String())
	if code != http.StatusOK || !strings.Contains(body, "up to 20 filters") || strings.Contains(body, `class="client-field-choices"`) {
		t.Fatalf("limit notice missing at 20 filters = %d: %s", code, body)
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
	if !strings.Contains(refused.Body.String(), `class="dashboard-workspace"`) || !strings.Contains(refused.Body.String(), `class="input w-full"`) {
		t.Fatalf("detail response did not use the templ dashboard and shared input: %s", refused.Body.String())
	}
}
