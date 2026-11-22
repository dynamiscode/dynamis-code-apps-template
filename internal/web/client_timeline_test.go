package web

import (
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
	if !strings.Contains(page, "First contact") || !strings.Contains(page, "Phone") || !strings.Contains(page, env.ownerID.String()) {
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
	if !strings.Contains(page, "#timeline-"+entryID) || strings.Index(page, "First contact") > strings.Index(page, "Correction") {
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
	if invalidCorrection.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidCorrection.Body.String(), "Draft correction") {
		t.Fatal("invalid correction not rejected with draft")
	}
	viewer := request(env.routes, http.MethodGet, detail, nil, env.viewerCookies, nil)
	if !strings.Contains(viewer.Body.String(), "First contact") || strings.Contains(viewer.Body.String(), `action="`+path+`"`) {
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
