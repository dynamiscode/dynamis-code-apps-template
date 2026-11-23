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
	noStage := request(env.routes, http.MethodPost, path+"/engagements", url.Values{"csrf": {env.csrf}}, env.cookies, nil)
	if noStage.Code != http.StatusUnprocessableEntity || !strings.Contains(noStage.Body.String(), "no open stage") {
		t.Fatalf("no stage: %d %s", noStage.Code, noStage.Body.String())
	}
	_, err = engagement.NewService(env.db, config.SQLite, env.auth).CreateStage(ctx, owner, env.workspaceID, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	created := request(env.routes, http.MethodPost, path+"/engagements", url.Values{"csrf": {env.csrf}, "notes": {"Other person"}}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	svc := engagement.NewService(env.db, config.SQLite, env.auth)
	items, err := svc.ListForClient(ctx, owner, env.workspaceID, client.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("engagements: %+v %v", items, err)
	}
	actionPath := path + "/engagements/" + items[0].ID.String() + "/next-action"
	quotePath := path + "/engagements/" + items[0].ID.String() + "/quote"
	assigneePath := path + "/engagements/" + items[0].ID.String() + "/assignee"
	agreementPath := path + "/engagements/" + items[0].ID.String() + "/agreement"
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
}
