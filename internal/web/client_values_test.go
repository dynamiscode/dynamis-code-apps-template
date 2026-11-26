package web

import (
	"example.com/dynamis-code/apps-template/internal/clients"
	"net/http"
	"net/url"
	"strings"
	"testing"
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
	fieldID := clients.FieldID(fieldKey).String()
	for _, want := range []string{
		`data-inline-saved=""`, `for="value-` + fieldID + `"`,
		`id="value-` + fieldID + `"`, `data-inline-input`, `data-inline-cancel`,
		`name="csrf" value="` + env.csrf + `"`, `name="version" value="1"`,
	} {
		if !strings.Contains(page.Body.String(), want) {
			t.Fatalf("empty text value missing %q: %s", want, page.Body.String())
		}
	}
	if strings.Contains(page.Body.String(), `formaction="`+valuePath+`/clear"`) {
		t.Fatalf("empty text value offers clear: %s", page.Body.String())
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
	if !strings.Contains(shown, `data-inline-saved="&lt;i&gt;Friend&lt;/i&gt;"`) ||
		!strings.Contains(shown, `formaction="`+valuePath+`/clear"`) {
		t.Fatalf("saved text value lacks raw value or clear action: %s", shown)
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
	if !strings.Contains(stale.Body.String(), `value="Stale"`) || !strings.Contains(stale.Body.String(), `value="2"`) {
		t.Fatalf("stale value draft or current version missing: %s", stale.Body.String())
	}
	if !strings.Contains(stale.Body.String(), `data-inline-saved="&lt;i&gt;Friend&lt;/i&gt;"`) ||
		!strings.Contains(stale.Body.String(), `data-inline-dirty-on-load`) ||
		!strings.Contains(stale.Body.String(), `data-inline-cancel`) {
		t.Fatalf("stale text editor lost saved value or recovery controls: %s", stale.Body.String())
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

func TestClientValuesUseTypedInputsAndValidate(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create client = %d: %s", created.Code, created.Body.String())
	}
	detail := created.Header().Get("Location")
	fieldsPath := "/workspaces/" + env.workspaceID.String() + "/client-fields"
	for _, fieldType := range []string{"number", "date", "yes_no"} {
		defined := request(env.routes, http.MethodPost, fieldsPath, url.Values{
			"csrf": {env.csrf}, "label": {"Field " + fieldType}, "type": {fieldType},
		}, env.cookies, nil)
		if defined.Code != http.StatusSeeOther {
			t.Fatalf("define %s field = %d: %s", fieldType, defined.Code, defined.Body.String())
		}
	}
	fieldIDs := map[string]string{}
	rows, err := env.db.Query("SELECT id, field_type FROM client_fields")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var key int64
		var fieldType string
		if err := rows.Scan(&key, &fieldType); err != nil {
			t.Fatal(err)
		}
		fieldIDs[fieldType] = clients.FieldID(key).String()
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	page := request(env.routes, http.MethodGet, detail, nil, env.cookies, nil).Body.String()
	if !strings.Contains(page, `href="`+fieldsPath+`">Add field</a>`) ||
		!strings.Contains(page, `<span>Manage custom fields</span>`) ||
		strings.Contains(page, `id="client-values-heading"`) {
		t.Fatalf("custom fields actions or layout missing: %s", page)
	}
	for _, want := range []string{`type="number"`, `data-entry-number="float"`, `type="date"`, `<option value="true">`, `<option value="false">`} {
		if !strings.Contains(page, want) {
			t.Fatalf("detail page lacks %s: %s", want, page)
		}
	}
	if strings.Count(page, `data-inline-input`) < 2 {
		t.Fatal("number and date fields do not offer inline editing")
	}
	yesStart := strings.Index(page, `<form action="`+detail+`/values/`+fieldIDs["yes_no"]+`"`)
	if yesStart < 0 || !strings.Contains(page[yesStart:], `for="value-`+fieldIDs["yes_no"]+`"`) {
		t.Fatalf("yes/no selector is not immediately visible and labeled: %s", page)
	}
	yesEnd := strings.Index(page[yesStart:], "</form>")
	if yesEnd < 0 || !strings.Contains(page[yesStart:yesStart+yesEnd], "data-confirm-select-dialog") ||
		!strings.Contains(page[yesStart:yesStart+yesEnd], "Field yes_no:") ||
		!strings.Contains(page[yesStart:yesStart+yesEnd], `name="version"`) {
		t.Fatal("yes/no confirmation or version missing from its form")
	}

	invalid := request(env.routes, http.MethodPost, detail+"/values/"+fieldIDs["number"], url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "value": {"abc"},
	}, env.cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "The value is invalid.") {
		t.Fatalf("invalid number = %d, want 422 with the invalid-value message: %s", invalid.Code, invalid.Body.String())
	}
	yes := request(env.routes, http.MethodPost, detail+"/values/"+fieldIDs["yes_no"], url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "value": {"true"},
	}, env.cookies, nil)
	if yes.Code != http.StatusSeeOther {
		t.Fatalf("set yes/no = %d: %s", yes.Code, yes.Body.String())
	}
	shown := request(env.routes, http.MethodGet, detail, nil, env.cookies, nil).Body.String()
	if !strings.Contains(shown, `<option value="true" selected>`) {
		t.Fatalf("stored yes/no value is not selected: %s", shown)
	}
	if !strings.Contains(shown, `action="`+detail+`/values/`+fieldIDs["yes_no"]+`/clear"`) {
		t.Fatalf("stored yes/no value cannot be cleared: %s", shown)
	}
	conflict := request(env.routes, http.MethodPost, detail+"/values/"+fieldIDs["yes_no"], url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "value": {"false"},
	}, env.cookies, nil)
	if conflict.Code != http.StatusPreconditionFailed || !strings.Contains(conflict.Body.String(), `<option value="false" selected>`) ||
		!strings.Contains(conflict.Body.String(), `data-dirty-on-load`) {
		t.Fatalf("yes/no conflict lost draft: %d %s", conflict.Code, conflict.Body.String())
	}
	viewerPage := request(env.routes, http.MethodGet, detail, nil, env.viewerCookies, nil).Body.String()
	if strings.Contains(viewerPage, `href="`+fieldsPath+`"`) {
		t.Fatalf("viewer can manage custom fields: %s", viewerPage)
	}
	if !strings.Contains(viewerPage, `<dd class="min-w-0 break-words">Yes</dd>`) {
		t.Fatalf("viewer does not see the yes/no value as Yes: %s", viewerPage)
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
