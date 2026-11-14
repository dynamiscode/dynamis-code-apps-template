package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDeleteClientFieldOverREST(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	fieldBase := "/api/v1/workspaces/" + workspaceID + "/client-fields"
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
	fieldPath := fieldBase + "/" + field.ID

	assertProblem(t, serveAuthorized(handler, http.MethodDelete, fieldPath, "", token, nil), http.StatusConflict, "field-not-archived")
	if archived := serveAuthorized(handler, http.MethodPost, fieldPath+"/archive", "", token, nil); archived.Code != http.StatusOK {
		t.Fatalf("archive field = %d %s", archived.Code, archived.Body.String())
	}
	deleted := serveAuthorized(handler, http.MethodDelete, fieldPath, "", token, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete archived field = %d %s", deleted.Code, deleted.Body.String())
	}
	if again := serveAuthorized(handler, http.MethodDelete, fieldPath, "", token, nil); again.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d %s, want 404", again.Code, again.Body.String())
	}
	listed := serveAuthorized(handler, http.MethodGet, fieldBase, "", token, nil)
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), field.ID) {
		t.Fatalf("list after delete = %d %s, want the field gone", listed.Code, listed.Body.String())
	}
}
