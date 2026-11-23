package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalogOperationsDocumentRoleAndScope(t *testing.T) {
	var document struct {
		Paths map[string]map[string]struct {
			Description string `json:"description"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(OpenAPI, &document); err != nil {
		t.Fatal(err)
	}
	for path, methods := range map[string][]string{
		"/api/v1/workspaces/{workspaceId}/catalog":                            {"get", "post"},
		"/api/v1/workspaces/{workspaceId}/catalog/{entryId}":                  {"put", "delete"},
		"/api/v1/workspaces/{workspaceId}/catalog/{entryId}/prices":           {"post"},
		"/api/v1/workspaces/{workspaceId}/catalog/{entryId}/prices/{priceId}": {"put", "delete"},
		"/api/v1/workspaces/{workspaceId}/catalog/{entryId}/archive":          {"post"},
		"/api/v1/workspaces/{workspaceId}/catalog/{entryId}/unarchive":        {"post"},
	} {
		for _, method := range methods {
			description := document.Paths[path][method].Description
			scope := "resources:write"
			if method == "get" {
				scope = "resources:read"
			}
			if !strings.Contains(description, scope) || method != "get" && (!strings.Contains(description, "owner") || !strings.Contains(description, "admin")) {
				t.Errorf("%s %s missing role or scope: %q", method, path, description)
			}
		}
	}
}
