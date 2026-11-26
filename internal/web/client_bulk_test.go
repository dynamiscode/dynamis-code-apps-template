package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var bulkClientInput = regexp.MustCompile(`<input[^>]*\bname="client_id"[^>]*>`)
var bulkClientValue = regexp.MustCompile(`\bvalue="([^"]+)"`)

// TestClientBulkBarAndEndpoint checks that the bulk bar shows only to users who can act on clients, that the
// endpoint archives the selected clients and returns to the list, and that a viewer cannot use it.
func TestClientBulkBarAndEndpoint(t *testing.T) {
	env := newClientsTestEnv(t)
	for _, name := range []string{"Ada", "Bea"} {
		created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
			"csrf": {env.csrf}, "display_name": {name}, "email": {strings.ToLower(name) + "@example.test"},
		}, env.cookies, nil)
		if created.Code != http.StatusSeeOther {
			t.Fatalf("create %s = %d", name, created.Code)
		}
	}
	owner := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil)
	if owner.Code != http.StatusOK || !strings.Contains(owner.Body.String(), `id="client-bulk"`) {
		t.Fatalf("owner list = %d, want the bulk bar", owner.Code)
	}
	if !regexp.MustCompile(`<button[^>]*class="btn btn-outline btn-xs"[^>]*value="archive"`).MatchString(owner.Body.String()) {
		t.Fatal("bulk archive is not a secondary action")
	}
	if !strings.Contains(owner.Body.String(), "their values, engagements, and engagement history") {
		t.Fatal("bulk deletion warning is missing")
	}
	// Each client renders a table checkbox and a card checkbox with the same ID, so keep one per ID.
	var ids []string
	seen := map[string]bool{}
	for _, input := range bulkClientInput.FindAllString(owner.Body.String(), -1) {
		match := bulkClientValue.FindStringSubmatch(input)
		if match == nil {
			continue
		}
		if !seen[match[1]] {
			seen[match[1]] = true
			ids = append(ids, match[1])
		}
	}
	if len(ids) != 2 {
		t.Fatalf("owner list has %d selectable clients, want 2", len(ids))
	}
	viewer := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.viewerCookies, nil)
	if strings.Contains(viewer.Body.String(), `id="client-bulk"`) {
		t.Fatal("viewer list shows the bulk bar")
	}

	selected := url.Values{"csrf": {env.csrf}, "return": {env.listPath()}, "action": {"archive"}}
	for _, id := range ids {
		selected.Add("client_id", id)
	}
	denied := request(env.routes, http.MethodPost, env.listPath()+"/bulk", url.Values{
		"csrf": {env.viewerCSRF}, "return": {env.listPath()}, "action": {"archive"}, "client_id": {ids[0]},
	}, env.viewerCookies, nil)
	if denied.Code == http.StatusSeeOther {
		t.Fatalf("viewer bulk archive = %d, want a refusal", denied.Code)
	}
	archived := request(env.routes, http.MethodPost, env.listPath()+"/bulk", selected, env.cookies, nil)
	if archived.Code != http.StatusSeeOther || archived.Header().Get("Location") != env.listPath() {
		t.Fatalf("bulk archive = %d to %q, want 303 back to the list", archived.Code, archived.Header().Get("Location"))
	}
	active := request(env.routes, http.MethodGet, env.listPath(), nil, env.cookies, nil)
	if strings.Contains(active.Body.String(), `name="client_id"`) {
		t.Fatal("archived clients still listed as active")
	}
	bad := request(env.routes, http.MethodPost, env.listPath()+"/bulk", url.Values{
		"csrf": {env.csrf}, "action": {"unarchive"}, "client_id": {ids[0]},
	}, env.cookies, nil)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("unknown bulk action = %d, want 400", bad.Code)
	}
}

func TestClientBulkDeleteRequiresConfirmation(t *testing.T) {
	env := newClientsTestEnv(t)
	var paths []string
	for _, name := range []string{"Ada", "Bea", "Cleo"} {
		created := request(env.routes, http.MethodPost, env.listPath(), url.Values{"csrf": {env.csrf}, "display_name": {name}}, env.cookies, nil)
		paths = append(paths, created.Header().Get("Location"))
	}
	values := url.Values{"csrf": {env.csrf}, "action": {"delete"}, "return": {env.listPath()}, "client_id": {
		strings.TrimPrefix(paths[0], env.listPath()+"/"), strings.TrimPrefix(paths[1], env.listPath()+"/"),
	}}
	prompt := request(env.routes, http.MethodPost, env.listPath()+"/bulk", values, env.cookies, nil)
	if prompt.Code != http.StatusOK || !strings.Contains(prompt.Body.String(), "Ada") || !strings.Contains(prompt.Body.String(), "Bea") || strings.Contains(prompt.Body.String(), "Cleo") {
		t.Fatalf("confirmation scope = %d %s", prompt.Code, prompt.Body.String())
	}
	for _, path := range paths {
		if got := request(env.routes, http.MethodGet, path, nil, env.cookies, nil); got.Code != http.StatusOK {
			t.Fatalf("unconfirmed deletion changed %s: %d", path, got.Code)
		}
	}
	values.Set("confirm_delete", "yes")
	deleted := request(env.routes, http.MethodPost, env.listPath()+"/bulk", values, env.cookies, nil)
	if deleted.Code != http.StatusSeeOther {
		t.Fatalf("confirmed bulk deletion = %d", deleted.Code)
	}
	for i, path := range paths {
		want := http.StatusNotFound
		if i == 2 {
			want = http.StatusOK
		}
		if got := request(env.routes, http.MethodGet, path, nil, env.cookies, nil); got.Code != want {
			t.Fatalf("detail %s = %d, want %d", path, got.Code, want)
		}
	}
}
