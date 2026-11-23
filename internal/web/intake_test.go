package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/intake"
	"example.com/dynamis-code/apps-template/internal/platform/config"
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

func TestPublicIntakeForm(t *testing.T) {
	env := newClientsTestEnv(t)
	owner, err := env.auth.Authorize(context.Background(), env.ownerID, env.workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engagement.NewService(env.db, config.SQLite, env.auth).CreateStage(context.Background(), owner, env.workspaceID, engagement.StageInput{Name: "New", Kind: "open"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	svc := intake.NewService(env.db, config.SQLite, env.auth)
	token, err := svc.Create(context.Background(), owner, env.workspaceID, "Custom consent", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	page := request(env.routes, http.MethodGet, "/intake/"+token, nil, nil, map[string]string{"Accept-Language": "es"})
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Custom consent") || !strings.Contains(page.Body.String(), "Nombre") {
		t.Fatalf("public form: status %d, body %s", page.Code, page.Body.String())
	}
	invalid := request(env.routes, http.MethodPost, "/intake/"+token, url.Values{"name": {"Visitor"}}, nil, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "Visitor") {
		t.Fatalf("invalid form: %d", invalid.Code)
	}
	result := request(env.routes, http.MethodPost, "/intake/"+token, url.Values{"name": {"Visitor"}, "email": {"visitor@example.com"}, "consent": {"on"}}, nil, nil)
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), "Thank you") || strings.Contains(result.Body.String(), "Visitor") || strings.Contains(result.Body.String(), "/workspaces/") {
		t.Fatalf("confirmation: %d, body %s", result.Code, result.Body.String())
	}
	bot := request(env.routes, http.MethodPost, "/intake/"+token, url.Values{"website": {"spam"}}, nil, nil)
	var count int
	if err := env.db.QueryRow(`SELECT COUNT(*) FROM clients`).Scan(&count); err != nil || bot.Code != http.StatusOK || bot.Body.String() != result.Body.String() || count != 1 {
		t.Fatalf("honeypot: status %d, clients %d, error %v", bot.Code, count, err)
	}
	missing := request(env.routes, http.MethodGet, "/intake/unknown", nil, nil, nil)
	if missing.Code != http.StatusNotFound || strings.Contains(missing.Body.String(), "Custom consent") {
		t.Fatalf("unknown link: %d", missing.Code)
	}
}
