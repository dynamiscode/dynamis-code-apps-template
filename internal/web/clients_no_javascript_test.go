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
	if !strings.Contains(after.Body.String(), "Unarchive") {
		t.Fatalf("archived client lacks Unarchive action: %s", after.Body.String())
	}
}
