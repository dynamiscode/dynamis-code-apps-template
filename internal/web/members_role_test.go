package web

import (
	"context"
	"net/http"
	"net/url"
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
		!strings.Contains(body, "Admins manage members, invitations, and workspace settings. Members can edit resources. Viewers can read resources.") ||
		!strings.Contains(body, "Transfer makes the selected member the new owner. Your role becomes admin.") ||
		!strings.Contains(body, `<option value="member" selected>Member</option>`) ||
		strings.Contains(body, `<option value="admin" selected>Admin</option>`) {
		t.Fatalf("member guidance and role selection = %d: %s", page.Code, body)
	}
	spanishCookies := append(cookies, &http.Cookie{Name: "locale", Value: "es"})
	spanish := request(handler, http.MethodGet, path, nil, spanishCookies, nil)
	if !strings.Contains(spanish.Body.String(), "Los administradores gestionan miembros, invitaciones") ||
		!strings.Contains(spanish.Body.String(), "Al transferir, el miembro seleccionado será el nuevo propietario.") {
		t.Fatal("member guidance is not translated to Spanish")
	}
}

func TestMemberRoleFormUpdatesIntendedMember(t *testing.T) {
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
	memberID, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "member-long-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := auth.Authorize(ctx, owner.UserID, workspaceID, identity.MembersRead)
	if err != nil {
		t.Fatal(err)
	}
	members, err := auth.ListMembers(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	var target identity.MemberSummary
	for _, candidate := range members {
		if candidate.UserID == memberID {
			target = candidate
			break
		}
	}
	if target.ID == 0 {
		t.Fatal("invited member was not listed")
	}
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/settings/members"
	page := request(handler, http.MethodGet, path, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `name="member_id" value="`+target.ID.String()+`"`) {
		t.Fatalf("members form = %d, want member identifier: %s", page.Code, page.Body.String())
	}
	response := request(handler, http.MethodPost, path, url.Values{
		"csrf":      {session.CSRFSecret},
		"member_id": {target.ID.String()},
		"role":      {string(identity.Viewer)},
		"action":    {"role"},
	}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("member role update = %d, want redirect: %s", response.Code, response.Body.String())
	}

	principal, err = auth.Authorize(ctx, owner.UserID, workspaceID, identity.MembersRead)
	if err != nil {
		t.Fatal(err)
	}
	members, err = auth.ListMembers(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range members {
		if got.UserID == memberID {
			if got.Role != identity.Viewer {
				t.Fatalf("updated member role = %q, want %q", got.Role, identity.Viewer)
			}
			return
		}
	}
	t.Fatal("updated member is missing")
}
