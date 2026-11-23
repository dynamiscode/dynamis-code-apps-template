package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestCatalogList(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	ctx := context.Background()
	actor, err := auth.Authorize(ctx, owner.UserID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(ctx, actor, "viewer@example.com", identity.Viewer, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	viewer, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "viewer-long-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, viewer, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	response := request(routes, http.MethodGet, path, nil, cookies, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "No catalog entries yet.") {
		t.Fatalf("catalog = %d %s", response.Code, response.Body.String())
	}
	home := request(routes, http.MethodGet, "/workspaces/"+workspaceID.String(), nil, cookies, nil)
	if home.Code != http.StatusOK || !strings.Contains(home.Body.String(), path) {
		t.Fatalf("workspace home lacks catalog link: %d", home.Code)
	}
	foreignPath := "/workspaces/" + identity.WorkspaceID(999999).String() + "/catalog"
	foreign := request(routes, http.MethodGet, foreignPath, nil, cookies, nil)
	if foreign.Code == http.StatusOK {
		t.Fatal("catalog returned 200 outside workspace")
	}
	denied := request(routes, http.MethodGet, path, nil, nil, nil)
	if denied.Code == http.StatusOK {
		t.Fatal("unauthenticated catalog returned 200")
	}
}
