package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestGeneratedContractMatchesEmbeddedOpenAPI(t *testing.T) {
	sum := sha256.Sum256(OpenAPI)
	if hex.EncodeToString(sum[:]) != OpenAPISHA256 {
		t.Fatal("generated contract is stale; run go generate ./api")
	}
	var document struct {
		OpenAPI string `json:"openapi"`
	}
	if err := json.Unmarshal(OpenAPI, &document); err != nil {
		t.Fatal(err)
	}
	if document.OpenAPI != OpenAPIVersion || len(Operations) == 0 {
		t.Fatalf("contract version = %q, operations = %d", document.OpenAPI, len(Operations))
	}
}

func TestItemsRoutesAreAbsent(t *testing.T) {
	var document struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(OpenAPI, &document); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/v1/workspaces/{workspaceId}/items",
		"/api/v1/workspaces/{workspaceId}/items/{itemId}",
		"/api/v1/workspaces/{workspaceId}/import",
	} {
		if _, ok := document.Paths[path]; ok {
			t.Errorf("removed route remains in OpenAPI: %s", path)
		}
	}
}
