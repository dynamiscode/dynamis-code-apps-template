package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestEngagementDashboardWeb(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	actor, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	svc := engagement.NewService(env.db, config.SQLite, env.auth)
	path := "/workspaces/" + env.workspaceID.String()
	empty := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), "No open stages yet.") {
		t.Fatalf("empty dashboard: %d", empty.Code)
	}
	stage, err := svc.CreateStage(ctx, actor, env.workspaceID, engagement.StageInput{Name: "Dashboard stage", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, actor, env.workspaceID, clients.Input{DisplayName: "Dashboard client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	for _, due := range []string{"2000-01-01", time.Now().UTC().Format("2006-01-02")} {
		item, err := svc.Create(ctx, actor, env.workspaceID, engagement.Input{Name: "Dashboard work " + due, ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.SetNextAction(ctx, actor, env.workspaceID, item.ID, engagement.NextAction{Text: "Follow up", DueDate: due, OwnerID: env.ownerID}, identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
	}
	for _, cookies := range [][]*http.Cookie{env.cookies, env.viewerCookies} {
		response := request(env.routes, http.MethodGet, path, nil, cookies, nil)
		for _, want := range []string{"Engagement dashboard", "Dashboard stage", "?stage=" + stage.ID.String(), "/engagements/due?filter=overdue", "/engagements/due?filter=today", "— 2"} {
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), want) {
				t.Fatalf("dashboard %d missing %q", response.Code, want)
			}
		}
	}
	if response := request(env.routes, http.MethodGet, path+"?page=0", nil, env.cookies, nil); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid page: %d", response.Code)
	}
	admin, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.auth.UpdateWorkspaceLocale(ctx, admin, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	response := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Panel de gestiones") {
		t.Fatalf("Spanish dashboard: %d", response.Code)
	}
}
