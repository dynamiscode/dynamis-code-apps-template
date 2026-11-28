package web

import (
	"context"
	"example.com/dynamis-code/apps-template/internal/identity"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestMemberRoleSelectShowsCurrentRole(t *testing.T) {
	handler, auth, workspaceID, owner := testWeb(t, 10)
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
	page := request(handler, http.MethodGet, path+"?all=1", nil, cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK ||
		!strings.Contains(body, "2 members") ||
		!strings.Contains(body, "Filter") ||
		!strings.Contains(body, "Admins manage members, invitations, and workspace settings. Members can edit resources. Viewers can read resources.") ||
		!strings.Contains(body, "Transfer makes the selected member the new owner. Your role becomes admin.") {
		t.Fatalf("member guidance and role selection = %d: %s", page.Code, body)
	}
	spanishCookies := append(cookies, &http.Cookie{Name: "locale", Value: "es"})
	spanish := request(handler, http.MethodGet, path+"?all=1", nil, spanishCookies, nil)
	if !strings.Contains(spanish.Body.String(), "2 miembros") ||
		!strings.Contains(spanish.Body.String(), "Filtrar") ||
		!strings.Contains(spanish.Body.String(), "Los administradores gestionan miembros, invitaciones") ||
		!strings.Contains(spanish.Body.String(), "Al transferir, el miembro seleccionado será el nuevo propietario.") {
		t.Fatal("member guidance is not translated to Spanish")
	}
}

func TestMemberRoleFormUpdatesIntendedMember(t *testing.T) {
	handler, auth, workspaceID, owner := testWeb(t, 10)
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
	page := request(handler, http.MethodGet, path+"?all=1", nil, cookies, nil)
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

func TestMemberRemovalAndTransferRequireServerConfirmation(t *testing.T) {
	env := newMembersListTestEnv(t)
	path := "/workspaces/" + env.workspaceID.String() + "/settings/members"
	principal, err := env.auth.Authorize(context.Background(), env.ownerID, env.workspaceID, identity.MembersRead)
	if err != nil {
		t.Fatal(err)
	}
	members, err := env.auth.ListMembers(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	var target identity.MemberSummary
	for _, member := range members {
		if member.Role == identity.Viewer {
			target = member
			break
		}
	}
	if target.ID == 0 {
		t.Fatal("viewer member missing")
	}
	for _, action := range []string{"remove", "transfer"} {
		response := request(env.routes, http.MethodPost, path, url.Values{
			"csrf": {env.csrf}, "member_id": {target.ID.String()}, "action": {action},
		}, env.cookies, nil)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("unconfirmed %s = %d, want 422: %s", action, response.Code, response.Body.String())
		}
	}
	members, err = env.auth.ListMembers(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("members after unconfirmed actions = %d, want 2", len(members))
	}
	for _, member := range members {
		if member.ID == target.ID && member.Role != identity.Viewer {
			t.Fatalf("unconfirmed actions changed target role to %q", member.Role)
		}
		if member.UserID == env.ownerID && member.Role != identity.Owner {
			t.Fatalf("unconfirmed actions changed owner role to %q", member.Role)
		}
	}
}

func TestMemberRemovalRequiresCSRFAndConfirmedOrdinaryForm(t *testing.T) {
	env := newMembersListTestEnv(t)
	ctx := context.Background()
	path := "/workspaces/" + env.workspaceID.String() + "/settings/members"
	principal, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.MembersRead)
	if err != nil {
		t.Fatal(err)
	}
	members, err := env.auth.ListMembers(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	var target identity.MemberSummary
	for _, member := range members {
		if member.Role == identity.Viewer {
			target = member
			break
		}
	}
	if target.ID == 0 {
		t.Fatal("viewer member missing")
	}
	form := url.Values{"member_id": {target.ID.String()}, "action": {"remove"}, "confirm_remove": {"yes"}}
	withoutCSRF := request(env.routes, http.MethodPost, path, form, env.cookies, nil)
	if withoutCSRF.Code != http.StatusForbidden {
		t.Fatalf("removal without CSRF = %d, want 403: %s", withoutCSRF.Code, withoutCSRF.Body.String())
	}
	form.Set("csrf", env.csrf)
	confirmed := request(env.routes, http.MethodPost, path, form, env.cookies, nil)
	if confirmed.Code != http.StatusSeeOther {
		t.Fatalf("confirmed removal = %d, want redirect: %s", confirmed.Code, confirmed.Body.String())
	}
	members, err = env.auth.ListMembers(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range members {
		if member.ID == target.ID {
			t.Fatal("confirmed removal left member in workspace")
		}
	}
}

func TestMemberOwnershipTransferRequiresOwnerPermissionAndConfirmation(t *testing.T) {
	env := newMembersListTestEnv(t)
	ctx := context.Background()
	path := "/workspaces/" + env.workspaceID.String() + "/settings/members"
	readPrincipal, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.MembersRead)
	if err != nil {
		t.Fatal(err)
	}
	members, err := env.auth.ListMembers(ctx, readPrincipal)
	if err != nil {
		t.Fatal(err)
	}
	var target identity.MemberSummary
	for _, member := range members {
		if member.Role == identity.Viewer {
			target = member
			break
		}
	}
	adminPrincipal, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.MembersManage)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.auth.ChangeMemberRole(ctx, adminPrincipal, target.ID, identity.Admin, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	adminSession, err := env.auth.CreateSession(ctx, target.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	adminCookies := []*http.Cookie{{Name: "session", Value: adminSession.Secret}, {Name: "csrf", Value: adminSession.CSRFSecret}}
	response := request(env.routes, http.MethodPost, path, url.Values{
		"csrf": {adminSession.CSRFSecret}, "member_id": {target.ID.String()}, "action": {"transfer"}, "confirm_transfer": {"yes"},
	}, adminCookies, nil)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("admin transfer = %d, want denied action: %s", response.Code, response.Body.String())
	}
	principal, err := env.auth.Authorize(ctx, target.UserID, env.workspaceID, identity.MembersRead)
	if err != nil || principal.Role != identity.Admin {
		t.Fatalf("admin role after denied transfer = %q, err=%v", principal.Role, err)
	}
	ownerSession, err := env.auth.CreateSession(ctx, env.ownerID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	ownerCookies := []*http.Cookie{{Name: "session", Value: ownerSession.Secret}, {Name: "csrf", Value: ownerSession.CSRFSecret}}
	response = request(env.routes, http.MethodPost, path, url.Values{
		"csrf": {ownerSession.CSRFSecret}, "member_id": {target.ID.String()}, "action": {"transfer"}, "confirm_transfer": {"yes"},
	}, ownerCookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("confirmed owner transfer = %d, want redirect: %s", response.Code, response.Body.String())
	}
	principal, err = env.auth.Authorize(ctx, target.UserID, env.workspaceID, identity.OwnershipTransfer)
	if err != nil || principal.Role != identity.Owner {
		t.Fatalf("transferred member role = %q, err=%v; want owner", principal.Role, err)
	}
}
