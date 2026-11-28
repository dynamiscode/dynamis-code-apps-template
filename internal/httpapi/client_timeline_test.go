package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestClientTimelineREST(t *testing.T) {
	h, db, ws, token := testHandlerWithBodyLimit(t, nil, "8192")
	base := "/api/v1/workspaces/" + ws + "/clients"
	create := func() string {
		t.Helper()
		r := serveAuthorized(h, http.MethodPost, base, `{"displayName":"Ada"}`, token, nil)
		if r.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", r.Code, r.Body.String())
		}
		var v struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(r.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v.ID
	}
	path := base + "/" + create() + "/timeline"
	first := serveAuthorized(h, http.MethodPost, path, `{"type":"note","text":"first"}`, token, nil)
	if first.Code != http.StatusCreated {
		t.Fatalf("add: %d %s", first.Code, first.Body.String())
	}
	var entry struct {
		ID        string `json:"id"`
		AuthorID  string `json:"authorId"`
		CreatedAt string `json:"createdAt"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &entry); err != nil || entry.ID == "" || entry.AuthorID == "" || entry.CreatedAt == "" {
		t.Fatalf("entry: %+v %v", entry, err)
	}
	second := serveAuthorized(h, http.MethodPost, path, `{"type":"call","channel":"phone","text":"second","correctsId":"`+entry.ID+`"}`, token, nil)
	if second.Code != http.StatusCreated {
		t.Fatalf("correction: %d %s", second.Code, second.Body.String())
	}
	list := serveAuthorized(h, http.MethodGet, path, "", token, nil)
	var entries []struct {
		ID         string  `json:"id"`
		Type       string  `json:"type"`
		Channel    string  `json:"channel"`
		Text       string  `json:"text"`
		CorrectsID *string `json:"correctsId"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &entries); list.Code != http.StatusOK || err != nil || len(entries) != 2 || entries[0].ID != entry.ID || entries[1].Type != "call" || entries[1].Channel != "phone" || entries[1].Text != "second" || entries[1].CorrectsID == nil || *entries[1].CorrectsID != entry.ID {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
	page := serveAuthorized(h, http.MethodGet, path+"?limit=1&page=2", "", token, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `"text":"second"`) || strings.Contains(page.Body.String(), `"text":"first"`) {
		t.Fatalf("second page: %d %s", page.Code, page.Body.String())
	}
	for i := 0; i < 49; i++ {
		if r := serveAuthorized(h, http.MethodPost, path, `{"type":"note","text":"extra"}`, token, nil); r.Code != http.StatusCreated {
			t.Fatalf("extra entry: %d %s", r.Code, r.Body.String())
		}
	}
	var defaultPage []json.RawMessage
	response := serveAuthorized(h, http.MethodGet, path, "", token, nil)
	if err := json.Unmarshal(response.Body.Bytes(), &defaultPage); response.Code != http.StatusOK || err != nil || len(defaultPage) != 50 {
		t.Fatalf("default page: status %d, %d entries, %v", response.Code, len(defaultPage), err)
	}
	for _, query := range []string{"?limit=101", "?page=0", "?limit=1&limit=2", "?unknown=1"} {
		assertProblem(t, serveAuthorized(h, http.MethodGet, path+query, "", token, nil), http.StatusBadRequest, "invalid-request")
	}
	for _, body := range []string{`{"type":"bad","text":"x"}`, `{"type":"note","channel":"bad","text":"x"}`, `{"type":"note","text":" "}`, `{"type":"note","text":"x","correctsId":"bad"}`, `{"type":"note","text":"x","correctsId":"` + clients.TimelineEntryID(999999).String() + `"}`} {
		assertProblem(t, serveAuthorized(h, http.MethodPost, path, body, token, nil), http.StatusBadRequest, "invalid-request")
	}
	tooLong := serveAuthorized(h, http.MethodPost, path, `{"type":"note","text":"`+strings.Repeat("x", 2001)+`"}`, token, nil)
	assertProblem(t, tooLong, http.StatusBadRequest, "invalid-request")
	if !strings.Contains(tooLong.Body.String(), "2000") {
		t.Fatalf("missing length detail: %s", tooLong.Body.String())
	}
	other := base + "/" + create() + "/timeline"
	assertProblem(t, serveAuthorized(h, http.MethodPost, other, `{"type":"note","text":"x","correctsId":"`+entry.ID+`"}`, token, nil), http.StatusBadRequest, "invalid-request")
	missing := serveAuthorized(h, http.MethodGet, base+"/"+clients.ClientID(999999).String()+"/timeline", "", token, nil)
	assertProblem(t, missing, http.StatusNotFound, "not-found")
	var foreignWorkspace int64
	if err := db.QueryRow("INSERT INTO workspaces (name, created_at) VALUES (?, ?) RETURNING id", "Foreign", "2026-11-11T00:00:00Z").Scan(&foreignWorkspace); err != nil {
		t.Fatal(err)
	}
	foreignID := create()
	parsedForeign, err := clients.ParseClientID(foreignID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE clients SET workspace_id = ? WHERE id = ?", foreignWorkspace, parsedForeign); err != nil {
		t.Fatal(err)
	}
	foreign := serveAuthorized(h, http.MethodGet, base+"/"+foreignID+"/timeline", "", token, nil)
	assertProblem(t, foreign, http.StatusNotFound, "not-found")
	var missingBody, foreignBody map[string]any
	if err := json.Unmarshal(missing.Body.Bytes(), &missingBody); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(foreign.Body.Bytes(), &foreignBody); err != nil {
		t.Fatal(err)
	}
	delete(missingBody, "instance")
	delete(missingBody, "requestId")
	delete(foreignBody, "instance")
	delete(foreignBody, "requestId")
	if !reflect.DeepEqual(missingBody, foreignBody) {
		t.Fatalf("foreign = %v, missing = %v", foreignBody, missingBody)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	var uid identity.UserID
	var wid identity.WorkspaceID
	if err := db.QueryRow("SELECT user_id FROM workspace_members LIMIT 1").Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT id FROM workspaces LIMIT 1").Scan(&wid); err != nil {
		t.Fatal(err)
	}
	principal, err := auth.Authorize(context.Background(), uid, wid, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	readOnly, err := auth.CreateAPIToken(context.Background(), principal, "read-only", []identity.Permission{identity.ResourcesRead}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if r := serveAuthorized(h, http.MethodGet, path, "", readOnly.Secret, nil); r.Code != http.StatusOK {
		t.Fatalf("read-only list: %d %s", r.Code, r.Body.String())
	}
	assertProblem(t, serveAuthorized(h, http.MethodPost, path, `{"type":"note","text":"x"}`, readOnly.Secret, nil), http.StatusForbidden, "insufficient-scope")
	afterDenied := serveAuthorized(h, http.MethodGet, path+"?limit=100", "", token, nil)
	var remaining []json.RawMessage
	if err := json.Unmarshal(afterDenied.Body.Bytes(), &remaining); afterDenied.Code != http.StatusOK || err != nil || len(remaining) != 51 {
		t.Fatalf("timeline after denied write = %d %s, %v", afterDenied.Code, afterDenied.Body.String(), err)
	}
}
