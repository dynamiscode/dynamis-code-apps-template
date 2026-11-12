package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestSingleWorkspaceModeHidesCreationAndReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	handler, auth, _, _, owner := testWebConfigured(t, 10, false, func(h *Handler) {
		h.SetWorkspaceMode(config.WorkspaceModeSingle)
	})
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}

	home := request(handler, http.MethodGet, "/", nil, cookies, nil)
	if home.Code != http.StatusFound || strings.Contains(home.Body.String(), `action="/workspaces"`) {
		t.Fatalf("single mode home = %d, want 302 to the oldest workspace", home.Code)
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
	handler, auth, _, _, owner := testWebConfigured(t, 10, false, func(h *Handler) {
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

	invalid := request(handler, http.MethodPost, "/workspaces", url.Values{
		"name": {""}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `action="/workspaces"`) {
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
