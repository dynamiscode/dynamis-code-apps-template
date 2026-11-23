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

func TestCatalogWebRoleMatrix(t *testing.T) {
	routes, auth, workspace, owner := testWeb(t, 2)
	ctx := context.Background()
	actor, err := auth.Authorize(ctx, owner.UserID, workspace, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	path := "/workspaces/" + workspace.String() + "/catalog"
	ownerSession, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	ownerCookies := []*http.Cookie{{Name: "session", Value: ownerSession.Secret}, {Name: "csrf", Value: ownerSession.CSRFSecret}}
	for _, role := range []identity.Role{identity.Owner, identity.Admin, identity.Member, identity.Viewer} {
		t.Run(string(role), func(t *testing.T) {
			user := owner.UserID
			if role != identity.Owner {
				inv, err := auth.CreateInvitation(ctx, actor, string(role)+"@example.com", role, time.Hour, identity.AuditContext{})
				if err != nil {
					t.Fatal(err)
				}
				user, _, err = auth.CreateInvitedLocalUser(ctx, inv.Secret, "long-enough-password", identity.AuditContext{})
				if err != nil {
					t.Fatal(err)
				}
			}
			session, err := auth.CreateSession(ctx, user, "local", "", time.Hour, identity.AuditContext{})
			if err != nil {
				t.Fatal(err)
			}
			cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
			if r := request(routes, http.MethodPost, path, url.Values{"csrf": {ownerSession.CSRFSecret}, "name": {"Seed"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Initial"}}, ownerCookies, nil); r.Code != http.StatusSeeOther {
				t.Fatal(r.Code)
			}
			ownerPage := request(routes, http.MethodGet, path, nil, ownerCookies, nil)
			match := regexp.MustCompile(`action="([^"]+/edit)"`).FindStringSubmatch(ownerPage.Body.String())
			if len(match) != 2 {
				t.Fatal("missing edit form")
			}
			entry := strings.TrimSuffix(match[1], "/edit")
			r := request(routes, http.MethodPost, entry+"/prices", url.Values{"csrf": {ownerSession.CSRFSecret}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Visit"}}, ownerCookies, nil)
			if r.Code != http.StatusSeeOther {
				t.Fatal(r.Code)
			}
			ownerPage = request(routes, http.MethodGet, path, nil, ownerCookies, nil)
			price := regexp.MustCompile(`action="([^"]+/prices/[^"]+/edit)"`).FindStringSubmatch(ownerPage.Body.String())
			if len(price) != 2 {
				t.Fatal("missing price form")
			}
			page := request(routes, http.MethodGet, path, nil, cookies, nil)
			writable := role == identity.Owner || role == identity.Admin
			if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Seed") {
				t.Fatalf("read: %d", page.Code)
			}
			if strings.Contains(page.Body.String(), `name="name"`) != writable || strings.Contains(page.Body.String(), "/archive") != writable || strings.Contains(page.Body.String(), "/prices/") != writable {
				t.Fatal("incorrect write controls")
			}
			want := http.StatusForbidden
			if writable {
				want = http.StatusSeeOther
			}
			form := url.Values{"csrf": {session.CSRFSecret}, "name": {"Edited"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Visit"}}
			for _, target := range []string{path, entry + "/edit", entry + "/prices", price[1], strings.TrimSuffix(price[1], "/edit") + "/delete", entry + "/archive", entry + "/unarchive", entry + "/delete"} {
				result := request(routes, http.MethodPost, target, form, cookies, nil)
				if result.Code != want {
					t.Errorf("%s: %d want %d", target, result.Code, want)
				}
			}
		})
	}
}
