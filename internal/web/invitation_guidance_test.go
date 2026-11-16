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

func TestInvitationPageExplainsAccountAndRoleRequirements(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	principal, err := auth.Authorize(context.Background(), owner.UserID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(context.Background(), principal, "invitee@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}

	path := "/invitations/" + invitation.Secret
	page := request(handler, http.MethodGet, path, nil, nil, nil)
	body := page.Body.String()
	for _, want := range []string{
		"Only the invited email address can accept this invitation. If you are signed in with another account, sign out and reopen this link.",
		"If you already have an account for the invited email, sign in instead of creating another account.",
		"Sign in, then accept",
		"/login?return_to=",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("invitation page is missing account guidance %q (status %d)", want, page.Code)
		}
	}
	spanish := request(handler, http.MethodGet, path, nil, []*http.Cookie{{Name: "locale", Value: "es"}}, nil)
	if !strings.Contains(spanish.Body.String(), "Solo la dirección de correo invitada puede aceptar esta invitación.") ||
		!strings.Contains(spanish.Body.String(), "Si ya tienes una cuenta con el correo invitado") {
		t.Fatal("invitation guidance is not translated to Spanish")
	}

	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	adminPage := request(handler, http.MethodGet, "/workspaces/"+workspaceID.String()+"/settings/invitations", nil, cookies, nil)
	if adminPage.Code != http.StatusOK || !strings.Contains(adminPage.Body.String(), "Admins manage members, invitations, and workspace settings.") {
		t.Fatalf("invitation management page = %d; role permissions missing", adminPage.Code)
	}
}

func TestInvitationFailureExplainsWrongAccountAndRecovery(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	principal, err := auth.Authorize(context.Background(), owner.UserID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(context.Background(), principal, "invitee@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/invitations/" + invitation.Secret

	page := request(handler, http.MethodGet, path, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Only the invited email address can accept this invitation") ||
		!strings.Contains(page.Body.String(), `action="/logout"`) {
		t.Fatalf("authenticated invitation page = %d; account recovery guidance missing", page.Code)
	}

	response := request(handler, http.MethodPost, path, url.Values{"action": {"accept"}, "csrf": {session.CSRFSecret}}, cookies, nil)
	if response.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(response.Body.String(), "Only the invited email address can accept this invitation") ||
		!strings.Contains(response.Body.String(), "Sign in, then accept") {
		t.Fatalf("wrong-account acceptance response = %d; recovery guidance missing", response.Code)
	}
	const safeFailure = "We could not accept this invitation. Check that you are signed in with the invited email. If the link is no longer active, ask the inviter to send a new one."
	if !strings.Contains(response.Body.String(), safeFailure) {
		t.Fatal("wrong-account response omitted the generic invitation failure")
	}
	expired := request(handler, http.MethodPost, "/invitations/unavailable-token", url.Values{
		"action": {"accept"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if expired.Code != http.StatusUnprocessableEntity || !strings.Contains(expired.Body.String(), safeFailure) {
		t.Fatal("unavailable-token acceptance did not keep the generic invitation failure")
	}
}

func TestUnavailableInvitationExplainsHowToRecover(t *testing.T) {
	handler, _, _, _, _ := testWeb(t, 10)
	response := request(handler, http.MethodGet, "/invitations/unavailable-token", nil, nil, nil)
	if response.Code != http.StatusNotFound ||
		!strings.Contains(response.Body.String(), "This invitation link is unavailable.") ||
		!strings.Contains(response.Body.String(), "Ask the person who invited you to send a new link.") {
		t.Fatalf("unavailable invitation response = %d; recovery guidance missing", response.Code)
	}
}
