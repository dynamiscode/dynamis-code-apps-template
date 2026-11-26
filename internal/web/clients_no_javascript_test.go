package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// Ported from scripts/playwright/clients-list.spec.mjs (javaScriptEnabled:false block).

func TestNoJavaScriptClientsCreateEditArchiveAndValidation(t *testing.T) {
	env := newClientsTestEnv(t)
	listPath := env.listPath()

	blank := request(env.routes, http.MethodPost, listPath, url.Values{"csrf": {env.csrf}, "display_name": {"   "}}, env.cookies, nil)
	if blank.Code != http.StatusUnprocessableEntity || !strings.Contains(blank.Body.String(), `role="alert"`) {
		t.Fatalf("blank client create = %d: %s", blank.Code, blank.Body.String())
	}
	created := request(env.routes, http.MethodPost, listPath, url.Values{"csrf": {env.csrf}, "display_name": {"Native client"}}, env.cookies, nil)
	detailPath := created.Header().Get("Location")
	if created.Code != http.StatusSeeOther || !regexp.MustCompile(`/clients/[^/?]+$`).MatchString(detailPath) {
		t.Fatalf("client create = %d, location %q", created.Code, detailPath)
	}
	saved := request(env.routes, http.MethodPost, detailPath, url.Values{"csrf": {env.csrf}, "version": {"1"}, "display_name": {"Native client updated"}}, env.cookies, nil)
	if saved.Code != http.StatusSeeOther {
		t.Fatalf("client save = %d: %s", saved.Code, saved.Body.String())
	}
	detail := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil)
	if !strings.Contains(detail.Body.String(), "Native client updated") {
		t.Fatalf("client detail missing saved name: %s", detail.Body.String())
	}
	archived := request(env.routes, http.MethodPost, detailPath+"/archive", url.Values{"csrf": {env.csrf}}, env.cookies, nil)
	if archived.Code != http.StatusSeeOther {
		t.Fatalf("client archive = %d: %s", archived.Code, archived.Body.String())
	}
	after := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil)
	if !strings.Contains(after.Body.String(), "Restore") {
		t.Fatalf("archived client lacks Restore action: %s", after.Body.String())
	}
}

func TestClientEditPreservesRejectedDraftAndStoredValues(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Original"}, "email": {"original@example.com"}, "notes": {"Original notes"},
	}, env.cookies, nil)
	path := created.Header().Get("Location")
	updated := request(env.routes, http.MethodPost, path, url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "display_name": {"Current"}, "email": {"current@example.com"}, "notes": {"Current notes"},
	}, env.cookies, nil)
	if updated.Code != http.StatusSeeOther {
		t.Fatalf("update = %d: %s", updated.Code, updated.Body.String())
	}
	conflict := request(env.routes, http.MethodPost, path, url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "display_name": {"My draft"}, "email": {"draft@example.com"}, "notes": {"Draft notes"},
	}, env.cookies, nil)
	body := conflict.Body.String()
	if conflict.Code != http.StatusPreconditionFailed || !strings.Contains(body, "Current notes") ||
		!strings.Contains(body, `value="My draft"`) || !strings.Contains(body, `value="draft@example.com"`) ||
		!strings.Contains(body, "Draft notes") || !strings.Contains(body, `value="2"`) ||
		!strings.Contains(body, "data-detail-dialog") {
		t.Fatalf("conflict lost stored values or draft: %d %s", conflict.Code, body)
	}
	view := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	if !strings.Contains(view.Body.String(), "Current notes") || strings.Contains(view.Body.String(), "Draft notes") {
		t.Fatalf("conflict changed stored client: %s", view.Body.String())
	}
}

func TestClientInlineFieldsRenderAndPreserveDrafts(t *testing.T) {
	env := newClientsTestEnv(t)
	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Original"}, "email": {"original@example.com"}, "phone": {"123"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	path := created.Header().Get("Location")
	page := request(env.routes, http.MethodGet, path, nil, env.cookies, nil).Body.String()
	for _, want := range []string{
		`<h1>Original</h1>`, `action="` + path + `/field"`, `name="field" value="display_name"`,
		`name="field" value="email"`, `name="field" value="phone"`, `name="field" value="notes"`, `type="email"`,
		`data-inline-cancel`, `name="version" value="1"`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("detail missing %q: %s", want, page)
		}
	}
	if strings.Contains(page, `data-detail-dialog`) {
		t.Fatalf("normal detail offers the full editor: %s", page)
	}
	viewer := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil).Body.String()
	if strings.Contains(viewer, `action="`+path+`/field"`) || !strings.Contains(viewer, "original@example.com") {
		t.Fatalf("viewer can edit or cannot see saved value: %s", viewer)
	}
	invalid := request(env.routes, http.MethodPost, path+"/field", url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "field": {"display_name"}, "value": {"   "},
	}, env.cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `value="   "`) ||
		!strings.Contains(invalid.Body.String(), `data-inline-dirty-on-load`) ||
		!strings.Contains(invalid.Body.String(), `aria-describedby="inline-display_name-error"`) {
		t.Fatalf("invalid draft lost: %d %s", invalid.Code, invalid.Body.String())
	}
	saved := request(env.routes, http.MethodPost, path+"/field", url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "field": {"phone"}, "value": {"456"},
	}, env.cookies, nil)
	if saved.Code != http.StatusSeeOther {
		t.Fatalf("save = %d: %s", saved.Code, saved.Body.String())
	}
	conflict := request(env.routes, http.MethodPost, path+"/field", url.Values{
		"csrf": {env.csrf}, "version": {"1"}, "field": {"email"}, "value": {"draft@example.com"},
	}, env.cookies, nil)
	body := conflict.Body.String()
	for _, want := range []string{`value="draft@example.com"`, `data-inline-explicit-retry`, `name="version" value="2"`, `Try again`, `data-inline-cancel`} {
		if conflict.Code != http.StatusConflict || !strings.Contains(body, want) {
			t.Fatalf("conflict missing %q: %d %s", want, conflict.Code, body)
		}
	}
}
