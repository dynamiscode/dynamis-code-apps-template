package web

import (
	"context"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type membersListTestEnv struct {
	routes      http.Handler
	auth        *identity.Service
	workspaceID identity.WorkspaceID
	ownerID     identity.UserID
	cookies     []*http.Cookie
	csrf        string
}

func newMembersListTestEnv(t *testing.T) membersListTestEnv {
	t.Helper()
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns = 1
	cfg.Database.MaxIdleConns = 1
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Members",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandlerWithServices(auth, nil, nil, cfg.HTTP, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler.EnableListPreferences(listprefs.NewService(db, cfg.Database.Driver, auth))
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	invitations, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(ctx, invitations, "viewer@example.com", identity.Viewer, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "viewer-long-password", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	return membersListTestEnv{
		routes: handler.Routes(), auth: auth, workspaceID: owner.WorkspaceID, ownerID: owner.UserID,
		cookies: []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}, csrf: session.CSRFSecret,
	}
}

func TestMembersListUsesSharedPresentationAndOptInPagination(t *testing.T) {
	env := newMembersListTestEnv(t)
	path := "/workspaces/" + env.workspaceID.String() + "/settings/members"
	all := request(env.routes, http.MethodGet, path+"?all=1", nil, env.cookies, nil)
	if all.Code != http.StatusOK || !strings.Contains(all.Body.String(), `class="list-view-table"`) ||
		!strings.Contains(all.Body.String(), `class="list-view-cards"`) ||
		!strings.Contains(all.Body.String(), `data-column="email"`) ||
		!strings.Contains(all.Body.String(), `aria-current="true"`) {
		t.Fatalf("default members list = %d, want shared unpaginated view: %s", all.Code, all.Body.String())
	}
	if !strings.Contains(all.Body.String(), `name="action" type="submit" value="remove"`) ||
		!strings.Contains(all.Body.String(), `name="action" type="submit" value="transfer"`) ||
		!strings.Contains(all.Body.String(), `type="checkbox" value="yes" required`) ||
		!strings.Contains(all.Body.String(), `data-webmcp-page="members"`) ||
		!strings.Contains(all.Body.String(), `data-webmcp-actions="member-role-update"`) ||
		!strings.Contains(all.Body.String(), `data-webmcp-actions="member-remove"`) ||
		!strings.Contains(all.Body.String(), `data-webmcp-actions="ownership-transfer"`) ||
		!strings.Contains(all.Body.String(), `aria-describedby="remove-consequence-`) ||
		!strings.Contains(all.Body.String(), `data-column="role"><a`) ||
		strings.Contains(all.Body.String(), `data-column="role" aria-sort=""`) ||
		!strings.Contains(all.Body.String(), `This removes their workspace access and stops sharing workspace resources with them.`) {
		t.Fatalf("member actions or no-JavaScript confirmation missing: %s", all.Body.String())
	}
	roleSorted := request(env.routes, http.MethodGet, path+"?all=1&sort=role", nil, env.cookies, nil)
	if !strings.Contains(roleSorted.Body.String(), `data-column="role" aria-sort="ascending"`) ||
		strings.Contains(roleSorted.Body.String(), `data-column="email" aria-sort="`) {
		t.Fatalf("member sort state = %d, want aria-sort only on active column: %s", roleSorted.Code, roleSorted.Body.String())
	}
	page := request(env.routes, http.MethodGet, path+"?all=1&limit=1&sort=email", nil, env.cookies, nil)
	if page.Code != http.StatusOK ||
		!strings.Contains(page.Body.String(), `limit=1`) || !strings.Contains(page.Body.String(), `page=2`) {
		t.Fatalf("opt-in member page = %d, want a link to page 2 and query controls: %s", page.Code, page.Body.String())
	}
	principal, err := env.auth.Authorize(context.Background(), env.ownerID, env.workspaceID, identity.MembersRead)
	if err != nil {
		t.Fatal(err)
	}
	members, err := env.auth.ListMembers(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) < 2 {
		t.Fatalf("members fixture has %d members, want at least 2 for a second page", len(members))
	}
	listBody := func(response *httptest.ResponseRecorder) string {
		body := response.Body.String()
		if start := strings.Index(body, `<div id="list-results">`); start >= 0 {
			return body[start:]
		}
		return body
	}
	secondPage := request(env.routes, http.MethodGet, path+"?all=1&limit=1&sort=email&page=2", nil, env.cookies, nil)
	if secondPage.Code != http.StatusOK || !strings.Contains(listBody(secondPage), "viewer@example.com") ||
		strings.Contains(listBody(secondPage), "owner@example.com") {
		t.Fatalf("second members page = %d: %s", secondPage.Code, secondPage.Body.String())
	}
	filtered := request(env.routes, http.MethodGet, path+"?all=1&role=viewer", nil, env.cookies, nil)
	if filtered.Code != http.StatusOK || !strings.Contains(listBody(filtered), "viewer@example.com") ||
		strings.Contains(listBody(filtered), "owner@example.com") {
		t.Fatalf("role-filtered members = %d: %s", filtered.Code, filtered.Body.String())
	}
	preference := request(env.routes, http.MethodPost, path+"/preferences/columns/email", url.Values{
		"csrf": {env.csrf}, "visible": {"false"},
	}, env.cookies, nil)
	if preference.Code != http.StatusSeeOther || preference.Header().Get("Location") != path {
		t.Fatalf("hide email preference = %d: %s", preference.Code, preference.Body.String())
	}
	hidden := request(env.routes, http.MethodGet, path+"?all=1", nil, env.cookies, nil)
	if hidden.Code != http.StatusOK || strings.Contains(hidden.Body.String(), `data-column="email"`) {
		t.Fatalf("saved member column visibility = %d: %s", hidden.Code, hidden.Body.String())
	}
}
