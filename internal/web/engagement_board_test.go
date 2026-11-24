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

func TestEngagementBoard(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	actor, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, actor, env.workspaceID, clients.Input{DisplayName: "Board client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	service := engagement.NewService(env.db, config.SQLite, env.auth)
	openStage, err := service.CreateStage(ctx, admin, env.workspaceID, engagement.StageInput{Name: "Board open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	var openItem engagement.ID
	var wonStage engagement.StageID
	for _, kind := range []string{"open", "won", "lost"} {
		item, err := service.Create(ctx, actor, env.workspaceID, engagement.Input{Name: "Occupant " + kind, ClientID: client.ID, StageID: openStage.ID}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		if kind == "open" {
			openItem = item.ID
		}
		if kind != "open" {
			stage, err := service.CreateStage(ctx, admin, env.workspaceID, engagement.StageInput{Name: "Board " + kind, Kind: kind}, identity.AuditContext{})
			if err != nil {
				t.Fatal(err)
			}
			if kind == "won" {
				wonStage = stage.ID
			}
			if err := service.Move(ctx, actor, env.workspaceID, item.ID, stage.ID, identity.AuditContext{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := service.ArchiveStage(ctx, admin, env.workspaceID, openStage.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	path := "/workspaces/" + env.workspaceID.String() + "/engagements/board"
	for _, tc := range []struct{ query, want, absent string }{{"", "Occupant open", "Occupant won"}, {"?kind=won", "Occupant won", "Occupant open"}, {"?kind=lost", "Occupant lost", "Occupant won"}} {
		for _, cookies := range [][]*http.Cookie{env.cookies, env.viewerCookies} {
			response := request(env.routes, http.MethodGet, path+tc.query, nil, cookies, nil)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), tc.want) || strings.Contains(response.Body.String(), tc.absent) {
				t.Fatalf("%s: status %d", tc.query, response.Code)
			}
			if tc.query == "" && !strings.Contains(response.Body.String(), "Archived") {
				t.Fatal("archived stage not marked")
			}
		}
	}
	movePath := "/workspaces/" + env.workspaceID.String() + "/clients/" + client.ID.String() + "/engagements/" + openItem.String() + "/stage"
	writer := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	for _, want := range []string{`action="` + movePath + `"`, `for="move-` + openItem.String() + `"`, `<option value="` + wonStage.String() + `"`, `name="return_to" value="board"`} {
		if !strings.Contains(writer.Body.String(), want) {
			t.Fatalf("board missing %q", want)
		}
	}
	if strings.Contains(writer.Body.String(), `<option value="`+openStage.ID.String()+`"`) {
		t.Fatal("archived stage offered")
	}
	viewer := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if strings.Contains(viewer.Body.String(), `action="`+movePath+`"`) {
		t.Fatal("viewer offered move control")
	}
	values := url.Values{"csrf": {env.viewerCSRF}, "stage": {wonStage.String()}, "return_to": {"board"}, "kind": {"lost"}}
	if denied := request(env.routes, http.MethodPost, movePath, values, env.viewerCookies, nil); denied.Code != http.StatusForbidden {
		t.Fatalf("viewer move: %d", denied.Code)
	}
	values.Set("csrf", env.csrf)
	if moved := request(env.routes, http.MethodPost, movePath, values, env.cookies, nil); moved.Code != http.StatusSeeOther || moved.Header().Get("Location") != path+"?kind=lost" {
		t.Fatalf("board move: %d %s", moved.Code, moved.Header().Get("Location"))
	}
	item, err := service.Get(ctx, actor, env.workspaceID, openItem)
	if err != nil || item.StageID != wonStage {
		t.Fatalf("stage after move: %v, %v", item.StageID, err)
	}
	response := request(env.routes, http.MethodGet, path+"?kind=invalid", nil, env.cookies, nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid kind: %d", response.Code)
	}
	if err := env.auth.UpdateWorkspaceLocale(ctx, admin, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	response = request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if !strings.Contains(response.Body.String(), "Tablero de etapas") {
		t.Fatal("board not localized")
	}
}
