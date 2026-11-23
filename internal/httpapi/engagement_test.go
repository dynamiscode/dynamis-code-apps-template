package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

func TestStagesThroughREST(t *testing.T) {
	handler, _, workspace, token := testHandler(t)
	path := "/api/v1/workspaces/" + workspace + "/engagement-stages"
	initial := serveAuthorized(handler, http.MethodGet, path, "", token, nil)
	if initial.Code != http.StatusOK || strings.TrimSpace(initial.Body.String()) != "[]" {
		t.Fatalf("initial = %d %s", initial.Code, initial.Body.String())
	}
	created := serveAuthorized(handler, http.MethodPost, path, `{"name":"Prospect","kind":"open"}`, token, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	list := serveAuthorized(handler, http.MethodGet, path, "", token, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Prospect") {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
}
