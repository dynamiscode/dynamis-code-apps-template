package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestActionQueueWeb(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	actor, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	svc := engagement.NewService(env.db, config.SQLite, env.auth)
	stage, err := svc.CreateStage(ctx, actor, env.workspaceID, engagement.StageInput{Name: "Open stage", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, actor, env.workspaceID, clients.Input{DisplayName: "Queue client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	var completed engagement.ID
	for i := 0; i < 22; i++ {
		item, err := svc.Create(ctx, actor, env.workspaceID, engagement.Input{Name: fmt.Sprintf("Case %02d", i), ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		due := "2020-01-01"
		if i == 21 {
			due = ""
			completed = item.ID
		}
		if err := svc.SetNextAction(ctx, actor, env.workspaceID, item.ID, engagement.NextAction{Text: fmt.Sprintf("Action %02d", i), DueDate: due, OwnerID: env.ownerID}, identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
	}
	path := "/workspaces/" + env.workspaceID.String() + "/engagements/due"
	first := request(env.routes, http.MethodGet, path+"?owner="+env.ownerID.String(), nil, env.viewerCookies, nil)
	for _, want := range []string{"Action 00", "Queue client", "Case 00", "Open stage", "Action queue", "page=2", "owner=" + env.ownerID.String()} {
		if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), want) {
			t.Fatalf("first page missing %q: %d", want, first.Code)
		}
	}
	if strings.Contains(first.Body.String(), "/action/complete") {
		t.Fatal("viewer sees completion control")
	}
	writer := request(env.routes, http.MethodGet, path+"?filter=undated", nil, env.cookies, nil)
	if !strings.Contains(writer.Body.String(), "/action/complete") {
		t.Fatal("writer missing completion control")
	}
	completePath := path[:strings.LastIndex(path, "/due")] + "/" + completed.String() + "/action/complete"
	reschedulePath := strings.TrimSuffix(completePath, "complete") + "reschedule"
	if !strings.Contains(writer.Body.String(), "/action/reschedule") || strings.Contains(first.Body.String(), "/action/reschedule") {
		t.Fatal("reschedule controls must be writer-only")
	}
	reschedule := url.Values{"csrf": {env.csrf}, "version": {"1"}, "due_date": {"2020-01-02"}}
	if got := request(env.routes, http.MethodPost, reschedulePath, url.Values{"csrf": {env.viewerCSRF}, "version": {"1"}, "due_date": {"2020-01-02"}}, env.viewerCookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("viewer reschedule: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, reschedulePath, reschedule, env.cookies, nil); got.Code != http.StatusSeeOther || !strings.Contains(got.Header().Get("Location"), "rescheduled=1") {
		t.Fatalf("reschedule: %d %s", got.Code, got.Header().Get("Location"))
	}
	if got := request(env.routes, http.MethodPost, reschedulePath, reschedule, env.cookies, nil); !strings.Contains(got.Header().Get("Location"), "conflict=1") {
		t.Fatalf("stale reschedule: %d %s", got.Code, got.Header().Get("Location"))
	}
	form := url.Values{"csrf": {env.csrf}, "version": {"2"}}
	if got := request(env.routes, http.MethodPost, completePath, url.Values{"version": {"1"}}, env.cookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("missing csrf: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, completePath, url.Values{"csrf": {env.viewerCSRF}, "version": {"2"}}, env.viewerCookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("viewer complete: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, completePath, form, env.cookies, nil); got.Code != http.StatusSeeOther {
		t.Fatalf("complete: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, completePath, form, env.cookies, nil); got.Code != http.StatusSeeOther || !strings.Contains(got.Header().Get("Location"), "conflict=1") {
		t.Fatalf("stale complete: %d %s", got.Code, got.Header().Get("Location"))
	}
	conflict := request(env.routes, http.MethodGet, path+"?conflict=1", nil, env.cookies, nil)
	if !strings.Contains(conflict.Body.String(), "This action changed") {
		t.Fatal("missing conflict feedback")
	}
	if strings.Contains(first.Body.String(), "Action 20") {
		t.Fatal("first page exceeded limit")
	}
	second := request(env.routes, http.MethodGet, path+"?owner="+env.ownerID.String()+"&page=2", nil, env.viewerCookies, nil)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), "Action 20") || !strings.Contains(second.Body.String(), "Previous") {
		t.Fatalf("second page: %d", second.Code)
	}
	undated := request(env.routes, http.MethodGet, path+"?filter=undated", nil, env.viewerCookies, nil)
	if strings.Contains(undated.Body.String(), "Action 21") || strings.Contains(undated.Body.String(), "Action 00") {
		t.Fatal("undated filter")
	}
	done := request(env.routes, http.MethodGet, path+"?filter=completed&owner="+env.ownerID.String(), nil, env.viewerCookies, nil)
	if !strings.Contains(done.Body.String(), "Action 21") || !strings.Contains(done.Body.String(), "Completed at") || strings.Contains(done.Body.String(), "/action/reopen") {
		t.Fatal("completed filter or viewer controls")
	}
	entries, err := svc.ListActionQueue(ctx, actor, env.workspaceID, engagement.ActionQueueOptions{Filter: engagement.ActionQueueCompleted, Page: 1, Limit: 20})
	if err != nil || len(entries.Entries) != 1 {
		t.Fatalf("completed entries: %v", err)
	}
	entry := entries.Entries[0]
	reopenPath := strings.TrimSuffix(completePath, "complete") + "reopen"
	reopen := url.Values{"csrf": {env.csrf}, "version": {fmt.Sprint(entry.Action.Version)}, "history_id": {fmt.Sprint(entry.HistoryID)}}
	writerDone := request(env.routes, http.MethodGet, path+"?filter=completed", nil, env.cookies, nil)
	if !strings.Contains(writerDone.Body.String(), "/action/reopen") {
		t.Fatal("writer missing reopen control")
	}
	viewerReopen := url.Values{"csrf": {env.viewerCSRF}, "version": {fmt.Sprint(entry.Action.Version)}, "history_id": {fmt.Sprint(entry.HistoryID)}}
	if got := request(env.routes, http.MethodPost, reopenPath, viewerReopen, env.viewerCookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("viewer reopen: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, reopenPath, reopen, env.cookies, nil); got.Code != http.StatusSeeOther {
		t.Fatalf("reopen: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, reopenPath, reopen, env.cookies, nil); !strings.Contains(got.Header().Get("Location"), "conflict=1") {
		t.Fatalf("stale reopen: %d %s", got.Code, got.Header().Get("Location"))
	}
	for _, query := range []string{"?filter=invalid", "?page=0", "?owner=invalid"} {
		if got := request(env.routes, http.MethodGet, path+query, nil, env.viewerCookies, nil); got.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", query, got.Code)
		}
	}
	if got := request(env.routes, http.MethodGet, path, nil, nil, nil); got.Code == http.StatusOK {
		t.Fatal("anonymous access")
	}
	spanish := request(env.routes, http.MethodGet, path+"?filter=completed", nil, append(append([]*http.Cookie{}, env.viewerCookies...), &http.Cookie{Name: "locale", Value: "es"}), nil)
	if !strings.Contains(spanish.Body.String(), "Cola de acciones") || !strings.Contains(spanish.Body.String(), "Completada el") {
		t.Fatal("Spanish queue")
	}
}
