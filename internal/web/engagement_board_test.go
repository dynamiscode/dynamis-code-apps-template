package web

import (
	"context"
	"net/http"
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
	for _, kind := range []string{"open", "won", "lost"} {
		item, err := service.Create(ctx, actor, env.workspaceID, engagement.Input{Name: "Occupant " + kind, ClientID: client.ID, StageID: openStage.ID}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		if kind != "open" {
			stage, err := service.CreateStage(ctx, admin, env.workspaceID, engagement.StageInput{Name: "Board " + kind, Kind: kind}, identity.AuditContext{})
			if err != nil {
				t.Fatal(err)
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
