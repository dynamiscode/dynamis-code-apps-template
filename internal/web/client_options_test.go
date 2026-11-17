package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
)

// TestClientChoiceOptionsManagedThroughForms covers the option manager on the
// custom-fields page: an owner adds and deletes choices with ordinary forms, and
// a blank label or a missing field is refused.
func TestClientChoiceOptionsManagedThroughForms(t *testing.T) {
	env := newClientsTestEnv(t)
	defined := request(env.routes, http.MethodPost, env.fieldsPath(), url.Values{
		"csrf": {env.csrf}, "label": {"Tier"}, "type": {"single_choice"},
	}, env.cookies, nil)
	if defined.Code != http.StatusSeeOther {
		t.Fatalf("define single-choice field = %d: %s", defined.Code, defined.Body.String())
	}
	var fieldKey int64
	if err := env.db.QueryRow("SELECT id FROM client_fields WHERE field_type = 'single_choice'").Scan(&fieldKey); err != nil {
		t.Fatal(err)
	}
	fieldShort := clients.FieldID(fieldKey).String()
	optionsPath := env.fieldsPath() + "/" + fieldShort + "/options"

	page := request(env.routes, http.MethodGet, env.fieldsPath(), nil, env.cookies, nil).Body.String()
	if !strings.Contains(page, `action="`+optionsPath+`"`) {
		t.Fatalf("owner fields page has no option form for the choice field: %s", page)
	}

	added := request(env.routes, http.MethodPost, optionsPath, url.Values{
		"csrf": {env.csrf}, "label": {"<i>Gold</i>"},
	}, env.cookies, nil)
	if added.Code != http.StatusSeeOther || added.Header().Get("Location") != env.fieldsPath() {
		t.Fatalf("add option = %d location %q: %s", added.Code, added.Header().Get("Location"), added.Body.String())
	}
	var optionKey int64
	if err := env.db.QueryRow("SELECT id FROM client_field_options").Scan(&optionKey); err != nil {
		t.Fatal(err)
	}
	optionShort := clients.OptionID(optionKey).String()
	page = request(env.routes, http.MethodGet, env.fieldsPath(), nil, env.cookies, nil).Body.String()
	if !strings.Contains(page, "&lt;i&gt;Gold&lt;/i&gt;") || strings.Contains(page, "<i>Gold</i>") {
		t.Fatalf("option label is not rendered exactly as typed: %s", page)
	}
	if !strings.Contains(page, optionsPath+"/"+optionShort+"/delete") {
		t.Fatalf("owner fields page does not offer deleting the option: %s", page)
	}

	blank := request(env.routes, http.MethodPost, optionsPath, url.Values{
		"csrf": {env.csrf}, "label": {"   "},
	}, env.cookies, nil)
	if blank.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blank option label = %d, want 422", blank.Code)
	}
	unknown := request(env.routes, http.MethodPost, env.fieldsPath()+"/"+clients.FieldID(999999).String()+"/options", url.Values{
		"csrf": {env.csrf}, "label": {"Gold"},
	}, env.cookies, nil)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("option on an unknown field = %d, want 404", unknown.Code)
	}

	deleted := request(env.routes, http.MethodPost, optionsPath+"/"+optionShort+"/delete", url.Values{
		"csrf": {env.csrf},
	}, env.cookies, nil)
	if deleted.Code != http.StatusSeeOther {
		t.Fatalf("delete option = %d: %s", deleted.Code, deleted.Body.String())
	}
	page = request(env.routes, http.MethodGet, env.fieldsPath(), nil, env.cookies, nil).Body.String()
	if strings.Contains(page, optionShort) {
		t.Fatalf("deleted option still on the fields page: %s", page)
	}
}

// TestClientChoiceValuesUseASelectAndViewersReadLabels covers the client page: an
// owner picks an option from a select, a typed label is refused, and a viewer
// reads the option's label with no form.
func TestClientChoiceValuesUseASelectAndViewersReadLabels(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create client = %d: %s", created.Code, created.Body.String())
	}
	detail := created.Header().Get("Location")
	defined := request(env.routes, http.MethodPost, env.fieldsPath(), url.Values{
		"csrf": {env.csrf}, "label": {"Tier"}, "type": {"single_choice"},
	}, env.cookies, nil)
	if defined.Code != http.StatusSeeOther {
		t.Fatalf("define field = %d: %s", defined.Code, defined.Body.String())
	}
	var fieldKey int64
	if err := env.db.QueryRow("SELECT id FROM client_fields").Scan(&fieldKey); err != nil {
		t.Fatal(err)
	}
	fieldShort := clients.FieldID(fieldKey).String()
	optionsPath := env.fieldsPath() + "/" + fieldShort + "/options"
	if added := request(env.routes, http.MethodPost, optionsPath, url.Values{
		"csrf": {env.csrf}, "label": {"Gold"},
	}, env.cookies, nil); added.Code != http.StatusSeeOther {
		t.Fatalf("add option = %d: %s", added.Code, added.Body.String())
	}
	var optionKey int64
	if err := env.db.QueryRow("SELECT id FROM client_field_options").Scan(&optionKey); err != nil {
		t.Fatal(err)
	}
	optionShort := clients.OptionID(optionKey).String()
	valuePath := detail + "/values/" + fieldShort

	owner := request(env.routes, http.MethodGet, detail, nil, env.cookies, nil).Body.String()
	if !strings.Contains(owner, `<select class="select" id="value-`+fieldShort+`"`) || !strings.Contains(owner, `<option value="`+optionShort+`"`) {
		t.Fatalf("owner detail has no select of the field's options: %s", owner)
	}

	refused := request(env.routes, http.MethodPost, valuePath, url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "value": {"Gold"},
	}, env.cookies, nil)
	if refused.Code != http.StatusUnprocessableEntity {
		t.Fatalf("typed label as a choice value = %d, want 422", refused.Code)
	}
	set := request(env.routes, http.MethodPost, valuePath, url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "value": {optionShort},
	}, env.cookies, nil)
	if set.Code != http.StatusSeeOther || set.Header().Get("Location") != detail {
		t.Fatalf("set choice value = %d location %q: %s", set.Code, set.Header().Get("Location"), set.Body.String())
	}

	viewer := request(env.routes, http.MethodGet, detail, nil, env.viewerCookies, nil).Body.String()
	if !strings.Contains(viewer, "<dd>Gold</dd>") {
		t.Fatalf("viewer does not read the option label: %s", viewer)
	}
	if strings.Contains(viewer, "<select") || strings.Contains(viewer, optionShort) {
		t.Fatalf("viewer page exposes the choice controls or ID: %s", viewer)
	}
}

// TestClientOptionDeleteAsksBeforeClearingValues covers the immediate option
// delete, which clears the option from every client that chose it, so its
// button asks first when scripting is available.
func TestClientOptionDeleteAsksBeforeClearingValues(t *testing.T) {
	env := newClientsTestEnv(t)
	defined := request(env.routes, http.MethodPost, env.fieldsPath(), url.Values{
		"csrf": {env.csrf}, "label": {"Tier"}, "type": {"single_choice"},
	}, env.cookies, nil)
	if defined.Code != http.StatusSeeOther {
		t.Fatalf("define field = %d: %s", defined.Code, defined.Body.String())
	}
	var fieldKey int64
	if err := env.db.QueryRow("SELECT id FROM client_fields WHERE field_type = 'single_choice'").Scan(&fieldKey); err != nil {
		t.Fatal(err)
	}
	fieldShort := clients.FieldID(fieldKey).String()
	added := request(env.routes, http.MethodPost, env.fieldsPath()+"/"+fieldShort+"/options", url.Values{
		"csrf": {env.csrf}, "label": {"Gold"},
	}, env.cookies, nil)
	if added.Code != http.StatusSeeOther {
		t.Fatalf("add option = %d: %s", added.Code, added.Body.String())
	}
	page := request(env.routes, http.MethodGet, env.fieldsPath(), nil, env.cookies, nil).Body.String()
	want := `data-confirm="Delete this option? Clients that chose it lose that value. This cannot be undone."`
	if !strings.Contains(page, want) {
		t.Fatalf("option delete has no confirmation: %s", page)
	}
}
