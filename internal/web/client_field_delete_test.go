package web

import (
	"example.com/dynamis-code/apps-template/internal/clients"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestArchivedFieldIsPermanentlyDeletedThroughForms covers the custom-fields
// page: the delete control appears only for an archived field, an active field
// is refused, and a confirmed delete removes the field.
func TestArchivedFieldIsPermanentlyDeletedThroughForms(t *testing.T) {
	env := newClientsTestEnv(t)
	for _, label := range []string{"Source", "Keep"} {
		defined := request(env.routes, http.MethodPost, env.fieldsPath(), url.Values{
			"csrf": {env.csrf}, "label": {label}, "type": {"text"},
		}, env.cookies, nil)
		if defined.Code != http.StatusSeeOther {
			t.Fatalf("define %s = %d: %s", label, defined.Code, defined.Body.String())
		}
	}
	var fieldKey int64
	if err := env.db.QueryRow("SELECT id FROM client_fields WHERE label = 'Source'").Scan(&fieldKey); err != nil {
		t.Fatal(err)
	}
	fieldShort := clients.FieldID(fieldKey).String()
	deletePath := env.fieldsPath() + "/" + fieldShort + "/delete"

	active := request(env.routes, http.MethodGet, env.fieldsPath(), nil, env.cookies, nil).Body.String()
	if strings.Contains(active, deletePath) {
		t.Fatalf("active field offers permanent delete: %s", active)
	}
	refused := request(env.routes, http.MethodPost, deletePath, url.Values{"csrf": {env.csrf}}, env.cookies, nil)
	if refused.Code != http.StatusConflict || !strings.Contains(refused.Body.String(), "Archive the custom field before deleting it.") {
		t.Fatalf("delete of an active field = %d: %s, want 409 with the archive message", refused.Code, refused.Body.String())
	}

	archived := request(env.routes, http.MethodPost, env.fieldsPath()+"/"+fieldShort+"/archive", url.Values{
		"csrf": {env.csrf},
	}, env.cookies, nil)
	if archived.Code != http.StatusSeeOther {
		t.Fatalf("archive field = %d: %s", archived.Code, archived.Body.String())
	}
	page := request(env.routes, http.MethodGet, env.fieldsPath(), nil, env.cookies, nil).Body.String()
	if !strings.Contains(page, `action="`+deletePath+`"`) || !strings.Contains(page, "data-confirm=") {
		t.Fatalf("archived field lacks a confirmed delete control: %s", page)
	}
	if !strings.Contains(page, `<p class="warning alert alert-warning py-2">Permanently delete this field? Its value is removed from every client and its choices are deleted. This cannot be undone.</p>`) {
		t.Fatalf("archived field does not explain the deletion effects without JavaScript: %s", page)
	}

	deleted := request(env.routes, http.MethodPost, deletePath, url.Values{"csrf": {env.csrf}}, env.cookies, nil)
	if deleted.Code != http.StatusSeeOther || deleted.Header().Get("Location") != env.fieldsPath() {
		t.Fatalf("delete archived field = %d location %q: %s", deleted.Code, deleted.Header().Get("Location"), deleted.Body.String())
	}
	page = request(env.routes, http.MethodGet, env.fieldsPath(), nil, env.cookies, nil).Body.String()
	if strings.Contains(page, fieldShort) || !strings.Contains(page, "Keep") {
		t.Fatalf("fields page after delete = %s; want Source gone and Keep listed", page)
	}
}
