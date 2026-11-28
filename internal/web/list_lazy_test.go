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
	env.seedClient(t, "Ada")
	page := request(env.routes, http.MethodGet, env.listPath()+"?all=1&q=ix&filter_field=displayName&filter_op=equals&filter_value=he", nil, env.cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `name="filter" value="displayName:equals:he"`) {
		t.Fatalf("search form lost the applied condition: %d %s", page.Code, body)
	}
}

// TestLiveResultsRefreshTheControlsThatCarryTheSearch checks the filter controls retain the search.
func TestLiveResultsRefreshTheControlsThatCarryTheSearch(t *testing.T) {
	env := newClientsTestEnv(t)
	env.seedClient(t, "Ada")
	fragment := request(env.routes, http.MethodGet, env.listPath()+"?part=results&q=ix", nil, env.cookies, map[string]string{"HX-Request": "true"}).Body.String()
	for _, marker := range []string{`<div id="list-filter-bar" hx-swap-oob="true">`, `<div id="list-filter-add" hx-swap-oob="true">`, `filter_field=displayName&amp;q=ix`} {
		if !strings.Contains(fragment, marker) {
			t.Fatalf("results fragment lacks out-of-band %s", marker)
		}
	}
	page := request(env.routes, http.MethodGet, env.listPath()+"?all=1&q=ix", nil, env.cookies, nil).Body.String()
	if strings.Contains(page, "hx-swap-oob") {
		t.Fatal("the full page must not carry out-of-band swaps")
	}
}

func TestClientFilterSwapPreservesSearchAndFullPageFallback(t *testing.T) {
	env := newClientsTestEnv(t)
	env.seedClient(t, "Ada")
	path := env.listPath() + "?filter=archiveStatus:equals:archived&q=Ada"
	fragment := request(env.routes, http.MethodGet, path, nil, env.cookies, map[string]string{"HX-Request": "true"})
	body := fragment.Body.String()
	if fragment.Code != http.StatusOK || !strings.HasPrefix(body, `<section id="client-list"`) || !strings.Contains(body, `q=Ada`) || !strings.Contains(body, `archiveStatus`) || strings.Contains(body, "<!doctype") {
		t.Fatal("client filter fragment must preserve search and archive status")
	}
	full := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	if !strings.Contains(full.Body.String(), "<!doctype html>") {
		t.Fatal("normal navigation must return the full page")
	}
	restored := request(env.routes, http.MethodGet, path, nil, env.cookies, map[string]string{"HX-Request": "true", "HX-History-Restore-Request": "true"})
	if !strings.Contains(restored.Body.String(), "<!doctype html>") {
		t.Fatal("history restoration must return the full page")
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
	// Column order only renders with rows on the page.
	seeded := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Contoso"},
	}, env.cookies, nil)
	if seeded.Code != http.StatusSeeOther {
		t.Fatalf("seed client = %d", seeded.Code)
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
	env.seedClient(t, "Ada")
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
	if !strings.Contains(cards, `aria-label="Row actions"`) || !strings.Contains(cards, "&#8942;") {
		t.Fatalf("mobile cards lack the vertical-dots actions menu")
	}
}

// TestArchivedClientsListOffersRestoreNotArchive checks archived rows offer restore, not archive.
func TestArchivedClientsListOffersRestoreNotArchive(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{"csrf": {env.csrf}, "display_name": {"Ada"}}, env.cookies, nil)
	location := created.Header().Get("Location")
	if created.Code != http.StatusSeeOther || location == "" {
		t.Fatalf("create = %d", created.Code)
	}
	archive := request(env.routes, http.MethodPost, location+"/archive", url.Values{"csrf": {env.csrf}, "confirm_archive": {"yes"}}, env.cookies, nil)
	if archive.Code != http.StatusSeeOther {
		t.Fatalf("archive = %d", archive.Code)
	}
	active := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil).Body.String()
	if !strings.Contains(active, `filter_field=archiveStatus`) || strings.Contains(active, location+"/unarchive") {
		t.Fatalf("active list must offer archive filtering and hide archived rows")
	}
	archived := request(env.routes, http.MethodGet, env.listPath()+"?all=1&filter=archiveStatus:equals:archived", nil, env.cookies, nil).Body.String()
	if !strings.Contains(archived, location+"/unarchive") || strings.Contains(archived, location+"/archive\"") || !strings.Contains(archived, "list-filter-chip-remove") {
		t.Fatalf("archived list must offer restore and a way back")
	}
}

func TestClientArchiveFilterPreservesQuery(t *testing.T) {
	env := newClientsTestEnv(t)
	env.seedClient(t, "Ada Active")
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{"csrf": {env.csrf}, "display_name": {"Ada Archived"}}, env.cookies, nil)
	result := request(env.routes, http.MethodPost, created.Header().Get("Location")+"/archive", url.Values{"csrf": {env.csrf}, "confirm_archive": {"yes"}}, env.cookies, nil)
	if result.Code != http.StatusSeeOther {
		t.Fatalf("archive = %d", result.Code)
	}
	query := "?q=Ada&sort=displayName&order=asc&filter=archiveStatus:equals:archived&filter=displayName:contains:Ada"
	page := request(env.routes, http.MethodGet, env.listPath()+query, nil, env.cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, "Ada Archived") || strings.Contains(body, "Ada Active") || !strings.Contains(body, "archiveStatus:equals:archived") || !strings.Contains(body, "displayName:contains:Ada") || !strings.Contains(body, `name="sort" value="displayName"`) {
		t.Fatalf("archive filter lost rows or query: %d %s", page.Code, body)
	}
}

func TestClientArchiveFilterAddStepAndValidation(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{"csrf": {env.csrf}, "display_name": {"Ada"}}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d", created.Code)
	}
	archived := request(env.routes, http.MethodPost, created.Header().Get("Location")+"/archive", url.Values{"csrf": {env.csrf}, "confirm_archive": {"yes"}}, env.cookies, nil)
	if archived.Code != http.StatusSeeOther {
		t.Fatalf("archive = %d", archived.Code)
	}
	step := request(env.routes, http.MethodGet, env.listPath()+"?filter_field=archiveStatus&q=Ada&sort=displayName", nil, env.cookies, nil)
	for _, want := range []string{`name="filter_field" value="archiveStatus"`, `value="active"`, `value="archived"`, `name="q" value="Ada"`, `name="sort" value="displayName"`} {
		if step.Code != http.StatusOK || !strings.Contains(step.Body.String(), want) {
			t.Fatalf("archive filter add step missing %s: %d %s", want, step.Code, step.Body.String())
		}
	}
	for _, query := range []string{
		"?filter=archiveStatus:equals:bogus",
		"?filter=archiveStatus:equals:active&filter=archiveStatus:equals:archived",
		"?filter=archiveStatus:equals:active,archived",
	} {
		if response := request(env.routes, http.MethodGet, env.listPath()+query, nil, env.cookies, nil); response.Code != http.StatusBadRequest {
			t.Fatalf("invalid archive filter %q = %d", query, response.Code)
		}
	}
}
