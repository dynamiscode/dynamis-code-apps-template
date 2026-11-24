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

func TestEngagementListRoute(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	actor, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
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
	path := "/workspaces/" + env.workspaceID.String() + "/engagements"
	query := url.Values{"q": {"List engagement"}, "stage": {stage.ID.String()}, "assignee": {"unassigned"}}
	for _, cookies := range [][]*http.Cookie{env.cookies, env.viewerCookies} {
		response := request(env.routes, http.MethodGet, path+"?"+query.Encode(), nil, cookies, nil)
		clientsPath := "/workspaces/" + env.workspaceID.String() + "/clients/" + client.ID.String()
		for _, want := range []string{
			`<a class="link rounded-field border border-base-300 px-1" href="` + clientsPath + `/engagements/` + item.ID.String() + `">List engagement</a>`,
			`<a class="link rounded-field border border-base-300 px-1" href="` + clientsPath + `">List client</a>`,
		} {
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), want) {
				t.Fatalf("status %d: missing %q", response.Code, want)
			}
		}
	}
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
}
