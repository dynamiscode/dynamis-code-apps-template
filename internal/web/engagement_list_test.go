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

func TestEngagementSettingsMenu(t *testing.T) {
	env := newClientsTestEnv(t)
	base := "/workspaces/" + env.workspaceID.String()
	path := base + "/engagements"
	stages := base + "/engagement-stages"
	for _, tc := range []struct {
		name    string
		cookies []*http.Cookie
		manage  bool
	}{
		{"owner", env.cookies, true},
		{"viewer", env.viewerCookies, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, query := range []string{"", "?view=board", "?q=missing"} {
				page := request(env.routes, http.MethodGet, path+query, nil, tc.cookies, nil)
				body := page.Body.String()
				for _, link := range []string{stages + "#stages", stages + "#engagement-label"} {
					if page.Code != http.StatusOK || strings.Contains(body, `href="`+link+`"`) != tc.manage {
						t.Fatalf("%s: settings link %s: status %d", query, link, page.Code)
					}
				}
				if strings.Contains(body, `data-engagement-create`) != tc.manage {
					t.Fatalf("%s: create action missing or unauthorized", query)
				}
				if tc.manage {
					settings := strings.Index(body, `aria-label="Settings"`)
					if settings < 0 || !strings.Contains(body, `class="dropdown dropdown-end"`) || !strings.Contains(body, `max-w-[90vw]`) || !strings.Contains(body, `<span aria-hidden="true">⋮</span>`) {
						t.Fatalf("%s: settings menu placement or accessibility", query)
					}
					if !strings.Contains(body, `<div class="dropdown-content z-50 w-64 max-w-[90vw] rounded-box bg-base-100 p-2 shadow-lg"><ul class="menu">`) {
						t.Fatal("settings menu must not let toolbar menu styles override dropdown positioning")
					}
				}
			}
		})
	}
	response := request(env.routes, http.MethodPost, stages+"/label", url.Values{"csrf": {env.csrf}, "label": {"Matter"}}, env.cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("set label: %d", response.Code)
	}
	owner, err := env.auth.Authorize(context.Background(), env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.auth.UpdateWorkspaceLocale(context.Background(), owner, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	page := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	if !strings.Contains(page.Body.String(), "Gestionar etapas") || !strings.Contains(page.Body.String(), "Cambiar nombre de Matter") {
		t.Fatalf("localized settings: %d", page.Code)
	}
}

func TestEngagementListRoute(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	actor, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	path := "/workspaces/" + env.workspaceID.String() + "/engagements"
	for _, query := range []string{"", "?view=board", "?q=missing", "?page=2"} {
		for _, tc := range []struct {
			cookies   []*http.Cookie
			canCreate bool
		}{{env.cookies, true}, {env.viewerCookies, false}} {
			for _, headers := range []map[string]string{nil, {"HX-Request": "true"}} {
				response := request(env.routes, http.MethodGet, path+query, nil, tc.cookies, headers)
				body := response.Body.String()
				if response.Code != http.StatusOK || !strings.Contains(body, "No engagements yet") || !strings.Contains(body, "Create an engagement to track work for a client.") ||
					strings.Contains(body, `id="engagement-search"`) || strings.Contains(body, `id="engagement-stage-filter"`) ||
					strings.Contains(body, `class="dropdown list-menu"`) || strings.Contains(body, `<table`) ||
					strings.Count(body, `data-engagement-create`) != map[bool]int{true: 1, false: 0}[tc.canCreate] ||
					!strings.Contains(body, `items-center justify-center text-center`) {
					t.Fatalf("empty engagement view %q: status %d", query, response.Code)
				}
			}
		}
	}
	client, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, actor, env.workspaceID, clients.Input{DisplayName: "List client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	service := engagement.NewService(env.db, config.SQLite, env.auth)
	stage, err := service.CreateStage(ctx, actor, env.workspaceID, engagement.StageInput{Name: "List stage", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	item, err := service.Create(ctx, actor, env.workspaceID, engagement.Input{Name: "List engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	for _, headers := range []map[string]string{nil, {"HX-Request": "true"}} {
		response := request(env.routes, http.MethodGet, path, nil, env.cookies, headers)
		body := response.Body.String()
		heading := strings.Index(body, `class="feature-list-heading"`)
		create := strings.Index(body, `data-engagement-create`)
		menu := strings.Index(body, "/engagement-stages#stages")
		toolbar := strings.Index(body, `class="list-toolbar"`)
		if heading < 0 || create < heading || menu < create || toolbar < menu {
			t.Fatalf("engagement heading actions must precede filters: %d", response.Code)
		}
	}
	listWithBulk := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	for _, want := range []string{`On this page: 1`, `data-count-template="COUNT selected"`, `Move selected`, `Move all on this page`, `name="page_id"`} {
		if !strings.Contains(listWithBulk.Body.String(), want) {
			t.Fatalf("bulk scope missing %q", want)
		}
	}
	for _, query := range []string{"?q=missing", "?q=missing&view=board"} {
		response := request(env.routes, http.MethodGet, path+query, nil, env.cookies, nil)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `id="engagement-search"`) ||
			!strings.Contains(response.Body.String(), `id="engagement-stage-filter"`) || strings.Contains(response.Body.String(), "No engagements yet") {
			t.Fatalf("filtered engagement view %q: status %d", query, response.Code)
		}
	}
	for _, headers := range []map[string]string{nil, {"HX-Request": "true"}} {
		response := request(env.routes, http.MethodGet, path+"?q=List+engagement&view=board&stage="+stage.ID.String()+"&assignee=unassigned&sort=name&direction=asc", nil, env.viewerCookies, headers)
		body := response.Body.String()
		for _, want := range []string{
			`method="get" action="` + path + `"`,
			`hx-trigger="submit, input changed delay:300ms from:#engagement-search"`,
			`hx-target="#engagement-view"`, `hx-sync="this:replace"`,
			`<noscript><button class="btn btn-outline btn-sm" type="submit">Search</button></noscript>`,
			`name="view" value="board"`, `name="sort" value="name"`, `name="direction" value="asc"`,
			`value="` + stage.ID.String() + `" selected`, `value="unassigned" selected`,
			`List engagement</a>`,
		} {
			if response.Code != http.StatusOK || !strings.Contains(body, want) {
				t.Fatalf("search form: status %d, missing %q", response.Code, want)
			}
		}
	}
	filtered := request(env.routes, http.MethodGet, path+"?q=List+engagement&view=board&stage="+stage.ID.String()+"&assignee=unassigned&sort=name&direction=asc", nil, env.cookies, nil)
	for _, want := range []string{
		`<strong>Stage</strong> <strong>List stage</strong>`,
		`<strong>Assignee</strong> <strong>Unassigned</strong>`,
		`assignee=unassigned&amp;direction=asc&amp;q=List+engagement&amp;sort=name&amp;view=board`,
		`direction=asc&amp;q=List+engagement&amp;sort=name&amp;view=board`,
		`Switch to table view`,
		`engagement_return=`,
	} {
		if !strings.Contains(filtered.Body.String(), want) {
			t.Fatalf("filtered list missing %q", want)
		}
	}
	query := url.Values{"q": {"List engagement"}, "stage": {stage.ID.String()}, "assignee": {"unassigned"}}
	for _, cookies := range [][]*http.Cookie{env.cookies, env.viewerCookies} {
		response := request(env.routes, http.MethodGet, path+"?"+query.Encode(), nil, cookies, nil)
		clientsPath := "/workspaces/" + env.workspaceID.String() + "/clients/" + client.ID.String()
		for _, want := range []string{
			`<td class="list-linked-cell"><a class="list-row-link" href="` + clientsPath + `/engagements/` + item.ID.String() + `?engagement_return=`,
			`<td class="list-linked-cell"><a class="list-row-link" href="` + clientsPath + `">List client</a></td>`,
			`<div class="list-view-table"><table`,
			`<caption class="sr-only">Engagements</caption>`,
			`<th scope="col" data-column="name">`,
			`<th scope="col" data-column="client">`,
			`<th scope="col" data-column="stage">`,
			`<th scope="col" data-column="assignee">`,
			`aria-label="Row actions"`,
			`direction=asc&amp;q=List+engagement&amp;sort=name`,
			`<td>List stage</td>`,
			`<td>Unassigned</td>`,
			`class="list-view-cards"`,
		} {
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), want) {
				t.Fatalf("status %d: missing %q", response.Code, want)
			}
		}
	}
	sortQuery := url.Values{"q": {"List engagement"}, "stage": {stage.ID.String()}, "sort": {"name"}, "direction": {"asc"}}
	sorted := request(env.routes, http.MethodGet, path+"?"+sortQuery.Encode(), nil, env.viewerCookies, nil)
	if sorted.Code != http.StatusOK || !strings.Contains(sorted.Body.String(), `data-column="name" aria-sort="ascending"`) ||
		!strings.Contains(sorted.Body.String(), `direction=desc&amp;q=List+engagement&amp;sort=name`) {
		t.Fatalf("sort state: %d", sorted.Code)
	}
	if bad := request(env.routes, http.MethodGet, path+"?sort=name%20DESC&direction=asc", nil, env.cookies, nil); bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid sort: %d", bad.Code)
	}
	otherStage, err := service.CreateStage(ctx, actor, env.workspaceID, engagement.StageInput{Name: "Other stage", Kind: "won"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.Create(ctx, actor, env.workspaceID, engagement.Input{Name: "Other engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Move(ctx, actor, env.workspaceID, other.ID, otherStage.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	query.Set("view", "board")
	for _, headers := range []map[string]string{nil, {"HX-Request": "true"}} {
		response := request(env.routes, http.MethodGet, path+"?"+query.Encode(), nil, env.cookies, headers)
		body := response.Body.String()
		if response.Code != http.StatusOK || !strings.Contains(body, `id="engagement-view"`) || !strings.Contains(body, "List engagement</a>") || strings.Contains(body, "Other engagement</a>") || strings.Contains(body, `id="stage-`+otherStage.ID.String()+`"`) {
			t.Fatalf("filtered board: %d", response.Code)
		}
		if headers != nil && strings.Contains(body, "<html") {
			t.Fatal("HTMX board returned a full page")
		}
		if headers == nil && !strings.Contains(body, "<html") {
			t.Fatal("ordinary board request did not return a full page")
		}
	}
	query.Set("q", "not present")
	if response := request(env.routes, http.MethodGet, path+"?"+query.Encode(), nil, env.cookies, nil); strings.Contains(response.Body.String(), "List engagement</a>") {
		t.Fatal("board search did not filter the engagement")
	}
	query.Set("q", "List engagement")
	query.Set("assignee", env.ownerID.String())
	if response := request(env.routes, http.MethodGet, path+"?"+query.Encode(), nil, env.cookies, nil); strings.Contains(response.Body.String(), "List engagement</a>") {
		t.Fatal("board assignee filter did not exclude an unassigned engagement")
	}
	query.Set("assignee", "unassigned")
	movePath := "/workspaces/" + env.workspaceID.String() + "/clients/" + client.ID.String() + "/engagements/" + item.ID.String() + "/stage"
	values := url.Values{"csrf": {env.csrf}, "stage": {otherStage.ID.String()}, "return_to": {"board_view"}, "q": {"List engagement"}, "filter_stage": {stage.ID.String()}, "assignee": {"unassigned"}}
	moved := request(env.routes, http.MethodPost, movePath, values, env.cookies, nil)
	if moved.Code != http.StatusSeeOther || moved.Header().Get("Location") != path+"?"+query.Encode() {
		t.Fatalf("board move return: %d %s", moved.Code, moved.Header().Get("Location"))
	}
	if err := service.Move(ctx, actor, env.workspaceID, item.ID, stage.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	query.Del("view")
	query.Set("q", "not present")
	response := request(env.routes, http.MethodGet, path+"?"+query.Encode(), nil, env.cookies, nil)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "List engagement</a>") {
		t.Fatalf("filter: %d", response.Code)
	}
	response = request(env.routes, http.MethodGet, path+"?stage=invalid", nil, env.cookies, nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid stage: %d", response.Code)
	}
	for i := 0; i < 25; i++ {
		if _, err := service.Create(ctx, actor, env.workspaceID, engagement.Input{Name: "List engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
	}
	sortedPages := url.Values{"q": {"List engagement"}, "sort": {"name"}, "direction": {"asc"}}
	first := request(env.routes, http.MethodGet, path+"?"+sortedPages.Encode(), nil, env.viewerCookies, nil)
	sortedPages.Set("page", "2")
	second := request(env.routes, http.MethodGet, path+"?"+sortedPages.Encode(), nil, env.viewerCookies, nil)
	itemPath := "/engagements/" + item.ID.String()
	if first.Code != http.StatusOK || second.Code != http.StatusOK ||
		!strings.Contains(first.Body.String(), itemPath) || strings.Contains(second.Body.String(), itemPath) ||
		!strings.Contains(first.Body.String(), "direction=asc&amp;page=2") {
		t.Fatal("sort order or pagination was not preserved")
	}
	query.Set("q", "List engagement")
	response = request(env.routes, http.MethodGet, path+"?"+query.Encode(), nil, env.cookies, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "page=2&amp;q=List+engagement") || !strings.Contains(response.Body.String(), "assignee=unassigned") {
		t.Fatalf("pagination: %d", response.Code)
	}
	query.Set("page", "2")
	response = request(env.routes, http.MethodGet, path+"?"+query.Encode(), nil, env.cookies, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "rel=\"prev\"") || !strings.Contains(response.Body.String(), item.ID.String()) {
		t.Fatalf("second page: %d", response.Code)
	}
	if err := service.SetAssignee(ctx, actor, env.workspaceID, item.ID, env.ownerID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	for _, headers := range []map[string]string{nil, {"HX-Request": "true"}} {
		assigned := request(env.routes, http.MethodGet, path+"?assignee="+env.ownerID.String(), nil, env.cookies, headers)
		body := assigned.Body.String()
		if assigned.Code != http.StatusOK || !strings.Contains(body, `<td>owner@example.com</td>`) ||
			!strings.Contains(body, `<dt>Assignee</dt><dd>owner@example.com</dd>`) ||
			(strings.Contains(body, "<html") != (headers == nil)) {
			t.Fatalf("assigned engagement list: status %d, body %s", assigned.Code, body)
		}
	}
	admin, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.auth.UpdateWorkspaceLocale(ctx, admin, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	response = request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Gestiones") {
		t.Fatalf("locale: %d", response.Code)
	}
	ownerList := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	viewerList := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if !strings.Contains(ownerList.Body.String(), `data-bulk-all`) || !strings.Contains(ownerList.Body.String(), `data-bulk-row`) || strings.Contains(viewerList.Body.String(), `data-bulk-row`) {
		t.Fatal("selection permissions")
	}
	bulkPath := path + "/bulk-move"
	values = url.Values{"csrf": {env.csrf}, "engagement_id": {item.ID.String(), other.ID.String()}, "stage": {otherStage.ID.String()}, "return": {path + "?q=List"}}
	if got := request(env.routes, http.MethodPost, bulkPath, values, env.cookies, nil); got.Code != http.StatusBadRequest {
		t.Fatalf("unconfirmed bulk move: %d", got.Code)
	}
	values.Set("confirm", "yes")
	values.Set("csrf", env.viewerCSRF)
	if got := request(env.routes, http.MethodPost, bulkPath, values, env.viewerCookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("viewer bulk move: %d", got.Code)
	}
	values.Set("csrf", env.csrf)
	values.Add("engagement_id", "invalid")
	if got := request(env.routes, http.MethodPost, bulkPath, values, env.cookies, nil); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid bulk selection: %d", got.Code)
	}
	values["engagement_id"] = []string{item.ID.String(), other.ID.String()}
	if got := request(env.routes, http.MethodPost, bulkPath, values, env.cookies, nil); got.Code != http.StatusSeeOther || got.Header().Get("Location") != path+"?q=List" {
		t.Fatalf("bulk move: %d %s", got.Code, got.Header().Get("Location"))
	}
	for _, id := range []engagement.ID{item.ID, other.ID} {
		moved, err := service.Get(ctx, actor, env.workspaceID, id)
		if err != nil || moved.StageID != otherStage.ID {
			t.Fatalf("bulk moved engagement: %+v %v", moved, err)
		}
	}
	values.Set("selection", "all")
	values.Set("stage", stage.ID.String())
	values["page_id"] = []string{item.ID.String(), other.ID.String()}
	if got := request(env.routes, http.MethodPost, bulkPath, values, env.cookies, nil); got.Code != http.StatusSeeOther {
		t.Fatalf("move page: %d", got.Code)
	}
	for _, id := range []engagement.ID{item.ID, other.ID} {
		moved, err := service.Get(ctx, actor, env.workspaceID, id)
		if err != nil || moved.StageID != stage.ID {
			t.Fatalf("page moved engagement: %+v %v", moved, err)
		}
	}
}

func TestEngagementListArchivedStageIsNotEmpty(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	actor, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, actor, env.workspaceID, clients.Input{DisplayName: "Archived stage client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	service := engagement.NewService(env.db, config.SQLite, env.auth)
	stage, err := service.CreateStage(ctx, actor, env.workspaceID, engagement.StageInput{Name: "Archived stage", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, actor, env.workspaceID, engagement.Input{Name: "Archived stage engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveStage(ctx, actor, env.workspaceID, stage.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	path := "/workspaces/" + env.workspaceID.String() + "/engagements"
	for _, query := range []string{"", "?q=missing"} {
		response := request(env.routes, http.MethodGet, path+query, nil, env.viewerCookies, nil)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `id="engagement-search"`) || strings.Contains(response.Body.String(), "No engagements yet") {
			t.Fatalf("archived stage view %q: status %d", query, response.Code)
		}
	}
}
