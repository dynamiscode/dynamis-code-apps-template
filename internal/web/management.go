package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

type exporter interface {
	Export(context.Context, identity.Principal, string, identity.AuditContext) ([]byte, error)
}

func (h *Handler) createWorkspace(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	_, err := h.identity.CreateWorkspace(request.Context(), identity.Principal{
		UserID: session.UserID, AuthMethod: session.AuthMethod, AuthLevel: session.AuthLevel,
	}, identity.WorkspaceCreateInput{
		Name: request.FormValue("name"), Locale: request.FormValue("locale"),
	}, auditContext(request))
	if err != nil {
		if errors.Is(err, identity.ErrMFARequired) {
			h.beginMFALogin(writer, request, session, "/")
			return
		}
		workspaces, listErr := h.identity.ListWorkspaces(request.Context(), session.UserID)
		if listErr != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		h.render(writer, http.StatusUnprocessableEntity, "home.html", pageData{
			Title: "Workspaces", NavPage: "home", CSRF: csrf, Workspaces: workspaces,
			Error: "Enter a workspace name between 1 and 120 characters.",
		})
		return
	}
	h.redirect(writer, request, "/")
}

func (h *Handler) managementPrincipal(
	writer http.ResponseWriter,
	request *http.Request,
	workspaceID string,
	permission identity.Permission,
) (identity.Principal, identity.Session, string, bool) {
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return identity.Principal{}, identity.Session{}, "", false
	}
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, permission)
	if !ok {
		return identity.Principal{}, identity.Session{}, "", false
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return identity.Principal{}, identity.Session{}, "", false
	}
	return principal, session, csrf, true
}

func (h *Handler) settingsPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if _, _, _, ok := h.workspaceSession(writer, request, workspaceID, identity.MembersRead); !ok {
		return
	}
	h.redirect(writer, request, "/workspaces/"+workspaceID+"/settings/members")
}

func (h *Handler) membersPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.MembersRead)
	if !ok {
		return
	}
	members, err := h.identity.ListMembers(request.Context(), principal)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusOK, "members.html", pageData{
		Title: "Members", NavPage: "members", NavSection: "settings", CSRF: csrf, Workspace: workspaceByID(workspaces, workspaceID),
		Workspaces: workspaces, Members: members,
		CanManage:   principal.Permissions[identity.MembersManage],
		CanTransfer: principal.Permissions[identity.OwnershipTransfer],
	})
}

func (h *Handler) memberMutation(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, _, _, ok := h.managementPrincipal(writer, request, workspaceID, identity.MembersManage)
	if !ok {
		return
	}
	userID := request.FormValue("user_id")
	var err error
	switch request.FormValue("action") {
	case "role":
		role := identity.Role(request.FormValue("role"))
		if role != identity.Admin && role != identity.Member && role != identity.Viewer {
			h.managementError(writer, request, workspaceID, "The member role is invalid.")
			return
		}
		err = h.identity.ChangeMemberRole(request.Context(), principal, userID, role, auditContext(request))
	case "remove":
		err = h.identity.RemoveMember(request.Context(), principal, userID, auditContext(request))
	case "transfer":
		err = h.identity.TransferOwnership(request.Context(), principal, userID, auditContext(request))
	default:
		h.managementError(writer, request, workspaceID, "The requested member action is invalid.")
		return
	}
	if err != nil {
		if errors.Is(err, identity.ErrLastOwner) {
			h.managementError(writer, request, workspaceID, "The final owner cannot be changed.")
			return
		}
		h.managementError(writer, request, workspaceID, "The member action could not be completed.")
		return
	}
	h.redirect(writer, request, "/workspaces/"+workspaceID+"/settings/members")
}

func (h *Handler) invitationsPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.InvitationsManage)
	if !ok {
		return
	}
	invitations, err := h.identity.ListInvitations(request.Context(), principal)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusOK, "invitations.html", pageData{
		Title: "Invitations", NavPage: "invitations", NavSection: "settings", CSRF: csrf, Workspace: workspaceByID(workspaces, workspaceID),
		Workspaces: workspaces, Invitations: invitations,
	})
}

func (h *Handler) invitationMutation(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, _, csrf, ok := h.managementPrincipal(writer, request, workspaceID, identity.InvitationsManage)
	if !ok {
		return
	}
	if request.FormValue("action") == "create" {
		lifetime, valid := invitationFormLifetime(request.FormValue("lifetime"))
		if !valid {
			h.managementError(writer, request, workspaceID, "The invitation lifetime is invalid.")
			return
		}
		invitation, err := h.identity.CreateInvitation(request.Context(), principal, request.FormValue("email"), identity.Role(request.FormValue("role")), lifetime, auditContext(request))
		if err != nil {
			h.managementError(writer, request, workspaceID, "The invitation could not be created.")
			return
		}
		link := invitationURL(h.publicURL, invitation.Secret)
		workspaceLocale, _ := h.identity.GetWorkspaceLocale(request.Context(), workspaceID)
		_, warning := h.deliverInvitation(request, invitation.Email, link, workspaceLocale)
		h.renderInvitationResult(writer, request, workspaceID, csrf, link, warning)
		return
	}
	invitationID := request.FormValue("invitation_id")
	switch request.FormValue("action") {
	case "resend":
		secret, err := h.identity.ResendInvitation(request.Context(), principal, invitationID, 0, auditContext(request))
		if err != nil {
			h.managementError(writer, request, workspaceID, "The invitation could not be resent.")
			return
		}
		invitations, err := h.identity.ListInvitations(request.Context(), principal)
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		for _, invitation := range invitations {
			if invitation.ID == invitationID {
				link := invitationURL(h.publicURL, secret)
				workspaceLocale, _ := h.identity.GetWorkspaceLocale(request.Context(), workspaceID)
				_, warning := h.deliverInvitation(request, invitation.Email, link, workspaceLocale)
				h.renderInvitationResult(writer, request, workspaceID, csrf, link, warning)
				return
			}
		}
		h.managementError(writer, request, workspaceID, "The invitation could not be found.")
	case "revoke":
		if err := h.identity.RevokeInvitation(request.Context(), principal, invitationID, auditContext(request)); err != nil {
			h.managementError(writer, request, workspaceID, "The invitation could not be revoked.")
			return
		}
		h.redirect(writer, request, "/workspaces/"+workspaceID+"/settings/invitations")
	default:
		h.managementError(writer, request, workspaceID, "The requested invitation action is invalid.")
	}
}

func (h *Handler) renderInvitationResult(writer http.ResponseWriter, request *http.Request, workspaceID, csrf, link, warning string) {
	principal, session, _, ok := h.workspaceSession(writer, request, workspaceID, identity.InvitationsManage)
	if !ok {
		return
	}
	invitations, err := h.identity.ListInvitations(request.Context(), principal)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusOK, "invitations.html", pageData{
		Title: "Invitations", NavPage: "invitations", NavSection: "settings", CSRF: csrf, Workspace: workspaceByID(workspaces, workspaceID),
		Workspaces: workspaces, Invitations: invitations, InvitationURL: link,
		DeliveryWarning: warning,
	})
}

func (h *Handler) tokensPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceRead)
	if !ok {
		return
	}
	tokens, err := h.identity.ListAPITokens(request.Context(), principal)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusOK, "tokens.html", pageData{
		Title: "API tokens", NavPage: "tokens", NavSection: "settings", CSRF: csrf, Workspace: workspaceByID(workspaces, workspaceID),
		Workspaces: workspaces, Tokens: tokens,
	})
}

func (h *Handler) tokenMutation(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, _, _, ok := h.managementPrincipal(writer, request, workspaceID, identity.WorkspaceRead)
	if !ok {
		return
	}
	if request.FormValue("action") == "create" {
		var expiresAt *time.Time
		if raw := strings.TrimSpace(request.FormValue("expires_at")); raw != "" {
			value, err := time.Parse("2006-01-02", raw)
			if err != nil {
				h.managementError(writer, request, workspaceID, "The token expiration is invalid.")
				return
			}
			expiresAt = &value
		}
		scopes := make([]identity.Permission, 0, len(request.Form["scope"]))
		for _, scope := range request.Form["scope"] {
			scopes = append(scopes, identity.Permission(scope))
		}
		token, err := h.identity.CreateAPIToken(request.Context(), principal, request.FormValue("name"), scopes, expiresAt, auditContext(request))
		if err != nil {
			h.managementError(writer, request, workspaceID, "The token could not be created.")
			return
		}
		workspaces, _ := h.identity.ListWorkspaces(request.Context(), principal.UserID)
		tokens, _ := h.identity.ListAPITokens(request.Context(), principal)
		h.render(writer, http.StatusOK, "tokens.html", pageData{
			Title: "API tokens", NavPage: "tokens", NavSection: "settings", CSRF: request.FormValue("csrf"), Workspace: workspaceByID(workspaces, workspaceID),
			Workspaces: workspaces, Tokens: tokens, TokenSecret: token.Secret,
		})
		return
	}
	tokenID := request.PathValue("tokenId")
	if request.FormValue("action") == "revoke" {
		if err := h.identity.RevokeAPIToken(request.Context(), principal, tokenID, auditContext(request)); err != nil {
			h.managementError(writer, request, workspaceID, "The token could not be revoked.")
			return
		}
		h.redirect(writer, request, "/workspaces/"+workspaceID+"/settings/tokens")
		return
	}
	h.managementError(writer, request, workspaceID, "The requested token action is invalid.")
}

func (h *Handler) provisioningPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.SCIMManage)
	if !ok {
		return
	}
	h.renderProvisioningPage(writer, request, principal, session, csrf, "")
}

func (h *Handler) provisioningMutation(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, session, csrf, ok := h.managementPrincipal(writer, request, workspaceID, identity.SCIMManage)
	if !ok {
		return
	}
	switch request.FormValue("action") {
	case "create":
		token, err := h.identity.CreateSCIMToken(request.Context(), principal, auditContext(request))
		if err != nil {
			h.managementError(writer, request, workspaceID, "The SCIM credential could not be created.")
			return
		}
		h.renderProvisioningPage(writer, request, principal, session, csrf, token.Secret)
	case "revoke":
		if err := h.identity.RevokeSCIMToken(request.Context(), principal, auditContext(request)); err != nil {
			h.managementError(writer, request, workspaceID, "The SCIM credential could not be revoked.")
			return
		}
		h.redirect(writer, request, "/workspaces/"+workspaceID+"/settings/provisioning")
	default:
		h.managementError(writer, request, workspaceID, "The requested SCIM action is invalid.")
	}
}

func (h *Handler) renderProvisioningPage(
	writer http.ResponseWriter,
	request *http.Request,
	principal identity.Principal,
	session identity.Session,
	csrf string,
	secret string,
) {
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	workspaceID := principal.WorkspaceID
	h.render(writer, http.StatusOK, "provisioning.html", pageData{
		Title: "SCIM provisioning", NavPage: "provisioning", NavSection: "settings", CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		SCIMEndpoint: scimEndpoint(h.publicURL, h.cfg.Secure, request, workspaceID), SCIMTokenSecret: secret,
	})
}

func scimEndpoint(publicURL string, secure bool, request *http.Request, workspaceID string) string {
	base := strings.TrimRight(strings.TrimSpace(publicURL), "/")
	if base == "" {
		scheme := request.URL.Scheme
		if scheme == "" {
			scheme = "http"
			if secure {
				scheme = "https"
			}
		}
		base = scheme + "://" + request.Host
	}
	return base + "/scim/v2/" + url.PathEscape(workspaceID)
}

func (h *Handler) sessionsPage(writer http.ResponseWriter, request *http.Request) {
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return
	}
	sessions, err := h.identity.ListSessions(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusOK, "sessions.html", pageData{Title: "Sessions", NavPage: "sessions", CSRF: csrf, Sessions: sessions})
}

func (h *Handler) sessionMutation(writer http.ResponseWriter, request *http.Request) {
	session, ok := h.validCSRFSession(writer, request)
	if !ok {
		return
	}
	if _, err := h.identity.RevokeSession(request.Context(), session.UserID, request.PathValue("sessionId"), auditContext(request)); err != nil {
		h.renderError(writer, http.StatusConflict)
		return
	}
	h.redirect(writer, request, "/sessions")
}

func (h *Handler) exportPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	_, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceExport)
	if !ok {
		return
	}
	if h.exporter == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusOK, "export.html", pageData{
		Title: "Export", NavPage: "export", NavSection: "settings", CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
	})
}

func (h *Handler) exportWorkspace(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, _, _, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceExport)
	if !ok {
		return
	}
	if h.exporter == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	encoded, err := h.exporter.Export(request.Context(), principal, workspaceID, auditContext(request))
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Disposition", `attachment; filename="workspace-export.json"`)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(encoded)
}

func (h *Handler) securityPage(writer http.ResponseWriter, request *http.Request) {
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return
	}
	status, _ := h.identity.MFAStatus(request.Context(), session.UserID)
	passkeys, _ := h.identity.ListPasskeys(request.Context(), session.UserID)
	h.render(writer, http.StatusOK, "security.html", pageData{
		Title: "Security", NavPage: "security", CSRF: csrf, OIDCProviders: h.providers(),
		Error: request.URL.Query().Get("error"), Email: session.UserID, MFAStatus: status, Passkeys: passkeys,
	})
}

func (h *Handler) securityMutation(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	session, _, ok := h.session(writer, request)
	if !ok {
		return
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	if h.oidc == nil || len(h.providers()) == 0 || request.FormValue("provider") == "" {
		h.render(writer, http.StatusUnprocessableEntity, "security.html", pageData{Title: "Security", NavPage: "security", CSRF: request.FormValue("csrf"), Error: "No OIDC provider is configured."})
		return
	}
	if err := h.identity.ReauthenticateLocal(request.Context(), session.UserID, request.FormValue("password")); err != nil {
		h.render(writer, http.StatusUnauthorized, "security.html", pageData{Title: "Security", NavPage: "security", CSRF: request.FormValue("csrf"), OIDCProviders: h.providers(), Error: "Reauthentication failed."})
		return
	}
	transaction, loginURL, err := h.oidc.BeginLink(request.Context(), h.identity, request.FormValue("provider"), session.ID, session.UserID)
	if err != nil {
		h.render(writer, http.StatusUnprocessableEntity, "security.html", pageData{Title: "Security", NavPage: "security", CSRF: request.FormValue("csrf"), OIDCProviders: h.providers(), Error: "The OIDC link could not be started."})
		return
	}
	h.setOIDCCookies(writer, "link", transaction)
	h.redirectURL(writer, request, loginURL)
}

func (h *Handler) invitationPage(writer http.ResponseWriter, request *http.Request) {
	secret := request.PathValue("secret")
	invitation, err := h.identity.InvitationForSecret(request.Context(), secret)
	if err != nil {
		h.render(writer, http.StatusNotFound, "error.html", pageData{Title: "Invitation unavailable", Error: "This invitation is invalid, expired, revoked, or already used."})
		return
	}
	csrf := ""
	authenticated := false
	if session, err := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session")); err == nil {
		csrf = cookieValue(request, "csrf")
		_ = session
		authenticated = csrf != ""
	} else {
		csrf, err = id.New()
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		h.setCookie(writer, "invitation_csrf", csrf, time.Now().Add(10*time.Minute), true)
	}
	h.render(writer, http.StatusOK, "invitation.html", pageData{
		Title: "Workspace invitation", CSRF: csrf, Invitation: invitation,
		InvitationSecret: secret, InvitationAuthenticated: authenticated,
		ReturnTo: "/invitations/" + url.PathEscape(secret),
	})
}

func (h *Handler) invitationPost(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	secret := request.PathValue("secret")
	if request.FormValue("action") == "register" {
		if !sameValue(cookieValue(request, "invitation_csrf"), request.FormValue("csrf")) {
			h.renderError(writer, http.StatusForbidden)
			return
		}
		userID, workspaceID, err := h.identity.CreateInvitedLocalUser(request.Context(), secret, request.FormValue("password"), auditContext(request))
		if err != nil {
			h.render(writer, http.StatusUnprocessableEntity, "invitation.html", pageData{Title: "Workspace invitation", Error: "The invitation is invalid or the password could not be accepted."})
			return
		}
		session, err := h.identity.CreateSession(request.Context(), userID, "local", "", 0, auditContext(request))
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		h.setCookie(writer, "session", session.Secret, session.ExpiresAt, true)
		h.setCookie(writer, "csrf", session.CSRFSecret, session.ExpiresAt, true)
		h.clearCookie(writer, "invitation_csrf")
		h.redirect(writer, request, "/workspaces/"+workspaceID+"/items")
		return
	}
	session, ok := h.validCSRFSession(writer, request)
	if !ok {
		return
	}
	workspaceID, err := h.identity.AcceptInvitation(request.Context(), secret, session.UserID, auditContext(request))
	if err != nil {
		h.render(writer, http.StatusUnprocessableEntity, "error.html", pageData{Title: "Invitation unavailable", Error: "This invitation is invalid, expired, revoked, already used, or belongs to another email."})
		return
	}
	h.redirect(writer, request, "/workspaces/"+workspaceID+"/items")
}

func invitationFormLifetime(raw string) (time.Duration, bool) {
	if strings.TrimSpace(raw) == "" {
		return 0, true
	}
	minutes, err := strconv.Atoi(raw)
	if err != nil || minutes < 1 || minutes > 30*24*60 {
		return 0, false
	}
	return time.Duration(minutes) * time.Minute, true
}

func (h *Handler) managementError(writer http.ResponseWriter, request *http.Request, workspaceID, message string) {
	workspaces, err := h.identity.ListWorkspaces(request.Context(), cookieUserID(request, h.identity))
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	_ = workspaceID
	h.render(writer, http.StatusUnprocessableEntity, "error.html", pageData{Title: "Request failed", Error: message, Workspaces: workspaces})
}

func (h *Handler) providers() []identity.OIDCProviderInfo {
	if h.oidc == nil {
		return nil
	}
	return h.oidc.Providers()
}

func (h *Handler) deliverInvitation(request *http.Request, recipient, link, workspaceLocale string) (bool, string) {
	if h.mailer == nil {
		return false, ""
	}
	locale, ok := i18n.ParseLocale(workspaceLocale)
	if !ok {
		locale = i18n.English
	}
	subject := h.catalog.Translate(locale, "email.invitation_subject", nil)
	body := h.catalog.Translate(locale, "email.invitation_body", map[string]any{"link": link})
	if err := h.mailer.Send(request.Context(), recipient, subject, body); err != nil {
		return false, "Invitation email could not be delivered. Share the invitation link manually."
	}
	return true, ""
}

func (h *Handler) setOIDCCookies(writer http.ResponseWriter, purpose string, transaction identity.OIDCTransaction) {
	expires := transaction.ExpiresAt
	h.setCookie(writer, "oidc_flow", purpose, expires, true)
	h.setCookie(writer, "oidc_state", transaction.State, expires, true)
	h.setCookie(writer, "oidc_verifier", transaction.PKCEVerifier, expires, true)
	h.setCookie(writer, "oidc_nonce", transaction.Nonce, expires, true)
}

func (h *Handler) redirectURL(writer http.ResponseWriter, request *http.Request, target string) {
	http.Redirect(writer, request, target, http.StatusFound)
}

func safeReturnTo(value string) string {
	if !isValidRedirect(value) {
		return "/"
	}
	value = strings.ReplaceAll(value, "\\", "/")
	parsed, err := url.Parse(value)
	if err != nil {
		return "/"
	}
	return parsed.RequestURI()
}

func isValidRedirect(value string) bool {
	value = strings.ReplaceAll(value, "\\", "/")
	parsed, err := url.Parse(value)
	return err == nil && parsed.Path != "" && parsed.Path[0] == '/' &&
		!strings.HasPrefix(parsed.Path, "//") && !strings.HasPrefix(parsed.Path, "/\\") &&
		parsed.Host == "" && parsed.Scheme == ""
}

func sameValue(a, b string) bool {
	return a != "" && b != "" && a == b
}

func invitationURL(publicURL, secret string) string {
	path := "/invitations/" + url.PathEscape(secret)
	if strings.TrimSpace(publicURL) == "" {
		return path
	}
	return strings.TrimRight(publicURL, "/") + path
}

func cookieUserID(request *http.Request, service *identity.Service) string {
	session, err := service.AuthenticateSession(request.Context(), cookieValue(request, "session"))
	if err != nil {
		return ""
	}
	return session.UserID
}
