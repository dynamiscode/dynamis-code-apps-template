package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestEngagementLabelThroughREST(t *testing.T) {
	handler, _, workspace, token := testHandler(t)
	path := "/api/v1/workspaces/" + workspace + "/engagement-label"
	if r := serveAuthorized(handler, http.MethodPut, path, `{"label":"Matter"}`, token, nil); r.Code != http.StatusNoContent {
		t.Fatalf("set: %d %s", r.Code, r.Body.String())
	}
	if r := serveAuthorized(handler, http.MethodGet, path, "", token, nil); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"label":"Matter"`) {
		t.Fatalf("get: %d %s", r.Code, r.Body.String())
	}
}

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

func TestEngagementThroughREST(t *testing.T) {
	handler, db, workspace, token := testHandler(t)
	base := "/api/v1/workspaces/" + workspace
	client := serveAuthorized(handler, http.MethodPost, base+"/clients", `{"displayName":"Client"}`, token, nil)
	if client.Code != http.StatusCreated {
		t.Fatalf("client: %d %s", client.Code, client.Body.String())
	}
	var c struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(client.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	noStage := serveAuthorized(handler, http.MethodPost, base+"/engagements", `{"clientId":"`+c.ID+`"}`, token, nil)
	if noStage.Code != http.StatusBadRequest || !strings.Contains(noStage.Body.String(), "open stage") {
		t.Fatalf("no stage: %d %s", noStage.Code, noStage.Body.String())
	}
	stage := serveAuthorized(handler, http.MethodPost, base+"/engagement-stages", `{"name":"Open","kind":"open"}`, token, nil)
	if stage.Code != http.StatusCreated {
		t.Fatal(stage.Body.String())
	}
	created := serveAuthorized(handler, http.MethodPost, base+"/engagements", `{"clientId":"`+c.ID+`","notes":"Other person"}`, token, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var e struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	got := serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", token, nil)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "Other person") {
		t.Fatalf("get: %d %s", got.Code, got.Body.String())
	}
	ctx := context.Background()
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	var userID identity.UserID
	var workspaceID identity.WorkspaceID
	if err := db.QueryRow("SELECT user_id, workspace_id FROM workspace_members LIMIT 1").Scan(&userID, &workspaceID); err != nil {
		t.Fatal(err)
	}
	principal, err := auth.Authorize(ctx, userID, workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	readOnly, err := auth.CreateAPIToken(ctx, principal, "read-only", []identity.Permission{identity.ResourcesRead}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actionPath := base + "/engagements/" + e.ID + "/next-action"
	quotePath := base + "/engagements/" + e.ID + "/quote"
	assigneePath := base + "/engagements/" + e.ID + "/assignee"
	if r := serveAuthorized(handler, http.MethodPut, assigneePath, `{"assigneeId":"`+userID.String()+`"}`, token, nil); r.Code != http.StatusNoContent {
		t.Fatalf("assign: %d %s", r.Code, r.Body.String())
	}
	if r := serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", readOnly.Secret, nil); !strings.Contains(r.Body.String(), `"assigneeId":"`+userID.String()+`"`) {
		t.Fatalf("assignment read: %s", r.Body.String())
	}
	assertProblem(t, serveAuthorized(handler, http.MethodPut, assigneePath, `{}`, readOnly.Secret, nil), http.StatusForbidden, "insufficient-scope")
	assertProblem(t, serveAuthorized(handler, http.MethodPut, assigneePath, `{"assigneeId":"invalid"}`, token, nil), http.StatusBadRequest, "invalid-request")
	assertProblem(t, serveAuthorized(handler, http.MethodPut, assigneePath, `{"assigneeId":"`+identity.UserID(999999).String()+`"}`, token, nil), http.StatusBadRequest, "invalid-request")
	if r := serveAuthorized(handler, http.MethodPut, assigneePath, `{}`, token, nil); r.Code != http.StatusNoContent {
		t.Fatalf("unassign: %d %s", r.Code, r.Body.String())
	}
	if r := serveAuthorized(handler, http.MethodPut, quotePath, `{"status":"sent","amount":"120.50","dateSent":"2026-11-23"}`, token, nil); r.Code != http.StatusNoContent {
		t.Fatalf("quote: %d %s", r.Code, r.Body.String())
	}
	if r := serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", readOnly.Secret, nil); !strings.Contains(r.Body.String(), `"amount":"120.50"`) {
		t.Fatalf("quote read: %s", r.Body.String())
	}
	assertProblem(t, serveAuthorized(handler, http.MethodPut, quotePath, `{"status":"wrong"}`, token, nil), http.StatusBadRequest, "invalid-request")
	assertProblem(t, serveAuthorized(handler, http.MethodPut, quotePath, `{"status":"sent"}`, readOnly.Secret, nil), http.StatusForbidden, "insufficient-scope")
	updated := serveAuthorized(handler, http.MethodPut, actionPath, `{"text":"Call","dueDate":"2020-01-01","ownerId":"`+userID.String()+`"}`, token, nil)
	if updated.Code != http.StatusNoContent {
		t.Fatalf("set action: %d %s", updated.Code, updated.Body.String())
	}
	got = serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", readOnly.Secret, nil)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"text":"Call"`) {
		t.Fatalf("read action: %d %s", got.Code, got.Body.String())
	}
	due := serveAuthorized(handler, http.MethodGet, base+"/engagements/due", "", readOnly.Secret, nil)
	if due.Code != http.StatusOK || !strings.Contains(due.Body.String(), e.ID) {
		t.Fatalf("due: %d %s", due.Code, due.Body.String())
	}
	assertProblem(t, serveAuthorized(handler, http.MethodPut, actionPath, `{}`, readOnly.Secret, nil), http.StatusForbidden, "insufficient-scope")
	assertProblem(t, serveAuthorized(handler, http.MethodPost, base+"/engagements", `{"clientId":"`+c.ID+`"}`, readOnly.Secret, nil), http.StatusForbidden, "insufficient-scope")
	if got := serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", readOnly.Secret, nil); got.Code != http.StatusOK {
		t.Fatalf("read only get: %d", got.Code)
	}
	assertProblem(t, serveAuthorized(handler, http.MethodDelete, base+"/clients/"+c.ID, "", token, nil), http.StatusConflict, "client-has-engagements")
}
