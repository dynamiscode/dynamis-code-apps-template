package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestMemberRoleSelectShowsCurrentRole(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	ctx := context.Background()
	invitations, err := auth.Authorize(ctx, owner.UserID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(ctx, invitations, "member@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "member-long-password", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/settings/members"
	page := request(handler, http.MethodGet, path, nil, cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK ||
		!strings.Contains(body, `<option value="member" selected>Member</option>`) ||
		strings.Contains(body, `<option value="admin" selected>Admin</option>`) {
		t.Fatalf("member role selection = %d, want current member role selected: %s", page.Code, body)
	}
}
