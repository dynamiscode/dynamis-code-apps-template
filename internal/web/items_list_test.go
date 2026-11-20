package web

import (
	"context"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestItemsListPresentationAndPreferences(t *testing.T) {
	handler, _, _, workspaceID, _ := testWeb(t, 10)
	cookies, csrf := loginWorkspaceUser(t, handler, "owner@example.com", "long-enough-password")
	path := "/workspaces/" + workspaceID.String() + "/items"

	created := request(handler, http.MethodPost, path, url.Values{
		"csrf": {csrf}, "idempotency_key": {"items-list-test"}, "title": {"Inline row"},
	}, cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create item = %d, want %d", created.Code, http.StatusSeeOther)
	}
	created = request(handler, http.MethodPost, path, url.Values{
		"csrf": {csrf}, "idempotency_key": {"items-list-test-two"}, "title": {"Inline sibling"},
	}, cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create second item = %d, want %d", created.Code, http.StatusSeeOther)
	}
	body := request(handler, http.MethodGet, path+"?all=1", nil, cookies, nil).Body.String()
	for _, required := range []string{
		"list-columns-panel", "item-list-view", `data-column="title"`,
		`data-column="status"`, `name="title"`, `name="status"`,
	} {
		if !strings.Contains(body, required) {
			t.Errorf("Items list missing marker %q", required)
		}
	}

	for _, column := range []string{"title"} {
		response := request(handler, http.MethodPost, path+"/preferences/columns/"+column, url.Values{
			"csrf": {csrf}, "visible": {"false"},
		}, cookies, nil)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("hide %s = %d, want %d", column, response.Code, http.StatusSeeOther)
		}
	}
	body = request(handler, http.MethodGet, path+"?all=1&status=active&search=Inline&sort=created_at&limit=1", nil, cookies, nil).Body.String()
	if !strings.Contains(body, `name="title"`) || !strings.Contains(body, `name="status"`) {
		t.Fatalf("editing controls missing with hidden columns: title=%t status=%t",
			strings.Contains(body, `name="title"`), strings.Contains(body, `name="status"`))
	}
	if strings.Contains(body, `data-column="title"`) || !strings.Contains(body, `data-column="status"`) {
		t.Fatalf("column visibility state: title=%t status=%t",
			strings.Contains(body, `data-column="title"`), strings.Contains(body, `data-column="status"`))
	}
	if !strings.Contains(body, `action="`+path+`/preferences/columns/title?`) ||
		!strings.Contains(body, "status=active") || !strings.Contains(body, "search=Inline") ||
		!strings.Contains(body, "sort=created_at") || !strings.Contains(body, "limit=1") ||
		!strings.Contains(body, "page=2") {
		t.Fatalf("list preferences URL markers: action=%t status=%t search=%t sort=%t limit=%t page=%t",
			strings.Contains(body, `action="`+path+`/preferences/columns/title?`),
			strings.Contains(body, "status=active"), strings.Contains(body, "search=Inline"),
			strings.Contains(body, "sort=created_at"), strings.Contains(body, "limit=1"), strings.Contains(body, "cursor="))
	}
	response := request(handler, http.MethodPost, path+"/preferences/columns/status", url.Values{
		"csrf": {csrf}, "visible": {"false"},
	}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("hide final visible column = %d, want %d", response.Code, http.StatusSeeOther)
	}
	noDataColumns := request(handler, http.MethodGet, path+"?all=1", nil, cookies, nil)
	noDataColumnBody := noDataColumns.Body.String()
	if noDataColumns.Code != http.StatusOK || strings.Contains(noDataColumnBody, `data-column="title"`) ||
		strings.Contains(noDataColumnBody, `data-column="status"`) {
		t.Fatalf("no-visible-data-columns state: status=%d title=%t status_column=%t",
			noDataColumns.Code, strings.Contains(noDataColumnBody, `data-column="title"`),
			strings.Contains(noDataColumnBody, `data-column="status"`))
	}
	noMatches := request(handler, http.MethodGet, path+"?all=1&search=Missing", nil, cookies, nil)
	noMatchesBody := noMatches.Body.String()
	if noMatches.Code != http.StatusOK || !strings.Contains(noMatchesBody, "No items yet.") ||
		strings.Contains(noMatchesBody, "Inline row") {
		t.Fatalf("title search state: status=%d empty=%t matching_row=%t", noMatches.Code,
			strings.Contains(noMatchesBody, "No items yet."), strings.Contains(noMatchesBody, "Inline row"))
	}
	noStatusMatches := request(handler, http.MethodGet, path+"?all=1&status=complete", nil, cookies, nil)
	noStatusMatchesEmpty := strings.Contains(noStatusMatches.Body.String(), "No items yet.")
	if noStatusMatches.Code != http.StatusOK || !noStatusMatchesEmpty {
		t.Fatalf("status filter state: status=%d empty=%t", noStatusMatches.Code,
			noStatusMatchesEmpty)
	}
	invalid := request(handler, http.MethodGet, path+"?all=1&status=archived", nil, cookies, nil)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status filter = %d, want 400", invalid.Code)
	}
	reset := request(handler, http.MethodPost, path+"/preferences/columns/reset?status=active&search=Inline", url.Values{
		"csrf": {csrf},
	}, cookies, nil)
	if reset.Code != http.StatusSeeOther || !strings.Contains(reset.Header().Get("Location"), "search=Inline") ||
		!strings.Contains(reset.Header().Get("Location"), "status=active") {
		t.Fatalf("reset columns = %d, Location=%q", reset.Code, reset.Header().Get("Location"))
	}
	body = request(handler, http.MethodGet, path+"?all=1", nil, cookies, nil).Body.String()
	if !strings.Contains(body, `data-column="title"`) || !strings.Contains(body, `data-column="status"`) {
		t.Fatalf("reset column state: title=%t status=%t",
			strings.Contains(body, `data-column="title"`), strings.Contains(body, `data-column="status"`))
	}
}

func TestItemsPageUsesProfileLocaleAndTheme(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	if err := auth.UpdateUserProfile(context.Background(), owner.UserID, identity.ProfileUpdateInput{
		Locale: "es", Timezone: "America/Bogota", Theme: "dark",
	}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	cookies, _ := loginWorkspaceUser(t, handler, "owner@example.com", "long-enough-password")
	path := "/workspaces/" + workspaceID.String() + "/items"
	response := request(handler, http.MethodGet, path+"?all=1", nil, cookies, nil)
	body := response.Body.String()
	spanish := hasHTMLRootLang(body, "es")
	dark := strings.Contains(body, `data-theme="dark"`)
	if response.Code != http.StatusOK || !spanish || !dark {
		t.Fatalf("Items locale/theme state: status=%d spanish=%t dark=%t", response.Code,
			spanish, dark)
	}
	for _, copy := range []string{"Título del nuevo elemento", "Aún no hay elementos."} {
		if !strings.Contains(body, copy) {
			t.Errorf("Items page missing Spanish marker %q", copy)
		}
	}
}

func TestItemsViewerCannotCreateEditOrDelete(t *testing.T) {
	handler, auth, itemService, workspaceID, owner := testWeb(t, 10)
	ownerCookies, ownerCSRF := loginWorkspaceUser(t, handler, "owner@example.com", "long-enough-password")
	path := "/workspaces/" + workspaceID.String() + "/items"
	created := request(handler, http.MethodPost, path, url.Values{
		"csrf": {ownerCSRF}, "idempotency_key": {"items-viewer-permission"}, "title": {"Protected item"},
	}, ownerCookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("owner create = %d, want %d", created.Code, http.StatusSeeOther)
	}
	ctx := context.Background()
	invitations, err := auth.Authorize(ctx, owner.UserID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(ctx, invitations, "items-viewer@example.com", identity.Viewer, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	viewerID, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "viewer-long-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	viewerSession, err := auth.CreateSession(ctx, viewerID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	viewerCookies := []*http.Cookie{{Name: "session", Value: viewerSession.Secret}, {Name: "csrf", Value: viewerSession.CSRFSecret}}
	viewer := request(handler, http.MethodGet, path+"?all=1", nil, viewerCookies, nil)
	viewerBody := viewer.Body.String()
	canEdit := strings.Contains(viewerBody, `data-item-edit`)
	canCreate := strings.Contains(viewerBody, `name="idempotency_key"`)
	if viewer.Code != http.StatusOK || canEdit || canCreate {
		t.Fatalf("viewer Items page = %d, edit=%t create=%t", viewer.Code,
			canEdit, canCreate)
	}
	if refused := request(handler, http.MethodPost, path, url.Values{
		"csrf": {viewerSession.CSRFSecret}, "idempotency_key": {"items-viewer-create"}, "title": {"Blocked"},
	}, viewerCookies, nil); refused.Code != http.StatusForbidden {
		t.Fatalf("viewer create = %d, want %d", refused.Code, http.StatusForbidden)
	}
	ownerPrincipal, err := auth.Authorize(ctx, owner.UserID, workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	page, err := itemService.List(ctx, ownerPrincipal, workspaceID, items.ListInput{Sort: "-created_at", Limit: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("owner item list = %+v, %v", page, err)
	}
	itemPath := path + "/" + page.Items[0].ID.String()
	for _, action := range []string{"update", "delete"} {
		refused := request(handler, http.MethodPost, itemPath, url.Values{
			"csrf": {viewerSession.CSRFSecret}, "action": {action}, "version": {"1"}, "title": {"Blocked"},
		}, viewerCookies, nil)
		if refused.Code != http.StatusForbidden {
			t.Errorf("viewer %s = %d, want %d", action, refused.Code, http.StatusForbidden)
		}
	}
}
