package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

type contractParameter struct {
	Ref      string `json:"$ref"`
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

type contractOperation struct {
	OperationID string                `json:"operationId"`
	Security    []map[string][]string `json:"security"`
	Parameters  []contractParameter   `json:"parameters"`
}

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

func TestStableV1ItemConsumerSurface(t *testing.T) {
	var document struct {
		Paths map[string]map[string]contractOperation `json:"paths"`
	}
	if err := json.Unmarshal(OpenAPI, &document); err != nil {
		t.Fatal(err)
	}

	required := map[string]map[string]string{
		"/api/v1/workspaces/{workspaceId}/items": {
			"get": "listItems", "post": "createItem",
		},
		"/api/v1/workspaces/{workspaceId}/items/{itemId}": {
			"get": "getItem", "patch": "updateItem", "delete": "deleteItem",
		},
	}
	for path, methods := range required {
		for method, operationID := range methods {
			operation, ok := document.Paths[path][method]
			if !ok || operation.OperationID != operationID {
				t.Fatalf("missing stable operation %s %s (%s)", method, path, operationID)
			}
			if len(operation.Security) != 1 || len(operation.Security[0]) != 1 {
				t.Fatalf("%s %s must require bearer authentication", method, path)
			}
			if _, ok := operation.Security[0]["bearerAuth"]; !ok {
				t.Fatalf("%s %s must require bearer authentication", method, path)
			}
		}
	}

	list := document.Paths["/api/v1/workspaces/{workspaceId}/items"]["get"]
	for _, ref := range []string{
		"#/components/parameters/PageLimit", "#/components/parameters/PageCursor",
	} {
		if !hasParameterRef(list.Parameters, ref) {
			t.Fatalf("listItems missing %s", ref)
		}
	}
	create := document.Paths["/api/v1/workspaces/{workspaceId}/items"]["post"]
	if !hasParameterRef(create.Parameters, "#/components/parameters/IdempotencyKey") {
		t.Fatal("createItem missing idempotency key")
	}
	for _, method := range []string{"patch", "delete"} {
		item := document.Paths["/api/v1/workspaces/{workspaceId}/items/{itemId}"][method]
		if !hasRequiredParameter(item.Parameters, "If-Match") {
			t.Fatalf("%s item operation missing required If-Match", method)
		}
	}
}

func hasParameterRef(parameters []contractParameter, ref string) bool {
	for _, parameter := range parameters {
		if parameter.Ref == ref {
			return true
		}
	}
	return false
}

func hasRequiredParameter(parameters []contractParameter, name string) bool {
	for _, parameter := range parameters {
		if parameter.Name == name && parameter.Required {
			return true
		}
	}
	return false
}
