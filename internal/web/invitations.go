package web

import (
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"github.com/a-h/templ"
	"net/http"
	"strings"
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
	data viewmodel.PageData,
	component func(viewmodel.PageData) templ.Component,
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

func (h *Handler) invitationSignIn(request *http.Request) invitationOutput {
	if err := request.ParseForm(); err != nil {
		return invitationOutput{status: http.StatusBadRequest}
	}
	csrf := request.FormValue("csrf")
	session, sessionErr := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session"))
	if sessionErr == nil {
		if !h.identity.VerifyCSRF(request.Context(), session.ID, csrf) {
			return invitationOutput{status: http.StatusForbidden}
		}
	} else if !sameValue(cookieValue(request, "invitation_csrf"), csrf) {
		return invitationOutput{status: http.StatusForbidden}
	}
	handoff, err := h.identity.BeginInvitationReturn(request.Context(), request.PathValue("secret"))
	if err != nil {
		return h.invitationPageResult(request, http.StatusNotFound, "invitations.unavailable")
	}
	return invitationOutput{kind: invitationHandoff, handoff: handoff, clearCSRF: sessionErr != nil}
}

func (h *Handler) invitationContinuePage(request *http.Request) invitationOutput {
	invitation, err := h.identity.InvitationForReturn(
		request.Context(), cookieValue(request, "invitation_handoff"),
		cookieValue(request, "invitation_handoff_binding"),
	)
	if err != nil {
		return invitationOutput{kind: invitationContinueMarkup, status: http.StatusNotFound, data: viewmodel.PageData{Title: "Workspace invitation", ErrorKey: "invitations.unavailable"}}
	}
	session, err := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session"))
	csrf := cookieValue(request, "csrf")
	if err != nil || csrf == "" || !h.identity.VerifyCSRF(request.Context(), session.ID, csrf) {
		return invitationOutput{kind: invitationLogin}
	}
	return invitationOutput{kind: invitationContinueMarkup, status: http.StatusOK, data: viewmodel.PageData{
		Title: "Workspace invitation", CSRF: csrf, Invitation: invitation,
		InvitationAuthenticated: true,
	}}
}

func (h *Handler) invitationContinuePost(request *http.Request) invitationOutput {
	if err := request.ParseForm(); err != nil {
		return invitationOutput{status: http.StatusBadRequest}
	}
	token := cookieValue(request, "invitation_handoff")
	binding := cookieValue(request, "invitation_handoff_binding")
	invitation, err := h.identity.InvitationForReturn(request.Context(), token, binding)
	if err != nil {
		return invitationOutput{kind: invitationContinueMarkup, status: http.StatusNotFound, data: viewmodel.PageData{Title: "Workspace invitation", ErrorKey: "invitations.unavailable"}}
	}
	session, err := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session"))
	if err != nil {
		return invitationOutput{kind: invitationLogin}
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		return invitationOutput{status: http.StatusForbidden}
	}
	workspaceID, err := h.identity.AcceptInvitationReturn(
		request.Context(), token, binding, session.UserID, auditContext(request),
	)
	if err != nil {
		return invitationOutput{kind: invitationContinueMarkup, status: http.StatusUnprocessableEntity, data: viewmodel.PageData{
			Title: "Workspace invitation", CSRF: cookieValue(request, "csrf"),
			Invitation: invitation, InvitationAuthenticated: true,
			ErrorKey: "invitations.accept_failed",
		}}
	}
	return invitationOutput{kind: invitationAccepted, target: "/workspaces/" + workspaceID.String()}
}
