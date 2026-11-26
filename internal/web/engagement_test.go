package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestWorkspaceCreateEngagementNavigation(t *testing.T) {
	env := newClientsTestEnv(t)
	home := "/workspaces/" + env.workspaceID.String()
	clientsPath := home + "/clients"
	for _, tc := range []struct {
		name    string
		cookies []*http.Cookie
		writer  bool
	}{
		{"writer", env.cookies, true},
		{"viewer", env.viewerCookies, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := request(env.routes, http.MethodGet, home, nil, tc.cookies, nil)
			link := `href="` + clientsPath + `"`
			if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), link) || strings.Contains(page.Body.String(), "Create engagement") != tc.writer {
				t.Fatalf("home: status=%d writer=%v", page.Code, tc.writer)
			}
			list := request(env.routes, http.MethodGet, home+"/engagements", nil, tc.cookies, nil)
			if strings.Contains(list.Body.String(), `href="`+home+`/engagements/new" hx-get="`+home+`/engagements/new"`) != tc.writer {
				t.Fatalf("engagement list create link: status=%d writer=%v", list.Code, tc.writer)
			}
			page = request(env.routes, http.MethodGet, clientsPath, nil, tc.cookies, nil)
			if page.Code != http.StatusOK {
				t.Fatalf("clients: status=%d writer=%v", page.Code, tc.writer)
			}
		})
	}
	principal, err := env.auth.Authorize(context.Background(), env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.auth.UpdateWorkspaceLocale(context.Background(), principal, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	page := request(env.routes, http.MethodGet, home, nil, env.cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Crear gestión") {
		t.Fatalf("Spanish home: status=%d", page.Code)
	}
	page = request(env.routes, http.MethodGet, clientsPath, nil, env.cookies, nil)
	if page.Code != http.StatusOK {
		t.Fatalf("Spanish clients: status=%d", page.Code)
	}
}

func TestEngagementLabelThroughWeb(t *testing.T) {
	env := newClientsTestEnv(t)
	path := "/workspaces/" + env.workspaceID.String() + "/engagement-stages"
	r := request(env.routes, http.MethodPost, path+"/label", url.Values{"csrf": {env.csrf}, "label": {"Matter"}}, env.cookies, nil)
	if r.Code != http.StatusSeeOther {
		t.Fatalf("set: %d %s", r.Code, r.Body.String())
	}
	r = request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), "Matter") {
		t.Fatalf("view: %d %s", r.Code, r.Body.String())
	}
	r = request(env.routes, http.MethodGet, "/workspaces/"+env.workspaceID.String()+"/clients", nil, env.viewerCookies, nil)
	if r.Code != http.StatusOK || strings.Contains(r.Body.String(), `href="`+path+`"`) {
		t.Fatalf("clients stage settings: %d %s", r.Code, r.Body.String())
	}
	for _, suffix := range []string{"", "/engagements", "/clients", "/client-fields"} {
		page := request(env.routes, http.MethodGet, "/workspaces/"+env.workspaceID.String()+suffix, nil, env.viewerCookies, nil)
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `href="/workspaces/`+env.workspaceID.String()+`/engagements">Matter</a>`) {
			t.Fatalf("custom sidebar on %s: %d", suffix, page.Code)
		}
	}
	owner, err := env.auth.Authorize(context.Background(), env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	other, err := env.auth.CreateWorkspace(context.Background(), owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherPage := request(env.routes, http.MethodGet, "/workspaces/"+other.String(), nil, env.cookies, nil)
	if strings.Contains(otherPage.Body.String(), ">Matter</a>") || !strings.Contains(otherPage.Body.String(), ">Engagements</a>") {
		t.Fatalf("other workspace label: %d", otherPage.Code)
	}
	if err := env.auth.UpdateWorkspaceLocale(context.Background(), owner, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	spanish := request(env.routes, http.MethodGet, "/workspaces/"+env.workspaceID.String()+"/engagements", nil, env.viewerCookies, nil)
	if spanish.Code != http.StatusOK || !strings.Contains(spanish.Body.String(), ">Matter</a>") {
		t.Fatalf("Spanish custom label: %d", spanish.Code)
	}
	r = request(env.routes, http.MethodPost, path+"/label", url.Values{"csrf": {env.viewerCSRF}, "label": {"Job"}}, env.viewerCookies, nil)
	if r.Code != http.StatusForbidden {
		t.Fatalf("viewer: %d", r.Code)
	}
	r = request(env.routes, http.MethodPost, path+"/label", url.Values{"csrf": {env.csrf}, "label": {""}}, env.cookies, nil)
	if r.Code != http.StatusSeeOther {
		t.Fatalf("reset: %d", r.Code)
	}
	r = request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if !strings.Contains(r.Body.String(), "Etapas de gestión") {
		t.Fatalf("default: %s", r.Body.String())
	}
	spanish = request(env.routes, http.MethodGet, "/workspaces/"+env.workspaceID.String()+"/engagements", nil, env.viewerCookies, nil)
	if !strings.Contains(spanish.Body.String(), ">Gestiones</a>") {
		t.Fatalf("Spanish default: %d", spanish.Code)
	}
}

func TestEngagementNewPage(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	owner, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, owner, env.workspaceID, clients.Input{DisplayName: "Example client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	stage, err := engagement.NewService(env.db, config.SQLite, env.auth).CreateStage(ctx, owner, env.workspaceID, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	path := "/workspaces/" + env.workspaceID.String() + "/engagements/new"
	modal := request(env.routes, http.MethodGet, path, nil, env.cookies, map[string]string{"HX-Request": "true"})
	if modal.Code != http.StatusOK || !strings.Contains(modal.Body.String(), `data-engagement-dialog`) || strings.Contains(modal.Body.String(), "<!doctype html>") {
		t.Fatalf("modal: %d", modal.Code)
	}
	picker := request(env.routes, http.MethodGet, path+"?q=Example", nil, env.cookies, nil)
	if picker.Code != http.StatusOK || !strings.Contains(picker.Body.String(), "Example client") || !strings.Contains(picker.Body.String(), `value="select:`+client.ID.String()+`"`) {
		t.Fatalf("picker: %d %s", picker.Code, picker.Body.String())
	}
	form := request(env.routes, http.MethodGet, path+"?client="+client.ID.String(), nil, env.cookies, nil)
	if form.Code != http.StatusOK || !strings.Contains(form.Body.String(), `name="client" value="`+client.ID.String()+`"`) || !strings.Contains(form.Body.String(), stage.ID.String()) {
		t.Fatalf("form: %d %s", form.Code, form.Body.String())
	}
	draft := url.Values{"csrf": {env.csrf}, "client": {client.ID.String()}, "name": {""}, "stage": {stage.ID.String()}, "notes": {"Draft notes"}}
	invalid := request(env.routes, http.MethodPost, path, draft, env.cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "Draft notes") || !strings.Contains(invalid.Body.String(), "Enter a valid name") {
		t.Fatalf("invalid: %d %s", invalid.Code, invalid.Body.String())
	}
	draft.Set("name", "New matter")
	created := request(env.routes, http.MethodPost, path, draft, env.cookies, nil)
	if created.Code != http.StatusSeeOther || !strings.Contains(created.Header().Get("Location"), "/clients/"+client.ID.String()+"/engagements/") {
		t.Fatalf("create: %d %s", created.Code, created.Header().Get("Location"))
	}
	detail := request(env.routes, http.MethodGet, created.Header().Get("Location"), nil, env.cookies, nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "New matter") || !strings.Contains(detail.Body.String(), "Draft notes") {
		t.Fatalf("detail: %d", detail.Code)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		denied := request(env.routes, method, path, url.Values{"csrf": {env.viewerCSRF}}, env.viewerCookies, nil)
		if denied.Code != http.StatusForbidden {
			t.Fatalf("viewer %s: %d", method, denied.Code)
		}
	}
}

func TestStageDraftRecoveryAndAuthorization(t *testing.T) {
	env := newClientsTestEnv(t)
	path := "/workspaces/" + env.workspaceID.String() + "/engagement-stages"
	name := strings.Repeat("N", 201)
	invalid := url.Values{"csrf": {env.csrf}, "name": {name}, "kind": {"won"}}
	page := request(env.routes, http.MethodPost, path, invalid, env.cookies, nil)
	if page.Code != http.StatusUnprocessableEntity || !strings.Contains(page.Body.String(), `value="`+name+`"`) || !strings.Contains(page.Body.String(), `value="won" selected`) {
		t.Fatalf("stage draft: %d %s", page.Code, page.Body.String())
	}
	denied := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.viewerCSRF}, "name": {name}, "kind": {"won"}}, env.viewerCookies, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("viewer invalid stage: %d", denied.Code)
	}
	created := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "name": {"New stage"}, "kind": {"won"}}, env.cookies, nil)
	if created.Code != http.StatusSeeOther || created.Header().Get("Location") != path {
		t.Fatalf("create: %d location %q", created.Code, created.Header().Get("Location"))
	}
	page = request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "New stage") || strings.Contains(page.Body.String(), name) {
		t.Fatalf("saved stage: %d %s", page.Code, page.Body.String())
	}
}

func TestEngagementLabelDraftRecoveryAndAuthorization(t *testing.T) {
	env := newClientsTestEnv(t)
	path := "/workspaces/" + env.workspaceID.String() + "/engagement-stages"
	saved := request(env.routes, http.MethodPost, path+"/label", url.Values{"csrf": {env.csrf}, "label": {"Matter"}}, env.cookies, nil)
	if saved.Code != http.StatusSeeOther {
		t.Fatalf("save label: %d", saved.Code)
	}
	label := strings.Repeat("L", 101)
	page := request(env.routes, http.MethodPost, path+"/label", url.Values{"csrf": {env.csrf}, "label": {label}}, env.cookies, nil)
	if page.Code != http.StatusUnprocessableEntity || !strings.Contains(page.Body.String(), `value="`+label+`"`) || !strings.Contains(page.Body.String(), "Matter") {
		t.Fatalf("label draft: %d %s", page.Code, page.Body.String())
	}
	denied := request(env.routes, http.MethodPost, path+"/label", url.Values{"csrf": {env.viewerCSRF}, "label": {label}}, env.viewerCookies, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("viewer invalid label: %d", denied.Code)
	}
	page = request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Matter") || strings.Contains(page.Body.String(), label) {
		t.Fatalf("saved label: %d %s", page.Code, page.Body.String())
	}
}

func TestStageNameExamples(t *testing.T) {
	env := newClientsTestEnv(t)
	path := "/workspaces/" + env.workspaceID.String() + "/engagement-stages"
	owner, err := env.auth.Authorize(context.Background(), env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		locale string
		want   []string
	}{
		{"en", []string{"Choose an example or enter your own stage name.", `value="Discovery"`, `value="Proposal"`, `value="Delivery"`}},
		{"es", []string{"Elige un ejemplo o introduce tu propio nombre de etapa.", `value="Descubrimiento"`, `value="Propuesta"`, `value="Ejecución"`}},
	} {
		if err := env.auth.UpdateWorkspaceLocale(context.Background(), owner, tc.locale, identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
		page := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
		for _, want := range append(tc.want, `list="stage-name-examples"`, `aria-describedby="stage-name-hint"`, "<datalist") {
			if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), want) {
				t.Fatalf("%s page missing %q: %d", tc.locale, want, page.Code)
			}
		}
	}
	stages, err := engagement.NewService(env.db, config.SQLite, env.auth).ListStages(context.Background(), owner, env.workspaceID)
	if err != nil || len(stages) != 0 {
		t.Fatalf("examples created stages: %v, %v", stages, err)
	}
	created := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "name": {"Custom discovery"}, "kind": {"open"}}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create edited example: %d", created.Code)
	}
	stages, err = engagement.NewService(env.db, config.SQLite, env.auth).ListStages(context.Background(), owner, env.workspaceID)
	if err != nil || len(stages) != 1 || stages[0].Name != "Custom discovery" {
		t.Fatalf("saved stages: %v, %v", stages, err)
	}
}

func TestStageSettingsOrderAndBreadcrumb(t *testing.T) {
	env := newClientsTestEnv(t)
	base := "/workspaces/" + env.workspaceID.String()
	path := base + "/engagement-stages"
	for _, stage := range []struct{ name, kind string }{
		{"Lost prospect", "lost"}, {"Active work", "open"}, {"Won project", "won"}, {"Another open", "open"},
	} {
		response := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "name": {stage.name}, "kind": {stage.kind}}, env.cookies, nil)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("create %s: %d", stage.name, response.Code)
		}
	}
	page := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `href="`+base+`/engagements">Engagements</a>`) || !strings.Contains(body, `aria-current="page"`) || !strings.Contains(body, `>Stages</span>`) {
		t.Fatalf("breadcrumb: %d", page.Code)
	}
	previous := -1
	for _, text := range []string{"Lost prospect", "Active work", "Won project", "Another open"} {
		index := strings.Index(body, text)
		if index <= previous {
			t.Fatalf("missing or unordered %q", text)
		}
		previous = index
	}
	viewer := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if strings.Contains(viewer.Body.String(), `name="direction"`) || strings.Contains(viewer.Body.String(), `name="confirm_archive"`) {
		t.Fatal("viewer sees stage management actions")
	}
}

func TestStageFiltersPreserveVisibleReordering(t *testing.T) {
	env := newClientsTestEnv(t)
	path := "/workspaces/" + env.workspaceID.String() + "/engagement-stages"
	for _, name := range []string{"First stage", "Archived stage", "Last stage"} {
		response := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "name": {name}, "kind": {"open"}}, env.cookies, nil)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("create %s: %d", name, response.Code)
		}
	}
	owner, err := env.auth.Authorize(context.Background(), env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	service := engagement.NewService(env.db, config.SQLite, env.auth)
	stages, err := service.ListStages(context.Background(), owner, env.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	response := request(env.routes, http.MethodPost, path+"/"+stages[1].ID.String()+"/archive", url.Values{"csrf": {env.csrf}, "confirm_archive": {"yes"}}, env.cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("archive: %d", response.Code)
	}
	response = request(env.routes, http.MethodPost, path+"/reorder", url.Values{"csrf": {env.csrf}, "stage": {stages[2].ID.String()}, "direction": {"up"}}, env.cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("reorder: %d", response.Code)
	}
	page := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	body := page.Body.String()
	first, last := strings.Index(body, "First stage"), strings.Index(body, "Last stage")
	if page.Code != http.StatusOK || last < 0 || first < last || strings.Contains(body, "Archived stage") {
		t.Fatalf("active order: %d %s", page.Code, body)
	}
	if err := env.auth.UpdateWorkspaceLocale(context.Background(), owner, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	page = request(env.routes, http.MethodGet, path+"?status=archived", nil, env.viewerCookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Archived stage") || !strings.Contains(page.Body.String(), "Filtrar") || !strings.Contains(page.Body.String(), "Archivada") {
		t.Fatalf("Spanish archived page: %d %s", page.Code, page.Body.String())
	}
}

func TestStagesThroughWeb(t *testing.T) {
	env := newClientsTestEnv(t)
	path := "/workspaces/" + env.workspaceID.String() + "/engagement-stages"
	initial := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	if initial.Code != http.StatusOK || !strings.Contains(initial.Body.String(), "No stages") {
		t.Fatalf("initial = %d %s", initial.Code, initial.Body.String())
	}
	created := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "name": {"Prospect"}, "kind": {"open"}}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	created = request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "name": {"Completed project"}, "kind": {"won"}}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create second = %d", created.Code)
	}
	owner, err := env.auth.Authorize(context.Background(), env.ownerID, env.workspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	stages, err := engagement.NewService(env.db, config.SQLite, env.auth).ListStages(context.Background(), owner, env.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	reordered := request(env.routes, http.MethodPost, path+"/reorder", url.Values{"csrf": {env.csrf}, "stage": {stages[1].ID.String()}, "direction": {"up"}}, env.cookies, nil)
	if reordered.Code != http.StatusSeeOther {
		t.Fatalf("reorder = %d", reordered.Code)
	}
	unconfirmed := request(env.routes, http.MethodPost, path+"/"+stages[0].ID.String()+"/archive", url.Values{"csrf": {env.csrf}}, env.cookies, nil)
	if unconfirmed.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unconfirmed archive = %d", unconfirmed.Code)
	}
	stagesAfter, err := engagement.NewService(env.db, config.SQLite, env.auth).ListStages(context.Background(), owner, env.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range stagesAfter {
		if stage.ID == stages[0].ID && stage.Archived {
			t.Fatal("unconfirmed archive changed stage")
		}
	}
	archived := request(env.routes, http.MethodPost, path+"/"+stages[0].ID.String()+"/archive", url.Values{"csrf": {env.csrf}, "confirm_archive": {"yes"}}, env.cookies, nil)
	if archived.Code != http.StatusSeeOther {
		t.Fatalf("archive = %d", archived.Code)
	}
	deniedUnconfirmed := request(env.routes, http.MethodPost, path+"/"+stages[1].ID.String()+"/archive", url.Values{"csrf": {env.viewerCSRF}}, env.viewerCookies, nil)
	if deniedUnconfirmed.Code != http.StatusForbidden {
		t.Fatalf("viewer unconfirmed archive = %d", deniedUnconfirmed.Code)
	}
	deniedArchive := request(env.routes, http.MethodPost, path+"/"+stages[1].ID.String()+"/archive", url.Values{"csrf": {env.viewerCSRF}, "confirm_archive": {"yes"}}, env.viewerCookies, nil)
	if deniedArchive.Code != http.StatusForbidden {
		t.Fatalf("viewer archive = %d", deniedArchive.Code)
	}
	deniedReorder := request(env.routes, http.MethodPost, path+"/reorder", url.Values{"csrf": {env.viewerCSRF}, "stage": {stages[1].ID.String()}, "direction": {"down"}}, env.viewerCookies, nil)
	if deniedReorder.Code != http.StatusForbidden {
		t.Fatalf("viewer reorder = %d", deniedReorder.Code)
	}
	for _, tc := range []struct {
		query, present, absent string
	}{
		{"", "Completed project", "Prospect"},
		{"?status=archived", "Prospect", "Completed project"},
		{"?status=unexpected", "Completed project", "Prospect"},
	} {
		for _, cookies := range [][]*http.Cookie{env.cookies, env.viewerCookies} {
			list := request(env.routes, http.MethodGet, path+tc.query, nil, cookies, nil)
			if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), tc.present) || strings.Contains(list.Body.String(), tc.absent) || !strings.Contains(list.Body.String(), `name="status"`) {
				t.Fatalf("list %s = %d %s", tc.query, list.Code, list.Body.String())
			}
		}
	}
	denied := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.viewerCSRF}, "name": {"Blocked"}, "kind": {"won"}}, env.viewerCookies, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("viewer create = %d", denied.Code)
	}
}

func TestEngagementThroughWeb(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	owner, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, owner, env.workspaceID, clients.Input{DisplayName: "Client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	path := "/workspaces/" + env.workspaceID.String() + "/clients/" + client.ID.String()
	page := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	if !strings.Contains(page.Body.String(), "no open stage") || !strings.Contains(page.Body.String(), "/engagement-stages") {
		t.Fatalf("missing setup guidance: %s", page.Body.String())
	}
	noStage := request(env.routes, http.MethodPost, path+"/engagements", url.Values{"csrf": {env.csrf}, "name": {"Engagement"}}, env.cookies, nil)
	if noStage.Code != http.StatusUnprocessableEntity || !strings.Contains(noStage.Body.String(), "no open stage") {
		t.Fatalf("no stage: %d %s", noStage.Code, noStage.Body.String())
	}
	stage, err := engagement.NewService(env.db, config.SQLite, env.auth).CreateStage(ctx, owner, env.workspaceID, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := engagement.NewService(env.db, config.SQLite, env.auth).CreateStage(ctx, owner, env.workspaceID, engagement.StageInput{Name: "Second", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	page = request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	createPath := "/workspaces/" + env.workspaceID.String() + "/engagements/new?client=" + client.ID.String() + "&from_client=" + client.ID.String()
	if !strings.Contains(page.Body.String(), `data-engagement-create`) || !strings.Contains(page.Body.String(), `hx-target="#engagement-create-modal"`) ||
		!strings.Contains(page.Body.String(), `/assets/htmx.min.js`) || strings.Contains(page.Body.String(), `id="engagement-name"`) {
		t.Fatalf("client page must link to modal instead of showing inline form: %s", page.Body.String())
	}
	form := request(env.routes, http.MethodGet, createPath, nil, env.cookies, nil)
	if form.Code != http.StatusOK || !strings.Contains(form.Body.String(), `name="client" value="`+client.ID.String()+`"`) ||
		!strings.Contains(form.Body.String(), `name="from_client" value="`+client.ID.String()+`"`) || !strings.Contains(form.Body.String(), stage.ID.String()) || !strings.Contains(form.Body.String(), second.ID.String()) {
		t.Fatalf("preselected create form = %d %s", form.Code, form.Body.String())
	}
	modal := request(env.routes, http.MethodGet, createPath, nil, env.cookies, map[string]string{"HX-Request": "true"})
	if modal.Code != http.StatusOK || !strings.Contains(modal.Body.String(), `data-engagement-dialog`) || !strings.Contains(modal.Body.String(), `name="client" value="`+client.ID.String()+`"`) {
		t.Fatalf("preselected create modal = %d %s", modal.Code, modal.Body.String())
	}
	invalidModalDraft := request(env.routes, http.MethodPost, "/workspaces/"+env.workspaceID.String()+"/engagements/new", url.Values{
		"csrf": {env.csrf}, "client": {client.ID.String()}, "from_client": {client.ID.String()},
		"name": {""}, "stage": {second.ID.String()}, "notes": {"Preserve draft"},
	}, env.cookies, nil)
	if invalidModalDraft.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidModalDraft.Body.String(), "Preserve draft") ||
		!strings.Contains(invalidModalDraft.Body.String(), `name="from_client" value="`+client.ID.String()+`"`) ||
		!strings.Contains(invalidModalDraft.Body.String(), `href="`+path+`"`) {
		t.Fatalf("preselected create validation = %d %s", invalidModalDraft.Code, invalidModalDraft.Body.String())
	}
	invalidName := strings.Repeat("x", 201)
	invalidCreate := request(env.routes, http.MethodPost, path+"/engagements", url.Values{"csrf": {env.csrf}, "name": {invalidName}, "stage": {second.ID.String()}, "notes": {"Keep these notes"}}, env.cookies, nil)
	for _, want := range []string{"Enter a valid name of up to 200 characters.", `aria-describedby="engagement-name-error"`, invalidName, "Keep these notes", `value="` + second.ID.String() + `" selected`} {
		if invalidCreate.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidCreate.Body.String(), want) {
			t.Fatalf("invalid create missing %q: %d %s", want, invalidCreate.Code, invalidCreate.Body.String())
		}
	}
	for _, stageValue := range []string{"", "invalid"} {
		invalidStage := request(env.routes, http.MethodPost, path+"/engagements", url.Values{"csrf": {env.csrf}, "name": {"Keep name"}, "stage": {stageValue}, "notes": {"Keep notes"}}, env.cookies, nil)
		for _, want := range []string{"Select a valid open stage.", `aria-describedby="engagement-stage-error"`, `value="Keep name"`, "Keep notes"} {
			if invalidStage.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidStage.Body.String(), want) {
				t.Fatalf("invalid stage %q missing %q: %d", stageValue, want, invalidStage.Code)
			}
		}
	}
	created := request(env.routes, http.MethodPost, path+"/engagements", url.Values{"csrf": {env.csrf}, "name": {"Engagement"}, "stage": {second.ID.String()}, "notes": {"Other person"}}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var savedStage engagement.StageID
	if err := env.db.QueryRowContext(ctx, `SELECT stage_id FROM engagements WHERE workspace_id = ?`, env.workspaceID).Scan(&savedStage); err != nil || savedStage != second.ID {
		t.Fatalf("selected stage = %v, %v", savedStage, err)
	}
	svc := engagement.NewService(env.db, config.SQLite, env.auth)
	items, err := svc.ListForClient(ctx, owner, env.workspaceID, client.ID)
	if err != nil || len(items) != 1 || items[0].Name != "Engagement" {
		t.Fatalf("engagements: %+v %v", items, err)
	}
	clientTiles := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	itemDetail := path + "/engagements/" + items[0].ID.String()
	if clientTiles.Code != http.StatusOK || !strings.Contains(clientTiles.Body.String(), `href="`+itemDetail+`"`) ||
		!strings.Contains(clientTiles.Body.String(), "Second") || strings.Contains(clientTiles.Body.String(), `action="`+itemDetail+`/stage"`) ||
		strings.Contains(clientTiles.Body.String(), `action="`+itemDetail+`/quote"`) {
		t.Fatalf("client engagement tiles: %d %s", clientTiles.Code, clientTiles.Body.String())
	}
	openStage := items[0].StageID
	actionPath := path + "/engagements/" + items[0].ID.String() + "/next-action"
	quotePath := path + "/engagements/" + items[0].ID.String() + "/quote"
	assigneePath := path + "/engagements/" + items[0].ID.String() + "/assignee"
	agreementPath := path + "/engagements/" + items[0].ID.String() + "/agreement"
	otherClient, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, owner, env.workspaceID, clients.Input{DisplayName: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	detailPath := path + "/engagements/" + items[0].ID.String()
	namePath := detailPath + "/name"
	writerDetail := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil)
	if strings.Contains(writerDetail.Body.String(), "engagement.stage") || !strings.Contains(writerDetail.Body.String(), `for="engagement-stage"`) ||
		!strings.Contains(writerDetail.Body.String(), `action="`+detailPath+`/notes"`) ||
		!strings.Contains(writerDetail.Body.String(), `name="notes_revision" value="`+engagement.NotesRevision(items[0].Notes)+`"`) ||
		!strings.Contains(writerDetail.Body.String(), `action="`+detailPath+`/stage"`) {
		t.Fatal("detail summary or editable forms missing")
	}
	for _, field := range []struct {
		path, label, selected, save string
	}{
		{detailPath + "/stage", `for="engagement-stage"`, `value="` + openStage.String() + `" selected`, "Save"},
		{assigneePath, `for="engagement-assignee"`, `value=""`, "Save assignee"},
	} {
		formStart := strings.Index(writerDetail.Body.String(), `action="`+field.path+`"`)
		if formStart < 0 {
			t.Fatalf("selector form %s missing", field.path)
		}
		sectionStart := strings.LastIndex(writerDetail.Body.String()[:formStart], "<section")
		if sectionStart < 0 {
			t.Fatalf("selector section %s missing", field.path)
		}
		sectionEnd := strings.Index(writerDetail.Body.String()[sectionStart:], "</section>")
		if sectionEnd < 0 {
			t.Fatalf("selector section %s is unclosed", field.path)
		}
		section := writerDetail.Body.String()[sectionStart : sectionStart+sectionEnd]
		for _, want := range []string{`method="post"`, `name="csrf" value="` + env.csrf + `"`, field.label, field.selected, `type="submit"`, field.save, "data-unsaved-warning", "data-confirm-select", "data-confirm-select-dialog", "Confirm change", "data-confirm-select-value"} {
			if !strings.Contains(section, want) {
				t.Fatalf("selector %s missing %q", field.path, want)
			}
		}
		if strings.Contains(section, "<details") || strings.Contains(section, "<summary") {
			t.Fatalf("selector %s is hidden behind disclosure", field.path)
		}
	}
	for _, want := range []string{
		`<nav aria-label="Clients">`,
		`href="/workspaces/` + env.workspaceID.String() + `/clients">Clients</a>`,
		`href="` + path + `">Client</a>`,
		`aria-current="page" class="break-words font-semibold">Engagement</span>`,
		`>Engagement</h1>`,
	} {
		if !strings.Contains(writerDetail.Body.String(), want) {
			t.Fatalf("detail breadcrumb missing %q", want)
		}
	}
	if strings.Contains(writerDetail.Body.String(), "Back to client: Client") {
		t.Fatal("detail has a duplicate back link")
	}
	if strings.Contains(writerDetail.Body.String(), `<p class="eyebrow">Client</p>`) {
		t.Fatal("detail repeats client name as an eyebrow")
	}
	viewerDetail := request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil)
	if !strings.Contains(writerDetail.Body.String(), `for="engagement-assignee"`) || !strings.Contains(writerDetail.Body.String(), ">Unassigned</option>") ||
		!strings.Contains(writerDetail.Body.String(), "Next action: No next action") {
		t.Fatal("writer unassigned detail missing")
	}
	for _, want := range []string{"Assignee: Unassigned", "Next action: No next action"} {
		if !strings.Contains(viewerDetail.Body.String(), want) {
			t.Fatalf("unassigned detail missing %q", want)
		}
	}
	if !strings.Contains(writerDetail.Body.String(), namePath) || strings.Contains(viewerDetail.Body.String(), namePath) {
		t.Fatalf("name form visibility: writer=%d viewer=%d", writerDetail.Code, viewerDetail.Code)
	}
	notesPath := detailPath + "/notes"
	if !strings.Contains(writerDetail.Body.String(), notesPath) || strings.Contains(viewerDetail.Body.String(), notesPath) {
		t.Fatal("notes form must be available only to writers")
	}
	if got := request(env.routes, http.MethodPost, notesPath, url.Values{"csrf": {env.viewerCSRF}, "notes": {"Denied"}}, env.viewerCookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("viewer notes edit: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, notesPath, url.Values{"notes": {"Denied"}}, env.cookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("notes edit without CSRF: %d", got.Code)
	}
	foreignNotesPath := "/workspaces/" + env.workspaceID.String() + "/clients/" + otherClient.ID.String() + "/engagements/" + items[0].ID.String() + "/notes"
	if got := request(env.routes, http.MethodPost, foreignNotesPath, url.Values{"csrf": {env.csrf}, "notes": {"Denied"}}, env.cookies, nil); got.Code != http.StatusNotFound {
		t.Fatalf("notes edit with wrong client: %d", got.Code)
	}
	notes := "## Plan\n\n**Important** and [safe](https://example.com)\n\n<script>alert(1)</script>"
	if got := request(env.routes, http.MethodPost, notesPath, url.Values{"csrf": {env.csrf}, "notes": {notes}}, env.cookies, nil); got.Code != http.StatusSeeOther {
		t.Fatalf("notes edit: %d", got.Code)
	}
	if got, err := svc.Get(ctx, owner, env.workspaceID, items[0].ID); err != nil || got.Notes != notes {
		t.Fatalf("notes persisted: %+v %v", got, err)
	}
	staleDraft := "Unsaved draft from another editor"
	conflict := request(env.routes, http.MethodPost, notesPath, url.Values{
		"csrf": {env.csrf}, "notes": {staleDraft}, "notes_revision": {engagement.NotesRevision("")},
	}, env.cookies, nil)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), staleDraft) ||
		!strings.Contains(conflict.Body.String(), "Important") ||
		!strings.Contains(conflict.Body.String(), `name="notes_revision" value="`+engagement.NotesRevision(notes)+`"`) ||
		!strings.Contains(conflict.Body.String(), `data-notes-conflict`) || !strings.Contains(conflict.Body.String(), "Retry save") {
		t.Fatalf("stale notes conflict: status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	if got, err := svc.Get(ctx, owner, env.workspaceID, items[0].ID); err != nil || got.Notes != notes {
		t.Fatalf("stale notes changed persisted value: %+v %v", got, err)
	}
	retry := request(env.routes, http.MethodPost, notesPath, url.Values{
		"csrf": {env.csrf}, "notes": {staleDraft}, "notes_revision": {engagement.NotesRevision(notes)},
	}, env.cookies, nil)
	if retry.Code != http.StatusSeeOther {
		t.Fatalf("notes retry: %d", retry.Code)
	}
	if got, err := svc.Get(ctx, owner, env.workspaceID, items[0].ID); err != nil || got.Notes != staleDraft {
		t.Fatalf("notes retry persisted value: %+v %v", got, err)
	}
	if got := request(env.routes, http.MethodPost, notesPath, url.Values{"csrf": {env.csrf}, "notes": {notes}}, env.cookies, nil); got.Code != http.StatusSeeOther {
		t.Fatalf("legacy notes edit: %d", got.Code)
	}
	detail := request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil)
	if !strings.Contains(detail.Body.String(), "<strong>Important</strong>") || strings.Contains(detail.Body.String(), "<script>alert(1)</script>") {
		t.Fatal("detail must render Markdown without active HTML")
	}
	tooLong := strings.Repeat("n", 5001)
	if got := request(env.routes, http.MethodPost, notesPath, url.Values{"csrf": {env.csrf}, "notes": {tooLong}, "notes_revision": {engagement.NotesRevision(notes)}}, env.cookies, nil); got.Code != http.StatusUnprocessableEntity || !strings.Contains(got.Body.String(), tooLong) ||
		!strings.Contains(got.Body.String(), `name="notes_revision" value="`+engagement.NotesRevision(notes)+`"`) {
		t.Fatalf("invalid notes must retain draft and revision: %d", got.Code)
	}
	previewPath := "/workspaces/" + env.workspaceID.String() + "/engagements/notes-preview"
	preview := request(env.routes, http.MethodPost, previewPath, url.Values{"csrf": {env.csrf}, "notes": {notes}}, env.cookies, nil)
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), "<strong>Important</strong>") || strings.Contains(preview.Body.String(), "<script>alert(1)</script>") {
		t.Fatalf("unsafe or invalid preview: %d %s", preview.Code, preview.Body.String())
	}
	if got := request(env.routes, http.MethodPost, previewPath, url.Values{"csrf": {env.viewerCSRF}, "notes": {notes}}, env.viewerCookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("viewer preview: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, notesPath, url.Values{"csrf": {env.csrf}, "notes": {"Other person"}}, env.cookies, nil); got.Code != http.StatusSeeOther {
		t.Fatalf("restore notes: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, namePath, url.Values{"csrf": {env.viewerCSRF}, "name": {"Denied"}}, env.viewerCookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("viewer rename: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, namePath, url.Values{"csrf": {env.csrf}, "name": {invalidName}}, env.cookies, nil); got.Code != http.StatusUnprocessableEntity || !strings.Contains(got.Body.String(), invalidName) || !strings.Contains(got.Body.String(), `data-inline-dirty-on-load open`) {
		t.Fatalf("invalid rename: %d %s", got.Code, got.Body.String())
	}
	if got := request(env.routes, http.MethodPost, namePath, url.Values{"csrf": {env.csrf}, "name": {"Renamed"}}, env.cookies, nil); got.Code != http.StatusSeeOther || got.Header().Get("Location") != detailPath+"?saved=1" {
		t.Fatalf("rename: %d %s", got.Code, got.Header().Get("Location"))
	}
	if got, err := svc.Get(ctx, owner, env.workspaceID, items[0].ID); err != nil || got.Name != "Renamed" {
		t.Fatalf("renamed engagement: %+v %v", got, err)
	}
	savedDetail := request(env.routes, http.MethodGet, detailPath+"?saved=1", nil, env.cookies, nil)
	if savedDetail.Code != http.StatusOK || !strings.Contains(savedDetail.Body.String(), "Engagement saved.") {
		t.Fatalf("saved confirmation: %d", savedDetail.Code)
	}
	if strings.Contains(writerDetail.Body.String(), "Engagement saved.") {
		t.Fatal("confirmation shown without successful edit")
	}
	clientPage := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if !strings.Contains(clientPage.Body.String(), `href="`+detailPath+`"`) {
		t.Fatal("client page missing engagement detail link")
	}
	for _, cookies := range [][]*http.Cookie{env.cookies, env.viewerCookies} {
		detail := request(env.routes, http.MethodGet, detailPath, nil, cookies, nil)
		if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "Renamed") || !strings.Contains(detail.Body.String(), "Other person") || !strings.Contains(detail.Body.String(), "Open") {
			t.Fatalf("detail: %d %s", detail.Code, detail.Body.String())
		}
	}
	if got := request(env.routes, http.MethodGet, detailPath, nil, nil, nil); got.Code == http.StatusOK {
		t.Fatal("anonymous engagement detail allowed")
	}
	wrongPath := "/workspaces/" + env.workspaceID.String() + "/clients/" + otherClient.ID.String() + "/engagements/" + items[0].ID.String()
	if got := request(env.routes, http.MethodGet, wrongPath, nil, env.cookies, nil); got.Code != http.StatusNotFound {
		t.Fatalf("wrong client detail: %d", got.Code)
	}
	if got := request(env.routes, http.MethodGet, path+"/engagements/invalid", nil, env.cookies, nil); got.Code != http.StatusNotFound {
		t.Fatalf("invalid engagement detail: %d", got.Code)
	}
	for suffix, form := range map[string]url.Values{
		"/name":        {"name": {"Wrong client"}},
		"/agreement":   {"recorded": {"1"}},
		"/assignee":    {"assignee_id": {env.ownerID.String()}},
		"/quote":       {"status": {"sent"}},
		"/next-action": {"text": {"Wrong client"}},
	} {
		form.Set("csrf", env.csrf)
		if got := request(env.routes, http.MethodPost, wrongPath+suffix, form, env.cookies, nil); got.Code != http.StatusNotFound {
			t.Fatalf("wrong client %s: %d", suffix, got.Code)
		}
	}
	if got, err := svc.Get(ctx, owner, env.workspaceID, items[0].ID); err != nil || got.Agreement.Recorded || got.AssigneeID != 0 || got.Quote.Status == "sent" || got.NextAction.Text != "" {
		t.Fatalf("wrong client mutated engagement: %+v %v", got, err)
	}
	if r := request(env.routes, http.MethodPost, agreementPath, url.Values{"csrf": {env.csrf}, "recorded": {"1"}, "return_to": {"detail"}}, env.cookies, nil); r.Code != http.StatusSeeOther || r.Header().Get("Location") != detailPath+"?saved=1" {
		t.Fatalf("agreement: %d %s", r.Code, r.Header().Get("Location"))
	}
	writerAgreement := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil)
	viewerAgreement := request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil)
	if !strings.Contains(writerAgreement.Body.String(), `action="`+agreementPath+`"`) || strings.Contains(viewerAgreement.Body.String(), `action="`+agreementPath+`"`) || !strings.Contains(viewerAgreement.Body.String(), "Signed agreement recorded") {
		t.Fatal("agreement form visibility or viewer summary")
	}
	invalidAgreement := request(env.routes, http.MethodPost, agreementPath, url.Values{"csrf": {env.csrf}, "recorded": {"1"}, "file_id": {"invalid-short-id"}, "return_to": {"detail"}}, env.cookies, nil)
	for _, want := range []string{`value="invalid-short-id"`, `aria-describedby="engagement-agreement-file-error"`, "Enter a valid file ID for this workspace.", `name="recorded" value="1" checked`} {
		if invalidAgreement.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidAgreement.Body.String(), want) {
			t.Fatalf("invalid agreement missing %q: %d", want, invalidAgreement.Code)
		}
	}
	workspaceAdmin, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.auth.UpdateWorkspaceLocale(ctx, workspaceAdmin, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	for _, localizedPath := range []string{path, detailPath} {
		localized := request(env.routes, http.MethodGet, localizedPath, nil, env.cookies, nil)
		if strings.Contains(localized.Body.String(), "engagement.stage") || !strings.Contains(localized.Body.String(), "Etapa") {
			t.Fatalf("Spanish stage translation missing on %s", localizedPath)
		}
		if localizedPath == detailPath && (!strings.Contains(localized.Body.String(), `<nav aria-label="Clientes">`) || !strings.Contains(localized.Body.String(), `>Clientes</a>`)) {
			t.Fatal("Spanish detail breadcrumb missing")
		}
	}
	spanishWriter := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil)
	if !strings.Contains(spanishWriter.Body.String(), ">Sin asignar</option>") || !strings.Contains(spanishWriter.Body.String(), "Próxima acción: Sin próxima acción") {
		t.Fatal("Spanish writer unassigned detail missing")
	}
	spanishViewer := request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil)
	if !strings.Contains(spanishViewer.Body.String(), "Asignado a: Sin asignar") || !strings.Contains(spanishViewer.Body.String(), "Próxima acción: Sin próxima acción") {
		t.Fatal("Spanish viewer unassigned detail missing")
	}
	spanishSaved := request(env.routes, http.MethodGet, detailPath+"?saved=1", nil, env.cookies, nil)
	if !strings.Contains(spanishSaved.Body.String(), "Gestión guardada.") {
		t.Fatal("Spanish confirmation missing")
	}
	spanishAgreement := request(env.routes, http.MethodPost, agreementPath, url.Values{"csrf": {env.csrf}, "file_id": {"invalid-short-id"}, "return_to": {"detail"}}, env.cookies, nil)
	if spanishAgreement.Code != http.StatusUnprocessableEntity || !strings.Contains(spanishAgreement.Body.String(), "Introduce un ID de archivo válido para este espacio de trabajo.") {
		t.Fatalf("Spanish agreement error: %d", spanishAgreement.Code)
	}
	if err := env.auth.UpdateWorkspaceLocale(ctx, workspaceAdmin, "en", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if r := request(env.routes, http.MethodPost, agreementPath, url.Values{"csrf": {env.viewerCSRF}}, env.viewerCookies, nil); r.Code != http.StatusForbidden {
		t.Fatalf("viewer agreement: %d", r.Code)
	}
	if r := request(env.routes, http.MethodPost, agreementPath, url.Values{"csrf": {env.csrf}, "file_id": {"invalid"}}, env.cookies, nil); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid file: %d", r.Code)
	}
	assigned := request(env.routes, http.MethodPost, assigneePath, url.Values{"csrf": {env.csrf}, "assignee_id": {env.ownerID.String()}, "return_to": {"detail"}}, env.cookies, nil)
	if assigned.Code != http.StatusSeeOther || assigned.Header().Get("Location") != detailPath+"?saved=1" {
		t.Fatalf("assign: %d %s", assigned.Code, assigned.Header().Get("Location"))
	}
	writerDetail = request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil)
	for _, want := range []string{`action="` + assigneePath + `"`, `for="engagement-assignee"`, `name="assignee_id"`, `value="` + env.ownerID.String() + `" selected`, `name="return_to" value="detail"`, `href="` + detailPath + `"`} {
		if !strings.Contains(writerDetail.Body.String(), want) {
			t.Fatalf("assignee detail missing %q", want)
		}
	}
	viewerDetail = request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil)
	if strings.Contains(viewerDetail.Body.String(), `action="`+assigneePath+`"`) || !strings.Contains(viewerDetail.Body.String(), "Assignee:") {
		t.Fatalf("viewer assignee summary or form visibility: %d", viewerDetail.Code)
	}
	if got, err := svc.Get(ctx, owner, env.workspaceID, items[0].ID); err != nil || got.AssigneeID != env.ownerID {
		t.Fatalf("assignment: %+v %v", got, err)
	}
	if denied := request(env.routes, http.MethodPost, assigneePath, url.Values{"csrf": {env.viewerCSRF}}, env.viewerCookies, nil); denied.Code != http.StatusForbidden {
		t.Fatalf("viewer assign: %d", denied.Code)
	}
	if invalid := request(env.routes, http.MethodPost, assigneePath, url.Values{"csrf": {env.csrf}, "assignee_id": {"invalid"}, "return_to": {"detail"}}, env.cookies, nil); invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "Select a valid workspace member.") {
		t.Fatalf("invalid assign: %d", invalid.Code)
	}
	invalidAssignee := request(env.routes, http.MethodPost, assigneePath, url.Values{"csrf": {env.csrf}, "assignee_id": {"invalid"}, "return_to": {"detail"}}, env.cookies, nil)
	for _, want := range []string{`aria-describedby="engagement-assignee-error"`, `id="engagement-assignee-error"`, `<option value="invalid" selected disabled>invalid — Unavailable</option>`} {
		if !strings.Contains(invalidAssignee.Body.String(), want) {
			t.Fatalf("invalid assignee missing %q", want)
		}
	}
	if omitted := request(env.routes, http.MethodPost, assigneePath, url.Values{"csrf": {env.csrf}, "return_to": {"detail"}}, env.cookies, nil); omitted.Code != http.StatusUnprocessableEntity {
		t.Fatalf("omitted assignee: %d", omitted.Code)
	}
	if invalid := request(env.routes, http.MethodPost, assigneePath, url.Values{"csrf": {env.csrf}, "assignee_id": {identity.UserID(999999).String()}}, env.cookies, nil); invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("nonmember assign: %d", invalid.Code)
	}
	if unassigned := request(env.routes, http.MethodPost, assigneePath, url.Values{"csrf": {env.csrf}}, env.cookies, nil); unassigned.Code != http.StatusSeeOther {
		t.Fatalf("unassign: %d", unassigned.Code)
	}
	quote := request(env.routes, http.MethodPost, quotePath, url.Values{"csrf": {env.csrf}, "status": {"sent"}, "amount": {"120.50"}, "date_sent": {"2026-11-23"}, "return_to": {"detail"}}, env.cookies, nil)
	if quote.Code != http.StatusSeeOther || quote.Header().Get("Location") != detailPath+"?saved=1" {
		t.Fatalf("quote: %d %s", quote.Code, quote.Header().Get("Location"))
	}
	quoteDetail := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil)
	for _, want := range []string{`action="` + quotePath + `"`, `name="return_to" value="detail"`, `value="sent" selected`, `value="120.50"`, `value="2026-11-23"`, `href="` + detailPath + `"`} {
		if !strings.Contains(quoteDetail.Body.String(), want) {
			t.Fatalf("quote detail missing %q", want)
		}
	}
	invalidQuote := request(env.routes, http.MethodPost, quotePath, url.Values{"csrf": {env.csrf}, "status": {"pending"}, "amount": {"123.456"}, "date_sent": {"2026-11-24"}, "return_to": {"detail"}}, env.cookies, nil)
	for _, want := range []string{"Enter a valid quote status, amount, and date.", `value="pending" selected`, `value="123.456"`, `value="2026-11-24"`} {
		if invalidQuote.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidQuote.Body.String(), want) {
			t.Fatalf("invalid quote missing %q: %d", want, invalidQuote.Code)
		}
	}
	badQuote := request(env.routes, http.MethodPost, quotePath, url.Values{"csrf": {env.csrf}, "status": {"bogus"}, "amount": {"not-money"}, "date_sent": {"bad-date"}, "return_to": {"detail"}}, env.cookies, nil)
	for _, want := range []string{`aria-describedby="engagement-quote-status-error"`, `aria-describedby="engagement-quote-amount-error"`, `aria-describedby="engagement-quote-date-error"`, `value="bogus" selected disabled`, `value="not-money"`, `value="bad-date"`, "Select a valid quote status.", "Enter a non-negative amount"} {
		if badQuote.Code != http.StatusUnprocessableEntity || !strings.Contains(badQuote.Body.String(), want) {
			t.Fatalf("invalid quote fields missing %q: %d", want, badQuote.Code)
		}
	}
	spanishQuote := request(env.routes, http.MethodPost, quotePath, url.Values{"csrf": {env.csrf}, "status": {"sent"}, "amount": {"-1"}, "return_to": {"detail"}}, append(append([]*http.Cookie{}, env.cookies...), &http.Cookie{Name: "locale", Value: "es"}), nil)
	if spanishQuote.Code != http.StatusUnprocessableEntity || !strings.Contains(spanishQuote.Body.String(), "Introduce un importe no negativo") || strings.Contains(spanishQuote.Body.String(), `id="engagement-quote-status-error"`) {
		t.Fatalf("Spanish quote error: %d", spanishQuote.Code)
	}
	viewerQuote := request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil)
	if strings.Contains(viewerQuote.Body.String(), `action="`+quotePath+`"`) || !strings.Contains(viewerQuote.Body.String(), "120.50") || !strings.Contains(viewerQuote.Body.String(), "2026-11-23") {
		t.Fatalf("viewer quote summary: %d", viewerQuote.Code)
	}
	deniedQuote := request(env.routes, http.MethodPost, quotePath, url.Values{"csrf": {env.viewerCSRF}, "status": {"sent"}}, env.viewerCookies, nil)
	if deniedQuote.Code != http.StatusForbidden {
		t.Fatalf("viewer quote: %d", deniedQuote.Code)
	}
	updated := request(env.routes, http.MethodPost, actionPath, url.Values{"version": {"0"}, "csrf": {env.csrf}, "text": {"Call client"}, "due_date": {"2020-01-01"}, "owner_id": {env.ownerID.String()}, "return_to": {"detail"}}, env.cookies, nil)
	if updated.Code != http.StatusSeeOther || updated.Header().Get("Location") != detailPath+"?saved=1" {
		t.Fatalf("set action: %d %s", updated.Code, updated.Header().Get("Location"))
	}
	writerAction := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil)
	for _, want := range []string{`action="` + actionPath + `"`, `name="return_to" value="detail"`, `name="version" value="1"`, `value="Call client"`, `value="2020-01-01"`, `value="` + env.ownerID.String() + `" selected`, `href="` + detailPath + `"`} {
		if !strings.Contains(writerAction.Body.String(), want) {
			t.Fatalf("action form missing %q", want)
		}
	}
	for _, want := range []string{
		`data-inline-title`,
		`<dialog class="relative" open data-detail-dialog aria-labelledby="engagement-action-dialog-title">`,
		`<dialog class="relative" open data-detail-dialog aria-labelledby="engagement-agreement-dialog-title">`,
		`<dialog class="relative" open data-detail-dialog aria-labelledby="engagement-quote-dialog-title">`,
	} {
		if !strings.Contains(writerAction.Body.String(), want) {
			t.Fatalf("detail editor missing %q", want)
		}
	}
	for _, path := range []string{actionPath, detailPath + "/agreement", quotePath} {
		if count := strings.Count(writerAction.Body.String(), `action="`+path+`"`); count != 1 {
			t.Fatalf("detail editor %s has %d forms, want one", path, count)
		}
	}
	viewerAction := request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil)
	if strings.Contains(viewerAction.Body.String(), `action="`+actionPath+`"`) || !strings.Contains(viewerAction.Body.String(), "Call client") || !strings.Contains(viewerAction.Body.String(), "2020-01-01") {
		t.Fatal("viewer action form or summary")
	}
	invalidAction := request(env.routes, http.MethodPost, actionPath, url.Values{"version": {"1"}, "csrf": {env.csrf}, "text": {"Keep this action"}, "due_date": {"2026-11-24"}, "return_to": {"detail"}}, env.cookies, nil)
	for _, want := range []string{"An owner is required", `name="version" value="1"`, `value="Keep this action"`, `value="2026-11-24"`, `action="` + actionPath + `"`} {
		if invalidAction.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidAction.Body.String(), want) {
			t.Fatalf("invalid action missing %q: %d", want, invalidAction.Code)
		}
	}
	if !strings.Contains(invalidAction.Body.String(), `data-detail-dirty-on-load open`) {
		t.Fatal("invalid action draft must reopen its editor")
	}
	invalidOwner := request(env.routes, http.MethodPost, actionPath, url.Values{"version": {"1"}, "csrf": {env.csrf}, "text": {"Keep this action"}, "owner_id": {"invalid"}, "return_to": {"detail"}}, env.cookies, nil)
	for _, want := range []string{`aria-describedby="engagement-next-action-owner-error"`, `id="engagement-next-action-owner-error"`, `<option value="invalid" selected disabled>invalid — Unavailable</option>`} {
		if invalidOwner.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidOwner.Body.String(), want) {
			t.Fatalf("invalid owner missing %q: %d", want, invalidOwner.Code)
		}
	}
	validOwner := request(env.routes, http.MethodPost, actionPath, url.Values{"version": {"1"}, "csrf": {env.csrf}, "text": {"Keep this action"}, "owner_id": {env.ownerID.String()}, "due_date": {"invalid"}, "return_to": {"detail"}}, env.cookies, nil)
	if validOwner.Code != http.StatusUnprocessableEntity || !strings.Contains(validOwner.Body.String(), `value="`+env.ownerID.String()+`" selected`) || !strings.Contains(validOwner.Body.String(), `aria-describedby="engagement-next-action-due-error"`) || !strings.Contains(validOwner.Body.String(), `value="invalid"`) || strings.Contains(validOwner.Body.String(), `id="engagement-next-action-owner-error"`) {
		t.Fatalf("valid owner not retained after date error: %d", validOwner.Code)
	}
	badText := request(env.routes, http.MethodPost, actionPath, url.Values{"version": {"1"}, "csrf": {env.csrf}, "text": {strings.Repeat("x", 5001)}, "owner_id": {env.ownerID.String()}, "return_to": {"detail"}}, env.cookies, nil)
	if badText.Code != http.StatusUnprocessableEntity || !strings.Contains(badText.Body.String(), `aria-describedby="engagement-next-action-error"`) || !strings.Contains(badText.Body.String(), strings.Repeat("x", 5001)) {
		t.Fatalf("invalid action text: %d", badText.Code)
	}
	if omitted := request(env.routes, http.MethodPost, actionPath, url.Values{"version": {"1"}, "csrf": {env.csrf}, "return_to": {"detail"}}, env.cookies, nil); omitted.Code != http.StatusUnprocessableEntity || !strings.Contains(omitted.Body.String(), `aria-describedby="engagement-next-action-owner-error"`) {
		t.Fatalf("omitted owner: %d", omitted.Code)
	}
	if got, err := svc.Get(ctx, owner, env.workspaceID, items[0].ID); err != nil || got.NextAction.Text != "Call client" {
		t.Fatalf("invalid action mutated engagement: %+v %v", got, err)
	}
	for _, fields := range []url.Values{
		{"version": {"0"}, "csrf": {env.csrf}, "text": {"Stale edit"}, "owner_id": {env.ownerID.String()}, "return_to": {"detail"}},
		{"version": {"0"}, "csrf": {env.csrf}, "clear": {"1"}, "return_to": {"detail"}},
	} {
		stale := request(env.routes, http.MethodPost, actionPath, fields, env.cookies, nil)
		if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "This action changed.") || !strings.Contains(stale.Body.String(), `name="version" value="1"`) {
			t.Fatalf("stale action: %d %s", stale.Code, stale.Body.String())
		}
		if fields.Get("clear") == "1" && !strings.Contains(stale.Body.String(), "Retry clear") {
			t.Fatal("stale clear missing explicit retry")
		}
		if got, err := svc.Get(ctx, owner, env.workspaceID, items[0].ID); err != nil || got.NextAction.Text != "Call client" {
			t.Fatalf("stale action mutated engagement: %+v %v", got, err)
		}
	}
	conflict = request(env.routes, http.MethodPost, actionPath, url.Values{"version": {"0"}, "csrf": {env.csrf}, "text": {"Draft call"}, "due_date": {"2026-11-24"}, "owner_id": {env.ownerID.String()}}, env.cookies, nil)
	for _, want := range []string{"Call client", `value="Draft call"`, `value="2026-11-24"`, `name="version" value="1"`, "Retry save"} {
		if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), want) {
			t.Fatalf("conflict missing %q: %d", want, conflict.Code)
		}
	}
	retried := request(env.routes, http.MethodPost, actionPath, url.Values{"version": {"1"}, "csrf": {env.csrf}, "text": {"Draft call"}, "due_date": {"2026-11-24"}, "owner_id": {env.ownerID.String()}, "return_to": {"detail"}}, env.cookies, nil)
	if retried.Code != http.StatusSeeOther {
		t.Fatalf("retry save: %d", retried.Code)
	}
	if got, err := svc.Get(ctx, owner, env.workspaceID, items[0].ID); err != nil || got.NextAction.Text != "Draft call" {
		t.Fatalf("retry action: %+v %v", got, err)
	}
	cleared := request(env.routes, http.MethodPost, actionPath, url.Values{"version": {"2"}, "csrf": {env.csrf}, "clear": {"1"}, "return_to": {"detail"}}, env.cookies, nil)
	if cleared.Code != http.StatusSeeOther || cleared.Header().Get("Location") != detailPath+"?saved=1" {
		t.Fatalf("clear action: %d %s", cleared.Code, cleared.Header().Get("Location"))
	}
	updated = request(env.routes, http.MethodPost, actionPath, url.Values{"version": {"3"}, "csrf": {env.csrf}, "text": {"Call client"}, "due_date": {"2020-01-01"}, "owner_id": {env.ownerID.String()}}, env.cookies, nil)
	if updated.Code != http.StatusSeeOther || updated.Header().Get("Location") != path {
		t.Fatalf("client action redirect: %d %s", updated.Code, updated.Header().Get("Location"))
	}
	viewer := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if viewer.Code != http.StatusOK || !strings.Contains(viewer.Body.String(), `href="`+detailPath+`"`) ||
		strings.Contains(viewer.Body.String(), "Create engagement") || strings.Contains(viewer.Body.String(), actionPath) {
		t.Fatalf("viewer client engagement tile: %d", viewer.Code)
	}
	viewerEngagement := request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil)
	if viewerEngagement.Code != http.StatusOK || !strings.Contains(viewerEngagement.Body.String(), "Signed agreement recorded") ||
		!strings.Contains(viewerEngagement.Body.String(), "Other person") || !strings.Contains(viewerEngagement.Body.String(), "Call client") ||
		!strings.Contains(viewerEngagement.Body.String(), "120.50") || strings.Contains(viewerEngagement.Body.String(), agreementPath) ||
		strings.Contains(viewerEngagement.Body.String(), quotePath) || strings.Contains(viewerEngagement.Body.String(), assigneePath) {
		t.Fatalf("viewer engagement detail: %d", viewerEngagement.Code)
	}
	due := request(env.routes, http.MethodGet, "/workspaces/"+env.workspaceID.String()+"/engagements/due", nil, env.viewerCookies, nil)
	if due.Code != http.StatusOK || !strings.Contains(due.Body.String(), "Call client") {
		t.Fatalf("due: %d %s", due.Code, due.Body.String())
	}
	deniedAction := request(env.routes, http.MethodPost, actionPath, url.Values{"csrf": {env.viewerCSRF}}, env.viewerCookies, nil)
	if deniedAction.Code != http.StatusForbidden {
		t.Fatalf("viewer action: %d", deniedAction.Code)
	}
	denied := request(env.routes, http.MethodPost, path+"/engagements", url.Values{"csrf": {env.viewerCSRF}}, env.viewerCookies, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("viewer create: %d", denied.Code)
	}
	target, err := svc.CreateStage(ctx, owner, env.workspaceID, engagement.StageInput{Name: "Won", Kind: "won"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	items, err = svc.ListForClient(ctx, owner, env.workspaceID, client.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("items: %v %v", items, err)
	}
	movePath := detailPath + "/stage"
	detail = request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `action="`+movePath+`"`) || !strings.Contains(detail.Body.String(), `name="return_to" value="detail"`) {
		t.Fatalf("writer detail stage form: %d", detail.Code)
	}
	viewerDetail = request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil)
	if viewerDetail.Code != http.StatusOK || strings.Contains(viewerDetail.Body.String(), `action="`+movePath+`"`) {
		t.Fatalf("viewer detail stage form: %d", viewerDetail.Code)
	}
	if got := request(env.routes, http.MethodPost, movePath, url.Values{"csrf": {env.viewerCSRF}, "stage": {target.ID.String()}}, env.viewerCookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("viewer move: %d", got.Code)
	}
	if err := svc.ArchiveStage(ctx, owner, env.workspaceID, openStage, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	archivedCurrent := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil)
	if !strings.Contains(archivedCurrent.Body.String(), `<option value="`+openStage.String()+`" selected disabled>Second — Unavailable</option>`) {
		t.Fatal("current archived stage must remain visible and unavailable")
	}
	archivedViewer := request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil)
	if !strings.Contains(archivedViewer.Body.String(), "Stage: Second") || strings.Contains(archivedViewer.Body.String(), `action="`+movePath+`"`) {
		t.Fatal("viewer must see archived stage without an editor")
	}
	if got := request(env.routes, http.MethodPost, movePath, url.Values{"csrf": {env.csrf}, "stage": {target.ID.String()}, "return_to": {"detail"}}, env.cookies, nil); got.Code != http.StatusSeeOther || got.Header().Get("Location") != detailPath+"?saved=1" {
		t.Fatalf("move: %d location=%q", got.Code, got.Header().Get("Location"))
	}
	if got := request(env.routes, http.MethodGet, path, nil, env.cookies, nil); strings.Contains(got.Body.String(), `<option value="`+openStage.String()+`"`) {
		t.Fatal("archived stage offered as a move target")
	}
	if got := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil); strings.Contains(got.Body.String(), `<option value="`+openStage.String()+`"`) {
		t.Fatal("archived stage offered on detail")
	}
	if got := request(env.routes, http.MethodPost, movePath, url.Values{"csrf": {env.csrf}, "stage": {openStage.String()}, "return_to": {"detail"}}, env.cookies, nil); got.Code != http.StatusUnprocessableEntity || !strings.Contains(got.Body.String(), "Select an available stage.") || !strings.Contains(got.Body.String(), `action="`+movePath+`"`) || !strings.Contains(got.Body.String(), `aria-describedby="engagement-stage-error"`) || !strings.Contains(got.Body.String(), `<option value="`+openStage.String()+`" selected disabled>`) {
		t.Fatalf("archived detail move: %d", got.Code)
	}
	if _, err := env.db.ExecContext(ctx, `UPDATE users SET display_name = 'History Actor' WHERE id = ?`, owner.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.db.ExecContext(ctx, `UPDATE engagement_stage_history SET created_at = '2025-01-02T15:04:00Z' WHERE workspace_id = ?`, env.workspaceID); err != nil {
		t.Fatal(err)
	}
	stages, err := svc.ListStages(ctx, owner, env.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	archivedName := ""
	for _, stage := range stages {
		if stage.ID == openStage {
			archivedName = stage.Name
		}
	}
	for _, historyPath := range []string{detailPath} {
		viewer = request(env.routes, http.MethodGet, historyPath, nil, env.viewerCookies, nil)
		body := viewer.Body.String()
		if viewer.Code != http.StatusOK || archivedName == "" || !strings.Contains(body, archivedName+"</span>") || !strings.Contains(body, "Won") || !strings.Contains(body, "data-engagement-history") || !strings.Contains(body, "History Actor") || !strings.Contains(body, "Jan 2, 2025 3:04 PM UTC") || strings.Contains(body, "Move stage") {
			t.Fatalf("viewer history: %d %s", viewer.Code, body)
		}
	}
	if err := env.auth.UpdateWorkspaceLocale(ctx, owner, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	viewer = request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil)
	if !strings.Contains(viewer.Body.String(), "2 de enero de 2025 15:04 UTC") {
		t.Fatalf("localized history: %d %s", viewer.Code, viewer.Body.String())
	}
	if _, err := env.db.ExecContext(ctx, `UPDATE engagement_stage_history SET actor_user_id = -1 WHERE workspace_id = ?`, env.workspaceID); err != nil {
		t.Fatal(err)
	}
	viewer = request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil)
	if !strings.Contains(viewer.Body.String(), "No disponible</span>") {
		t.Fatalf("missing actor: %d %s", viewer.Code, viewer.Body.String())
	}
}
