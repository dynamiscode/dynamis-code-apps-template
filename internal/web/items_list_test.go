package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestItemsListPresentationAndPreferences(t *testing.T) {
	handler, _, _, workspaceID, _ := testWeb(t, 10)
	cookies, csrf := loginWorkspaceUser(t, handler, "owner@example.com", "long-enough-password")
	path := "/workspaces/" + workspaceID.String() + "/items"

	created := request(handler, http.MethodPost, path, url.Values{
		"csrf": {csrf}, "idempotency_key": {"items-list-test"}, "title": {"Inline row"},
	}, cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create item = %d: %s", created.Code, created.Body.String())
	}
	created = request(handler, http.MethodPost, path, url.Values{
		"csrf": {csrf}, "idempotency_key": {"items-list-test-two"}, "title": {"Inline sibling"},
	}, cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create second item = %d: %s", created.Code, created.Body.String())
	}
	body := request(handler, http.MethodGet, path, nil, cookies, nil).Body.String()
	for _, required := range []string{
		"list-view-columns", "item-list-view", `data-column="title"`,
		`data-column="status"`, `name="title"`, `name="status"`,
	} {
		if !strings.Contains(body, required) {
			t.Errorf("Items list missing %q: %s", required, body)
		}
	}

	for _, column := range []string{"title"} {
		response := request(handler, http.MethodPost, path+"/preferences/columns/"+column, url.Values{
			"csrf": {csrf}, "visible": {"false"},
		}, cookies, nil)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("hide %s = %d: %s", column, response.Code, response.Body.String())
		}
	}
	body = request(handler, http.MethodGet, path+"?status=active&search=Inline&sort=created_at&limit=1", nil, cookies, nil).Body.String()
	if !strings.Contains(body, `name="title"`) || !strings.Contains(body, `name="status"`) {
		t.Fatalf("editing controls disappeared with their columns hidden: %s", body)
	}
	if strings.Contains(body, `data-column="title"`) || !strings.Contains(body, `data-column="status"`) {
		t.Fatalf("column preference did not hide only the selected column: %s", body)
	}
	if !strings.Contains(body, `action="`+path+`/preferences/columns/title?`) ||
		!strings.Contains(body, "status=active") || !strings.Contains(body, "search=Inline") ||
		!strings.Contains(body, "sort=created_at") || !strings.Contains(body, "limit=1") ||
		!strings.Contains(body, "cursor=") {
		t.Fatalf("list preferences did not preserve URL state: %s", body)
	}
	response := request(handler, http.MethodPost, path+"/preferences/columns/status", url.Values{
		"csrf": {csrf}, "visible": {"false"},
	}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("hide final visible column = %d: %s", response.Code, response.Body.String())
	}
	noColumns := request(handler, http.MethodGet, path, nil, cookies, nil)
	if noColumns.Code != http.StatusOK || !strings.Contains(noColumns.Body.String(), "Choose a column to show this list.") {
		t.Fatalf("no-visible-column state = %d: %s", noColumns.Code, noColumns.Body.String())
	}
	noMatches := request(handler, http.MethodGet, path+"?search=Missing", nil, cookies, nil)
	if noMatches.Code != http.StatusOK || !strings.Contains(noMatches.Body.String(), "No items yet.") ||
		strings.Contains(noMatches.Body.String(), "Inline row") {
		t.Fatalf("title search did not filter rows: %d %s", noMatches.Code, noMatches.Body.String())
	}
	noStatusMatches := request(handler, http.MethodGet, path+"?status=complete", nil, cookies, nil)
	if noStatusMatches.Code != http.StatusOK || !strings.Contains(noStatusMatches.Body.String(), "No items yet.") {
		t.Fatalf("status filter did not filter rows: %d %s", noStatusMatches.Code, noStatusMatches.Body.String())
	}
	invalid := request(handler, http.MethodGet, path+"?status=archived", nil, cookies, nil)
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
	body = request(handler, http.MethodGet, path, nil, cookies, nil).Body.String()
	if !strings.Contains(body, `data-column="title"`) || !strings.Contains(body, `data-column="status"`) {
		t.Fatalf("reset did not restore default columns: %s", body)
	}
}

func TestItemsPageUsesProfileLocaleAndTheme(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	if err := auth.UpdateUserProfile(context.Background(), owner.UserID, identity.ProfileUpdateInput{
		Locale: "es", Theme: "dark",
	}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	cookies, _ := loginWorkspaceUser(t, handler, "owner@example.com", "long-enough-password")
	path := "/workspaces/" + workspaceID.String() + "/items"
	response := request(handler, http.MethodGet, path, nil, cookies, nil)
	body := response.Body.String()
	if response.Code != http.StatusOK || !hasHTMLRootLang(body, "es") || !strings.Contains(body, `data-theme="dark"`) {
		t.Fatalf("Items locale/theme = %d: %s", response.Code, body)
	}
	for _, copy := range []string{"Título del nuevo elemento", "Aún no hay elementos."} {
		if !strings.Contains(body, copy) {
			t.Errorf("Items page lacks Spanish copy %q: %s", copy, body)
		}
	}
}
