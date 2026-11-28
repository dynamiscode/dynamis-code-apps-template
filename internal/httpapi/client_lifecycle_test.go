package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestClientArchiveUnarchiveAndDeleteOverREST(t *testing.T) {
	handler, db, workspaceID, token := testHandler(t)
	base := "/api/v1/workspaces/" + workspaceID + "/clients"
	created := serveAuthorized(handler, http.MethodPost, base, `{"displayName":"Ada"}`, token, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	var client struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &client); err != nil {
		t.Fatal(err)
	}
	clientPath := base + "/" + client.ID

	for _, probe := range []string{"1", "00000000000000000000000000000000"} {
		assertProblem(t, serveAuthorized(handler, http.MethodPost, base+"/"+probe+"/archive", "", token, nil), http.StatusNotFound, "not-found")
		assertProblem(t, serveAuthorized(handler, http.MethodPost, base+"/"+probe+"/unarchive", "", token, nil), http.StatusNotFound, "not-found")
		assertProblem(t, serveAuthorized(handler, http.MethodDelete, base+"/"+probe, "", token, nil), http.StatusNotFound, "not-found")
	}

	archived := serveAuthorized(handler, http.MethodPost, clientPath+"/archive", "", token, nil)
	if archived.Code != http.StatusOK {
		t.Fatalf("archive = %d %s", archived.Code, archived.Body.String())
	}
	if archived.Header().Get("ETag") == "" {
		t.Fatal("archive response has no ETag")
	}
	var body map[string]any
	if err := json.Unmarshal(archived.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["archivedAt"] == nil || body["version"] != float64(2) {
		t.Fatalf("archive body = %v; want archivedAt set and version 2", body)
	}
	if again := serveAuthorized(handler, http.MethodPost, clientPath+"/archive", "", token, nil); again.Code != http.StatusOK {
		t.Fatalf("repeat archive = %d %s", again.Code, again.Body.String())
	}

	if list := serveAuthorized(handler, http.MethodGet, base, "", token, nil); list.Code != http.StatusOK || !listHasClients(t, list.Body.Bytes(), 0) {
		t.Fatalf("default list after archive = %d %s; want no clients", list.Code, list.Body.String())
	}
	if list := serveAuthorized(handler, http.MethodGet, base+"?archived=archived", "", token, nil); list.Code != http.StatusOK || !listHasClients(t, list.Body.Bytes(), 1) {
		t.Fatalf("archived list = %d %s; want one client", list.Code, list.Body.String())
	}
	assertProblem(t, serveAuthorized(handler, http.MethodGet, base+"?archived=bogus", "", token, nil), http.StatusBadRequest, "invalid-request")

	unarchived := serveAuthorized(handler, http.MethodPost, clientPath+"/unarchive", "", token, nil)
	if unarchived.Code != http.StatusOK {
		t.Fatalf("unarchive = %d %s", unarchived.Code, unarchived.Body.String())
	}
	if err := json.Unmarshal(unarchived.Body.Bytes(), &body); err != nil || body["archivedAt"] != nil {
		t.Fatalf("unarchive body = %v, %v; want archivedAt null", body, err)
	}

	ctx := context.Background()
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	var userID identity.UserID
	if err := db.QueryRow("SELECT user_id FROM workspace_members LIMIT 1").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	var parsed identity.WorkspaceID
	if err := db.QueryRow("SELECT id FROM workspaces LIMIT 1").Scan(&parsed); err != nil {
		t.Fatal(err)
	}
	principal, err := auth.Authorize(ctx, userID, parsed, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	readOnly, err := auth.CreateAPIToken(ctx, principal, "read-only", []identity.Permission{identity.ResourcesRead}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	writeOnly, err := auth.CreateAPIToken(ctx, principal, "write-only", []identity.Permission{identity.ResourcesWrite}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	assertProblem(t, serveAuthorized(handler, http.MethodPost, clientPath+"/archive", "", readOnly.Secret, nil), http.StatusForbidden, "insufficient-scope")
	if response := serveAuthorized(handler, http.MethodDelete, clientPath, "", writeOnly.Secret, nil); response.Code != http.StatusForbidden {
		t.Fatalf("write-only token delete = %d %s; want 403", response.Code, response.Body.String())
	}
	afterDenied := serveAuthorized(handler, http.MethodGet, clientPath, "", token, nil)
	var unchanged struct {
		ArchivedAt *string `json:"archivedAt"`
		Version    int     `json:"version"`
	}
	if err := json.Unmarshal(afterDenied.Body.Bytes(), &unchanged); afterDenied.Code != http.StatusOK || err != nil || unchanged.ArchivedAt != nil || unchanged.Version != 3 {
		t.Fatalf("client after denied writes = %d %s, %v", afterDenied.Code, afterDenied.Body.String(), err)
	}

	if deleted := serveAuthorized(handler, http.MethodDelete, clientPath, "", token, nil); deleted.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", deleted.Code, deleted.Body.String())
	}
	assertProblem(t, serveAuthorized(handler, http.MethodGet, clientPath, "", token, nil), http.StatusNotFound, "not-found")
	assertProblem(t, serveAuthorized(handler, http.MethodDelete, clientPath, "", token, nil), http.StatusNotFound, "not-found")
}

func listHasClients(t *testing.T, raw []byte, want int) bool {
	t.Helper()
	var page struct {
		Clients []map[string]any `json:"clients"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	return len(page.Clients) == want
}
