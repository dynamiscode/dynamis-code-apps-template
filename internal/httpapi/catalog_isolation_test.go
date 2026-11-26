package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestCatalogRESTWorkspaceIsolation(t *testing.T) {
	routes, db, workspace, token := testHandler(t)
	ctx := context.Background()
	auth, err := identity.NewService(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	ws := identity.ParseWorkspaceIDOrZero(workspace)
	seedCatalogCurrencies(t, db, workspace)
	var ownerID identity.UserID
	if err := db.QueryRowContext(ctx, "SELECT user_id FROM workspace_members WHERE workspace_id = ? AND role = 'owner'", ws).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authorize(ctx, ownerID, ws, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	other, err := auth.CreateWorkspace(ctx, owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, err := auth.Authorize(ctx, ownerID, other, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	otherToken, err := auth.CreateAPIToken(ctx, otherOwner, "other", []identity.Permission{identity.ResourcesRead, identity.ResourcesWrite}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/workspaces/" + workspace + "/catalog"
	otherPath := "/api/v1/workspaces/" + other.String() + "/catalog"
	var entry, row struct {
		ID string `json:"id"`
	}
	r := serveAuthorized(routes, http.MethodPost, path, `{"name":"Private","prices":[{"mode":"manual","currency":"USD","variant":"Initial"}]}`, token, nil)
	if err := json.Unmarshal(r.Body.Bytes(), &entry); err != nil || entry.ID == "" {
		t.Fatal(r.Body.String())
	}
	r = serveAuthorized(routes, http.MethodPost, path+"/"+entry.ID+"/prices", `{"mode":"fixed","amount":"0.12","currency":"EUR","variant":"Private"}`, token, nil)
	if err := json.Unmarshal(r.Body.Bytes(), &row); err != nil || row.ID == "" {
		t.Fatal(r.Body.String())
	}
	r = serveAuthorized(routes, http.MethodGet, otherPath+"?includeArchived=true", "", otherToken.Secret, nil)
	if r.Code != http.StatusOK || strings.Contains(r.Body.String(), "Private") {
		t.Fatalf("foreign list: %d %s", r.Code, r.Body.String())
	}
	target := otherPath + "/" + entry.ID
	for _, op := range []struct {
		method, suffix, body string
		want                 int
	}{
		{http.MethodPut, "", `{"name":"Stolen"}`, 404}, {http.MethodPost, "/archive", "", 404}, {http.MethodPost, "/unarchive", "", 404}, {http.MethodDelete, "", "", 404},
		{http.MethodPost, "/prices", `{"mode":"manual","currency":"USD","variant":"x"}`, 422}, {http.MethodPut, "/prices/" + row.ID, `{"mode":"manual","currency":"USD","variant":"x"}`, 404}, {http.MethodDelete, "/prices/" + row.ID, "", 404},
	} {
		r = serveAuthorized(routes, op.method, target+op.suffix, op.body, otherToken.Secret, map[string]string{"If-Match": `"v1"`})
		if r.Code != op.want {
			t.Errorf("%s %s: %d", op.method, op.suffix, r.Code)
		}
	}
	r = serveAuthorized(routes, http.MethodGet, path, "", token, nil)
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"amount":"0.12"`) || !strings.Contains(r.Body.String(), `"name":"Private"`) {
		t.Fatalf("original: %d %s", r.Code, r.Body.String())
	}
}
