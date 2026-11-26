package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var clientViewApplyLink = regexp.MustCompile(`/lists/clients/views/(\d+)/apply`)

func TestSavedClientViewOpensAsTheDefaultAndCanBeDeleted(t *testing.T) {
	env := newClientsTestEnv(t)
	env.seedClient(t, "Acme Corp")
	viewsPath := "/workspaces/" + env.workspaceID.String() + "/lists/clients/views"

	created := request(env.routes, http.MethodPost, viewsPath, url.Values{
		"csrf": {env.csrf}, "name": {"Acme"}, "query": {"sort=displayName"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create view = %d: %s", created.Code, created.Body.String())
	}
	body := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil).Body.String()
	match := clientViewApplyLink.FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("saved view is not in the switcher: %s", body)
	}
	id := match[1]

	if set := request(env.routes, http.MethodPost, viewsPath+"/"+id+"/default", url.Values{"csrf": {env.csrf}}, env.cookies, nil); set.Code != http.StatusSeeOther {
		t.Fatalf("make default = %d: %s", set.Code, set.Body.String())
	}
	opened := request(env.routes, http.MethodGet, env.listPath(), nil, env.cookies, nil)
	if opened.Code != http.StatusSeeOther || opened.Header().Get("Location") != env.listPath()+"?sort=displayName" {
		t.Fatalf("opening the list = %d to %q, want the default view", opened.Code, opened.Header().Get("Location"))
	}

	renamed := request(env.routes, http.MethodPost, viewsPath+"/"+id+"/rename", url.Values{"csrf": {env.csrf}, "name": {"Acme renamed"}}, env.cookies, nil)
	if renamed.Code != http.StatusSeeOther {
		t.Fatalf("rename view = %d: %s", renamed.Code, renamed.Body.String())
	}
	if list := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil).Body.String(); !strings.Contains(list, "Acme renamed") {
		t.Fatal("renamed view is not listed under its new name")
	}

	if deleted := request(env.routes, http.MethodPost, viewsPath+"/"+id+"/delete", url.Values{"csrf": {env.csrf}}, env.cookies, nil); deleted.Code != http.StatusSeeOther {
		t.Fatalf("delete view = %d: %s", deleted.Code, deleted.Body.String())
	}
	after := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil).Body.String()
	if clientViewApplyLink.MatchString(after) {
		t.Fatal("deleted view is still listed in the switcher")
	}
}

func TestSavedClientViewAvailableOnEmptyList(t *testing.T) {
	env := newClientsTestEnv(t)
	viewsPath := "/workspaces/" + env.workspaceID.String() + "/lists/clients/views"
	created := request(env.routes, http.MethodPost, viewsPath, url.Values{
		"csrf": {env.csrf}, "name": {"Empty view"}, "query": {"sort=displayName"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create view = %d: %s", created.Code, created.Body.String())
	}
	body := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil).Body.String()
	if !strings.Contains(body, "Empty view") || !clientViewApplyLink.MatchString(body) {
		t.Fatal("saved view missing from empty client list")
	}
}

func TestArchivedClientsDoNotOfferToSaveAView(t *testing.T) {
	env := newClientsTestEnv(t)
	// An archived scope with a filter still counts as content, so the views
	// switcher renders and can explain why saving is unavailable.
	body := request(env.routes, http.MethodGet, env.listPath()+"?archived=archived&all=1&filter=displayName:contains:Acme", nil, env.cookies, nil).Body.String()
	if !strings.Contains(body, "Views are for active clients.") {
		t.Fatalf("archived list does not explain why views cannot be saved: %s", body)
	}
	if strings.Contains(body, `action="/workspaces/`+env.workspaceID.String()+`/lists/clients/views"`) {
		t.Fatal("archived list renders the save-view form")
	}
}

func TestSavedViewCannotBeUsedThroughAnotherList(t *testing.T) {
	env := newClientsTestEnv(t)
	base := "/workspaces/" + env.workspaceID.String() + "/lists/"
	created := request(env.routes, http.MethodPost, base+"clients/views", url.Values{
		"csrf": {env.csrf}, "name": {"Original"}, "query": {"sort=displayName"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d", created.Code)
	}
	body := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil).Body.String()
	match := clientViewApplyLink.FindStringSubmatch(body)
	if match == nil {
		t.Fatal("missing saved view")
	}
	for _, action := range []string{"apply", "update", "rename", "delete", "default"} {
		response := request(env.routes, http.MethodPost, base+"members/views/"+match[1]+"/"+action, url.Values{
			"csrf": {env.csrf}, "name": {"Changed"}, "query": {"sort=email"},
		}, env.cookies, nil)
		want := http.StatusNotFound
		if action == "default" {
			want = http.StatusBadRequest
		}
		if response.Code != want {
			t.Errorf("%s = %d, want %d", action, response.Code, want)
		}
	}
	body = request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil).Body.String()
	if !strings.Contains(body, "Original") || strings.Contains(body, "Changed") {
		t.Fatalf("view changed through another list: %s", body)
	}
}
