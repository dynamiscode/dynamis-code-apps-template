package web

import (
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/http"
	"time"
)

func (h *Handler) mfaPage(request *http.Request) authPageOutput {
	challenge := cookieValue(request, "mfa_challenge")
	if challenge == "" {
		return authPageOutput{kind: authPageRedirect, target: "/login"}
	}
	options, err := h.identity.MFALoginOptions(request.Context(), challenge)
	if err != nil {
		return authPageOutput{kind: authPageMFAExpired}
	}
	csrf, err := id.New()
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	return authPageOutput{kind: authPageMFAView, status: http.StatusOK, expires: options.ExpiresAt, data: viewmodel.PageData{Title: "Multi-factor authentication", CSRF: csrf, MFAChallenge: challenge, MFAOptions: options.PasskeyJSON}}
}

func (h *Handler) mfaTOTP(request *http.Request) authPageOutput {
	if err := request.ParseForm(); err != nil || !h.validMFACSRF(request) {
		return authPageOutput{status: http.StatusForbidden}
	}
	session, err := h.identity.CompleteTOTPLogin(request.Context(), cookieValue(request, "mfa_challenge"), request.FormValue("code"), auditContext(request))
	if err != nil {
		return h.mfaPageError(request, "The verification code is invalid.")
	}
	return authPageOutput{kind: authPageMFAVerified, session: session}
}

func (h *Handler) mfaRecovery(request *http.Request) authPageOutput {
	if err := request.ParseForm(); err != nil || !h.validMFACSRF(request) {
		return authPageOutput{status: http.StatusForbidden}
	}
	session, err := h.identity.CompleteRecoveryLogin(request.Context(), cookieValue(request, "mfa_challenge"), request.FormValue("code"), auditContext(request))
	if err != nil {
		return h.mfaPageError(request, "The recovery code is invalid.")
	}
	return authPageOutput{kind: authPageMFAVerified, session: session}
}

func (h *Handler) mfaPasskey(request *http.Request) authPageOutput {
	if request.Header.Get("X-MFA-CSRF") == "" || request.Header.Get("X-MFA-CSRF") != cookieValue(request, "mfa_csrf") {
		return authPageOutput{status: http.StatusForbidden}
	}
	session, err := h.identity.CompletePasskeyLogin(request.Context(), cookieValue(request, "mfa_challenge"), request, auditContext(request))
	if err != nil {
		return h.mfaPageError(request, "The passkey verification failed.")
	}
	return authPageOutput{kind: authPagePasskeyVerified, session: session}
}

func (h *Handler) beginMFALogin(writer http.ResponseWriter, request *http.Request, session identity.Session, returnTo string) {
	challenge, err := h.identity.BeginMFALoginWithMethod(request.Context(), session.UserID, session.AuthMethod, session.OIDCProviderID, auditContext(request))
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.setMFAChallengeCookies(writer, challenge, returnTo)
	h.redirect(writer, request, "/mfa")
}

// setMFAChallengeCookies starts a browser MFA challenge. returnTo is sanitized here,
// so callers pass the raw value.
func (h *Handler) setMFAChallengeCookies(writer http.ResponseWriter, challenge identity.MFALoginChallenge, returnTo string) {
	h.setCookie(writer, "mfa_challenge", challenge.Token, challenge.ExpiresAt, true)
	h.setCookie(writer, "mfa_return_to", safeReturnTo(returnTo), challenge.ExpiresAt, true)
}

func (h *Handler) setMFAFormCookie(writer http.ResponseWriter, csrf string, expires time.Time) {
	h.setCookie(writer, "mfa_csrf", csrf, expires, true)
}

// clearMFAChallengeCookies ends a browser MFA challenge on every path that ends it.
// It clears every cookie that setMFAChallengeCookies or mfaPage sets.
func (h *Handler) clearMFAChallengeCookies(writer http.ResponseWriter) {
	h.clearCookie(writer, "mfa_challenge")
	h.clearCookie(writer, "mfa_csrf")
	h.clearCookie(writer, "mfa_return_to")
}

func (h *Handler) completeMFASession(writer http.ResponseWriter, request *http.Request, session identity.NewSession) string {
	returnTo := safeReturnTo(cookieValue(request, "mfa_return_to"))
	h.setCookie(writer, "session", session.Secret, session.ExpiresAt, true)
	h.setCookie(writer, "csrf", session.CSRFSecret, session.ExpiresAt, true)
	h.clearMFAChallengeCookies(writer)
	return returnTo
}

func (h *Handler) mfaPageError(request *http.Request, message string) authPageOutput {
	options, _ := h.identity.MFALoginOptions(request.Context(), cookieValue(request, "mfa_challenge"))
	return authPageOutput{kind: authPageMFAView, status: http.StatusUnauthorized, data: viewmodel.PageData{Title: "Multi-factor authentication", CSRF: cookieValue(request, "mfa_csrf"), MFAChallenge: cookieValue(request, "mfa_challenge"), MFAOptions: options.PasskeyJSON, MFAError: message}}
}

func (h *Handler) validMFACSRF(request *http.Request) bool {
	return cookieValue(request, "mfa_csrf") != "" && cookieValue(request, "mfa_csrf") == request.FormValue("csrf")
}

func (h *Handler) securityTOTPStart(request *http.Request) authPageOutput {
	session, csrf, ok := h.checkSession(request)
	if !ok {
		return authSessionFailure(http.StatusSeeOther)
	}
	if err := request.ParseForm(); err != nil || !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		return authPageOutput{status: http.StatusForbidden}
	}
	enrollment, err := h.identity.BeginTOTPEnrollment(request.Context(), session.UserID, session.ID, request.FormValue("password"), auditContext(request))
	if err != nil {
		return h.securityErrorOutput(csrf, "Fresh authentication failed.")
	}
	status, _ := h.identity.MFAStatus(request.Context(), session.UserID)
	passkeys, _ := h.identity.ListPasskeys(request.Context(), session.UserID)
	return authPageOutput{kind: authPageSecurity, status: http.StatusOK, data: viewmodel.PageData{Title: "Security", NavPage: "security", CSRF: csrf, OIDCProviders: h.providers(), TOTPEnrollment: &enrollment, Passkeys: passkeys, MFAStatus: status}}
}

func (h *Handler) securityTOTPComplete(request *http.Request) authPageOutput {
	session, csrf, ok := h.checkSession(request)
	if !ok {
		return authSessionFailure(http.StatusSeeOther)
	}
	if err := request.ParseForm(); err != nil || !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		return authPageOutput{status: http.StatusForbidden}
	}
	codes, err := h.identity.CompleteTOTPEnrollment(request.Context(), session.ID, request.FormValue("challenge"), request.FormValue("code"), auditContext(request))
	if err != nil {
		return h.securityErrorOutput(csrf, "The verification code is invalid.")
	}
	status, _ := h.identity.MFAStatus(request.Context(), session.UserID)
	passkeys, _ := h.identity.ListPasskeys(request.Context(), session.UserID)
	return authPageOutput{kind: authPageSecurity, status: http.StatusOK, data: viewmodel.PageData{Title: "Security", NavPage: "security", CSRF: csrf, OIDCProviders: h.providers(), MFARecoveryCodes: codes, Passkeys: passkeys, MFAStatus: status}}
}

func (h *Handler) securityTOTPRemove(request *http.Request) authPageOutput {
	session, csrf, ok := h.checkSession(request)
	if !ok {
		return authSessionFailure(http.StatusSeeOther)
	}
	if err := request.ParseForm(); err != nil || !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		return authPageOutput{status: http.StatusForbidden}
	}
	if err := h.identity.RemoveTOTP(request.Context(), session.UserID, session.ID, request.FormValue("password"), auditContext(request)); err != nil {
		return h.securityErrorOutput(csrf, "The authenticator could not be removed.")
	}
	return authPageOutput{kind: authPageRedirect, target: "/security"}
}

func (h *Handler) securityPasskeyRemove(request *http.Request) authPageOutput {
	session, csrf, ok := h.checkSession(request)
	if !ok {
		return authSessionFailure(http.StatusSeeOther)
	}
	if err := request.ParseForm(); err != nil || !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		return authPageOutput{status: http.StatusForbidden}
	}
	if err := h.identity.RemovePasskey(request.Context(), session.UserID, session.ID, request.PathValue("passkeyId"), request.FormValue("password"), auditContext(request)); err != nil {
		return h.securityErrorOutput(csrf, "The passkey could not be removed.")
	}
	return authPageOutput{kind: authPageRedirect, target: "/security"}
}

func (h *Handler) securityErrorOutput(csrf, message string) authPageOutput {
	return authPageOutput{kind: authPageSecurity, status: http.StatusUnauthorized, data: viewmodel.PageData{Title: "Security", NavPage: "security", CSRF: csrf, OIDCProviders: h.providers(), MFAError: message}}
}
