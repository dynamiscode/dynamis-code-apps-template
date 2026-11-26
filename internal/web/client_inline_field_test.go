package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestClientInlineFieldUpdatesAndConflictRecovery(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada"}, "email": {"ada@example.com"}, "phone": {"123"}, "notes": {"Private"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	detail := created.Header().Get("Location")
	updated := request(env.routes, http.MethodPost, detail+"/field", url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "field": {"phone"}, "value": {" 456 "},
	}, env.cookies, nil)
	if updated.Code != http.StatusSeeOther || updated.Header().Get("Location") != detail {
		t.Fatalf("update phone = %d, location %q: %s", updated.Code, updated.Header().Get("Location"), updated.Body.String())
	}
	staleValues := url.Values{"csrf": {env.csrf}, "version": {"1"}, "field": {"display_name"}, "value": {"Grace <Draft>"}}
	for range 2 {
		stale := request(env.routes, http.MethodPost, detail+"/field", staleValues, env.cookies, nil)
		if stale.Code != http.StatusConflict {
			t.Fatalf("stale update = %d: %s", stale.Code, stale.Body.String())
		}
		for _, want := range []string{"Grace &lt;Draft&gt;", "Ada", "456", "ada@example.com"} {
			if !strings.Contains(stale.Body.String(), want) {
				t.Fatalf("conflict missing %q: %s", want, stale.Body.String())
			}
		}
		for _, want := range []string{`data-inline-saved="Ada"`, `data-inline-explicit-retry`, `name="field" value="display_name"`, `name="version" value="2"`, `data-inline-cancel`} {
			if !strings.Contains(stale.Body.String(), want) {
				t.Fatalf("conflict recovery missing %q: %s", want, stale.Body.String())
			}
		}
	}
	invalid := request(env.routes, http.MethodPost, detail+"/field", url.Values{
		"csrf": {env.csrf}, "version": {"2"}, "field": {"email"}, "value": {"bad-email"},
	}, env.cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "bad-email") || !strings.Contains(invalid.Body.String(), "ada@example.com") {
		t.Fatalf("invalid email = %d: %s", invalid.Code, invalid.Body.String())
	}
	blank := request(env.routes, http.MethodPost, detail+"/field", url.Values{
		"csrf": {env.csrf}, "version": {"2"}, "field": {"display_name"}, "value": {"   "},
	}, env.cookies, nil)
	if blank.Code != http.StatusUnprocessableEntity || !strings.Contains(blank.Body.String(), `value="   "`) || !strings.Contains(blank.Body.String(), `data-inline-saved="Ada"`) {
		t.Fatalf("blank name draft = %d: %s", blank.Code, blank.Body.String())
	}
	longPhone := strings.Repeat("9", 51)
	invalidPhone := request(env.routes, http.MethodPost, detail+"/field", url.Values{
		"csrf": {env.csrf}, "version": {"2"}, "field": {"phone"}, "value": {longPhone},
	}, env.cookies, nil)
	if invalidPhone.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidPhone.Body.String(), longPhone) || !strings.Contains(invalidPhone.Body.String(), `data-inline-saved="456"`) {
		t.Fatalf("invalid phone draft = %d: %s", invalidPhone.Code, invalidPhone.Body.String())
	}
	retry := request(env.routes, http.MethodPost, detail+"/field", url.Values{
		"csrf": {env.csrf}, "version": {"2"}, "field": {"display_name"}, "value": {"Grace"},
	}, env.cookies, nil)
	if retry.Code != http.StatusSeeOther {
		t.Fatalf("explicit retry = %d: %s", retry.Code, retry.Body.String())
	}
	email := request(env.routes, http.MethodPost, detail+"/field", url.Values{
		"csrf": {env.csrf}, "version": {"3"}, "field": {"email"}, "value": {"grace@example.com"},
	}, env.cookies, nil)
	if email.Code != http.StatusSeeOther {
		t.Fatalf("update email = %d: %s", email.Code, email.Body.String())
	}
	var name, storedEmail, phone, notes string
	var version int
	if err := env.db.QueryRow(`SELECT display_name, email, phone, notes, version FROM clients`).Scan(&name, &storedEmail, &phone, &notes, &version); err != nil {
		t.Fatal(err)
	}
	if name != "Grace" || storedEmail != "grace@example.com" || phone != "456" || notes != "Private" || version != 4 {
		t.Fatalf("stored fields = %q %q %q %q version %d", name, storedEmail, phone, notes, version)
	}
	for _, tc := range []struct{ field, version string }{{"phone", "4"}, {"email", "5"}} {
		cleared := request(env.routes, http.MethodPost, detail+"/field", url.Values{
			"csrf": {env.csrf}, "version": {tc.version}, "field": {tc.field}, "value": {""},
		}, env.cookies, nil)
		if cleared.Code != http.StatusSeeOther {
			t.Fatalf("clear %s = %d: %s", tc.field, cleared.Code, cleared.Body.String())
		}
	}
	if err := env.db.QueryRow(`SELECT display_name, email, phone, notes, version FROM clients`).Scan(&name, &storedEmail, &phone, &notes, &version); err != nil {
		t.Fatal(err)
	}
	if name != "Grace" || storedEmail != "" || phone != "" || notes != "Private" || version != 6 {
		t.Fatalf("cleared fields = %q %q %q %q version %d", name, storedEmail, phone, notes, version)
	}
}

func TestClientInlineNotesSaveValidateAndRetry(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada"}, "email": {"ada@example.com"}, "notes": {"Original"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	detail := created.Header().Get("Location")
	post := func(version, value string) *httptest.ResponseRecorder {
		return request(env.routes, http.MethodPost, detail+"/field", url.Values{
			"csrf": {env.csrf}, "version": {version}, "field": {"notes"}, "value": {value},
		}, env.cookies, nil)
	}
	if saved := post("1", "First line\nSecond line"); saved.Code != http.StatusSeeOther {
		t.Fatalf("save = %d: %s", saved.Code, saved.Body.String())
	}
	page := request(env.routes, http.MethodGet, detail, nil, env.cookies, nil)
	for _, want := range []string{`action="` + detail + `/field"`, `name="field" value="notes"`, `name="value"`, "First line"} {
		if !strings.Contains(page.Body.String(), want) {
			t.Fatalf("notes editor missing %q: %s", want, page.Body.String())
		}
	}
	long := strings.Repeat("界", 5001)
	if invalid := post("2", long); invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), long) {
		t.Fatalf("invalid notes draft = %d", invalid.Code)
	}
	conflict := post("1", "Unsaved draft\nNext line")
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "Unsaved draft\nNext line") ||
		!strings.Contains(conflict.Body.String(), `data-notes-conflict`) || !strings.Contains(conflict.Body.String(), "Try again") || !strings.Contains(conflict.Body.String(), `name="version" value="2"`) {
		t.Fatalf("conflict lost draft or retry: %d %s", conflict.Code, conflict.Body.String())
	}
	var name, email, notes string
	var version int
	if err := env.db.QueryRow(`SELECT display_name, email, notes, version FROM clients`).Scan(&name, &email, &notes, &version); err != nil {
		t.Fatal(err)
	}
	if name != "Ada" || email != "ada@example.com" || notes != "First line\nSecond line" || version != 2 {
		t.Fatalf("stored client = %q %q %q version %d", name, email, notes, version)
	}
	if retried := post("2", "Unsaved draft\nNext line"); retried.Code != http.StatusSeeOther {
		t.Fatalf("retry = %d: %s", retried.Code, retried.Body.String())
	}
	if cleared := post("3", ""); cleared.Code != http.StatusSeeOther {
		t.Fatalf("clear = %d: %s", cleared.Code, cleared.Body.String())
	}
	if err := env.db.QueryRow(`SELECT notes, version FROM clients`).Scan(&notes, &version); err != nil || notes != "" || version != 4 {
		t.Fatalf("cleared notes = %q version %d: %v", notes, version, err)
	}
}

func TestClientInlineFieldRejectsForgedRequests(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{"csrf": {env.csrf}, "display_name": {"Ada"}}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	detail := created.Header().Get("Location")
	for _, tc := range []struct {
		name, path string
		values     url.Values
		cookies    []*http.Cookie
		status     int
	}{
		{"viewer", detail + "/field", url.Values{"csrf": {env.viewerCSRF}, "version": {"1"}, "field": {"phone"}, "value": {"999"}}, env.viewerCookies, http.StatusForbidden},
		{"csrf", detail + "/field", url.Values{"csrf": {"wrong"}, "version": {"1"}, "field": {"phone"}, "value": {"999"}}, env.cookies, http.StatusForbidden},
		{"field", detail + "/field", url.Values{"csrf": {env.csrf}, "version": {"1"}, "field": {"archived"}, "value": {"forged"}}, env.cookies, http.StatusBadRequest},
		{"version", detail + "/field", url.Values{"csrf": {env.csrf}, "version": {"-1"}, "field": {"phone"}, "value": {"999"}}, env.cookies, http.StatusBadRequest},
		{"missing value", detail + "/field", url.Values{"csrf": {env.csrf}, "version": {"1"}, "field": {"phone"}}, env.cookies, http.StatusBadRequest},
		{"client", env.listPath() + "/" + clients.ClientID(999999).String() + "/field", url.Values{"csrf": {env.csrf}, "version": {"1"}, "field": {"phone"}, "value": {"999"}}, env.cookies, http.StatusNotFound},
		{"workspace", "/workspaces/" + identity.WorkspaceID(999999).String() + "/clients/" + strings.TrimPrefix(detail, env.listPath()+"/") + "/field", url.Values{"csrf": {env.csrf}, "version": {"1"}, "field": {"phone"}, "value": {"999"}}, env.cookies, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := request(env.routes, http.MethodPost, tc.path, tc.values, tc.cookies, nil)
			if got.Code != tc.status {
				t.Fatalf("response = %d, want %d: %s", got.Code, tc.status, got.Body.String())
			}
		})
	}
	var name, phone, notes string
	var version int
	if err := env.db.QueryRow(`SELECT display_name, phone, notes, version FROM clients`).Scan(&name, &phone, &notes, &version); err != nil {
		t.Fatal(err)
	}
	if name != "Ada" || phone != "" || notes != "" || version != 1 {
		t.Fatalf("forged requests changed client: %q %q %q version %d", name, phone, notes, version)
	}
}
