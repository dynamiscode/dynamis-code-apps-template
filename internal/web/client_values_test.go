package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
)

// TestClientValuesSetAndClearThroughForms drives the detail page's value forms.
// Each accepted write redirects (POST-redirect-GET); refusals re-render the page.
func TestClientValuesSetAndClearThroughForms(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create client = %d: %s", created.Code, created.Body.String())
	}
	detail := created.Header().Get("Location")
	defined := request(env.routes, http.MethodPost, "/workspaces/"+env.workspaceID.String()+"/client-fields", url.Values{
		"csrf": {env.csrf}, "label": {"Source"}, "type": {"text"},
	}, env.cookies, nil)
	if defined.Code != http.StatusSeeOther {
		t.Fatalf("define field = %d: %s", defined.Code, defined.Body.String())
	}
	var fieldKey int64
	if err := env.db.QueryRow("SELECT id FROM client_fields").Scan(&fieldKey); err != nil {
		t.Fatal(err)
	}
	valuePath := detail + "/values/" + clients.FieldID(fieldKey).String()

	page := request(env.routes, http.MethodGet, detail, nil, env.cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `name="value"`) ||
		!strings.Contains(page.Body.String(), `action="`+valuePath+`"`) {
		t.Fatalf("owner detail has no value form: %d %s", page.Code, page.Body.String())
	}

	set := request(env.routes, http.MethodPost, valuePath, url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "value": {"<i>Friend</i>"},
	}, env.cookies, nil)
	if set.Code != http.StatusSeeOther || set.Header().Get("Location") != detail {
		t.Fatalf("set value = %d location %q: %s", set.Code, set.Header().Get("Location"), set.Body.String())
	}
	shown := request(env.routes, http.MethodGet, detail, nil, env.cookies, nil).Body.String()
	if !strings.Contains(shown, "&lt;i&gt;Friend&lt;/i&gt;") || strings.Contains(shown, "<i>Friend</i>") {
		t.Fatalf("stored value is not rendered as text: %s", shown)
	}

	blank := request(env.routes, http.MethodPost, valuePath, url.Values{
		"csrf": {env.csrf}, "version": {"2"}, "value": {"   "},
	}, env.cookies, nil)
	if blank.Code != http.StatusUnprocessableEntity || !strings.Contains(blank.Body.String(), "To remove a value, use Clear.") {
		t.Fatalf("blank value = %d, want 422 with the clear hint: %s", blank.Code, blank.Body.String())
	}
	stale := request(env.routes, http.MethodPost, valuePath, url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "value": {"Stale"},
	}, env.cookies, nil)
	if stale.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale value = %d, want 412", stale.Code)
	}

	cleared := request(env.routes, http.MethodPost, valuePath+"/clear", url.Values{
		"csrf": {env.csrf}, "version": {"2"},
	}, env.cookies, nil)
	if cleared.Code != http.StatusSeeOther {
		t.Fatalf("clear value = %d: %s", cleared.Code, cleared.Body.String())
	}
	after := request(env.routes, http.MethodGet, detail, nil, env.cookies, nil).Body.String()
	if strings.Contains(after, "Friend") {
		t.Fatalf("cleared value still shown: %s", after)
	}

	unknown := request(env.routes, http.MethodPost, detail+"/values/"+clients.FieldID(999999).String(), url.Values{
		"csrf": {env.csrf}, "version": {"3"}, "value": {"Ghost"},
	}, env.cookies, nil)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown field value = %d, want 404", unknown.Code)
	}
}

func TestClientValuesShowReadOnlyToViewersAndRefuseTheirWrites(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada"},
	}, env.cookies, nil)
	detail := created.Header().Get("Location")
	request(env.routes, http.MethodPost, "/workspaces/"+env.workspaceID.String()+"/client-fields", url.Values{
		"csrf": {env.csrf}, "label": {"Source"}, "type": {"text"},
	}, env.cookies, nil)
	var fieldKey int64
	if err := env.db.QueryRow("SELECT id FROM client_fields").Scan(&fieldKey); err != nil {
		t.Fatal(err)
	}
	valuePath := detail + "/values/" + clients.FieldID(fieldKey).String()
	request(env.routes, http.MethodPost, valuePath, url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "value": {"Friend"},
	}, env.cookies, nil)

	viewer := request(env.routes, http.MethodGet, detail, nil, env.viewerCookies, nil)
	body := viewer.Body.String()
	if viewer.Code != http.StatusOK || !strings.Contains(body, "Friend") || !strings.Contains(body, "Source") {
		t.Fatalf("viewer detail = %d, want the value shown: %s", viewer.Code, body)
	}
	if strings.Contains(body, `name="value"`) || strings.Contains(body, valuePath+"/clear") {
		t.Fatalf("viewer detail offers value controls: %s", body)
	}
	refused := request(env.routes, http.MethodPost, valuePath, url.Values{
		"csrf": {env.viewerCSRF}, "version": {"2"}, "value": {"Blocked"},
	}, env.viewerCookies, nil)
	if refused.Code != http.StatusForbidden {
		t.Fatalf("viewer set = %d, want 403", refused.Code)
	}
	refusedClear := request(env.routes, http.MethodPost, valuePath+"/clear", url.Values{
		"csrf": {env.viewerCSRF}, "version": {"2"},
	}, env.viewerCookies, nil)
	if refusedClear.Code != http.StatusForbidden {
		t.Fatalf("viewer clear = %d, want 403", refusedClear.Code)
	}
}

func TestClientValuesHideArchivedFieldsAndRefuseWritesToThem(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada"},
	}, env.cookies, nil)
	detail := created.Header().Get("Location")
	request(env.routes, http.MethodPost, "/workspaces/"+env.workspaceID.String()+"/client-fields", url.Values{
		"csrf": {env.csrf}, "label": {"Source"}, "type": {"text"},
	}, env.cookies, nil)
	var fieldKey int64
	if err := env.db.QueryRow("SELECT id FROM client_fields").Scan(&fieldKey); err != nil {
		t.Fatal(err)
	}
	fieldShort := clients.FieldID(fieldKey).String()
	valuePath := detail + "/values/" + fieldShort
	request(env.routes, http.MethodPost, env.fieldsPath()+"/"+fieldShort+"/archive", url.Values{"csrf": {env.csrf}}, env.cookies, nil)

	page := request(env.routes, http.MethodGet, detail, nil, env.cookies, nil).Body.String()
	if strings.Contains(page, valuePath) {
		t.Fatalf("archived field still has a value form: %s", page)
	}
	archived := request(env.routes, http.MethodPost, valuePath, url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "value": {"Late"},
	}, env.cookies, nil)
	if archived.Code != http.StatusConflict || !strings.Contains(archived.Body.String(), "This custom field is archived.") {
		t.Fatalf("archived field write = %d, want 409 with the archived message: %s", archived.Code, archived.Body.String())
	}
}
