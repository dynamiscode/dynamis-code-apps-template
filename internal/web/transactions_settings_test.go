package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/transactions"
)

func TestTransactionsSettingsPagesAndPermissions(t *testing.T) {
	ctx := context.Background()
	h, auth, workspace, owner, db := testWebWithMFARawDB(t, 2, false)
	h.EnableTransactions(transactions.NewService(db, auth))
	actor, err := auth.Authorize(ctx, owner.UserID, workspace, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	base := "/workspaces/" + workspace.String()
	for _, role := range []identity.Role{identity.Owner, identity.Admin, identity.Member, identity.Viewer} {
		user := owner.UserID
		if role != identity.Owner {
			invite, err := auth.CreateInvitation(ctx, actor, string(role)+"@example.com", role, time.Hour, identity.AuditContext{})
			if err != nil {
				t.Fatal(err)
			}
			user, _, err = auth.CreateInvitedLocalUser(ctx, invite.Secret, "long-enough-password", identity.AuditContext{})
			if err != nil {
				t.Fatal(err)
			}
		}
		session, err := auth.CreateSession(ctx, user, "local", "", time.Hour, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
		for _, suffix := range []string{"", "/methods", "/categories"} {
			page := request(h.Routes(), http.MethodGet, base+"/settings/transactions"+suffix, nil, cookies, nil)
			want := http.StatusForbidden
			if role == identity.Owner || role == identity.Admin {
				want = http.StatusOK
			}
			if page.Code != want {
				t.Fatalf("%s %s: %d want %d", role, suffix, page.Code, want)
			}
			if want == http.StatusOK {
				for _, text := range []string{base + "/transactions", base + "/settings/transactions/methods", base + "/settings/transactions/categories"} {
					if !strings.Contains(page.Body.String(), text) {
						t.Errorf("%s %s missing %s", role, suffix, text)
					}
				}
			}
		}
	}
}

func TestTransactionsSettingsDraftsAndRedirects(t *testing.T) {
	ctx := context.Background()
	h, auth, workspace, owner, db := testWebWithMFARawDB(t, 2, false)
	h.EnableTransactions(transactions.NewService(db, auth))
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	base := "/workspaces/" + workspace.String()
	form := url.Values{"csrf": {session.CSRFSecret}, "name": {"Travel"}}
	created := request(h.Routes(), http.MethodPost, base+"/transactions/categories", form, cookies, nil)
	if created.Code != http.StatusSeeOther || !strings.Contains(created.Header().Get("Location"), "/settings/transactions/categories?category_saved=1") {
		t.Fatalf("create category: %d %s", created.Code, created.Header().Get("Location"))
	}
	duplicate := request(h.Routes(), http.MethodPost, base+"/transactions/categories", form, cookies, nil)
	if duplicate.Code != http.StatusUnprocessableEntity || !strings.Contains(duplicate.Body.String(), `value="Travel"`) || !strings.Contains(duplicate.Body.String(), `aria-describedby="transactions-category-error"`) {
		t.Fatalf("duplicate: %d %s", duplicate.Code, duplicate.Body)
	}
	method := request(h.Routes(), http.MethodPost, base+"/transactions/methods", url.Values{"csrf": {session.CSRFSecret}, "name": {" "}, "enabled": {"true"}}, cookies, nil)
	if method.Code != http.StatusUnprocessableEntity || !strings.Contains(method.Body.String(), `aria-describedby="transactions-method-error"`) {
		t.Fatalf("invalid method: %d %s", method.Code, method.Body)
	}
	created = request(h.Routes(), http.MethodPost, base+"/transactions/methods", url.Values{"csrf": {session.CSRFSecret}, "name": {"Wire"}, "enabled": {"true"}}, cookies, nil)
	if created.Code != http.StatusSeeOther || !strings.Contains(created.Header().Get("Location"), "/settings/transactions/methods?method_saved=1") {
		t.Fatalf("create method: %d %s", created.Code, created.Header().Get("Location"))
	}
}
