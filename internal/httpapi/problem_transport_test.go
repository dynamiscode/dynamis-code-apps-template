package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProblemTransportEscapesMarkupAndPreservesText(t *testing.T) {
	const detail = "<button>Retry</button>"
	result := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/example", nil)
	writeProblem(result, request, http.StatusBadRequest, "invalid-input", detail)
	if result.Code != http.StatusBadRequest || result.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("problem response = %d, MIME %q", result.Code, result.Header().Get("Content-Type"))
	}
	if strings.ContainsAny(result.Body.String(), "<>") {
		t.Fatal("problem transport emitted literal markup delimiters")
	}
	var response problem
	if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil || response.Detail != detail {
		t.Fatalf("problem JSON lost supplied text: %v", err)
	}
}
