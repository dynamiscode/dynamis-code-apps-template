package web

import (
	"fmt"
	"html"
	"net/http"
	"net/url"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"strings"
	"testing"
)

func TestClientTimelineForms(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{"csrf": {env.csrf}, "display_name": {"Ada"}}, env.cookies, nil)
	detail := created.Header().Get("Location")
	path := detail + "/timeline"
	first := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "type": {"call"}, "channel": {"phone"}, "text": {"First contact"}}, env.cookies, nil)
	if first.Code != http.StatusSeeOther {
		t.Fatalf("add = %d: %s", first.Code, first.Body.String())
	}
	page := request(env.routes, http.MethodGet, detail, nil, env.cookies, nil).Body.String()
	if !strings.Contains(page, `data-timeline-composer`) || !strings.Contains(page, `data-timeline-dialog`) || !strings.Contains(page, `timeline-vertical`) || strings.Index(page, `id="timeline-text"`) > strings.Index(page, `id="timeline-type"`) {
		t.Fatal("timeline or writing-first form missing")
	}
	if strings.Contains(page, `maxlength="2000"`) {
		t.Fatal("UTF-16 maxlength rejects valid supplementary characters")
	}
	unicode := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "type": {"note"}, "text": {strings.Repeat("😀", 2000)}}, env.cookies, nil)
	if unicode.Code != http.StatusSeeOther {
		t.Fatalf("valid Unicode entry = %d", unicode.Code)
	}
	if !strings.Contains(page, "First contact") || !strings.Contains(page, "Phone") || !strings.Contains(page, "owner@example.com") {
		t.Fatalf("missing entry details: %s", page)
	}
	var key int64
	if err := env.db.QueryRow("SELECT id FROM client_timeline_entries LIMIT 1").Scan(&key); err != nil {
		t.Fatal(err)
	}
	entryID := clients.TimelineEntryID(key).String()
	corrected := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "type": {"note"}, "text": {"Correction"}, "corrects": {entryID}}, env.cookies, nil)
	if corrected.Code != http.StatusSeeOther {
		t.Fatalf("correction = %d", corrected.Code)
	}
	page = request(env.routes, http.MethodGet, detail, nil, env.cookies, nil).Body.String()
	if !strings.Contains(page, "#timeline-"+entryID) || strings.Index(page, "First contact") < strings.Index(page, "Correction") {
		t.Fatal("correction reference or ordering missing")
	}
	for _, tc := range []struct{ typ, text, message string }{{"note", "", "Text is required"}, {"note", strings.Repeat("a", 2001), "2,000"}, {"invalid", "draft text", "entry type"}} {
		response := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "type": {tc.typ}, "text": {tc.text}}, env.cookies, nil)
		if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), tc.message) || !strings.Contains(response.Body.String(), tc.text) {
			t.Fatalf("invalid input = %d, want %q and draft", response.Code, tc.message)
		}
	}
	spanishCookies := append(append([]*http.Cookie(nil), env.cookies...), &http.Cookie{Name: "locale", Value: "es"})
	spanish := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "type": {"note"}, "text": {""}}, spanishCookies, nil)
	if spanish.Code != http.StatusUnprocessableEntity || !strings.Contains(spanish.Body.String(), "El texto es obligatorio") {
		t.Fatal("Spanish validation missing")
	}
	invalidCorrection := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "type": {"note"}, "text": {"Draft correction"}, "corrects": {clients.TimelineEntryID(999999).String()}}, env.cookies, nil)
	if invalidCorrection.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidCorrection.Body.String(), "Draft correction") || !strings.Contains(invalidCorrection.Body.String(), "data-timeline-invalid") || !strings.Contains(invalidCorrection.Body.String(), `<details class="collapse collapse-arrow bg-base-200 mt-3" open`) || !strings.Contains(invalidCorrection.Body.String(), `value="`+clients.TimelineEntryID(999999).String()+`" selected`) {
		t.Fatal("invalid correction not rejected with visible draft")
	}
	invalidPage := invalidCorrection.Body.String()
	if summary, timeline := strings.Index(invalidPage, `data-timeline-error-summary`), strings.Index(invalidPage, `class="client-timeline`); summary < 0 || timeline < 0 || summary > timeline || !strings.Contains(invalidPage, `href="#timeline-text"`) {
		t.Fatal("timeline error summary must precede entries and link to draft")
	}
	viewer := request(env.routes, http.MethodGet, detail, nil, env.viewerCookies, nil)
	if !strings.Contains(viewer.Body.String(), "First contact") || strings.Contains(viewer.Body.String(), `action="`+path+`?`) {
		t.Fatal("viewer timeline or permissions incorrect")
	}
	denied := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.viewerCSRF}, "type": {"note"}, "text": {"blocked"}}, env.viewerCookies, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("viewer write = %d", denied.Code)
	}
	unknown := request(env.routes, http.MethodGet, env.listPath()+"/"+clients.ClientID(999999).String(), nil, env.viewerCookies, nil)
	foreign := request(env.routes, http.MethodGet, "/workspaces/"+identity.WorkspaceID(999999).String()+"/clients/"+clients.ClientID(1).String(), nil, env.viewerCookies, nil)
	for _, response := range []string{unknown.Body.String(), foreign.Body.String()} {
		if strings.Contains(response, "Timeline") || strings.Contains(response, "First contact") {
			t.Fatal("unreadable client leaks timeline")
		}
	}
	if unknown.Code != http.StatusNotFound || foreign.Code != http.StatusNotFound {
		t.Fatal("unreadable client did not return 404")
	}
}

func TestClientTimelinePagination(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{"csrf": {env.csrf}, "display_name": {"Paged"}}, env.cookies, nil)
	detail := created.Header().Get("Location")
	for i := 0; i < 51; i++ {
		response := request(env.routes, http.MethodPost, detail+"/timeline", url.Values{"csrf": {env.csrf}, "type": {"note"}, "text": {fmt.Sprintf("entry-%02d", i)}}, env.cookies, nil)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("entry %d: %d", i, response.Code)
		}
	}
	first := request(env.routes, http.MethodGet, detail, nil, env.cookies, nil).Body.String()
	second := request(env.routes, http.MethodGet, detail+"?timeline_page=2", nil, env.viewerCookies, nil).Body.String()
	if !strings.Contains(first, "entry-50") || strings.Contains(first, "entry-00") || !strings.Contains(first, "timeline_page=2") {
		t.Fatal("first page or next link incorrect")
	}
	if !strings.Contains(second, "entry-00") || strings.Contains(second, "entry-50") || !strings.Contains(second, "timeline_page=1") {
		t.Fatal("second page or previous link incorrect")
	}
}

func TestClientTimelineCorrectionAcrossPagesAndReturn(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{"csrf": {env.csrf}, "display_name": {"Paged"}}, env.cookies, nil)
	detail := created.Header().Get("Location")
	path := detail + "/timeline"
	for i := range 50 {
		response := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "type": {"note"}, "text": {fmt.Sprintf("entry-%02d", i)}}, env.cookies, nil)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("entry %d: %d", i, response.Code)
		}
	}
	var key int64
	if err := env.db.QueryRow("SELECT MIN(id) FROM client_timeline_entries").Scan(&key); err != nil {
		t.Fatal(err)
	}
	target := clients.TimelineEntryID(key).String()
	context := "q=Paged&sort=displayName"
	response := request(env.routes, http.MethodPost, path+"?return="+url.QueryEscape(context), url.Values{"csrf": {env.csrf}, "type": {"note"}, "text": {"Correction"}, "corrects": {target}}, env.cookies, nil)
	if response.Code != http.StatusSeeOther || !strings.Contains(response.Header().Get("Location"), "return=") {
		t.Fatalf("correction redirect: %d %q", response.Code, response.Header().Get("Location"))
	}
	browse := request(env.routes, http.MethodPost, path+"?return="+url.QueryEscape(context), url.Values{"csrf": {env.csrf}, "type": {"call"}, "channel": {"phone"}, "text": {"unsaved draft"}, "timeline_search": {"entry-00"}, "timeline_browse": {"1"}}, env.cookies, nil)
	if browse.Code != http.StatusOK || !strings.Contains(browse.Body.String(), "unsaved draft") || !strings.Contains(browse.Body.String(), `value="`+target+`"`) || !strings.Contains(browse.Body.String(), "data-timeline-invalid") {
		t.Fatalf("search did not preserve draft or find older entry: %d %s", browse.Code, browse.Body.String())
	}
	page := request(env.routes, http.MethodGet, response.Header().Get("Location"), nil, env.cookies, nil).Body.String()
	if !strings.Contains(html.UnescapeString(page), "#timeline-"+target) || !strings.Contains(page, "timeline_page=2") {
		t.Fatalf("cross-page correction link missing: %s", page)
	}
	if !strings.Contains(page, "owner@example.com") || !strings.Contains(page, `id="timeline-type" name="type" class="select w-full"`) {
		t.Fatal("recognizable author or native timeline select missing")
	}
}
