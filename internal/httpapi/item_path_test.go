package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"example.com/dynamis-code/apps-template/internal/items"
)

// Malformed, unknown, and foreign item IDs, including a workspace short ID, must
// return the same not-found response as a missing item.
func TestItemPathIsShortIDOnlyAndUniform(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)

	probes := map[string]string{
		"malformed":          "1",
		"hex legacy form":    "00000000000000000000000000000000",
		"workspace short ID": workspaceID,
		"unknown short ID":   items.ItemID(999999).String(),
	}
	var reference map[string]any
	for name, probe := range probes {
		response := serveAuthorized(handler, http.MethodGet, "/api/v1/workspaces/"+workspaceID+"/items/"+probe, "", token, nil)
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
