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
	if reordered.Code != http.StatusBadRequest {
		t.Fatalf("reorder = %d %s", reordered.Code, reordered.Body.String())
	}
	count := serveAuthorized(handler, http.MethodGet, path+"/"+first.ID+"/engagement-count", "", token, nil)
	if count.Code != http.StatusOK || !strings.Contains(count.Body.String(), `"count":0`) {
		t.Fatalf("count = %d %s", count.Code, count.Body.String())
	}
	snapshot := strings.TrimSpace(serveAuthorized(handler, http.MethodGet, path, "", token, nil).Body.String())
	request := `{"ids":["` + second.ID + `","` + first.ID + `"],"snapshot":` + snapshot + `,"movedId":"` + first.ID + `","newKind":"won","expectedCount":0,"confirmed":false}`
	assertProblem(t, serveAuthorized(handler, http.MethodPost, path+"/reorder", request, token, nil), http.StatusConflict, "confirmation-required")
	request = strings.Replace(request, `"confirmed":false`, `"confirmed":true`, 1)
	if response := serveAuthorized(handler, http.MethodPost, path+"/reorder", request, token, nil); response.Code != http.StatusNoContent {
		t.Fatalf("confirmed move = %d %s", response.Code, response.Body.String())
	}
	assertProblem(t, serveAuthorized(handler, http.MethodPost, path+"/reorder", request, token, nil), http.StatusConflict, "stale-stages")
	archived := serveAuthorized(handler, http.MethodPost, path+"/"+first.ID+"/archive", "", token, nil)
	if archived.Code != http.StatusNoContent {
		t.Fatalf("archive = %d %s", archived.Code, archived.Body.String())
	}
	list := serveAuthorized(handler, http.MethodGet, path, "", token, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"archived":true`) || !strings.Contains(list.Body.String(), "Won") || !strings.Contains(list.Body.String(), "Prospect") || strings.Index(list.Body.String(), "Won") > strings.Index(list.Body.String(), "Prospect") {
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
	assertProblem(t, serveAuthorized(handler, http.MethodPost, base+"/engagements", `{"clientId":"`+c.ID+`"}`, token, nil), http.StatusBadRequest, "invalid-request")
	noStage := serveAuthorized(handler, http.MethodPost, base+"/engagements", `{"name":"Engagement","clientId":"`+c.ID+`"}`, token, nil)
	if noStage.Code != http.StatusBadRequest {
		t.Fatalf("no stage: %d %s", noStage.Code, noStage.Body.String())
	}
	stage := serveAuthorized(handler, http.MethodPost, base+"/engagement-stages", `{"name":"Open","kind":"open"}`, token, nil)
	if stage.Code != http.StatusCreated {
		t.Fatal(stage.Body.String())
	}
	var open struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(stage.Body.Bytes(), &open); err != nil {
		t.Fatal(err)
	}
	assertProblem(t, serveAuthorized(handler, http.MethodPost, base+"/engagements", `{"name":"Engagement","clientId":"`+c.ID+`"}`, token, nil), http.StatusBadRequest, "invalid-request")
	created := serveAuthorized(handler, http.MethodPost, base+"/engagements", `{"name":"Engagement","clientId":"`+c.ID+`","stageId":"`+open.ID+`","notes":"Other person"}`, token, nil)
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
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "Other person") || !strings.Contains(got.Body.String(), `"name":"Engagement"`) {
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
	assertDeniedUnchanged := func(method, path, body string) {
		t.Helper()
		before := serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", token, nil)
		if before.Code != http.StatusOK {
			t.Fatalf("read before denied write: %d %s", before.Code, before.Body.String())
		}
		assertProblem(t, serveAuthorized(handler, method, path, body, readOnly.Secret, nil), http.StatusForbidden, "insufficient-scope")
		after := serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", token, nil)
		if after.Code != http.StatusOK || after.Body.String() != before.Body.String() {
			t.Fatalf("denied write changed engagement: before %s, after %d %s", before.Body.String(), after.Code, after.Body.String())
		}
	}
	namePath := base + "/engagements/" + e.ID + "/name"
	assertProblem(t, serveAuthorized(handler, http.MethodPut, namePath, `{}`, token, nil), http.StatusBadRequest, "invalid-request")
	assertProblem(t, serveAuthorized(handler, http.MethodPut, base+"/engagements/not-an-id/name", `{"name":"New"}`, token, nil), http.StatusNotFound, "not-found")
	assertDeniedUnchanged(http.MethodPut, namePath, `{"name":"Denied"}`)
	for _, name := range []string{"", "   ", strings.Repeat("é", 201)} {
		body, err := json.Marshal(map[string]string{"name": name})
		if err != nil {
			t.Fatal(err)
		}
		assertProblem(t, serveAuthorized(handler, http.MethodPut, namePath, string(body), token, nil), http.StatusBadRequest, "invalid-request")
	}
	if r := serveAuthorized(handler, http.MethodPut, namePath, `{"name":"  Élan  "}`, token, nil); r.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", r.Code, r.Body.String())
	}
	if r := serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", readOnly.Secret, nil); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"name":"Élan"`) {
		t.Fatalf("renamed: %d %s", r.Code, r.Body.String())
	}
	var auditCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE event_type = 'engagement.name_updated'`).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("rename audit = %d, %v", auditCount, err)
	}
	longName := strings.Repeat("é", 201)
	if _, err := db.Exec(`UPDATE engagements SET name = ?`, longName); err != nil {
		t.Fatal(err)
	}
	longBody, err := json.Marshal(map[string]string{"name": longName})
	if err != nil {
		t.Fatal(err)
	}
	if r := serveAuthorized(handler, http.MethodPut, namePath, string(longBody), token, nil); r.Code != http.StatusNoContent {
		t.Fatalf("unchanged migrated name: %d %s", r.Code, r.Body.String())
	}
	actionPath := base + "/engagements/" + e.ID + "/next-action"
	quotePath := base + "/engagements/" + e.ID + "/quote"
	assigneePath := base + "/engagements/" + e.ID + "/assignee"
	agreementPath := base + "/engagements/" + e.ID + "/agreement"
	if r := serveAuthorized(handler, http.MethodPut, agreementPath, `{"recorded":true}`, token, nil); r.Code != http.StatusNoContent {
		t.Fatalf("agreement: %d %s", r.Code, r.Body.String())
	}
	if r := serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", readOnly.Secret, nil); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"agreement":{"recorded":true`) {
		t.Fatalf("agreement read: %d %s", r.Code, r.Body.String())
	}
	assertDeniedUnchanged(http.MethodPut, agreementPath, `{"recorded":false}`)
	assertProblem(t, serveAuthorized(handler, http.MethodPut, agreementPath, `{"recorded":true,"fileId":"invalid"}`, token, nil), http.StatusBadRequest, "invalid-request")
	if r := serveAuthorized(handler, http.MethodPut, assigneePath, `{"assigneeId":"`+userID.String()+`"}`, token, nil); r.Code != http.StatusNoContent {
		t.Fatalf("assign: %d %s", r.Code, r.Body.String())
	}
	if r := serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", readOnly.Secret, nil); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"assigneeId":"`+userID.String()+`"`) {
		t.Fatalf("assignment read: %d %s", r.Code, r.Body.String())
	}
	assertDeniedUnchanged(http.MethodPut, assigneePath, `{}`)
	assertProblem(t, serveAuthorized(handler, http.MethodPut, assigneePath, `{"assigneeId":"invalid"}`, token, nil), http.StatusBadRequest, "invalid-request")
	assertProblem(t, serveAuthorized(handler, http.MethodPut, assigneePath, `{"assigneeId":"`+identity.UserID(999999).String()+`"}`, token, nil), http.StatusBadRequest, "invalid-request")
	if r := serveAuthorized(handler, http.MethodPut, assigneePath, `{}`, token, nil); r.Code != http.StatusNoContent {
		t.Fatalf("unassign: %d %s", r.Code, r.Body.String())
	}
	if r := serveAuthorized(handler, http.MethodPut, quotePath, `{"status":"sent","amount":"120.50","dateSent":"2026-11-23"}`, token, nil); r.Code != http.StatusNoContent {
		t.Fatalf("quote: %d %s", r.Code, r.Body.String())
	}
	if r := serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", readOnly.Secret, nil); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"amount":"120.50"`) {
		t.Fatalf("quote read: %d %s", r.Code, r.Body.String())
	}
	assertProblem(t, serveAuthorized(handler, http.MethodPut, quotePath, `{"status":"wrong"}`, token, nil), http.StatusBadRequest, "invalid-request")
	assertDeniedUnchanged(http.MethodPut, quotePath, `{"status":"sent"}`)
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
	assertDeniedUnchanged(http.MethodPut, actionPath, `{}`)
	var countBefore int
	if err := db.QueryRow("SELECT COUNT(*) FROM engagements WHERE workspace_id = ?", workspaceID).Scan(&countBefore); err != nil {
		t.Fatal(err)
	}
	assertProblem(t, serveAuthorized(handler, http.MethodPost, base+"/engagements", `{"name":"Denied","clientId":"`+c.ID+`","stageId":"`+open.ID+`"}`, readOnly.Secret, nil), http.StatusForbidden, "insufficient-scope")
	var countAfter int
	if err := db.QueryRow("SELECT COUNT(*) FROM engagements WHERE workspace_id = ?", workspaceID).Scan(&countAfter); err != nil || countAfter != countBefore {
		t.Fatalf("engagement count after denied create = %d, want %d: %v", countAfter, countBefore, err)
	}
	if got := serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", readOnly.Secret, nil); got.Code != http.StatusOK {
		t.Fatalf("read only get: %d", got.Code)
	}
	won := serveAuthorized(handler, http.MethodPost, base+"/engagement-stages", `{"name":"Won","kind":"won"}`, token, nil)
	var target struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(won.Body.Bytes(), &target); err != nil {
		t.Fatal(err)
	}
	movePath := base + "/engagements/" + e.ID + "/stage"
	assertDeniedUnchanged(http.MethodPost, movePath, `{"stageId":"`+target.ID+`"}`)
	if moved := serveAuthorized(handler, http.MethodPost, movePath, `{"stageId":"`+target.ID+`"}`, token, nil); moved.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", moved.Code, moved.Body.String())
	}
	if got := serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", readOnly.Secret, nil); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"toStageId":"`+target.ID+`"`) {
		t.Fatalf("history: %d %s", got.Code, got.Body.String())
	}
	if deleted := serveAuthorized(handler, http.MethodDelete, base+"/clients/"+c.ID, "", token, nil); deleted.Code != http.StatusNoContent {
		t.Fatalf("delete client: %d %s", deleted.Code, deleted.Body.String())
	}
	assertProblem(t, serveAuthorized(handler, http.MethodGet, base+"/engagements/"+e.ID, "", readOnly.Secret, nil), http.StatusNotFound, "not-found")
}
