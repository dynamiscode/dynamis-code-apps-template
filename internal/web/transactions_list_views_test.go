package web

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/listviews"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/transactions"
)

func TestTransactionsViewRoutesDenyViewer(t *testing.T) {
	ctx := context.Background()
	h, auth, workspace, owner, db := testWebWithMFARawDB(t, 2, false)
	h.EnableTransactions(transactions.NewService(db, auth))
	h.EnableListPreferences(listprefs.NewService(db, config.SQLite, auth))
	views := listviews.NewService(db, config.SQLite, auth)
	h.EnableListViews(views)
	principal, err := auth.Authorize(ctx, owner.UserID, workspace, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	view, err := views.Create(ctx, principal, workspace, listviews.ListTransactions, "Shared", true, listviews.State{})
	if err != nil {
		t.Fatal(err)
	}
	inviter, err := auth.Authorize(ctx, owner.UserID, workspace, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invite, err := auth.CreateInvitation(ctx, inviter, "viewer@example.com", identity.Viewer, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := auth.CreateInvitedLocalUser(ctx, invite.Secret, "long-enough-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, user, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	base := "/workspaces/" + workspace.String() + "/lists/transactions/views"
	id := strconv.FormatInt(view.ID, 10)
	for _, suffix := range []string{"", "/" + id + "/apply", "/" + id + "/update", "/" + id + "/rename", "/" + id + "/delete", "/" + id + "/default", "/clear/default"} {
		response := request(h.Routes(), http.MethodPost, base+suffix, url.Values{"csrf": {session.CSRFSecret}, "name": {"Changed"}}, cookies, nil)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s = %d, want 403", suffix, response.Code)
		}
	}
}
