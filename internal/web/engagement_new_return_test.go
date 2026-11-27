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

func TestEngagementCreatePreservesClientListContext(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	actor, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, actor, env.workspaceID, clients.Input{DisplayName: "Context client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	stage, err := engagement.NewService(env.db, config.SQLite, env.auth).CreateStage(ctx, actor, env.workspaceID, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	returnQuery := url.Values{"q": {"Ada"}, "page": {"2"}}.Encode()
	encoded := url.QueryEscape(returnQuery)
	clientPath := env.listPath() + "/" + client.ID.String()
	newPath := "/workspaces/" + env.workspaceID.String() + "/engagements/new"
	getPath := newPath + "?client=" + client.ID.String() + "&from_client=" + client.ID.String() + "&return=" + encoded
	postPath := newPath + "?return=" + encoded
	for _, headers := range []map[string]string{nil, {"HX-Request": "true"}} {
		page := request(env.routes, http.MethodGet, getPath, nil, env.cookies, headers)
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `action="`+postPath+`"`) || !strings.Contains(page.Body.String(), `href="`+clientPath+`?return=`+encoded+`"`) {
			t.Fatalf("creation form lost return context: %d %s", page.Code, page.Body.String())
		}
	}
	values := url.Values{"csrf": {env.csrf}, "client": {client.ID.String()}, "from_client": {client.ID.String()}, "name": {""}, "stage": {stage.ID.String()}}
	invalid := request(env.routes, http.MethodPost, postPath, values, env.cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `action="`+postPath+`"`) || !strings.Contains(invalid.Body.String(), `href="`+clientPath+`?return=`+encoded+`"`) {
		t.Fatalf("validation lost return context: %d %s", invalid.Code, invalid.Body.String())
	}
	values.Set("picker_action", "search")
	picker := request(env.routes, http.MethodPost, postPath, values, env.cookies, nil)
	if picker.Code != http.StatusOK || !strings.Contains(picker.Body.String(), `action="`+postPath+`"`) {
		t.Fatalf("picker lost return context: %d %s", picker.Code, picker.Body.String())
	}
	values.Del("picker_action")
	values.Set("name", "Engagement")
	created := request(env.routes, http.MethodPost, postPath, values, env.cookies, nil)
	location, err := url.Parse(created.Header().Get("Location"))
	if err != nil || created.Code != http.StatusSeeOther || !strings.HasPrefix(location.Path, clientPath+"/engagements/") || location.Query().Get("return") != returnQuery {
		t.Fatalf("creation lost return context: %d %q, %v", created.Code, created.Header().Get("Location"), err)
	}
}

func TestEngagementCreatePreservesEngagementListOrigin(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	actor, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, actor, env.workspaceID, clients.Input{DisplayName: "Origin client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	stage, err := engagement.NewService(env.db, config.SQLite, env.auth).CreateStage(ctx, actor, env.workspaceID, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	list := "/workspaces/" + env.workspaceID.String() + "/engagements"
	origin := list + "?q=Acme&view=board"
	path := list + "/new?engagement_return=" + url.QueryEscape(origin)
	for _, headers := range []map[string]string{nil, {"HX-Request": "true"}} {
		page := request(env.routes, http.MethodGet, path, nil, env.cookies, headers)
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `href="`+list+`?q=Acme&amp;view=board"`) || !strings.Contains(page.Body.String(), `action="`+path+`"`) {
			t.Fatalf("creation lost origin: %d %s", page.Code, page.Body.String())
		}
	}
	values := url.Values{"csrf": {env.csrf}, "client": {client.ID.String()}, "stage": {stage.ID.String()}, "name": {"Draft"}, "notes": {"Draft notes"}, "picker_action": {"search"}}
	picker := request(env.routes, http.MethodPost, path, values, env.cookies, nil)
	if picker.Code != http.StatusOK || !strings.Contains(picker.Body.String(), `action="`+path+`"`) || !strings.Contains(picker.Body.String(), "Draft notes") {
		t.Fatalf("picker lost origin or draft: %d %s", picker.Code, picker.Body.String())
	}
	values.Del("picker_action")
	created := request(env.routes, http.MethodPost, path, values, env.cookies, nil)
	location, err := url.Parse(created.Header().Get("Location"))
	if err != nil || created.Code != http.StatusSeeOther || location.Query().Get("engagement_return") != origin {
		t.Fatalf("creation lost origin: %d %q %v", created.Code, created.Header().Get("Location"), err)
	}
	for _, invalid := range []string{"https://evil.example" + list, "//evil.example" + list, list + "/new", list + "#fragment"} {
		page := request(env.routes, http.MethodGet, list+"/new?engagement_return="+url.QueryEscape(invalid), nil, env.cookies, nil)
		if strings.Contains(page.Body.String(), "evil.example") || strings.Contains(page.Body.String(), "engagement_return=") {
			t.Fatalf("unsafe origin retained: %q", invalid)
		}
	}
}

func TestEngagementReturnPath(t *testing.T) {
	env := newClientsTestEnv(t)
	list := "/workspaces/" + env.workspaceID.String() + "/engagements"
	for _, path := range []string{list + "?view=board&q=Acme", list + "/due?filter=overdue&owner=1&page=2"} {
		if got := engagementReturnPath(env.workspaceID, path); got != path {
			t.Fatalf("origin %q became %q", path, got)
		}
	}
	for _, path := range []string{"//evil.example" + list, "https://evil.example" + list, list + "/new", list + "#fragment", "/workspaces/other/engagements"} {
		if got := engagementReturnPath(env.workspaceID, path); got != "" {
			t.Fatalf("unsafe origin %q accepted as %q", path, got)
		}
	}
}
