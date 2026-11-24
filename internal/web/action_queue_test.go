package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestActionHistoryVisibleToReaders(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	actor, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	svc := engagement.NewService(env.db, config.SQLite, env.auth)
	stage, err := svc.CreateStage(ctx, actor, env.workspaceID, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, actor, env.workspaceID, clients.Input{DisplayName: "History client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	item, err := svc.Create(ctx, actor, env.workspaceID, engagement.Input{Name: "History case", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.db.ExecContext(ctx, `UPDATE users SET display_name = 'History Person' WHERE id = ?`, env.ownerID); err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []string{"completed", "replaced", "cleared", "reopened"} {
		if _, err := env.db.ExecContext(ctx, `INSERT INTO engagement_action_history (workspace_id, engagement_id, action_text, due_date, owner_id, actor_user_id, outcome, created_at) VALUES (?, ?, ?, '2025-02-03', ?, ?, ?, '2025-01-02T15:04:00Z')`, env.workspaceID, item.ID, "History "+outcome, env.ownerID, env.ownerID, outcome); err != nil {
			t.Fatal(err)
		}
	}
	path := "/workspaces/" + env.workspaceID.String() + "/clients/" + client.ID.String() + "/engagements/" + item.ID.String()
	page := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	for _, want := range []string{"Action history", "History completed", "History replaced", "History cleared", "History reopened", "Completed:", "Replaced:", "Cleared:", "Reopened:", "History Person", "2025-02-03", "Jan 2, 2025 3:04 PM UTC"} {
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), want) {
			t.Fatalf("history missing %q: status %d", want, page.Code)
		}
	}
	spanish := request(env.routes, http.MethodGet, path, nil, append(append([]*http.Cookie{}, env.viewerCookies...), &http.Cookie{Name: "locale", Value: "es"}), nil)
	for _, want := range []string{"Historial de acciones", "Completada:", "2 de enero de 2025 15:04 UTC"} {
		if !strings.Contains(spanish.Body.String(), want) {
			t.Fatalf("Spanish history missing %q", want)
		}
	}
	if _, err := env.db.ExecContext(ctx, `UPDATE engagement_action_history SET owner_id = NULL, actor_user_id = NULL WHERE engagement_id = ?`, item.ID); err != nil {
		t.Fatal(err)
	}
	page = request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	history := strings.SplitN(strings.SplitN(page.Body.String(), `id="action-history"`, 2)[1], `</section>`, 2)[0]
	historyText := strings.Join(strings.Fields(regexp.MustCompile(`<[^>]*>`).ReplaceAllString(history, " ")), " ")
	if !strings.Contains(historyText, "Owner: Unassigned") || !strings.Contains(historyText, "By: Unavailable") {
		t.Fatalf("unassigned owner or unavailable actor missing: %s", historyText)
	}
	for i := 0; i < 50; i++ {
		if _, err := env.db.ExecContext(ctx, `INSERT INTO engagement_action_history (workspace_id, engagement_id, action_text, outcome, created_at) VALUES (?, ?, 'Paged history', 'cleared', '2025-01-02T15:04:00Z')`, env.workspaceID, item.ID); err != nil {
			t.Fatal(err)
		}
	}
	page = request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if !strings.Contains(page.Body.String(), "action_history_page=2") || strings.Contains(page.Body.String(), "History completed") {
		t.Fatal("history first page not bounded")
	}
	page = request(env.routes, http.MethodGet, path+"?action_history_page=2", nil, env.viewerCookies, nil)
	if !strings.Contains(page.Body.String(), "History completed") || !strings.Contains(page.Body.String(), "action_history_page=1") {
		t.Fatal("history second page missing entries or previous link")
	}
	if _, _, err := svc.ListActionHistory(ctx, identity.Principal{}, env.workspaceID, item.ID, 1); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("unauthorized history: %v", err)
	}
}

func TestRescheduleRecovery(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	actor, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	svc := engagement.NewService(env.db, config.SQLite, env.auth)
	stage, err := svc.CreateStage(ctx, actor, env.workspaceID, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, actor, env.workspaceID, clients.Input{DisplayName: "Recovery client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	item, err := svc.Create(ctx, actor, env.workspaceID, engagement.Input{Name: "Recovery case", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetNextAction(ctx, actor, env.workspaceID, item.ID, engagement.NextAction{Text: "Original action", DueDate: "2020-01-01"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	path := "/workspaces/" + env.workspaceID.String() + "/engagements/" + item.ID.String() + "/action/reschedule"
	form := url.Values{"csrf": {env.csrf}, "version": {"1"}, "due_date": {"not-a-date"}, "filter": {"today"}}
	invalid := request(env.routes, http.MethodPost, path, form, env.cookies, nil)
	for _, want := range []string{`value="not-a-date"`, "Original action", "2020-01-01", `name="version" value="1"`} {
		if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), want) {
			t.Errorf("invalid recovery missing %q (status %d)", want, invalid.Code)
		}
	}
	if err := svc.SetNextAction(ctx, actor, env.workspaceID, item.ID, engagement.NextAction{Text: "Replacement action", DueDate: "2030-01-01"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	form.Set("due_date", "2031-02-03")
	stale := request(env.routes, http.MethodPost, path, form, env.cookies, nil)
	for _, want := range []string{`value="2031-02-03"`, "Replacement action", "2030-01-01", `name="version" value="2"`} {
		if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), want) {
			t.Errorf("stale recovery missing %q (status %d)", want, stale.Code)
		}
	}
	form.Set("version", "2")
	retry := request(env.routes, http.MethodPost, path, form, env.cookies, nil)
	if retry.Code != http.StatusSeeOther {
		t.Fatalf("explicit retry: %d", retry.Code)
	}
	updated, err := svc.Get(ctx, actor, env.workspaceID, item.ID)
	if err != nil || updated.NextAction.Text != "Replacement action" || updated.NextAction.DueDate != "2031-02-03" {
		t.Fatalf("explicit retry did not reschedule observed action: %+v %v", updated.NextAction, err)
	}
	if err := svc.SetNextAction(ctx, actor, env.workspaceID, item.ID, engagement.NextAction{}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	gone := request(env.routes, http.MethodPost, path, form, env.cookies, nil)
	if gone.Code != http.StatusConflict || !strings.Contains(gone.Body.String(), "2031-02-03") || strings.Contains(gone.Body.String(), `name="version"`) {
		t.Errorf("removed action recovery lost draft or permits retry: status %d", gone.Code)
	}
}

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
	if !strings.Contains(writer.Body.String(), "/action/complete") || !strings.Contains(writer.Body.String(), `name="filter" value="undated"`) {
		t.Fatal("writer missing completion control or filter context")
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
	if got := request(env.routes, http.MethodPost, reschedulePath, reschedule, env.cookies, nil); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), `value="2020-01-02"`) {
		t.Fatalf("stale reschedule must preserve draft: %d", got.Code)
	}
	form := url.Values{"csrf": {env.csrf}, "version": {"2"}, "filter": {"overdue"}, "owner": {env.ownerID.String()}}
	if got := request(env.routes, http.MethodPost, completePath, url.Values{"version": {"1"}}, env.cookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("missing csrf: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, completePath, url.Values{"csrf": {env.viewerCSRF}, "version": {"2"}}, env.viewerCookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("viewer complete: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, completePath, form, env.cookies, nil); got.Code != http.StatusSeeOther {
		t.Fatalf("complete: %d", got.Code)
	} else {
		location := got.Header().Get("Location")
		if !strings.Contains(location, "filter=overdue") || !strings.Contains(location, "owner="+env.ownerID.String()) || !strings.Contains(location, "status=completed") {
			t.Fatalf("complete redirect: %s", location)
		}
		page := request(env.routes, http.MethodGet, location, nil, env.cookies, nil)
		if !strings.Contains(page.Body.String(), "Action completed.") {
			t.Fatal("missing completion feedback")
		}
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
	reopen := url.Values{"csrf": {env.csrf}, "version": {fmt.Sprint(entry.Action.Version)}, "history_id": {fmt.Sprint(entry.HistoryID)}, "filter": {"completed"}, "owner": {env.ownerID.String()}}
	writerDone := request(env.routes, http.MethodGet, path+"?filter=completed&owner="+env.ownerID.String(), nil, env.cookies, nil)
	if !strings.Contains(writerDone.Body.String(), "/action/reopen") || !strings.Contains(writerDone.Body.String(), `name="owner" value="`+env.ownerID.String()+`"`) {
		t.Fatal("writer missing reopen control or owner context")
	}
	viewerReopen := url.Values{"csrf": {env.viewerCSRF}, "version": {fmt.Sprint(entry.Action.Version)}, "history_id": {fmt.Sprint(entry.HistoryID)}}
	if got := request(env.routes, http.MethodPost, reopenPath, viewerReopen, env.viewerCookies, nil); got.Code != http.StatusForbidden {
		t.Fatalf("viewer reopen: %d", got.Code)
	}
	if got := request(env.routes, http.MethodPost, reopenPath, reopen, env.cookies, nil); got.Code != http.StatusSeeOther {
		t.Fatalf("reopen: %d", got.Code)
	} else {
		location := got.Header().Get("Location")
		if !strings.Contains(location, "filter=completed") || !strings.Contains(location, "owner="+env.ownerID.String()) || !strings.Contains(location, "status=reopened") {
			t.Fatalf("reopen redirect: %s", location)
		}
		page := request(env.routes, http.MethodGet, location, nil, append(append([]*http.Cookie{}, env.cookies...), &http.Cookie{Name: "locale", Value: "es"}), nil)
		if !strings.Contains(page.Body.String(), "Acción reabierta.") {
			t.Fatal("missing localized reopen feedback")
		}
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
