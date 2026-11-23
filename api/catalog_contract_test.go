package api

import (
	"encoding/json"
	"testing"
)

func TestCatalogPriceInvariantContract(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(OpenAPI, &doc); err != nil {
		t.Fatal(err)
	}
	paths := doc["paths"].(map[string]any)
	create := paths["/api/v1/workspaces/{workspaceId}/catalog"].(map[string]any)["post"].(map[string]any)
	body := create["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	prices := body["properties"].(map[string]any)["prices"].(map[string]any)
	if prices["minItems"] != float64(1) {
		t.Fatal("catalog create must require at least one price")
	}
	required := body["required"].([]any)
	found := false
	for _, field := range required {
		found = found || field == "prices"
	}
	if !found {
		t.Fatal("catalog create must require prices")
	}
	deletePrice := paths["/api/v1/workspaces/{workspaceId}/catalog/{entryId}/prices/{priceId}"].(map[string]any)["delete"].(map[string]any)
	if _, ok := deletePrice["responses"].(map[string]any)["409"]; !ok {
		t.Fatal("last-price conflict not documented")
	}
}

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
