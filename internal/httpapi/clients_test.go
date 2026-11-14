package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// Malformed, unknown, foreign, and integer client IDs must return the same
// not-found response as a missing client.
func TestClientPathIsShortIDOnlyAndUniform(t *testing.T) {
	handler, db, workspaceID, token := testHandler(t)
	created := serveAuthorized(handler, http.MethodPost, "/api/v1/workspaces/"+workspaceID+"/clients",
		`{"displayName":"Ada"}`, token, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	var client map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &client); err != nil {
		t.Fatal(err)
	}
	shortID := client["id"].(string)
	var integerKey int64
	if err := db.QueryRow("SELECT id FROM clients").Scan(&integerKey); err != nil {
		t.Fatal(err)
	}

	probes := map[string]string{
		"malformed":          "1",
		"integer key":        strconv.FormatInt(integerKey, 10),
		"hex legacy form":    "00000000000000000000000000000000",
		"workspace short ID": workspaceID,
		"unknown short ID":   clients.ClientID(999999).String(),
	}
	var reference map[string]any
	for name, probe := range probes {
		response := serveAuthorized(handler, http.MethodGet, "/api/v1/workspaces/"+workspaceID+"/clients/"+probe, "", token, nil)
		assertProblem(t, response, http.StatusNotFound, "not-found")
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		delete(body, "instance")
		delete(body, "requestId")
		if reference == nil {
			reference = body
			continue
		}
		if !reflect.DeepEqual(reference, body) {
			t.Fatalf("%s response = %v, want the same body as %v", name, body, reference)
		}
	}
	response := serveAuthorized(handler, http.MethodGet, "/api/v1/workspaces/"+workspaceID+"/clients/"+shortID, "", token, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("get by short ID = %d %s", response.Code, response.Body.String())
	}
}

func TestClientCreateAndUpdateUseShortIDsAndValidation(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	base := "/api/v1/workspaces/" + workspaceID + "/clients"

	created := serveAuthorized(handler, http.MethodPost, base, `{"displayName":"Ada","email":"ada@example.com"}`, token, nil)
	if created.Code != http.StatusCreated || created.Header().Get("Location") == "" || created.Header().Get("ETag") != `"v1"` {
		t.Fatalf("create = %d headers %v %s", created.Code, created.Header(), created.Body.String())
	}
	var client struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &client); err != nil {
		t.Fatal(err)
	}
	if _, err := clients.ParseClientID(client.ID); err != nil || client.DisplayName != "Ada" {
		t.Fatalf("created client = %+v, %v", client, err)
	}

	assertProblem(t, serveAuthorized(handler, http.MethodPost, base, `{"displayName":"  "}`, token, nil), http.StatusBadRequest, "invalid-request")
	assertProblem(t, serveAuthorized(handler, http.MethodPost, base, `{"displayName":"Ada","email":"Ada <ada@example.com>"}`, token, nil), http.StatusBadRequest, "invalid-request")
	assertProblem(t, serveAuthorized(handler, http.MethodPost, base, `{"displayName":"Ada","id":7}`, token, nil), http.StatusBadRequest, "invalid-request")

	itemPath := base + "/" + client.ID
	assertProblem(t, serveAuthorized(handler, http.MethodPatch, itemPath, `{"phone":"1"}`, token, nil), http.StatusPreconditionRequired, "precondition-required")
	assertProblem(t, serveAuthorized(handler, http.MethodPatch, itemPath, `{"phone":"1"}`, token, map[string]string{"If-Match": `"v9"`}), http.StatusPreconditionFailed, "precondition-failed")
	updated := serveAuthorized(handler, http.MethodPatch, itemPath, `{"phone":"+1 555 0100"}`, token, map[string]string{"If-Match": `"v1"`})
	if updated.Code != http.StatusOK || updated.Header().Get("ETag") != `"v2"` {
		t.Fatalf("update = %d %s", updated.Code, updated.Body.String())
	}
	list := serveAuthorized(handler, http.MethodGet, base, "", token, nil)
	if list.Code != http.StatusOK || !json.Valid(list.Body.Bytes()) {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	var page struct {
		Clients []map[string]any `json:"clients"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &page); err != nil || len(page.Clients) != 1 {
		t.Fatalf("list page = %+v, %v", page, err)
	}
	if got := serveAuthorized(handler, http.MethodGet, base+"?limit=0", "", token, nil); got.Code != http.StatusBadRequest {
		t.Fatalf("zero limit = %d", got.Code)
	}
}

func TestClientWritesNeedWriteScope(t *testing.T) {
	handler, db, workspaceID, _ := testHandler(t)
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
	response := serveAuthorized(handler, http.MethodPost, "/api/v1/workspaces/"+workspaceID+"/clients",
		`{"displayName":"Blocked"}`, readOnly.Secret, nil)
	assertProblem(t, response, http.StatusForbidden, "insufficient-scope")
}
