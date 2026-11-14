package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestClientValuesSetListAndClearOverREST(t *testing.T) {
	handler, db, workspaceID, token := testHandler(t)
	clientBase := "/api/v1/workspaces/" + workspaceID + "/clients"
	fieldBase := "/api/v1/workspaces/" + workspaceID + "/client-fields"

	created := serveAuthorized(handler, http.MethodPost, clientBase, `{"displayName":"Ada"}`, token, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create client = %d %s", created.Code, created.Body.String())
	}
	var client struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &client); err != nil {
		t.Fatal(err)
	}
	defined := serveAuthorized(handler, http.MethodPost, fieldBase, `{"label":"Source","type":"text"}`, token, nil)
	if defined.Code != http.StatusCreated {
		t.Fatalf("define field = %d %s", defined.Code, defined.Body.String())
	}
	var field struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(defined.Body.Bytes(), &field); err != nil {
		t.Fatal(err)
	}

	valuePath := clientBase + "/" + client.ID + "/values/" + field.ID
	assertProblem(t, serveAuthorized(handler, http.MethodPut, valuePath, `{"value":"Friend"}`, token, nil), http.StatusPreconditionRequired, "precondition-required")
	assertProblem(t, serveAuthorized(handler, http.MethodPut, valuePath, `{"value":"Friend"}`, token, map[string]string{"If-Match": `"v9"`}), http.StatusPreconditionFailed, "precondition-failed")
	assertProblem(t, serveAuthorized(handler, http.MethodPut, valuePath, `{"value":"Friend","id":7}`, token, map[string]string{"If-Match": `"v1"`}), http.StatusBadRequest, "invalid-request")
	assertProblem(t, serveAuthorized(handler, http.MethodPut, valuePath, `{"value":"   "}`, token, map[string]string{"If-Match": `"v1"`}), http.StatusBadRequest, "invalid-request")

	set := serveAuthorized(handler, http.MethodPut, valuePath, `{"value":"Friend"}`, token, map[string]string{"If-Match": `"v1"`})
	if set.Code != http.StatusOK || set.Header().Get("ETag") != `"v2"` {
		t.Fatalf("set value = %d headers %v %s", set.Code, set.Header(), set.Body.String())
	}
	listed := serveAuthorized(handler, http.MethodGet, clientBase+"/"+client.ID+"/values", "", token, nil)
	var values struct {
		Values []struct {
			FieldID string  `json:"fieldId"`
			Label   string  `json:"label"`
			Type    string  `json:"type"`
			Value   *string `json:"value"`
		} `json:"values"`
	}
	if listed.Code != http.StatusOK || json.Unmarshal(listed.Body.Bytes(), &values) != nil ||
		len(values.Values) != 1 || values.Values[0].FieldID != field.ID || values.Values[0].Label != "Source" ||
		values.Values[0].Value == nil || *values.Values[0].Value != "Friend" {
		t.Fatalf("list values = %d %s", listed.Code, listed.Body.String())
	}

	cleared := serveAuthorized(handler, http.MethodDelete, valuePath, "", token, map[string]string{"If-Match": `"v2"`})
	if cleared.Code != http.StatusOK || cleared.Header().Get("ETag") != `"v3"` {
		t.Fatalf("clear value = %d headers %v %s", cleared.Code, cleared.Header(), cleared.Body.String())
	}
	listed = serveAuthorized(handler, http.MethodGet, clientBase+"/"+client.ID+"/values", "", token, nil)
	if err := json.Unmarshal(listed.Body.Bytes(), &values); err != nil || len(values.Values) != 1 || values.Values[0].Value != nil {
		t.Fatalf("list after clear = %s, %v", listed.Body.String(), err)
	}

	var integerClientKey, integerFieldKey int64
	if err := db.QueryRow("SELECT id FROM clients").Scan(&integerClientKey); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT id FROM client_fields").Scan(&integerFieldKey); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		clientBase + "/" + strconv.FormatInt(integerClientKey, 10) + "/values",
		clientBase + "/1/values",
	} {
		assertProblem(t, serveAuthorized(handler, http.MethodGet, path, "", token, nil), http.StatusNotFound, "not-found")
	}
	for _, path := range []string{
		clientBase + "/" + client.ID + "/values/" + strconv.FormatInt(integerFieldKey, 10),
		clientBase + "/" + client.ID + "/values/" + clients.FieldID(999999).String(),
	} {
		assertProblem(t, serveAuthorized(handler, http.MethodPut, path, `{"value":"Ghost"}`, token, map[string]string{"If-Match": `"v3"`}), http.StatusNotFound, "not-found")
	}
}

func TestClientValuesReadOnlyTokenReadsButCannotWrite(t *testing.T) {
	handler, db, workspaceID, token := testHandler(t)
	clientBase := "/api/v1/workspaces/" + workspaceID + "/clients"
	created := serveAuthorized(handler, http.MethodPost, clientBase, `{"displayName":"Ada"}`, token, nil)
	var client struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &client); err != nil {
		t.Fatal(err)
	}
	defined := serveAuthorized(handler, http.MethodPost, "/api/v1/workspaces/"+workspaceID+"/client-fields", `{"label":"Source","type":"text"}`, token, nil)
	var field struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(defined.Body.Bytes(), &field); err != nil {
		t.Fatal(err)
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

	valuePath := clientBase + "/" + client.ID + "/values/" + field.ID
	if listed := serveAuthorized(handler, http.MethodGet, clientBase+"/"+client.ID+"/values", "", readOnly.Secret, nil); listed.Code != http.StatusOK {
		t.Fatalf("read-only list = %d %s", listed.Code, listed.Body.String())
	}
	assertProblem(t, serveAuthorized(handler, http.MethodPut, valuePath, `{"value":"Blocked"}`, readOnly.Secret, map[string]string{"If-Match": `"v1"`}), http.StatusForbidden, "insufficient-scope")
	assertProblem(t, serveAuthorized(handler, http.MethodDelete, valuePath, "", readOnly.Secret, map[string]string{"If-Match": `"v1"`}), http.StatusForbidden, "insufficient-scope")
}
