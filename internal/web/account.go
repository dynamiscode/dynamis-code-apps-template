package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func (h *Handler) accountPage(writer http.ResponseWriter, request *http.Request) {
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return
	}
	data, ok := h.accountData(request, session.UserID, csrf)
	if !ok {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	data.CurrentAuthMethod = session.AuthMethod
	data.Saved = request.URL.Query().Get("saved") == "1"
	h.render(writer, http.StatusOK, "account.html", data)
}

func (h *Handler) accountData(request *http.Request, userID, csrf string) (pageData, bool) {
	profile, err := h.identity.GetUserProfile(request.Context(), userID)
	if err != nil {
		return pageData{}, false
	}
	preferences, err := h.identity.GetNotificationPreferences(request.Context(), userID, "")
	if err != nil {
		return pageData{}, false
	}
	return pageData{
		Title: "Account", NavPage: "account", CSRF: csrf, Profile: profile,
		NotificationPreferences: preferences,
	}, true
}

func (h *Handler) accountProfileMutation(writer http.ResponseWriter, request *http.Request) {
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
	err := h.identity.UpdateUserProfile(request.Context(), session.UserID, identity.ProfileUpdateInput{
		DisplayName: request.FormValue("display_name"), Locale: request.FormValue("locale"),
		Timezone: request.FormValue("timezone"), Theme: request.FormValue("theme"),
	}, auditContext(request))
	if err != nil {
		data, loaded := h.accountData(request, session.UserID, csrf)
		if !loaded {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		data.Error = "The account settings are invalid."
		h.render(writer, http.StatusUnprocessableEntity, "account.html", data)
		return
	}
	h.redirect(writer, request, "/account?saved=1")
}

func (h *Handler) accountNotificationMutation(writer http.ResponseWriter, request *http.Request) {
	session, csrf, ok := h.validCSRFSessionWithCSRF(writer, request)
	if !ok {
		return
	}
	if err := h.identity.SetNotificationPreference(request.Context(), session.UserID,
		request.FormValue("notification_type"), request.FormValue("enabled") == "true", auditContext(request)); err != nil {
		data, loaded := h.accountData(request, session.UserID, csrf)
		if !loaded {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		data.CurrentAuthMethod = session.AuthMethod
		data.Error = "The notification preference is invalid."
		h.render(writer, http.StatusUnprocessableEntity, "account.html", data)
		return
	}
	h.redirect(writer, request, "/account?saved=1")
}

func (h *Handler) accountPasswordMutation(writer http.ResponseWriter, request *http.Request) {
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
	if request.FormValue("new_password") != request.FormValue("password_confirmation") {
		data, loaded := h.accountData(request, session.UserID, csrf)
		if !loaded {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		data.Error = "The passwords do not match."
		h.render(writer, http.StatusUnprocessableEntity, "account.html", data)
		return
	}
	if err := h.identity.ChangePassword(request.Context(), session.UserID,
		request.FormValue("current_password"), request.FormValue("new_password"), auditContext(request)); err != nil {
		data, loaded := h.accountData(request, session.UserID, csrf)
		if !loaded {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		data.Error = "The current password or new password is invalid."
		h.render(writer, http.StatusUnprocessableEntity, "account.html", data)
		return
	}
	newSession, err := h.identity.CreateSession(request.Context(), session.UserID, "local", "", 0, auditContext(request))
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.setCookie(writer, "session", newSession.Secret, newSession.ExpiresAt, true)
	h.setCookie(writer, "csrf", newSession.CSRFSecret, newSession.ExpiresAt, true)
	h.redirect(writer, request, "/account?saved=1")
}

func (h *Handler) accountDeleteMutation(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	session, ok := h.validCSRFSession(writer, request)
	if !ok {
		return
	}
	err := h.identity.DeleteAccount(request.Context(), session.UserID, request.FormValue("password"), auditContext(request))
	if err != nil {
		data, loaded := h.accountData(request, session.UserID, cookieValue(request, "csrf"))
		if !loaded {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		if errors.Is(err, identity.ErrOwnedWorkspace) {
			data.Error = "Transfer workspace ownership before deleting this account."
		} else {
			data.Error = "Account deletion requires the current password."
		}
		h.render(writer, http.StatusUnprocessableEntity, "account.html", data)
		return
	}
	h.clearCookie(writer, "session")
	h.clearCookie(writer, "csrf")
	h.redirect(writer, request, "/login")
}

func (h *Handler) emailVerificationMutation(writer http.ResponseWriter, request *http.Request) {
	session, csrf, ok := h.validCSRFSessionWithCSRF(writer, request)
	if !ok {
		return
	}
	verification, err := h.identity.BeginEmailVerification(request.Context(), session.UserID, auditContext(request))
	if errors.Is(err, identity.ErrEmailAlreadyVerified) {
		h.redirect(writer, request, "/account?saved=1")
		return
	}
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	link := accountURL(h.publicURL, "/account/verify-email/", verification.Secret)
	if h.mailer == nil {
		data, loaded := h.accountData(request, session.UserID, csrf)
		if !loaded {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		data.DeliveryWarning = "Email delivery is not configured."
		h.render(writer, http.StatusServiceUnavailable, "account.html", data)
		return
	}
	locale := h.accountLocale(request, session.UserID)
	if err := h.mailer.Send(request.Context(), verification.Email,
		h.catalog.Translate(locale, "email.verification_subject", nil),
		h.catalog.Translate(locale, "email.verification_body", map[string]any{"link": link})); err != nil {
		data, loaded := h.accountData(request, session.UserID, csrf)
		if !loaded {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		data.DeliveryWarning = "Verification email could not be delivered. Try again later."
		h.render(writer, http.StatusServiceUnavailable, "account.html", data)
		return
	}
	h.redirect(writer, request, "/account?saved=1")
}

func (h *Handler) emailVerification(writer http.ResponseWriter, request *http.Request) {
	err := h.identity.VerifyEmail(request.Context(), request.PathValue("secret"), auditContext(request))
	data := pageData{Title: "Email verification"}
	if err != nil {
		data.Error = "The email verification link is invalid or expired."
		h.render(writer, http.StatusUnprocessableEntity, "verification.html", data)
		return
	}
	data.Saved = true
	h.render(writer, http.StatusOK, "verification.html", data)
}

func (h *Handler) passwordResetPage(writer http.ResponseWriter, request *http.Request) {
	token, err := id.New()
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.setCookie(writer, "reset_csrf", token, time.Now().Add(10*time.Minute), true)
	h.render(writer, http.StatusOK, "password-reset.html", pageData{Title: "Reset password", CSRF: token})
}

func (h *Handler) passwordResetRequest(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil || !validResetCSRF(request) {
		h.render(writer, http.StatusForbidden, "password-reset.html", pageData{Title: "Reset password", Error: "The password reset form expired. Reload and try again."})
		return
	}
	reset, err := h.identity.BeginPasswordReset(request.Context(), request.FormValue("email"), auditContext(request))
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	if reset.Secret != "" && h.mailer != nil {
		link := accountURL(h.publicURL, "/password-reset/", reset.Secret)
		locale := h.pageLocale(writer, pageData{})
		_ = h.mailer.Send(request.Context(), reset.Email,
			h.catalog.Translate(locale, "email.password_reset_subject", nil),
			h.catalog.Translate(locale, "email.password_reset_body", map[string]any{"link": link}))
	}
	h.clearCookie(writer, "reset_csrf")
	h.render(writer, http.StatusOK, "password-reset.html", pageData{Title: "Reset password", Saved: true})
}

func (h *Handler) passwordResetTokenPage(writer http.ResponseWriter, request *http.Request) {
	token, err := id.New()
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.setCookie(writer, "reset_csrf", token, time.Now().Add(10*time.Minute), true)
	h.render(writer, http.StatusOK, "password-reset.html", pageData{
		Title: "Choose a new password", CSRF: token, PasswordResetSecret: request.PathValue("secret"),
	})
}

func (h *Handler) passwordResetComplete(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil || !validResetCSRF(request) {
		h.render(writer, http.StatusForbidden, "password-reset.html", pageData{Title: "Choose a new password", Error: "The password reset form expired. Reload and try again."})
		return
	}
	if request.FormValue("password") != request.FormValue("password_confirmation") {
		h.render(writer, http.StatusUnprocessableEntity, "password-reset.html", pageData{
			Title: "Choose a new password", Error: "The passwords do not match.", PasswordResetSecret: request.PathValue("secret"),
		})
		return
	}
	if err := h.identity.CompletePasswordReset(request.Context(), request.PathValue("secret"), request.FormValue("password"), auditContext(request)); err != nil {
		h.render(writer, http.StatusUnprocessableEntity, "password-reset.html", pageData{Title: "Choose a new password", Error: "The password reset link is invalid or expired."})
		return
	}
	h.clearCookie(writer, "reset_csrf")
	h.clearCookie(writer, "session")
	h.clearCookie(writer, "csrf")
	h.render(writer, http.StatusOK, "password-reset.html", pageData{Title: "Password reset", Saved: true, PasswordResetCompleted: true})
}

func (h *Handler) validCSRFSessionWithCSRF(writer http.ResponseWriter, request *http.Request) (identity.Session, string, bool) {
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return identity.Session{}, "", false
	}
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return identity.Session{}, "", false
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return identity.Session{}, "", false
	}
	return session, csrf, true
}

func validResetCSRF(request *http.Request) bool {
	return sameValue(cookieValue(request, "reset_csrf"), request.FormValue("csrf"))
}

func accountURL(publicURL, prefix, secret string) string {
	path := prefix + url.PathEscape(secret)
	if strings.TrimSpace(publicURL) == "" {
		return path
	}
	return strings.TrimRight(publicURL, "/") + path
}

func (h *Handler) accountLocale(request *http.Request, userID string) i18n.Locale {
	profile, err := h.identity.GetUserProfile(request.Context(), userID)
	if err == nil {
		if locale, ok := i18n.ParseLocale(profile.Locale); ok {
			return locale
		}
	}
	return i18n.English
}
