package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestCatalogWebWorkspaceIsolation(t *testing.T) {
	routes, auth, workspace, owner := testWeb(t, 2)
	ctx := context.Background()
	actor, err := auth.Authorize(ctx, owner.UserID, workspace, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	other, err := auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspace.String() + "/catalog"
	otherPath := "/workspaces/" + other.String() + "/catalog"
	form := url.Values{"csrf": {session.CSRFSecret}, "name": {"Private"}, "mode": {"fixed"}, "amount": {"0.123456789012345678"}, "currency": {"BTC"}, "variant": {"Private"}}
	if r := request(routes, http.MethodPost, path, form, cookies, nil); r.Code != http.StatusSeeOther {
		t.Fatal(r.Code)
	}
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	match := regexp.MustCompile(`action="([^"]+/edit)"`).FindStringSubmatch(page.Body.String())
	if len(match) != 2 {
		t.Fatal("missing entry")
	}
	entry := strings.TrimSuffix(match[1], "/edit")
	if r := request(routes, http.MethodPost, entry+"/prices", form, cookies, nil); r.Code != http.StatusSeeOther {
		t.Fatal(r.Code)
	}
	page = request(routes, http.MethodGet, path, nil, cookies, nil)
	match = regexp.MustCompile(`action="([^"]+/prices/[^"]+/edit)"`).FindStringSubmatch(page.Body.String())
	if len(match) != 2 {
		t.Fatal("missing price")
	}
	price := match[1]
	page = request(routes, http.MethodGet, otherPath+"?includeArchived=true", nil, cookies, nil)
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), "Private") {
		t.Fatalf("foreign list: %d", page.Code)
	}
	foreign := strings.Replace(entry, path, otherPath, 1)
	foreignPrice := strings.Replace(price, path, otherPath, 1)
	for _, op := range []struct {
		target string
		want   int
	}{{foreign + "/edit", 404}, {foreign + "/archive", 404}, {foreign + "/unarchive", 404}, {foreign + "/delete", 404}, {foreign + "/prices", 422}, {foreignPrice, 404}, {strings.TrimSuffix(foreignPrice, "/edit") + "/delete", 404}} {
		r := request(routes, http.MethodPost, op.target, form, cookies, nil)
		if r.Code != op.want {
			t.Errorf("%s: %d want %d", op.target, r.Code, op.want)
		}
	}
	page = request(routes, http.MethodGet, path, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Private") || !strings.Contains(page.Body.String(), "0.123456789012345678") {
		t.Fatal("original changed")
	}
}
