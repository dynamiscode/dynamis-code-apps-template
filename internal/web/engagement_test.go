package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

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
