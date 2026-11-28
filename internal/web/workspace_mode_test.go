package web

import (
	"context"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSingleWorkspaceModeHidesCreationAndReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	handler, auth, workspaceID, owner := testWebConfigured(t, 10, false, func(h *Handler) {
		h.SetWorkspaceMode(config.WorkspaceModeSingle)
	})
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}

	home := request(handler, http.MethodGet, "/", nil, cookies, nil)
	if home.Code != http.StatusFound || home.Header().Get("Location") != "/workspaces/"+workspaceID.String() || strings.Contains(home.Body.String(), `action="/workspaces"`) {
		t.Fatalf("single mode home = %d, location = %q, want 302 to the oldest workspace", home.Code, home.Header().Get("Location"))
	}

	created := request(handler, http.MethodPost, "/workspaces", url.Values{
		"name": {"Blocked"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if created.Code != http.StatusNotFound {
		t.Fatalf("single mode POST /workspaces = %d, want 404", created.Code)
	}
	workspaces, err := auth.ListWorkspaces(ctx, owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 1 {
		t.Fatalf("single mode created a workspace: have %d, want 1", len(workspaces))
	}
}

func TestMultiWorkspaceModeKeepsCreation(t *testing.T) {
	ctx := context.Background()
	handler, auth, _, owner := testWebConfigured(t, 10, false, func(h *Handler) {
		h.SetWorkspaceMode(config.WorkspaceModeMulti)
	})
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}

	home := request(handler, http.MethodGet, "/", nil, cookies, nil)
	if home.Code != http.StatusOK || !strings.Contains(home.Body.String(), `action="/workspaces"`) {
		t.Fatalf("multi mode home = %d, create form missing", home.Code)
	}
	if !strings.Contains(home.Body.String(), `name="csrf" value="`+session.CSRFSecret+`"`) ||
		!strings.Contains(home.Body.String(), `data-webmcp-page="home"`) ||
		!strings.Contains(home.Header().Get("Content-Type"), "text/html") {
		t.Fatal("workspace selection lacks its templ HTML response, CSRF field, or WebMCP marker")
	}

	invalid := request(handler, http.MethodPost, "/workspaces", url.Values{
		"name": {""}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `action="/workspaces"`) || !strings.Contains(invalid.Body.String(), `role="alert"`) {
		t.Fatalf("multi mode invalid create = %d, form not re-rendered", invalid.Code)
	}

	created := request(handler, http.MethodPost, "/workspaces", url.Values{
		"name": {"Second"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if created.Code != http.StatusSeeOther || created.Header().Get("Location") != "/" {
		t.Fatalf("multi mode POST /workspaces = %d, %s", created.Code, created.Body.String())
	}
	workspaces, err := auth.ListWorkspaces(ctx, owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 2 {
		t.Fatalf("multi mode workspaces = %d, want 2", len(workspaces))
	}
}

func TestWorkspaceHomeOffersSettings(t *testing.T) {
	handler, auth, _, owner := testWeb(t, 10, nil)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	workspaces, err := auth.ListWorkspaces(context.Background(), owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	page := request(handler, http.MethodGet, "/workspaces/"+workspaces[0].ID.String(), nil, cookies, nil)

	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `href="/workspaces/`+workspaces[0].ID.String()+`/settings"`) {
		t.Fatalf("workspace home lacks settings link: %d, %s", page.Code, page.Body.String())
	}
	if strings.Contains(page.Body.String(), `href="/workspaces/`+workspaces[0].ID.String()+`/items"`) {
		t.Fatal("workspace home links to removed Items feature")
	}
	if !strings.Contains(page.Body.String(), `<details class="dropdown dropdown-end">`) ||
		!strings.Contains(page.Body.String(), `href="/"`) {
		t.Fatalf("workspace home lacks native workspace-switching navigation: %s", page.Body.String())
	}
}
