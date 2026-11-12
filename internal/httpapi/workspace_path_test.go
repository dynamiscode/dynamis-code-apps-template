package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// Malformed, unknown, and foreign workspace IDs must return the same not-found
// response, and integer keys must never be accepted in a path.
func TestWorkspacePathIsShortIDOnlyAndUniform(t *testing.T) {
	handler, db, workspaceID, token := testHandler(t)

	ownerKey, err := identity.ParseWorkspaceID(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	var otherKey int64
	if err := db.QueryRow(
		"INSERT INTO workspaces (name, created_at) VALUES (?, ?) RETURNING id",
		"Foreign", "2026-11-11T00:00:00Z",
	).Scan(&otherKey); err != nil {
		t.Fatal(err)
	}

	probes := map[string]string{
		"malformed":        "1",
		"integer key":      "12",
		"decimal owner":    strconv.FormatInt(int64(ownerKey), 10),
		"hex legacy form":  "00000000000000000000000000000000",
		"foreign short ID": identity.WorkspaceID(otherKey).String(),
		"unknown short ID": identity.WorkspaceID(otherKey + 1000).String(),
	}
	var reference map[string]any
	for name, probe := range probes {
		response := serveAuthorized(handler, http.MethodGet, "/api/v1/workspaces/"+probe+"/items", "", token, nil)
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
}
