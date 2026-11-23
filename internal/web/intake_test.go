package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestIntakeManagementThroughWeb(t *testing.T) {
	env := newClientsTestEnv(t)
	path := "/workspaces/" + env.workspaceID.String() + "/intake"
	page := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Link disabled") {
		t.Fatalf("initial: %d", page.Code)
	}
	created := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "action": {"create"}, "consent": {"I agree"}}, env.cookies, nil)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), "/intake/") || !strings.Contains(created.Body.String(), "Link active") {
		t.Fatalf("create: %d", created.Code)
	}
	denied := request(env.routes, http.MethodGet, path, nil, env.viewerCookies, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("viewer page: %d", denied.Code)
	}
	denied = request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.viewerCSRF}, "action": {"disable"}}, env.viewerCookies, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("viewer disable: %d", denied.Code)
	}
	disabled := request(env.routes, http.MethodPost, path, url.Values{"csrf": {env.csrf}, "action": {"disable"}}, env.cookies, nil)
	if disabled.Code != http.StatusSeeOther {
		t.Fatalf("disable: %d", disabled.Code)
	}
}
