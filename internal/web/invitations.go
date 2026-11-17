package web

import (
	"net/http"
	"strings"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"github.com/a-h/templ"
)

const (
	invitationContinuePath  = "/invitations/continue"
	invitationLoginLocation = "/login?return_to=%2Finvitations%2Fcontinue"
)

func (h *Handler) invitationReturnActive(request *http.Request) bool {
	_, err := h.identity.InvitationForReturn(
		request.Context(), cookieValue(request, "invitation_handoff"),
		cookieValue(request, "invitation_handoff_binding"),
	)
	return err == nil
}

func (h *Handler) invitationLoginReturnTo(request *http.Request, candidate string) string {
	if candidate == invitationContinuePath && h.invitationReturnActive(request) {
		return invitationContinuePath
	}
	if strings.HasPrefix(candidate, "/invitations/") {
		return "/"
	}
	return safeReturnTo(candidate)
}

func (h *Handler) renderInvitationComponent(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	data pageData,
	component func(pageData) templ.Component,
) {
	data.FilesEnabled = h.filesEnabled
	data.ClientsEnabled = h.clientsEnabled
	data.SingleWorkspace = h.workspaceMode == config.WorkspaceModeSingle
	h.localizePage(writer, &data)
	setInvitationSecurityHeaders(writer)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.WriteHeader(status)
	_ = component(data).Render(request.Context(), writer)
}

func setInvitationSecurityHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
}

func (h *Handler) invitationSignIn(writer http.ResponseWriter, request *http.Request) {
	setInvitationSecurityHeaders(writer)
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	csrf := request.FormValue("csrf")
	session, sessionErr := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session"))
	if sessionErr == nil {
		if !h.identity.VerifyCSRF(request.Context(), session.ID, csrf) {
			h.renderError(writer, http.StatusForbidden)
			return
		}
	} else if !sameValue(cookieValue(request, "invitation_csrf"), csrf) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	handoff, err := h.identity.BeginInvitationReturn(request.Context(), request.PathValue("secret"))
	if err != nil {
		h.renderInvitationPage(writer, request, http.StatusNotFound, "invitations.unavailable")
		return
	}
	h.setCookie(writer, "invitation_handoff", handoff.Token, handoff.ExpiresAt, true)
	h.setCookie(writer, "invitation_handoff_binding", handoff.Binding, handoff.ExpiresAt, true)
	if sessionErr != nil {
		h.clearCookie(writer, "invitation_csrf")
	}
	http.Redirect(writer, request, invitationLoginLocation, http.StatusSeeOther)
}

func (h *Handler) invitationContinuePage(writer http.ResponseWriter, request *http.Request) {
	setInvitationSecurityHeaders(writer)
	invitation, err := h.identity.InvitationForReturn(
		request.Context(), cookieValue(request, "invitation_handoff"),
		cookieValue(request, "invitation_handoff_binding"),
	)
	if err != nil {
		h.renderInvitationComponent(writer, request, http.StatusNotFound,
			pageData{Title: "Workspace invitation", ErrorKey: "invitations.unavailable"}, invitationContinueView)
		return
	}
	session, err := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session"))
	csrf := cookieValue(request, "csrf")
	if err != nil || csrf == "" || !h.identity.VerifyCSRF(request.Context(), session.ID, csrf) {
		http.Redirect(writer, request, invitationLoginLocation, http.StatusSeeOther)
		return
	}
	h.renderInvitationComponent(writer, request, http.StatusOK, pageData{
		Title: "Workspace invitation", CSRF: csrf, Invitation: invitation,
		InvitationAuthenticated: true,
	}, invitationContinueView)
}

func (h *Handler) invitationContinuePost(writer http.ResponseWriter, request *http.Request) {
	setInvitationSecurityHeaders(writer)
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	token := cookieValue(request, "invitation_handoff")
	binding := cookieValue(request, "invitation_handoff_binding")
	invitation, err := h.identity.InvitationForReturn(request.Context(), token, binding)
	if err != nil {
		h.renderInvitationComponent(writer, request, http.StatusNotFound,
			pageData{Title: "Workspace invitation", ErrorKey: "invitations.unavailable"}, invitationContinueView)
		return
	}
	session, err := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session"))
	if err != nil {
		http.Redirect(writer, request, invitationLoginLocation, http.StatusSeeOther)
		return
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	workspaceID, err := h.identity.AcceptInvitationReturn(
		request.Context(), token, binding, session.UserID, auditContext(request),
	)
	if err != nil {
		h.renderInvitationComponent(writer, request, http.StatusUnprocessableEntity, pageData{
			Title: "Workspace invitation", CSRF: cookieValue(request, "csrf"),
			Invitation: invitation, InvitationAuthenticated: true,
			ErrorKey: "invitations.accept_failed",
		}, invitationContinueView)
		return
	}
	h.clearCookie(writer, "invitation_handoff")
	h.clearCookie(writer, "invitation_handoff_binding")
	h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/items")
}
