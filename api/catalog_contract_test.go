package api

import (
	"encoding/json"
	"testing"
)

func TestCatalogLifecycleOperationsDocumentUnauthorized(t *testing.T) {
	var document struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Ref string `json:"$ref"`
			} `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(OpenAPI, &document); err != nil {
		t.Fatal(err)
	}
	for path, method := range map[string]string{
		"/api/v1/workspaces/{workspaceId}/catalog/{entryId}":           "delete",
		"/api/v1/workspaces/{workspaceId}/catalog/{entryId}/archive":   "post",
		"/api/v1/workspaces/{workspaceId}/catalog/{entryId}/unarchive": "post",
	} {
		if got := document.Paths[path][method].Responses["401"].Ref; got != "#/components/responses/Problem" {
			t.Errorf("%s %s: 401 response ref = %q", method, path, got)
		}
	}
}
