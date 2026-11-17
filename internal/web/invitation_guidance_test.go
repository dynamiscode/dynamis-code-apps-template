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
		"/sign-in\" method=\"post\"",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("invitation page is missing account guidance %q (status %d)", want, page.Code)
		}
	}
	if strings.Contains(body, "/login?return_to=/invitations/") {
		t.Fatal("invitation page includes the invitation secret in the login return target")
	}
	if page.Header().Get("Cache-Control") != "private, no-store" || page.Header().Get("Referrer-Policy") != "no-referrer" ||
		page.Header().Get("X-Robots-Tag") != "noindex, nofollow, noarchive" {
		t.Fatalf("invitation page security headers = %+v", page.Header())
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

func TestInvitationSignInUsesBoundOpaqueReturnAndPreservesWrongAccountRecovery(t *testing.T) {
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
	csrf := responseCookie(page, "invitation_csrf").Value
	invalidCSRF := request(handler, http.MethodPost, path+"/sign-in", url.Values{"csrf": {"wrong"}}, []*http.Cookie{
		{Name: "invitation_csrf", Value: csrf},
	}, nil)
	if invalidCSRF.Code != http.StatusForbidden || responseCookie(invalidCSRF, "invitation_handoff").Value != "" {
		t.Fatalf("anonymous sign-in with invalid CSRF = %d, handoff issued=%v", invalidCSRF.Code, responseCookie(invalidCSRF, "invitation_handoff").Value != "")
	}
	started := request(handler, http.MethodPost, path+"/sign-in", url.Values{"csrf": {csrf}}, []*http.Cookie{{Name: "invitation_csrf", Value: csrf}}, nil)
	if started.Code != http.StatusSeeOther || started.Header().Get("Location") != invitationLoginLocation {
		t.Fatalf("handoff start = %d, expected fixed login return target; raw invitation secret exposed=%v", started.Code, strings.Contains(started.Header().Get("Location"), invitation.Secret))
	}
	if strings.Contains(started.Header().Get("Location"), invitation.Secret) {
		t.Fatal("handoff redirect exposes invitation secret")
	}
	handoffCookie := responseCookie(started, "invitation_handoff")
	bindingCookie := responseCookie(started, "invitation_handoff_binding")
	if handoffCookie.Value == "" || bindingCookie.Value == "" || handoffCookie.Value == invitation.Secret || bindingCookie.Value == invitation.Secret ||
		!handoffCookie.HttpOnly || !bindingCookie.HttpOnly || handoffCookie.SameSite != http.SameSiteLaxMode || bindingCookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("handoff cookies do not have opaque, browser-bound cookie properties")
	}
	loginCookies := []*http.Cookie{handoffCookie, bindingCookie}
	login := request(handler, http.MethodGet, invitationLoginLocation, nil, loginCookies, nil)
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), `name="return_to" value="/invitations/continue"`) ||
		strings.Contains(login.Body.String(), invitation.Secret) {
		t.Fatalf("login return target status/body = %d, secret exposed=%v", login.Code, strings.Contains(login.Body.String(), invitation.Secret))
	}

	wrongSession, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	boundCookies := append(loginCookies, &http.Cookie{Name: "session", Value: wrongSession.Secret}, &http.Cookie{Name: "csrf", Value: wrongSession.CSRFSecret})
	continued := request(handler, http.MethodGet, invitationContinuePath, nil, boundCookies, nil)
	if continued.Code != http.StatusOK || !strings.Contains(continued.Body.String(), "invitee@example.com") {
		t.Fatalf("continue page = %d, invited email missing", continued.Code)
	}
	wrongAccountPage := request(handler, http.MethodGet, path, nil, boundCookies, nil)
	if wrongAccountPage.Code != http.StatusOK || !strings.Contains(wrongAccountPage.Body.String(), `action="`+path+`/sign-in"`) {
		t.Fatalf("signed-in invitation page = %d, account recovery form missing", wrongAccountPage.Code)
	}
	invalidSessionCSRF := request(handler, http.MethodPost, path+"/sign-in", url.Values{"csrf": {"wrong"}}, boundCookies, nil)
	if invalidSessionCSRF.Code != http.StatusForbidden || responseCookie(invalidSessionCSRF, "invitation_handoff").Value != "" {
		t.Fatalf("signed-in recovery with invalid CSRF = %d, handoff issued=%v", invalidSessionCSRF.Code, responseCookie(invalidSessionCSRF, "invitation_handoff").Value != "")
	}
	recovery := request(handler, http.MethodPost, path+"/sign-in", url.Values{"csrf": {wrongSession.CSRFSecret}}, boundCookies, nil)
	if recovery.Code != http.StatusSeeOther || recovery.Header().Get("Location") != invitationLoginLocation ||
		responseCookie(recovery, "invitation_handoff").Value == "" {
		t.Fatalf("signed-in account recovery = %d, fixed target retained=%v, handoff issued=%v", recovery.Code, recovery.Header().Get("Location") == invitationLoginLocation, responseCookie(recovery, "invitation_handoff").Value != "")
	}
	wrongAccount := request(handler, http.MethodPost, invitationContinuePath, url.Values{"csrf": {wrongSession.CSRFSecret}}, boundCookies, nil)
	if wrongAccount.Code != http.StatusUnprocessableEntity || !strings.Contains(wrongAccount.Body.String(), "Only the invited email address can accept") {
		t.Fatalf("wrong account response = %d", wrongAccount.Code)
	}
	stillActive := request(handler, http.MethodGet, invitationContinuePath, nil, boundCookies, nil)
	if stillActive.Code != http.StatusOK {
		t.Fatalf("wrong account consumed handoff: status %d", stillActive.Code)
	}
	loggedOut := request(handler, http.MethodPost, "/logout", url.Values{"csrf": {wrongSession.CSRFSecret}}, boundCookies, nil)
	if loggedOut.Code != http.StatusSeeOther || loggedOut.Header().Get("Location") != invitationLoginLocation || strings.Contains(loggedOut.Header().Get("Location"), invitation.Secret) {
		t.Fatalf("wrong-account logout = %d, fixed return target correct=%v, raw invitation secret exposed=%v", loggedOut.Code, loggedOut.Header().Get("Location") == invitationLoginLocation, strings.Contains(loggedOut.Header().Get("Location"), invitation.Secret))
	}
	ordinarySession, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	ordinaryLogout := request(handler, http.MethodPost, "/logout", url.Values{"csrf": {ordinarySession.CSRFSecret}}, []*http.Cookie{
		{Name: "session", Value: ordinarySession.Secret}, {Name: "csrf", Value: ordinarySession.CSRFSecret},
	}, nil)
	if ordinaryLogout.Code != http.StatusSeeOther || ordinaryLogout.Header().Get("Location") != "/login" {
		t.Fatalf("ordinary logout = %d; default login redirect preserved=%v", ordinaryLogout.Code, ordinaryLogout.Header().Get("Location") == "/login")
	}
}

func TestInvitationContinueRejectsInvalidOpaqueState(t *testing.T) {
	handler, _, _, _, _ := testWeb(t, 10)
	response := request(handler, http.MethodGet, invitationContinuePath, nil, []*http.Cookie{
		{Name: "invitation_handoff", Value: "invalid-state"},
		{Name: "invitation_handoff_binding", Value: "invalid-binding"},
	}, nil)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "This invitation link is unavailable.") ||
		response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("invalid handoff response = %d headers=%+v", response.Code, response.Header())
	}
}

func TestInvitationReturnSurvivesMFAWithoutSecretReturnTarget(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWebWithMFA(t, 10, true)
	ctx := context.Background()
	principal, err := auth.Authorize(ctx, owner.UserID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(ctx, principal, "mfa-invitee@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := auth.BeginInvitationReturn(ctx, invitation.Secret)
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := auth.BeginTOTPEnrollment(ctx, owner.UserID, session.ID, "long-enough-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CompleteTOTPEnrollment(ctx, session.ID, enrollment.Challenge, webTOTPCode(enrollment.Secret, time.Now()), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{
		{Name: "invitation_handoff", Value: handoff.Token},
		{Name: "invitation_handoff_binding", Value: handoff.Binding},
	}
	loginPage := request(handler, http.MethodGet, invitationLoginLocation, nil, cookies, nil)
	loginCSRF := responseCookie(loginPage, "login_csrf").Value
	cookies = append(cookies, &http.Cookie{Name: "login_csrf", Value: loginCSRF})
	login := request(handler, http.MethodPost, "/login", url.Values{
		"csrf": {loginCSRF}, "email": {"owner@example.com"}, "password": {"long-enough-password"},
		"return_to": {invitationContinuePath},
	}, cookies, nil)
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/mfa" ||
		responseCookie(login, "mfa_return_to").Value != invitationContinuePath || strings.Contains(login.Header().Get("Location"), invitation.Secret) {
		t.Fatalf("invitation MFA return = %d, fixed target retained=%v, raw invitation secret exposed=%v", login.Code, responseCookie(login, "mfa_return_to").Value == invitationContinuePath, strings.Contains(login.Header().Get("Location"), invitation.Secret))
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
