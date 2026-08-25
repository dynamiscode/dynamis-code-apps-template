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
