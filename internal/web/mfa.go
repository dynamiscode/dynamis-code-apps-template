package web

import (
	"net/http"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func (h *Handler) mfaPage(writer http.ResponseWriter, request *http.Request) {
	challenge := cookieValue(request, "mfa_challenge")
	if challenge == "" {
		h.redirect(writer, request, "/login")
		return
	}
	options, err := h.identity.MFALoginOptions(request.Context(), challenge)
	if err != nil {
		h.clearCookie(writer, "mfa_challenge")
		h.clearCookie(writer, "mfa_return_to")
		h.redirect(writer, request, "/login")
		return
	}
	csrf, err := id.New()
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.setCookie(writer, "mfa_csrf", csrf, options.ExpiresAt, true)
	h.render(writer, http.StatusOK, "mfa.html", pageData{Title: "Multi-factor authentication", CSRF: csrf, MFAChallenge: challenge, MFAOptions: options.PasskeyJSON})
}

func (h *Handler) mfaTOTP(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil || !h.validMFACSRF(request) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	session, err := h.identity.CompleteTOTPLogin(request.Context(), cookieValue(request, "mfa_challenge"), request.FormValue("code"), auditContext(request))
	if err != nil {
		h.mfaPageError(writer, request, "The verification code is invalid.")
		return
	}
	returnTo := h.completeMFASession(writer, request, session)
	h.redirect(writer, request, returnTo)
}

func (h *Handler) mfaRecovery(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil || !h.validMFACSRF(request) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	session, err := h.identity.CompleteRecoveryLogin(request.Context(), cookieValue(request, "mfa_challenge"), request.FormValue("code"), auditContext(request))
	if err != nil {
		h.mfaPageError(writer, request, "The recovery code is invalid.")
		return
	}
	returnTo := h.completeMFASession(writer, request, session)
	h.redirect(writer, request, returnTo)
}

func (h *Handler) mfaPasskey(writer http.ResponseWriter, request *http.Request) {
	if request.Header.Get("X-MFA-CSRF") == "" || request.Header.Get("X-MFA-CSRF") != cookieValue(request, "mfa_csrf") {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	session, err := h.identity.CompletePasskeyLogin(request.Context(), cookieValue(request, "mfa_challenge"), request, auditContext(request))
	if err != nil {
		h.mfaPageError(writer, request, "The passkey verification failed.")
		return
	}
	returnTo := h.completeMFASession(writer, request, session)
	writer.Header().Set("X-MFA-Return-To", returnTo)
	writer.WriteHeader(http.StatusNoContent)
}

func (h *Handler) completeMFASession(writer http.ResponseWriter, request *http.Request, session identity.NewSession) string {
	returnTo := safeReturnTo(cookieValue(request, "mfa_return_to"))
	h.setCookie(writer, "session", session.Secret, session.ExpiresAt, true)
	h.setCookie(writer, "csrf", session.CSRFSecret, session.ExpiresAt, true)
	h.clearCookie(writer, "mfa_challenge")
	h.clearCookie(writer, "mfa_csrf")
	h.clearCookie(writer, "mfa_return_to")
	return returnTo
}

func (h *Handler) mfaPageError(writer http.ResponseWriter, request *http.Request, message string) {
	options, _ := h.identity.MFALoginOptions(request.Context(), cookieValue(request, "mfa_challenge"))
	h.render(writer, http.StatusUnauthorized, "mfa.html", pageData{Title: "Multi-factor authentication", CSRF: cookieValue(request, "mfa_csrf"), MFAChallenge: cookieValue(request, "mfa_challenge"), MFAOptions: options.PasskeyJSON, MFAError: message})
}

func (h *Handler) validMFACSRF(request *http.Request) bool {
	return cookieValue(request, "mfa_csrf") != "" && cookieValue(request, "mfa_csrf") == request.FormValue("csrf")
}

func (h *Handler) securityTOTPStart(writer http.ResponseWriter, request *http.Request) {
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return
	}
	if err := request.ParseForm(); err != nil || !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	enrollment, err := h.identity.BeginTOTPEnrollment(request.Context(), session.UserID, session.ID, request.FormValue("password"), auditContext(request))
	if err != nil {
		h.securityError(writer, request, csrf, "Fresh authentication failed.")
		return
	}
	status, _ := h.identity.MFAStatus(request.Context(), session.UserID)
	passkeys, _ := h.identity.ListPasskeys(request.Context(), session.UserID)
	h.render(writer, http.StatusOK, "security.html", pageData{Title: "Security", NavPage: "security", CSRF: csrf, OIDCProviders: h.providers(), TOTPEnrollment: &enrollment, Passkeys: passkeys, MFAStatus: status})
}

func (h *Handler) securityTOTPComplete(writer http.ResponseWriter, request *http.Request) {
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return
	}
	if err := request.ParseForm(); err != nil || !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	codes, err := h.identity.CompleteTOTPEnrollment(request.Context(), session.ID, request.FormValue("challenge"), request.FormValue("code"), auditContext(request))
	if err != nil {
		h.securityError(writer, request, csrf, "The verification code is invalid.")
		return
	}
	status, _ := h.identity.MFAStatus(request.Context(), session.UserID)
	passkeys, _ := h.identity.ListPasskeys(request.Context(), session.UserID)
	h.render(writer, http.StatusOK, "security.html", pageData{Title: "Security", NavPage: "security", CSRF: csrf, OIDCProviders: h.providers(), MFARecoveryCodes: codes, Passkeys: passkeys, MFAStatus: status})
}

func (h *Handler) securityTOTPRemove(writer http.ResponseWriter, request *http.Request) {
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return
	}
	if err := request.ParseForm(); err != nil || !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	if err := h.identity.RemoveTOTP(request.Context(), session.UserID, session.ID, request.FormValue("password"), auditContext(request)); err != nil {
		h.securityError(writer, request, csrf, "The authenticator could not be removed.")
		return
	}
	h.redirect(writer, request, "/security")
}

func (h *Handler) securityPasskeyRemove(writer http.ResponseWriter, request *http.Request) {
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return
	}
	if err := request.ParseForm(); err != nil || !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	if err := h.identity.RemovePasskey(request.Context(), session.UserID, session.ID, request.PathValue("passkeyId"), request.FormValue("password"), auditContext(request)); err != nil {
		h.securityError(writer, request, csrf, "The passkey could not be removed.")
		return
	}
	h.redirect(writer, request, "/security")
}

func (h *Handler) securityError(writer http.ResponseWriter, request *http.Request, csrf, message string) {
	h.render(writer, http.StatusUnauthorized, "security.html", pageData{Title: "Security", NavPage: "security", CSRF: csrf, OIDCProviders: h.providers(), MFAError: message})
}
