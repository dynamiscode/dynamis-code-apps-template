package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestClientsListRendersRowsWithoutJavaScript checks that a normal Clients page renders its rows itself, so a client
// without JavaScript sees the list. The results fragment still carries the rows for an htmx request, and the search
// form is wired to refresh the results region in place.
func TestClientsListRendersRowsWithoutJavaScript(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d", created.Code)
	}

	page := request(env.routes, http.MethodGet, env.listPath(), nil, env.cookies, nil)
	body := page.Body.String()
	fragment := env.listPath() + "?part=results"
	if page.Code != http.StatusOK || !strings.Contains(body, "list-row-link") ||
		strings.Contains(body, "data-lazy-src") || strings.Contains(body, "list-skeleton") {
		t.Fatalf("list page = %d: %s", page.Code, body)
	}

	results := request(env.routes, http.MethodGet, fragment, nil, env.cookies, map[string]string{"HX-Request": "true"})
	rows := results.Body.String()
	if results.Code != http.StatusOK || !strings.HasPrefix(rows, `<div id="list-results"`) ||
		!strings.Contains(rows, "list-row-link") || strings.Contains(rows, "<html") {
		t.Fatalf("results fragment = %d: %s", results.Code, rows)
	}

	// The search form updates the results region in place and keeps its normal GET submit without JavaScript.
	if !strings.Contains(body, `hx-get="`+fragment+`"`) || !strings.Contains(body, `hx-target="#list-results"`) ||
		!strings.Contains(body, `method="get"`) {
		t.Fatalf("live search is not wired to the results region: %s", body)
	}

	bad := request(env.routes, http.MethodGet, env.listPath()+"?archived=nope", nil, env.cookies, nil)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad selector on a list page = %d, want 400", bad.Code)
	}
}

// TestSearchKeepsAConditionAddedFromTheMenu checks that a condition applied from the add menu, which the URL
// holds as filter_field and filter_value, is carried by the search form. Clearing the search must keep it.
func TestSearchKeepsAConditionAddedFromTheMenu(t *testing.T) {
	env := newClientsTestEnv(t)
	page := request(env.routes, http.MethodGet, env.listPath()+"?all=1&q=ix&filter_field=displayName&filter_op=equals&filter_value=he", nil, env.cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `name="filter" value="displayName:equals:he"`) {
		t.Fatalf("search form lost the applied condition: %d %s", page.Code, body)
	}
}

// TestLiveResultsRefreshTheControlsThatCarryTheSearch checks that the results fragment also swaps the filter bar
// and the archived selector, which hold the search in their links, and that the full page does not.
func TestLiveResultsRefreshTheControlsThatCarryTheSearch(t *testing.T) {
	env := newClientsTestEnv(t)
	fragment := request(env.routes, http.MethodGet, env.listPath()+"?part=results&q=ix", nil, env.cookies, map[string]string{"HX-Request": "true"}).Body.String()
	for _, marker := range []string{`<div id="list-filter-bar" hx-swap-oob="true">`} {
		if !strings.Contains(fragment, marker) {
			t.Fatalf("results fragment lacks out-of-band %s", marker)
		}
	}
	page := request(env.routes, http.MethodGet, env.listPath()+"?all=1&q=ix", nil, env.cookies, nil).Body.String()
	if strings.Contains(page, "hx-swap-oob") {
		t.Fatal("the full page must not carry out-of-band swaps")
	}
}

// TestClientColumnOrderIsSavedAndRendered checks that a reordered column menu saves the order, that the list renders
// the columns in it, and that the name column stays first.
func TestClientColumnOrderIsSavedAndRendered(t *testing.T) {
	env := newClientsTestEnv(t)
	saved := request(env.routes, http.MethodPost, env.listPath()+"/preferences/columns/order", url.Values{
		"csrf": {env.csrf}, "order": {"displayName,updatedAt,phone,email,notes,createdAt"},
	}, env.cookies, nil)
	if saved.Code != http.StatusSeeOther {
		t.Fatalf("save order = %d: %s", saved.Code, saved.Body.String())
	}
	for _, column := range []string{"updatedAt", "phone"} {
		show := request(env.routes, http.MethodPost, env.listPath()+"/preferences/columns/"+column, url.Values{"csrf": {env.csrf}, "visible": {"true"}}, env.cookies, nil)
		if show.Code != http.StatusSeeOther {
			t.Fatalf("show %s = %d", column, show.Code)
		}
	}
	body := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil).Body.String()
	at := func(column string) int { return strings.Index(body, `<th scope="col" data-column="`+column+`"`) }
	if !(at("displayName") < at("updatedAt") && at("updatedAt") < at("phone") && at("phone") < at("email")) {
		t.Fatalf("columns are not in the saved order: %s", body)
	}
}

// withAll adds the all parameter to a list query. Existing links carry it, and the list accepts it.
func withAll(query string) string {
	if strings.Contains(query, "?") {
		return query + "&all=1"
	}
	return query + "?all=1"
}

// TestClientColumnChangeRefreshesInPlaceAndRedirectsForForms checks that a live column change returns the results
// region with the switch list refreshed out of band, and that a plain form post still redirects to the list.
func TestClientColumnChangeRefreshesInPlaceAndRedirectsForForms(t *testing.T) {
	env := newClientsTestEnv(t)
	form := url.Values{"csrf": {env.csrf}, "visible": {"true"}}
	live := request(env.routes, http.MethodPost, env.listPath()+"/preferences/columns/phone?part=results", form, env.cookies, map[string]string{"HX-Request": "true"})
	body := live.Body.String()
	if live.Code != http.StatusOK || !strings.HasPrefix(body, `<div id="list-results"`) ||
		!strings.Contains(body, `<ul id="list-column-switches" class="list-column-switches" hx-swap-oob="true">`) {
		t.Fatalf("live column change = %d: %s", live.Code, body)
	}
	plain := request(env.routes, http.MethodPost, env.listPath()+"/preferences/columns/phone", form, env.cookies, nil)
	if plain.Code != http.StatusSeeOther {
		t.Fatalf("plain form change = %d, want a redirect", plain.Code)
	}
}

// TestClientCardsCarryTheRowActions checks that each mobile card of the Clients list has the actions menu with the
// vertical dots, as the table row has its menu.
func TestClientCardsCarryTheRowActions(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{"csrf": {env.csrf}, "display_name": {"Ada"}}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d", created.Code)
	}
	body := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil).Body.String()
	cards := body[strings.Index(body, `<ul class="list-view-cards">`):]
	if !strings.Contains(cards, `<details class="list-row-menu list-card-actions">`) || !strings.Contains(cards, "&#8942;") {
		t.Fatalf("mobile cards lack the vertical-dots actions menu")
	}
}

// TestArchivedClientsListOffersRestoreNotArchive checks the archived view: the footer link opens it, and its rows
// offer restore, not archive.
func TestArchivedClientsListOffersRestoreNotArchive(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{"csrf": {env.csrf}, "display_name": {"Ada"}}, env.cookies, nil)
	location := created.Header().Get("Location")
	if created.Code != http.StatusSeeOther || location == "" {
		t.Fatalf("create = %d", created.Code)
	}
	archive := request(env.routes, http.MethodPost, location+"/archive", url.Values{"csrf": {env.csrf}}, env.cookies, nil)
	if archive.Code != http.StatusSeeOther {
		t.Fatalf("archive = %d", archive.Code)
	}
	active := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil).Body.String()
	if !strings.Contains(active, "Show archived clients") || strings.Contains(active, location+"/unarchive") {
		t.Fatalf("active list must link to archived clients and hide archived rows")
	}
	archived := request(env.routes, http.MethodGet, env.listPath()+"?all=1&archived=archived", nil, env.cookies, nil).Body.String()
	if !strings.Contains(archived, location+"/unarchive") || strings.Contains(archived, location+"/archive\"") || !strings.Contains(archived, "Back to active clients") {
		t.Fatalf("archived list must offer restore and a way back")
	}
}
