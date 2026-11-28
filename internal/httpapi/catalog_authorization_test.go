package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestCatalogRESTCompleteRoleMatrix(t *testing.T) {
	routes, db, workspace, ownerToken := testHandler(t)
	ctx := context.Background()
	auth, err := identity.NewService(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(workspace)
	var ownerID identity.UserID
	if err := db.QueryRowContext(ctx, "SELECT user_id FROM workspace_members WHERE workspace_id = ? AND role = 'owner'", workspaceID).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, ownerID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/workspaces/" + workspace + "/catalog"
	for _, role := range []identity.Role{identity.Owner, identity.Admin, identity.Member, identity.Viewer} {
		t.Run(string(role), func(t *testing.T) {
			user := ownerID
			if role != identity.Owner {
				inv, err := auth.CreateInvitation(ctx, actor, string(role)+"@example.com", role, time.Hour, identity.AuditContext{})
				if err != nil {
					t.Fatal(err)
				}
				user, _, err = auth.CreateInvitedLocalUser(ctx, inv.Secret, "long-enough-password", identity.AuditContext{})
				if err != nil {
					t.Fatal(err)
				}
			}
			principal, err := auth.Authorize(ctx, user, workspaceID, identity.ResourcesRead)
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
			seed := serveAuthorized(routes, http.MethodPost, path, `{"name":"Seed","prices":[{"mode":"manual","currency":"USD","variant":"Initial"}]}`, ownerToken, nil)
			var entry struct {
				ID       string `json:"id"`
				Revision int64  `json:"revision"`
			}
			if err := json.Unmarshal(seed.Body.Bytes(), &entry); err != nil || entry.ID == "" {
				t.Fatal(seed.Body.String())
			}
			entryPath := path + "/" + entry.ID
			seedPrice := serveAuthorized(routes, http.MethodPost, entryPath+"/prices", `{"mode":"manual","currency":"USD","variant":"Visit"}`, ownerToken, nil)
			var row struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(seedPrice.Body.Bytes(), &row); err != nil || row.ID == "" {
				t.Fatal(seedPrice.Body.String())
			}
			if r := serveAuthorized(routes, http.MethodGet, path, "", token.Secret, nil); r.Code != http.StatusOK {
				t.Fatalf("read: %d", r.Code)
			}
			writable := role == identity.Owner || role == identity.Admin
			for _, op := range []struct {
				method, target, body string
				success              int
			}{
				{http.MethodPost, path, `{"name":"New","prices":[{"mode":"manual","currency":"USD","variant":"Initial"}]}`, http.StatusCreated},
				{http.MethodPut, entryPath, `{"name":"Edited"}`, http.StatusNoContent},
				{http.MethodPost, entryPath + "/prices", `{"mode":"manual","currency":"USD","variant":"Other"}`, http.StatusCreated},
				{http.MethodPut, entryPath + "/prices/" + row.ID, `{"mode":"manual","currency":"USD","variant":"Edited"}`, http.StatusNoContent},
				{http.MethodDelete, entryPath + "/prices/" + row.ID, "", http.StatusNoContent},
				{http.MethodPost, entryPath + "/archive", "", http.StatusNoContent},
				{http.MethodPost, entryPath + "/unarchive", "", http.StatusNoContent},
				{http.MethodDelete, entryPath, "", http.StatusNoContent},
			} {
				want := http.StatusForbidden
				if writable {
					want = op.success
				}
				var before string
				if !writable {
					list := serveAuthorized(routes, http.MethodGet, path+"?includeArchived=true", "", ownerToken, nil)
					if list.Code != http.StatusOK {
						t.Fatalf("catalog before denied write = %d %s", list.Code, list.Body.String())
					}
					before = list.Body.String()
				}
				result := serveAuthorized(routes, op.method, op.target, op.body, token.Secret, map[string]string{"If-Match": etag(entry.Revision)})
				if result.Code != want {
					t.Errorf("%s %s: %d want %d", op.method, op.target, result.Code, want)
				}
				if !writable {
					list := serveAuthorized(routes, http.MethodGet, path+"?includeArchived=true", "", ownerToken, nil)
					if list.Code != http.StatusOK || list.Body.String() != before {
						t.Fatalf("%s %s changed catalog despite refusal: %d %s", op.method, op.target, list.Code, list.Body.String())
					}
				}
			}
		})
	}
}
