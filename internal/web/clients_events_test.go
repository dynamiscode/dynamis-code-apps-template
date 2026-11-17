package web

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestClientEventsStreamDeliversChangesToViewersOnly(t *testing.T) {
	env := newClientsTestEnv(t)
	server := httptest.NewServer(env.routes)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	streamURL := server.URL + "/workspaces/" + env.workspaceID.String() + "/clients/events"
	stream, _ := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	addCookies(stream, env.viewerCookies)
	response, err := server.Client().Do(stream)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("viewer stream = %d, want 200", response.StatusCode)
	}
	reader := bufio.NewReader(response.Body)
	readUntil(t, reader, "event: resync")

	owner, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	owner.AuthMethod = "test"
	created, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(
		ctx, owner, env.workspaceID, clients.Input{DisplayName: "Secret Name"}, identity.AuditContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	event := readUntil(t, reader, "event: client.changed") + readUntil(t, reader, "\n\n")
	if !strings.Contains(event, created.ID.String()) || strings.Contains(event, "Secret Name") || !strings.Contains(event, `"schemaVersion":1`) {
		t.Fatalf("client change event = %q", event)
	}
}

func TestClientEventsStreamRefusesOtherWorkspaces(t *testing.T) {
	env := newClientsTestEnv(t)
	ctx := context.Background()
	owner, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	owner.AuthMethod = "test"
	otherWorkspace, err := env.auth.CreateWorkspace(ctx, owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	// The viewer belongs to the first workspace only, so the second one answers
	// not found, the same as a missing workspace.
	refused := request(env.routes, http.MethodGet, "/workspaces/"+otherWorkspace.String()+"/clients/events", nil, env.viewerCookies, nil)
	if refused.Code != http.StatusNotFound {
		t.Fatalf("viewer stream for another workspace = %d, want 404", refused.Code)
	}
	refusedFragment := request(env.routes, http.MethodGet, "/workspaces/"+otherWorkspace.String()+"/clients", nil,
		env.viewerCookies, map[string]string{"HX-Request": "true"})
	if refusedFragment.Code != http.StatusNotFound {
		t.Fatalf("viewer fragment for another workspace = %d, want 404", refusedFragment.Code)
	}
	missing := request(env.routes, http.MethodGet, "/workspaces/00000000000000000000000000000000/clients/events", nil, env.cookies, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing workspace stream = %d, want 404", missing.Code)
	}
}

func TestClientListHTMXRefreshRendersLiveFragment(t *testing.T) {
	env := newClientsTestEnv(t)
	listPath := "/workspaces/" + env.workspaceID.String() + "/clients"
	eventsAttribute := `data-events="` + listPath + `/events"`

	fragment := request(env.routes, http.MethodGet, listPath, nil, env.cookies, map[string]string{"HX-Request": "true"})
	if fragment.Code != http.StatusOK {
		t.Fatalf("fragment = %d, want 200", fragment.Code)
	}
	body := fragment.Body.String()
	if strings.Contains(body, "<!doctype") || !strings.Contains(body, `id="client-list"`) || !strings.Contains(body, eventsAttribute) || !strings.Contains(body, `id="realtime-status"`) {
		t.Fatalf("HTMX refresh should return the live client list fragment, got %q", body)
	}

	page := request(env.routes, http.MethodGet, listPath, nil, env.cookies, nil)
	if !strings.Contains(page.Body.String(), "<!doctype html>") || !strings.Contains(page.Body.String(), eventsAttribute) {
		t.Fatal("full client list page should keep the live attributes")
	}
}
