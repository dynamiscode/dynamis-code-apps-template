package web

import (
	"net/http"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

type invitationOutputKind uint8

const (
	invitationError invitationOutputKind = iota
	invitationPageMarkup
	invitationContinueMarkup
	invitationRedirect
	invitationLogin
	invitationHandoff
	invitationRegistered
	invitationAccepted
)

type invitationOutput struct {
	kind      invitationOutputKind
	status    int
	data      viewmodel.PageData
	newCSRF   string
	clearCSRF bool
	handoff   identity.InvitationReturnHandoff
	session   identity.NewSession
	target    string
}

// serveInvitation keeps the browser capability out of invitation decisions
// and applies secret-bearing route headers to every outcome before writing.
func (h *Handler) serveInvitation(decide func(*http.Request) invitationOutput) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		setInvitationSecurityHeaders(writer)
		result := decide(request)
		switch result.kind {
		case invitationPageMarkup:
			if result.newCSRF != "" {
				h.setCookie(writer, "invitation_csrf", result.newCSRF, time.Now().Add(10*time.Minute), true)
			}
			h.renderInvitationComponent(writer, request, result.status, result.data, invitationView)
		case invitationContinueMarkup:
			h.renderInvitationComponent(writer, request, result.status, result.data, invitationContinueView)
		case invitationRedirect:
			h.redirect(writer, request, result.target)
		case invitationLogin:
			http.Redirect(writer, request, invitationLoginLocation, http.StatusSeeOther)
		case invitationHandoff:
			h.setCookie(writer, "invitation_handoff", result.handoff.Token, result.handoff.ExpiresAt, true)
			h.setCookie(writer, "invitation_handoff_binding", result.handoff.Binding, result.handoff.ExpiresAt, true)
			if result.clearCSRF {
				h.clearCookie(writer, "invitation_csrf")
			}
			http.Redirect(writer, request, invitationLoginLocation, http.StatusSeeOther)
		case invitationRegistered:
			h.setCookie(writer, "session", result.session.Secret, result.session.ExpiresAt, true)
			h.setCookie(writer, "csrf", result.session.CSRFSecret, result.session.ExpiresAt, true)
			h.clearCookie(writer, "invitation_csrf")
			h.redirect(writer, request, result.target)
		case invitationAccepted:
			h.clearCookie(writer, "invitation_handoff")
			h.clearCookie(writer, "invitation_handoff_binding")
			h.redirect(writer, request, result.target)
		case invitationError:
			switch result.status {
			case http.StatusBadRequest, http.StatusForbidden:
				h.renderError(writer, result.status)
			default:
				h.renderError(writer, http.StatusInternalServerError)
			}
		default:
			h.renderError(writer, http.StatusInternalServerError)
		}
	}
}
