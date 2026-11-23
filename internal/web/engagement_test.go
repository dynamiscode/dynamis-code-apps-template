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
			page = request(env.routes, http.MethodGet, clientsPath, nil, tc.cookies, nil)
			if page.Code != http.StatusOK || strings.Contains(page.Body.String(), "Select a client to create an engagement.") != tc.writer {
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
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Selecciona un cliente para crear una gestión.") {
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
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), "Matter stages") {
		t.Fatalf("navigation: %d %s", r.Code, r.Body.String())
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
	if !strings.Contains(r.Body.String(), "Engagement stages") {
		t.Fatalf("default: %s", r.Body.String())
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
	created = request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "name": {"Won"}, "kind": {"won"}}, env.cookies, nil)
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
	archived := request(env.routes, http.MethodPost, path+"/"+stages[0].ID.String()+"/archive", url.Values{"csrf": {env.csrf}}, env.cookies, nil)
	if archived.Code != http.StatusSeeOther {
		t.Fatalf("archive = %d", archived.Code)
	}
	deniedArchive := request(env.routes, http.MethodPost, path+"/"+stages[1].ID.String()+"/archive", url.Values{"csrf": {env.viewerCSRF}}, env.viewerCookies, nil)
	if deniedArchive.Code != http.StatusForbidden {
		t.Fatalf("viewer archive = %d", deniedArchive.Code)
	}
	deniedReorder := request(env.routes, http.MethodPost, path+"/reorder", url.Values{"csrf": {env.viewerCSRF}, "stage": {stages[1].ID.String()}, "direction": {"down"}}, env.viewerCookies, nil)
	if deniedReorder.Code != http.StatusForbidden {
		t.Fatalf("viewer reorder = %d", deniedReorder.Code)
	}
	list := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Archived") || strings.Index(list.Body.String(), "Won") > strings.Index(list.Body.String(), "Prospect") {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
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
	for _, want := range []string{`for="engagement-name"`, `id="engagement-name"`, `name="name"`, `required`, `maxlength="200"`, `name="stage"`, stage.ID.String(), second.ID.String()} {
		if !strings.Contains(page.Body.String(), want) {
			t.Fatalf("creation form missing %q", want)
		}
	}
	invalidName := strings.Repeat("x", 201)
	invalidCreate := request(env.routes, http.MethodPost, path+"/engagements", url.Values{"csrf": {env.csrf}, "name": {invalidName}, "stage": {second.ID.String()}, "notes": {"Keep these notes"}}, env.cookies, nil)
	for _, want := range []string{"The engagement is invalid.", invalidName, "Keep these notes", `value="` + second.ID.String() + `" selected`} {
		if invalidCreate.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidCreate.Body.String(), want) {
			t.Fatalf("invalid create missing %q: %d %s", want, invalidCreate.Code, invalidCreate.Body.String())
		}
	}
	missingStage := request(env.routes, http.MethodPost, path+"/engagements", url.Values{"csrf": {env.csrf}, "name": {"Engagement"}}, env.cookies, nil)
	if missingStage.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing stage: %d", missingStage.Code)
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
	viewerDetail := request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil)
	if !strings.Contains(writerDetail.Body.String(), namePath) || strings.Contains(viewerDetail.Body.String(), namePath) {
		t.Fatalf("name form visibility: writer=%d viewer=%d", writerDetail.Code, viewerDetail.Code)
	}
	if got := request(env.routes, http.MethodPost, namePath, url.Values{"csrf": {env.viewerCSRF}, "name": {"Denied"}}, env.viewerCookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("viewer rename: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, namePath, url.Values{"csrf": {env.csrf}, "name": {invalidName}}, env.cookies, nil); got.Code != http.StatusUnprocessableEntity || !strings.Contains(got.Body.String(), invalidName) {
		t.Fatalf("invalid rename: %d %s", got.Code, got.Body.String())
	}
	if got := request(env.routes, http.MethodPost, namePath, url.Values{"csrf": {env.csrf}, "name": {"Renamed"}}, env.cookies, nil); got.Code != http.StatusSeeOther || got.Header().Get("Location") != detailPath {
		t.Fatalf("rename: %d %s", got.Code, got.Header().Get("Location"))
	}
	if got, err := svc.Get(ctx, owner, env.workspaceID, items[0].ID); err != nil || got.Name != "Renamed" {
		t.Fatalf("renamed engagement: %+v %v", got, err)
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
	if r := request(env.routes, http.MethodPost, agreementPath, url.Values{"csrf": {env.csrf}, "recorded": {"1"}}, env.cookies, nil); r.Code != http.StatusSeeOther {
		t.Fatalf("agreement: %d %s", r.Code, r.Body.String())
	}
	if r := request(env.routes, http.MethodPost, agreementPath, url.Values{"csrf": {env.viewerCSRF}}, env.viewerCookies, nil); r.Code != http.StatusForbidden {
		t.Fatalf("viewer agreement: %d", r.Code)
	}
	if r := request(env.routes, http.MethodPost, agreementPath, url.Values{"csrf": {env.csrf}, "file_id": {"invalid"}}, env.cookies, nil); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid file: %d", r.Code)
	}
	assigned := request(env.routes, http.MethodPost, assigneePath, url.Values{"csrf": {env.csrf}, "assignee_id": {env.ownerID.String()}}, env.cookies, nil)
	if assigned.Code != http.StatusSeeOther {
		t.Fatalf("assign: %d %s", assigned.Code, assigned.Body.String())
	}
	if got, err := svc.Get(ctx, owner, env.workspaceID, items[0].ID); err != nil || got.AssigneeID != env.ownerID {
		t.Fatalf("assignment: %+v %v", got, err)
	}
	if denied := request(env.routes, http.MethodPost, assigneePath, url.Values{"csrf": {env.viewerCSRF}}, env.viewerCookies, nil); denied.Code != http.StatusForbidden {
		t.Fatalf("viewer assign: %d", denied.Code)
	}
	if invalid := request(env.routes, http.MethodPost, assigneePath, url.Values{"csrf": {env.csrf}, "assignee_id": {"invalid"}}, env.cookies, nil); invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid assign: %d", invalid.Code)
	}
	if invalid := request(env.routes, http.MethodPost, assigneePath, url.Values{"csrf": {env.csrf}, "assignee_id": {identity.UserID(999999).String()}}, env.cookies, nil); invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("nonmember assign: %d", invalid.Code)
	}
	if unassigned := request(env.routes, http.MethodPost, assigneePath, url.Values{"csrf": {env.csrf}}, env.cookies, nil); unassigned.Code != http.StatusSeeOther {
		t.Fatalf("unassign: %d", unassigned.Code)
	}
	quote := request(env.routes, http.MethodPost, quotePath, url.Values{"csrf": {env.csrf}, "status": {"sent"}, "amount": {"120.50"}, "date_sent": {"2026-11-23"}}, env.cookies, nil)
	if quote.Code != http.StatusSeeOther {
		t.Fatalf("quote: %d %s", quote.Code, quote.Body.String())
	}
	deniedQuote := request(env.routes, http.MethodPost, quotePath, url.Values{"csrf": {env.viewerCSRF}, "status": {"sent"}}, env.viewerCookies, nil)
	if deniedQuote.Code != http.StatusForbidden {
		t.Fatalf("viewer quote: %d", deniedQuote.Code)
	}
	updated := request(env.routes, http.MethodPost, actionPath, url.Values{"csrf": {env.csrf}, "text": {"Call client"}, "due_date": {"2020-01-01"}, "owner_id": {env.ownerID.String()}}, env.cookies, nil)
	if updated.Code != http.StatusSeeOther {
		t.Fatalf("set action: %d %s", updated.Code, updated.Body.String())
	}
	viewer := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if viewer.Code != http.StatusOK || !strings.Contains(viewer.Body.String(), "Signed agreement recorded") || strings.Contains(viewer.Body.String(), agreementPath) || !strings.Contains(viewer.Body.String(), "Other person") || !strings.Contains(viewer.Body.String(), "Call client") || !strings.Contains(viewer.Body.String(), "120.50") || strings.Contains(viewer.Body.String(), quotePath) || strings.Contains(viewer.Body.String(), assigneePath) || strings.Contains(viewer.Body.String(), "Create engagement") || strings.Contains(viewer.Body.String(), actionPath) {
		t.Fatalf("viewer: %d notes=%v create=%v", viewer.Code, strings.Contains(viewer.Body.String(), "Other person"), strings.Contains(viewer.Body.String(), "Create engagement"))
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
	detail := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil)
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
	if got := request(env.routes, http.MethodPost, movePath, url.Values{"csrf": {env.csrf}, "stage": {target.ID.String()}, "return_to": {"detail"}}, env.cookies, nil); got.Code != http.StatusSeeOther || got.Header().Get("Location") != detailPath {
		t.Fatalf("move: %d location=%q", got.Code, got.Header().Get("Location"))
	}
	if err := svc.ArchiveStage(ctx, owner, env.workspaceID, openStage, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if got := request(env.routes, http.MethodGet, path, nil, env.cookies, nil); strings.Contains(got.Body.String(), `<option value="`+openStage.String()+`"`) {
		t.Fatal("archived stage offered as a move target")
	}
	if got := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil); strings.Contains(got.Body.String(), `<option value="`+openStage.String()+`"`) {
		t.Fatal("archived stage offered on detail")
	}
	if got := request(env.routes, http.MethodPost, movePath, url.Values{"csrf": {env.csrf}, "stage": {openStage.String()}, "return_to": {"detail"}}, env.cookies, nil); got.Code != http.StatusUnprocessableEntity || !strings.Contains(got.Body.String(), "Select an available stage.") || !strings.Contains(got.Body.String(), `action="`+movePath+`"`) {
		t.Fatalf("archived detail move: %d", got.Code)
	}
	viewer = request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if viewer.Code != http.StatusOK || !strings.Contains(viewer.Body.String(), "Won") || !strings.Contains(viewer.Body.String(), "data-engagement-history") || !strings.Contains(viewer.Body.String(), owner.UserID.String()) || strings.Contains(viewer.Body.String(), "Move stage") {
		t.Fatalf("viewer history: %d %s", viewer.Code, viewer.Body.String())
	}
}
