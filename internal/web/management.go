package web

import (
	"context"
	"errors"
	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type exporter interface {
	Export(context.Context, identity.Principal, identity.WorkspaceID, identity.AuditContext) ([]byte, error)
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
		h.render(writer, http.StatusUnprocessableEntity, "home.html", viewmodel.PageData{
			Title: "Workspaces", NavPage: "home", CSRF: csrf, Workspaces: workspaces,
			CanCreateWorkspace: h.workspaceCreationEnabled(),
			Error:              "Enter a workspace name between 1 and 120 characters.",
		})
		return
	}
	h.redirect(writer, request, "/")
}

func (h *Handler) managementPrincipal(
	writer http.ResponseWriter,
	request *http.Request,
	workspaceID identity.WorkspaceID,
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
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	if _, _, _, ok := h.workspaceSession(writer, request, workspaceID, identity.MembersRead); !ok {
		return
	}
	h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/settings/members")
}

func (h *Handler) membersPage(writer http.ResponseWriter, request *http.Request) {
	h.renderMembersList(writer, request)
}

func (h *Handler) memberMutation(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, _, _, ok := h.managementPrincipal(writer, request, workspaceID, identity.MembersManage)
	if !ok {
		return
	}
	memberID, _ := identity.ParseMemberID(request.FormValue("member_id")) // malformed IDs match no member
	var err error
	switch request.FormValue("action") {
	case "role":
		role := identity.Role(request.FormValue("role"))
		if role != identity.Admin && role != identity.Member && role != identity.Viewer {
			h.managementError(writer, request, workspaceID, "The member role is invalid.")
			return
		}
		err = h.identity.ChangeMemberRole(request.Context(), principal, memberID, role, auditContext(request))
	case "remove":
		if request.FormValue("confirm_remove") != "yes" {
			h.managementError(writer, request, workspaceID, "Confirm member removal to continue.")
			return
		}
		err = h.identity.RemoveMember(request.Context(), principal, memberID, auditContext(request))
	case "transfer":
		if request.FormValue("confirm_transfer") != "yes" {
			h.managementError(writer, request, workspaceID, "Confirm ownership transfer to continue.")
			return
		}
		err = h.identity.TransferOwnership(request.Context(), principal, memberID, auditContext(request))
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
	h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/settings/members")
}

func (h *Handler) invitationsPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
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
	h.renderInvitationComponent(writer, request, http.StatusOK, viewmodel.PageData{
		Title: "Invitations", NavPage: "invitations", NavSection: "settings", CSRF: csrf, Workspace: workspaceByID(workspaces, workspaceID),
		Workspaces: workspaces, Invitations: invitations,
	}, invitationManagementView)
}

func (h *Handler) invitationMutation(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
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
		_, warning := h.deliverInvitation(request, workspaceID.String(), invitation.Email, link, workspaceLocale)
		h.renderInvitationResult(writer, request, workspaceID, csrf, link, warning)
		return
	}
	invitationID, _ := identity.ParseInvitationID(request.FormValue("invitation_id")) // malformed IDs match no invitation
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
				_, warning := h.deliverInvitation(request, workspaceID.String(), invitation.Email, link, workspaceLocale)
				h.renderInvitationResult(writer, request, workspaceID, csrf, link, warning)
				return
			}
		}
		h.managementError(writer, request, workspaceID, "The invitation could not be found.")
	case "revoke":
		if request.FormValue("confirm_revoke") != "yes" {
			h.managementError(writer, request, workspaceID, "Confirm invitation revocation to continue.")
			return
		}
		if err := h.identity.RevokeInvitation(request.Context(), principal, invitationID, auditContext(request)); err != nil {
			h.managementError(writer, request, workspaceID, "The invitation could not be revoked.")
			return
		}
		h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/settings/invitations")
	default:
		h.managementError(writer, request, workspaceID, "The requested invitation action is invalid.")
	}
}

func (h *Handler) renderInvitationResult(writer http.ResponseWriter, request *http.Request, workspaceID identity.WorkspaceID, csrf, link, warning string) {
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
	h.renderInvitationComponent(writer, request, http.StatusOK, viewmodel.PageData{
		Title: "Invitations", NavPage: "invitations", NavSection: "settings", CSRF: csrf, Workspace: workspaceByID(workspaces, workspaceID),
		Workspaces: workspaces, Invitations: invitations, InvitationURL: link,
		DeliveryWarning: warning,
	}, invitationManagementView)
}

func (h *Handler) tokensPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, _, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceRead)
	if !ok {
		return
	}
	h.renderTokenSettingsPage(writer, request, principal, csrf, http.StatusOK, "", "")
}

func (h *Handler) tokenMutation(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, _, csrf, ok := h.managementPrincipal(writer, request, workspaceID, identity.WorkspaceRead)
	if !ok {
		return
	}
	if request.FormValue("action") == "create" {
		var expiresAt *time.Time
		if raw := strings.TrimSpace(request.FormValue("expires_at")); raw != "" {
			value, err := time.Parse("2006-01-02", raw)
			if err != nil {
				h.renderTokenSettingsPage(writer, request, principal, csrf, http.StatusUnprocessableEntity, "", "The token expiration is invalid.")
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
			h.renderTokenSettingsPage(writer, request, principal, csrf, http.StatusUnprocessableEntity, "", "The token could not be created.")
			return
		}
		h.renderTokenSettingsPage(writer, request, principal, csrf, http.StatusOK, token.Secret, "")
		return
	}
	tokenID, _ := identity.ParseAPITokenID(request.PathValue("tokenId")) // malformed IDs match no token
	if request.FormValue("action") == "revoke" {
		if err := h.identity.RevokeAPIToken(request.Context(), principal, tokenID, auditContext(request)); err != nil {
			h.renderTokenSettingsPage(writer, request, principal, csrf, http.StatusUnprocessableEntity, "", "The token could not be revoked.")
			return
		}
		h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/settings/tokens")
		return
	}
	h.renderTokenSettingsPage(writer, request, principal, csrf, http.StatusUnprocessableEntity, "", "The requested token action is invalid.")
}

func (h *Handler) renderTokenSettingsPage(
	writer http.ResponseWriter,
	request *http.Request,
	principal identity.Principal,
	csrf string,
	status int,
	secret string,
	errorMessage string,
) {
	workspaces, err := h.identity.ListWorkspaces(request.Context(), principal.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	tokens, err := h.identity.ListAPITokens(request.Context(), principal)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.renderSettingsPage(writer, request, status, viewmodel.PageData{
		Title: "API tokens", NavPage: "tokens", NavSection: "settings", CSRF: csrf,
		Workspace: workspaceByID(workspaces, principal.WorkspaceID), Workspaces: workspaces,
		Tokens: tokens, TokenSecret: secret, Error: errorMessage,
	}, apiTokenSettingsPage)
}

func (h *Handler) provisioningPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.SCIMManage)
	if !ok {
		return
	}
	h.renderProvisioningPage(writer, request, principal, session, csrf, http.StatusOK, "", "")
}

func (h *Handler) provisioningMutation(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.managementPrincipal(writer, request, workspaceID, identity.SCIMManage)
	if !ok {
		return
	}
	switch request.FormValue("action") {
	case "create":
		token, err := h.identity.CreateSCIMToken(request.Context(), principal, auditContext(request))
		if err != nil {
			h.renderProvisioningPage(writer, request, principal, session, csrf, http.StatusUnprocessableEntity, "", "The SCIM credential could not be created.")
			return
		}
		h.renderProvisioningPage(writer, request, principal, session, csrf, http.StatusOK, token.Secret, "")
	case "revoke":
		if err := h.identity.RevokeSCIMToken(request.Context(), principal, auditContext(request)); err != nil {
			h.renderProvisioningPage(writer, request, principal, session, csrf, http.StatusUnprocessableEntity, "", "The SCIM credential could not be revoked.")
			return
		}
		h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/settings/provisioning")
	default:
		h.renderProvisioningPage(writer, request, principal, session, csrf, http.StatusUnprocessableEntity, "", "The requested SCIM action is invalid.")
	}
}

func (h *Handler) renderProvisioningPage(
	writer http.ResponseWriter,
	request *http.Request,
	principal identity.Principal,
	session identity.Session,
	csrf string,
	status int,
	secret string,
	errorMessage string,
) {
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	workspaceID := principal.WorkspaceID
	h.renderSettingsPage(writer, request, status, viewmodel.PageData{
		Title: "SCIM provisioning", NavPage: "provisioning", NavSection: "settings", CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		SCIMEndpoint: scimEndpoint(h.publicURL, h.cfg.Secure, request, workspaceID), SCIMTokenSecret: secret, Error: errorMessage,
	}, scimProvisioningSettingsPage)
}

func scimEndpoint(publicURL string, secure bool, request *http.Request, workspaceID identity.WorkspaceID) string {
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
	return base + "/scim/v2/" + url.PathEscape(workspaceID.String())
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
	h.render(writer, http.StatusOK, "sessions-page", viewmodel.PageData{Title: "Sessions", NavPage: "sessions", CSRF: csrf, Sessions: sessions, CurrentSessionID: session.ID, Saved: request.URL.Query().Get("revoked") == "1"})
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
	h.redirect(writer, request, "/sessions?revoked=1")
}

func (h *Handler) exportPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
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
	h.renderSettingsPage(writer, request, http.StatusOK, viewmodel.PageData{
		Title: "Export", NavPage: "export", NavSection: "settings", CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		DefaultWorkspaceHref: h.defaultHomeHref(request.Context(), session.UserID),
	}, exportSettingsView)
}

func (h *Handler) auditHistoryPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceExport)
	if !ok {
		return
	}
	// The page shell renders first and forms.js fetches the table as a fragment
	// (lazy loading). Without JavaScript, the placeholder links to ?all=1, which
	// renders the table inline.
	fragment := request.Header.Get("HX-Request") == "true" && request.URL.Query().Get("part") == "table"
	lazy := !fragment && request.URL.Query().Get("all") != "1"
	var history []identity.AuditHistoryEntry
	if !lazy {
		var err error
		history, err = h.identity.ListAuditHistory(request.Context(), principal, workspaceID)
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
	}
	// Emails name actors in history rows only, so the member lookup is skipped for an empty or lazy table.
	var emails map[identity.UserID]string
	if len(history) > 0 {
		emails = h.auditActorEmails(request.Context(), principal)
	}
	if fragment {
		data := viewmodel.PageData{AuditHistory: history, AuditActorEmails: emails}
		h.render(writer, http.StatusOK, "audit-table", data)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	data := viewmodel.PageData{
		Title: "Audit history", NavPage: "audit", NavSection: "settings", CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		AuditHistory: history, AuditActorEmails: emails, AuditLazy: lazy, CurrentPath: request.URL.Path,
		DefaultWorkspaceHref: h.defaultHomeHref(request.Context(), session.UserID),
	}
	h.render(writer, http.StatusOK, "audit-history", data)
}

// auditActorEmails maps workspace users to their emails so the audit table can name actors. A user who has left
// the workspace is absent, and the table falls back to the user ID. A caller without member read access gets none.
func (h *Handler) auditActorEmails(ctx context.Context, principal identity.Principal) map[identity.UserID]string {
	members, err := h.identity.ListMembers(ctx, principal)
	if err != nil {
		return nil
	}
	emails := make(map[identity.UserID]string, len(members))
	for _, member := range members {
		emails[member.UserID] = member.Email
	}
	return emails
}

func (h *Handler) exportWorkspace(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
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
	h.render(writer, http.StatusOK, "security-page", viewmodel.PageData{
		Title: "Security", NavPage: "security", CSRF: csrf, OIDCProviders: h.providers(),
		Error: request.URL.Query().Get("error"), Email: session.UserID.String(), MFAStatus: status, Passkeys: passkeys,
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
		h.render(writer, http.StatusUnprocessableEntity, "security-page", viewmodel.PageData{Title: "Security", NavPage: "security", CSRF: request.FormValue("csrf"), Error: "No OIDC provider is configured."})
		return
	}
	if err := h.identity.ReauthenticateLocal(request.Context(), session.UserID, request.FormValue("password")); err != nil {
		h.render(writer, http.StatusUnauthorized, "security-page", viewmodel.PageData{Title: "Security", NavPage: "security", CSRF: request.FormValue("csrf"), OIDCProviders: h.providers(), Error: "Reauthentication failed."})
		return
	}
	transaction, loginURL, err := h.oidc.BeginLink(request.Context(), h.identity, request.FormValue("provider"), session.ID, session.UserID)
	if err != nil {
		h.render(writer, http.StatusUnprocessableEntity, "security-page", viewmodel.PageData{Title: "Security", NavPage: "security", CSRF: request.FormValue("csrf"), OIDCProviders: h.providers(), Error: "The OIDC link could not be started."})
		return
	}
	h.setOIDCCookies(writer, "link", transaction)
	h.redirectURL(writer, request, loginURL)
}

func (h *Handler) invitationPage(writer http.ResponseWriter, request *http.Request) {
	setInvitationSecurityHeaders(writer)
	h.renderInvitationPage(writer, request, http.StatusOK, "")
}

func (h *Handler) renderInvitationPage(writer http.ResponseWriter, request *http.Request, status int, errorKey string) {
	secret := request.PathValue("secret")
	invitation, err := h.identity.InvitationForSecret(request.Context(), secret)
	if err != nil {
		if status == http.StatusOK {
			status = http.StatusNotFound
			errorKey = "invitations.unavailable"
		}
		if errorKey == "" {
			errorKey = "invitations.unavailable"
		}
		h.renderInvitationComponent(writer, request, status, viewmodel.PageData{
			Title: "Workspace invitation", InvitationSecret: secret,
			ErrorKey: errorKey,
		}, invitationView)
		return
	}
	csrf := ""
	authenticated := false
	if session, err := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session")); err == nil {
		csrf = cookieValue(request, "csrf")
		_ = session
		authenticated = csrf != ""
	} else {
		csrf = cookieValue(request, "invitation_csrf")
		if csrf == "" {
			csrf, err = id.New()
			if err != nil {
				h.renderError(writer, http.StatusInternalServerError)
				return
			}
			h.setCookie(writer, "invitation_csrf", csrf, time.Now().Add(10*time.Minute), true)
		}
	}
	h.renderInvitationComponent(writer, request, status, viewmodel.PageData{
		Title: "Workspace invitation", CSRF: csrf, Invitation: invitation,
		InvitationSecret: secret, InvitationAuthenticated: authenticated,
		ErrorKey: errorKey,
	}, invitationView)
}

func (h *Handler) invitationPost(writer http.ResponseWriter, request *http.Request) {
	setInvitationSecurityHeaders(writer)
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
			h.renderInvitationPage(writer, request, http.StatusUnprocessableEntity, "invitations.registration_failed")
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
		h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/items")
		return
	}
	session, ok := h.validCSRFSession(writer, request)
	if !ok {
		return
	}
	workspaceID, err := h.identity.AcceptInvitation(request.Context(), secret, session.UserID, auditContext(request))
	if err != nil {
		h.renderInvitationPage(writer, request, http.StatusUnprocessableEntity, "invitations.accept_failed")
		return
	}
	h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/items")
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

func (h *Handler) managementError(writer http.ResponseWriter, request *http.Request, workspaceID identity.WorkspaceID, message string) {
	workspaces, err := h.identity.ListWorkspaces(request.Context(), cookieUserID(request, h.identity))
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusUnprocessableEntity, "error.html", viewmodel.PageData{
		Title: "Request failed", Error: message, Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
	})
}

func (h *Handler) providers() []identity.OIDCProviderInfo {
	if h.oidc == nil {
		return nil
	}
	return h.oidc.Providers()
}

func (h *Handler) deliverInvitation(request *http.Request, workspaceID, recipient, link, workspaceLocale string) (bool, string) {
	locale, ok := i18n.ParseLocale(workspaceLocale)
	if !ok {
		locale = i18n.English
	}
	subject := h.catalog.Translate(locale, "email.invitation_subject", nil)
	body := h.catalog.Translate(locale, "email.invitation_body", map[string]any{"link": link})
	if err := appmail.SendFor(request.Context(), h.mailer, workspaceID, recipient, subject, body); err != nil {
		if errors.Is(err, appmail.ErrNotConfigured) {
			return false, ""
		}
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

func cookieUserID(request *http.Request, service *identity.Service) identity.UserID {
	session, err := service.AuthenticateSession(request.Context(), cookieValue(request, "session"))
	if err != nil {
		return 0
	}
	return session.UserID
}
