package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestCatalogCreateAndList(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	path := "/api/v1/workspaces/" + workspaceID + "/catalog"
	created := serveAuthorized(handler, http.MethodPost, path, `{"name":"Consultation","durationMinutes":30}`, token, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	list := serveAuthorized(handler, http.MethodGet, path, "", token, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"name":"Consultation"`) || !strings.Contains(list.Body.String(), `"durationMinutes":30`) {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	invalid := serveAuthorized(handler, http.MethodPost, path, `{"name":"  "}`, token, nil)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid create = %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestCatalogRESTRolePermissions(t *testing.T) {
	handler, db, workspace, _ := testHandler(t)
	ctx := context.Background()
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(workspace)
	var ownerID identity.UserID
	if err := db.QueryRowContext(ctx, "SELECT user_id FROM workspace_members WHERE workspace_id = ? AND role = 'owner'", workspaceID).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authorize(ctx, ownerID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/workspaces/" + workspace + "/catalog"
	for _, role := range []identity.Role{identity.Owner, identity.Admin, identity.Member, identity.Viewer} {
		userID := ownerID
		if role != identity.Owner {
			invitation, err := auth.CreateInvitation(ctx, owner, string(role)+"@example.com", role, time.Hour, identity.AuditContext{})
			if err != nil {
				t.Fatal(err)
			}
			userID, _, err = auth.CreateInvitedLocalUser(ctx, invitation.Secret, "long-enough-password", identity.AuditContext{})
			if err != nil {
				t.Fatal(err)
			}
		}
		principal, err := auth.Authorize(ctx, userID, workspaceID, identity.ResourcesRead)
		if err != nil {
			t.Fatal(err)
		}
		scopes := []identity.Permission{identity.ResourcesRead}
		if role != identity.Viewer {
			scopes = append(scopes, identity.ResourcesWrite)
		}
		token, err := auth.CreateAPIToken(ctx, principal, "catalog", scopes, nil, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		response := serveAuthorized(handler, http.MethodPost, path, `{"name":"Role test"}`, token.Secret, nil)
		want := http.StatusForbidden
		if role == identity.Owner || role == identity.Admin {
			want = http.StatusCreated
		}
		if response.Code != want {
			t.Errorf("%s create = %d %s, want %d", role, response.Code, response.Body.String(), want)
		}
		response = serveAuthorized(handler, http.MethodGet, path, "", token.Secret, nil)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Role test") {
			t.Errorf("%s list = %d %s", role, response.Code, response.Body.String())
		}
	}
}

func TestCatalogList(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	path := "/api/v1/workspaces/" + workspaceID + "/catalog"
	response := serveAuthorized(handler, http.MethodGet, path, "", token, nil)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("catalog = %d %s", response.Code, response.Body.String())
	}
	foreignPath := "/api/v1/workspaces/" + identity.WorkspaceID(999999).String() + "/catalog"
	foreign := serveAuthorized(handler, http.MethodGet, foreignPath, "", token, nil)
	if foreign.Code == http.StatusOK {
		t.Fatal("catalog returned 200 outside workspace")
	}
	denied := serveAuthorized(handler, http.MethodGet, path, "", "invalid", nil)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d", denied.Code)
	}
}
