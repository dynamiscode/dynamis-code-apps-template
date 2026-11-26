package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestEngagementClientSearchFragment(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	owner, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	service := clients.NewService(env.db, config.SQLite, env.auth, 50)
	client, err := service.Create(ctx, owner, env.workspaceID, clients.Input{DisplayName: "Acme client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.Create(ctx, owner, env.workspaceID, clients.Input{DisplayName: "Other client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	stage, err := engagement.NewService(env.db, config.SQLite, env.auth).CreateStage(ctx, owner, env.workspaceID, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	path := "/workspaces/" + env.workspaceID.String() + "/engagements/new"
	small := request(env.routes, http.MethodGet, path, nil, env.cookies, map[string]string{"HX-Request": "true"})
	if strings.Contains(small.Body.String(), `id="client-search"`) {
		t.Fatal("client picker with five or fewer choices must not show search")
	}
	for i := range 4 {
		if _, err := service.Create(ctx, owner, env.workspaceID, clients.Input{DisplayName: fmt.Sprintf("Extra client %d", i)}, identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
	}
	modal := request(env.routes, http.MethodGet, path, nil, env.cookies, map[string]string{"HX-Request": "true"})
	for _, want := range []string{`hx-trigger="input changed delay:300ms"`, `hx-sync="#engagement-client-results:replace"`, `hx-target="#engagement-client-results"`, `role="status"`, `data-engagement-cancel`, `aria-label="Close"`, `data-client-picker`, `entry-picker-panel`, `data-entry-picker`, `data-entry-field`, `data-selected-client data-entry-required`, `formnovalidate`} {
		if !strings.Contains(modal.Body.String(), want) {
			t.Errorf("modal missing %q", want)
		}
	}
	fragment := request(env.routes, http.MethodGet, path+"?part=client-results&modal=1&q=Acme", nil, env.cookies, map[string]string{"HX-Request": "true"})
	body := fragment.Body.String()
	if fragment.Code != http.StatusOK || !strings.Contains(body, "Acme client") || strings.Contains(body, "Other client") || strings.Contains(body, "<dialog") || !strings.Contains(body, `value="select:`+client.ID.String()+`"`) || !strings.Contains(body, `data-client-option`) {
		t.Fatalf("search fragment: %d %s", fragment.Code, body)
	}
	page := request(env.routes, http.MethodGet, path+"?q=Acme", nil, env.cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Acme client") || strings.Contains(page.Body.String(), "Other client") {
		t.Fatalf("native search: %d %s", page.Code, page.Body.String())
	}
	for _, headers := range []map[string]string{nil, {"HX-Request": "true"}} {
		form := request(env.routes, http.MethodGet, path+"?client="+client.ID.String(), nil, env.cookies, headers)
		for _, want := range []string{`name="name"`, `name="notes"`, `name="stage"`, "Acme client", `data-notes-controls hidden`, `role="tab"`, `data-notes-preview`, `aria-label="Bold"`, `aria-label="Italic"`, `aria-label="Heading"`, `aria-label="Bulleted list"`, `aria-label="Link"`, `aria-label="Code"`, `aria-describedby="engagement-notes-help"`, "Markdown is supported.", `data-notes-count hidden`} {
			if form.Code != http.StatusOK || !strings.Contains(form.Body.String(), want) {
				t.Fatalf("create form missing %q: %d", want, form.Code)
			}
		}
	}
	draft := url.Values{"csrf": {env.csrf}, "client": {client.ID.String()}, "name": {"Draft project"}, "notes": {"# Draft\n\nKeep this"}, "stage": {stage.ID.String()}, "q": {"Other"}, "picker_action": {"search"}}
	searchResult := request(env.routes, http.MethodPost, path, draft, env.cookies, nil)
	for _, want := range []string{`value="Draft project"`, "# Draft", "Keep this", `value="` + stage.ID.String() + `" selected`, `value="` + client.ID.String() + `"`, "Other client", `data-client-picker open`} {
		if searchResult.Code != http.StatusOK || !strings.Contains(searchResult.Body.String(), want) {
			t.Fatalf("search lost %q: %d %s", want, searchResult.Code, searchResult.Body.String())
		}
	}
	draft.Set("picker_action", "select:"+other.ID.String())
	chosen := request(env.routes, http.MethodPost, path, draft, env.cookies, nil)
	for _, want := range []string{`value="Draft project"`, "# Draft", "Keep this", `value="` + stage.ID.String() + `" selected`, `value="` + other.ID.String() + `"`, "Other client"} {
		if chosen.Code != http.StatusOK || !strings.Contains(chosen.Body.String(), want) {
			t.Fatalf("selection lost %q: %d %s", want, chosen.Code, chosen.Body.String())
		}
	}
	invalidName := strings.Repeat("x", 201)
	rejected := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "client": {other.ID.String()}, "name": {invalidName}, "notes": {"# Keep me"}, "stage": {stage.ID.String()}}, env.cookies, nil)
	if rejected.Code != http.StatusUnprocessableEntity || !strings.Contains(rejected.Body.String(), invalidName) || !strings.Contains(rejected.Body.String(), "# Keep me") || !strings.Contains(rejected.Body.String(), other.ID.String()) {
		t.Fatalf("validation lost draft: %d", rejected.Code)
	}
	afterRejection := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "client": {other.ID.String()}, "name": {invalidName}, "notes": {"# Keep me"}, "stage": {stage.ID.String()}, "picker_action": {"select:" + client.ID.String()}}, env.cookies, nil)
	if afterRejection.Code != http.StatusOK || !strings.Contains(afterRejection.Body.String(), invalidName) || !strings.Contains(afterRejection.Body.String(), "# Keep me") || !strings.Contains(afterRejection.Body.String(), `value="`+client.ID.String()+`"`) {
		t.Fatalf("selection after validation lost draft: %d", afterRejection.Code)
	}
	if got := request(env.routes, http.MethodPost, path, draft, env.viewerCookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("viewer picker: %d", got.Code)
	}
	draft.Del("csrf")
	if got := request(env.routes, http.MethodPost, path, draft, env.cookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("picker without CSRF: %d", got.Code)
	}
	invalidNotes := strings.Repeat("n", 5001)
	invalid := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "client": {client.ID.String()}, "stage": {stage.ID.String()}, "name": {"Project"}, "notes": {invalidNotes}}, env.cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "5,000") || !strings.Contains(invalid.Body.String(), invalidNotes) {
		t.Fatalf("invalid notes must retain draft: %d", invalid.Code)
	}
	notes := "    code block\n\n**Contact** the client.  \n"
	created := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "client": {client.ID.String()}, "stage": {stage.ID.String()}, "name": {"Project"}, "notes": {notes}}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	items, err := engagement.NewService(env.db, config.SQLite, env.auth).ListForClient(ctx, owner, env.workspaceID, client.ID)
	if err != nil || len(items) != 1 || items[0].Notes != notes {
		t.Fatalf("created notes: %+v %v", items, err)
	}
	if _, err := service.Archive(ctx, owner, env.workspaceID, other.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	for _, headers := range []map[string]string{nil, {"HX-Request": "true"}} {
		rejected := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "client": {other.ID.String()}, "stage": {stage.ID.String()}, "name": {"Rejected project"}}, env.cookies, headers)
		if rejected.Code != http.StatusUnprocessableEntity || rejected.Header().Get("Location") != "" {
			t.Fatalf("archived client create = %d, location %q", rejected.Code, rejected.Header().Get("Location"))
		}
	}
	items, err = engagement.NewService(env.db, config.SQLite, env.auth).ListForClient(ctx, owner, env.workspaceID, other.ID)
	if err != nil || len(items) != 0 {
		t.Fatalf("archived client engagements = %+v, %v", items, err)
	}
}

func TestEngagementClientPickerPaginationKeepsDraft(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	owner, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	service := clients.NewService(env.db, config.SQLite, env.auth, 50)
	var selected clients.Client
	for i := range 26 {
		client, err := service.Create(ctx, owner, env.workspaceID, clients.Input{DisplayName: fmt.Sprintf("Client %02d", i)}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			selected = client
		}
	}
	stage, err := engagement.NewService(env.db, config.SQLite, env.auth).CreateStage(ctx, owner, env.workspaceID, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	path := "/workspaces/" + env.workspaceID.String() + "/engagements/new"
	draft := url.Values{"csrf": {env.csrf}, "client": {selected.ID.String()}, "name": {"Draft"}, "notes": {"**Notes**"}, "stage": {stage.ID.String()}, "picker_action": {"page:2"}}
	page := request(env.routes, http.MethodPost, path, draft, env.cookies, nil)
	for _, want := range []string{`value="Draft"`, "**Notes**", `value="` + stage.ID.String() + `" selected`, `value="` + selected.ID.String() + `"`, `value="page:1"`} {
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), want) {
			t.Fatalf("pagination lost %q: %d", want, page.Code)
		}
	}
	fragment := request(env.routes, http.MethodGet, path+"?part=client-results&page=2&q=Client", nil, env.cookies, map[string]string{"HX-Request": "true"})
	if fragment.Code != http.StatusOK || !strings.Contains(fragment.Body.String(), `value="page:1"`) || strings.Contains(fragment.Body.String(), "<dialog") {
		t.Fatalf("pagination fragment: %d %s", fragment.Code, fragment.Body.String())
	}
}
