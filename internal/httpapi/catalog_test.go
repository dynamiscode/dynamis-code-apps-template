package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestCatalogList(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	path := "/api/v1/workspaces/" + workspaceID + "/catalog"
	response := serveAuthorized(handler, http.MethodGet, path, "", token, nil)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("catalog = %d %s", response.Code, response.Body.String())
	}
	foreignPath := "/api/v1/workspaces/" + identity.WorkspaceID(999999).String() + "/catalog"
	foreign := serveAuthorized(handler, http.MethodGet, foreignPath, "", token, nil)
	if foreign.Code == http.StatusOK {
		t.Fatal("catalog returned 200 outside workspace")
	}
	denied := serveAuthorized(handler, http.MethodGet, path, "", "invalid", nil)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d", denied.Code)
	}
}
