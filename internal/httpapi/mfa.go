package httpapi

import (
	"errors"
	"net/http"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func (h *handler) mfaLoginTOTP(writer http.ResponseWriter, request *http.Request) {
	var input struct{ Challenge, Code string }
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	session, err := h.identity.CompleteTOTPLogin(request.Context(), input.Challenge, input.Code, h.auditContext(request))
	if err != nil {
		h.mfaProblem(writer, request, err)
		return
	}
	h.writeAuthenticatedSession(writer, request, session)
}

func (h *handler) mfaLoginRecovery(writer http.ResponseWriter, request *http.Request) {
	var input struct{ Challenge, Code string }
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	session, err := h.identity.CompleteRecoveryLogin(request.Context(), input.Challenge, input.Code, h.auditContext(request))
	if err != nil {
		h.mfaProblem(writer, request, err)
		return
	}
	h.writeAuthenticatedSession(writer, request, session)
}

func (h *handler) mfaLoginPasskey(writer http.ResponseWriter, request *http.Request) {
	challenge := request.Header.Get("X-MFA-Challenge")
	if challenge == "" {
		h.invalidRequest(writer, request, "The MFA challenge is invalid.")
		return
	}
	session, err := h.identity.CompletePasskeyLogin(request.Context(), challenge, request, h.auditContext(request))
	if err != nil {
		h.mfaProblem(writer, request, err)
		return
	}
	h.writeAuthenticatedSession(writer, request, session)
}

func (h *handler) mfaStatus(writer http.ResponseWriter, request *http.Request) {
	session, ok := h.sessionCookie(writer, request, false)
	if !ok {
		return
	}
	status, err := h.identity.MFAStatus(request.Context(), session.UserID)
	if err != nil {
		h.internal(writer, request)
		return
	}
	passkeys, err := h.identity.ListPasskeys(request.Context(), session.UserID)
	if err != nil {
		h.internal(writer, request)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"status": status, "passkeys": passkeys})
}

func (h *handler) mfaTotpEnroll(writer http.ResponseWriter, request *http.Request) {
	session, ok := h.sessionCookie(writer, request, true)
	if !ok {
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	enrollment, err := h.identity.BeginTOTPEnrollment(request.Context(), session.UserID, session.ID, input.Password, h.auditContext(request))
	if err != nil {
		h.mfaProblem(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusOK, enrollment)
}

func (h *handler) mfaTotpComplete(writer http.ResponseWriter, request *http.Request) {
	session, ok := h.sessionCookie(writer, request, true)
	if !ok {
		return
	}
	var input struct{ Challenge, Code string }
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	codes, err := h.identity.CompleteTOTPEnrollment(request.Context(), session.ID, input.Challenge, input.Code, h.auditContext(request))
	if err != nil {
		h.mfaProblem(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusOK, map[string]any{"recoveryCodes": codes})
}

func (h *handler) mfaTotpRemove(writer http.ResponseWriter, request *http.Request) {
	session, ok := h.sessionCookie(writer, request, true)
	if !ok {
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	if err := h.identity.RemoveTOTP(request.Context(), session.UserID, session.ID, input.Password, h.auditContext(request)); err != nil {
		h.mfaProblem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) mfaPasskeyOptions(writer http.ResponseWriter, request *http.Request) {
	session, ok := h.sessionCookie(writer, request, true)
	if !ok {
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	enrollment, err := h.identity.BeginPasskeyEnrollment(request.Context(), session.UserID, session.ID, input.Password, h.auditContext(request))
	if err != nil {
		h.mfaProblem(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusOK, enrollment)
}

func (h *handler) mfaPasskeyComplete(writer http.ResponseWriter, request *http.Request) {
	session, ok := h.sessionCookie(writer, request, true)
	if !ok {
		return
	}
	challenge := request.Header.Get("X-MFA-Challenge")
	if challenge == "" {
		h.invalidRequest(writer, request, "The MFA challenge is invalid.")
		return
	}
	name := request.Header.Get("X-MFA-Name")
	codes, err := h.identity.CompletePasskeyEnrollment(request.Context(), session.UserID, session.ID, challenge, name, request, h.auditContext(request))
	if err != nil {
		h.mfaProblem(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusOK, map[string]any{"recoveryCodes": codes})
}

func (h *handler) mfaPasskeyRemove(writer http.ResponseWriter, request *http.Request) {
	session, ok := h.sessionCookie(writer, request, true)
	if !ok {
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	if err := h.identity.RemovePasskey(request.Context(), session.UserID, session.ID, request.PathValue("passkeyId"), input.Password, h.auditContext(request)); err != nil {
		h.mfaProblem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) sessionCookie(writer http.ResponseWriter, request *http.Request, csrf bool) (identity.Session, bool) {
	cookie, err := request.Cookie("session")
	if err != nil {
		h.unauthorized(writer, request)
		return identity.Session{}, false
	}
	session, err := h.identity.AuthenticateSession(request.Context(), cookie.Value)
	if err != nil {
		h.unauthorized(writer, request)
		return identity.Session{}, false
	}
	if csrf && !h.identity.VerifyCSRF(request.Context(), session.ID, request.Header.Get("X-CSRF-Token")) {
		writeProblem(writer, request, http.StatusForbidden, "csrf-invalid", "The CSRF token is invalid.")
		return identity.Session{}, false
	}
	return session, true
}

func (h *handler) writeAuthenticatedSession(writer http.ResponseWriter, request *http.Request, session identity.NewSession) {
	policy := identity.BrowserCookiePolicy(h.cfg.Secure)
	maxAge := int(session.ExpiresAt.Sub(time.Now()).Seconds())
	for _, cookie := range []*http.Cookie{
		{Name: "session", Value: session.Secret, Path: "/", Expires: session.ExpiresAt, MaxAge: maxAge, HttpOnly: policy.HTTPOnly, Secure: policy.Secure, SameSite: policy.SameSite},
		{Name: "csrf", Value: session.CSRFSecret, Path: "/", Expires: session.ExpiresAt, MaxAge: maxAge, HttpOnly: true, Secure: policy.Secure, SameSite: policy.SameSite},
	} {
		http.SetCookie(writer, cookie)
	}
	writeJSON(writer, http.StatusOK, map[string]any{"csrfToken": session.CSRFSecret, "expiresAt": session.ExpiresAt})
}

func (h *handler) mfaProblem(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrMFAUnavailable):
		writeProblem(writer, request, http.StatusNotFound, "mfa-unavailable", "Multi-factor authentication is unavailable.")
	case errors.Is(err, identity.ErrMFARequired), errors.Is(err, identity.ErrInvalidMFAChallenge), errors.Is(err, identity.ErrInvalidMFACode), errors.Is(err, identity.ErrLastMFAFactor):
		writeProblem(writer, request, http.StatusUnauthorized, "mfa-invalid", "The multi-factor authentication request is invalid.")
	case errors.Is(err, identity.ErrInvalidCredentials):
		writeProblem(writer, request, http.StatusUnauthorized, "invalid-credentials", "Reauthentication failed.")
	default:
		h.internal(writer, request)
	}
}
