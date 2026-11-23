package httpapi

import (
	"encoding/json"
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
	var first struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	secondResponse := serveAuthorized(handler, http.MethodPost, path, `{"name":"Won","kind":"won"}`, token, nil)
	var second struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(secondResponse.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	reordered := serveAuthorized(handler, http.MethodPost, path+"/reorder", `{"ids":["`+second.ID+`","`+first.ID+`"]}`, token, nil)
	if reordered.Code != http.StatusNoContent {
		t.Fatalf("reorder = %d %s", reordered.Code, reordered.Body.String())
	}
	archived := serveAuthorized(handler, http.MethodPost, path+"/"+first.ID+"/archive", "", token, nil)
	if archived.Code != http.StatusNoContent {
		t.Fatalf("archive = %d %s", archived.Code, archived.Body.String())
	}
	list := serveAuthorized(handler, http.MethodGet, path, "", token, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"archived":true`) || strings.Index(list.Body.String(), "Won") > strings.Index(list.Body.String(), "Prospect") {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
}
