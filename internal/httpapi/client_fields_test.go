package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestClientFieldDefineArchiveRestoreOverREST(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	base := "/api/v1/workspaces/" + workspaceID + "/client-fields"

	created := serveAuthorized(handler, http.MethodPost, base, `{"label":"Source","type":"text"}`, token, nil)
	if created.Code != http.StatusCreated || created.Header().Get("Location") == "" {
		t.Fatalf("define = %d headers %v %s", created.Code, created.Header(), created.Body.String())
	}
	var field struct {
		ID         string  `json:"id"`
		Label      string  `json:"label"`
		Type       string  `json:"type"`
		ArchivedAt *string `json:"archivedAt"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &field); err != nil {
		t.Fatal(err)
	}
	if field.Label != "Source" || field.Type != "text" || field.ArchivedAt != nil || field.ID == "" {
		t.Fatalf("define body = %+v", field)
	}
	if strings.Contains(created.Body.String(), `"slot"`) {
		t.Fatalf("slot leaks into the response: %s", created.Body.String())
	}
	if got := clients.ParseFieldIDOrZero(field.ID); got == 0 {
		t.Fatalf("response ID %q is not a client_field short ID", field.ID)
	}

	list := serveAuthorized(handler, http.MethodGet, base, "", token, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), field.ID) {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}

	archived := serveAuthorized(handler, http.MethodPost, base+"/"+field.ID+"/archive", "", token, nil)
	if archived.Code != http.StatusOK || !strings.Contains(archived.Body.String(), `"archivedAt":"`) {
		t.Fatalf("archive = %d %s", archived.Code, archived.Body.String())
	}
	forms := serveAuthorized(handler, http.MethodGet, base, "", token, nil)
	if strings.Contains(forms.Body.String(), field.ID) {
		t.Fatalf("archived field listed for forms: %s", forms.Body.String())
	}
	management := serveAuthorized(handler, http.MethodGet, base+"?includeArchived=true", "", token, nil)
	if !strings.Contains(management.Body.String(), field.ID) {
		t.Fatalf("archived field missing from management list: %s", management.Body.String())
	}

	restored := serveAuthorized(handler, http.MethodPost, base+"/"+field.ID+"/restore", "", token, nil)
	if restored.Code != http.StatusOK || strings.Contains(restored.Body.String(), `"archivedAt":"`) {
		t.Fatalf("restore = %d %s", restored.Code, restored.Body.String())
	}

	blank := serveAuthorized(handler, http.MethodPost, base, `{"label":"Age","type":"number"}`, token, nil)
	assertProblem(t, blank, http.StatusBadRequest, "invalid-request")
	unknown := serveAuthorized(handler, http.MethodPost, base+"/"+clients.FieldID(999999).String()+"/archive", "", token, nil)
	assertProblem(t, unknown, http.StatusNotFound, "not-found")
	integer := serveAuthorized(handler, http.MethodPost, base+"/1/archive", "", token, nil)
	assertProblem(t, integer, http.StatusNotFound, "not-found")
}

func TestClientFieldWritesRefuseMembers(t *testing.T) {
	handler, db, workspaceID, _ := testHandler(t)
	ctx := context.Background()
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	var ownerID identity.UserID
	var parsed identity.WorkspaceID
	if err := db.QueryRow("SELECT user_id FROM workspace_members LIMIT 1").Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT id FROM workspaces LIMIT 1").Scan(&parsed); err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authorize(ctx, ownerID, parsed, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	var memberID identity.UserID
	if err := db.QueryRow("INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id",
		"rest-member@example.com", "2026-11-13T00:00:00Z").Scan(&memberID); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddMember(ctx, owner, memberID, identity.Member, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	member, err := auth.Authorize(ctx, memberID, parsed, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	writeToken, err := auth.CreateAPIToken(ctx, member, "member-write", []identity.Permission{identity.ResourcesWrite}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	response := serveAuthorized(handler, http.MethodPost, "/api/v1/workspaces/"+workspaceID+"/client-fields",
		`{"label":"Blocked","type":"text"}`, writeToken.Secret, nil)
	assertProblem(t, response, http.StatusForbidden, "insufficient-scope")

	// An admin mints a token with the owner-level scope, then is demoted. The
	// scope gate checks the token against the current role, so the stale scope
	// no longer counts. Service-level refusal is covered in the clients package.
	var adminID identity.UserID
	if err := db.QueryRow("INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id",
		"rest-admin@example.com", "2026-11-13T00:00:00Z").Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddMember(ctx, owner, adminID, identity.Admin, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	admin, err := auth.Authorize(ctx, adminID, parsed, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	demotedToken, err := auth.CreateAPIToken(ctx, admin, "admin-workspace-update", []identity.Permission{identity.WorkspaceUpdate}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	var adminMemberID identity.MemberID
	if err := db.QueryRow("SELECT id FROM workspace_members WHERE workspace_id = ? AND user_id = ?",
		parsed, adminID).Scan(&adminMemberID); err != nil {
		t.Fatal(err)
	}
	if err := auth.ChangeMemberRole(ctx, owner, adminMemberID, identity.Member, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	response = serveAuthorized(handler, http.MethodPost, "/api/v1/workspaces/"+workspaceID+"/client-fields",
		`{"label":"Blocked","type":"text"}`, demotedToken.Secret, nil)
	assertProblem(t, response, http.StatusForbidden, "insufficient-scope")
}
