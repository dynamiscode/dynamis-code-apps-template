package web

import (
	"errors"
	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/http"
	"net/url"
	"strings"
)

func (h *Handler) accountPage(request *http.Request) authPageOutput {
	session, csrf, ok := h.checkSession(request)
	if !ok {
		return authSessionFailure(http.StatusSeeOther)
	}
	data, ok := h.accountData(request, session.UserID, csrf)
	if !ok {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	data.CurrentAuthMethod = session.AuthMethod
	data.Saved = request.URL.Query().Get("saved") == "1"
	return authPageOutput{kind: authPageAccount, status: http.StatusOK, data: data}
}

func (h *Handler) accountData(request *http.Request, userID identity.UserID, csrf string) (viewmodel.PageData, bool) {
	profile, err := h.identity.GetUserProfile(request.Context(), userID)
	if err != nil {
		return viewmodel.PageData{}, false
	}
	preferences, err := h.identity.GetNotificationPreferences(request.Context(), userID, 0)
	if err != nil {
		return viewmodel.PageData{}, false
	}
	return viewmodel.PageData{
		Title: "Account", NavPage: "account", CSRF: csrf, Profile: profile,
		NotificationPreferences: preferences,
		DefaultWorkspaceHref:    h.defaultHomeHref(request.Context(), userID),
	}, true
}

func (h *Handler) accountProfileMutation(request *http.Request) authPageOutput {
	if err := request.ParseForm(); err != nil {
		return authPageOutput{status: http.StatusBadRequest}
	}
	session, csrf, ok := h.checkSession(request)
	if !ok {
		return authSessionFailure(http.StatusSeeOther)
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		return authPageOutput{status: http.StatusForbidden}
	}
	err := h.identity.UpdateUserProfile(request.Context(), session.UserID, identity.ProfileUpdateInput{
		DisplayName: request.FormValue("display_name"), Locale: request.FormValue("locale"),
		Timezone: request.FormValue("timezone"), Theme: request.FormValue("theme"),
	}, auditContext(request))
	if err != nil {
		data, loaded := h.accountData(request, session.UserID, csrf)
		if !loaded {
			return authPageOutput{status: http.StatusInternalServerError}
		}
		data.Error = "The account settings are invalid."
		return authPageOutput{kind: authPageAccount, status: http.StatusUnprocessableEntity, data: data}
	}
	return authPageOutput{kind: authPageThemeChanged, theme: strings.TrimSpace(request.FormValue("theme")), target: "/account?saved=1"}
}

func (h *Handler) accountNotificationMutation(request *http.Request) authPageOutput {
	session, status := h.checkCSRFSession(request)
	if status != http.StatusOK {
		return authSessionFailure(status)
	}
	csrf := cookieValue(request, "csrf")
	if err := h.identity.SetNotificationPreference(request.Context(), session.UserID,
		request.FormValue("notification_type"), request.FormValue("enabled") == "true", auditContext(request)); err != nil {
		data, loaded := h.accountData(request, session.UserID, csrf)
		if !loaded {
			return authPageOutput{status: http.StatusInternalServerError}
		}
		data.CurrentAuthMethod = session.AuthMethod
		data.Error = "The notification preference is invalid."
		return authPageOutput{kind: authPageAccount, status: http.StatusUnprocessableEntity, data: data}
	}
	return authPageOutput{kind: authPageRedirect, target: "/account?saved=1"}
}

func (h *Handler) accountPasswordMutation(request *http.Request) authPageOutput {
	if err := request.ParseForm(); err != nil {
		return authPageOutput{status: http.StatusBadRequest}
	}
	session, csrf, ok := h.checkSession(request)
	if !ok {
		return authSessionFailure(http.StatusSeeOther)
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		return authPageOutput{status: http.StatusForbidden}
	}
	if request.FormValue("new_password") != request.FormValue("password_confirmation") {
		data, loaded := h.accountData(request, session.UserID, csrf)
		if !loaded {
			return authPageOutput{status: http.StatusInternalServerError}
		}
		data.CurrentAuthMethod = session.AuthMethod
		data.ErrorKey = "account.password_mismatch"
		return authPageOutput{kind: authPageAccount, status: http.StatusUnprocessableEntity, data: data}
	}
	if err := h.identity.ChangePassword(request.Context(), session.UserID,
		request.FormValue("current_password"), request.FormValue("new_password"), auditContext(request)); err != nil {
		data, loaded := h.accountData(request, session.UserID, csrf)
		if !loaded {
			return authPageOutput{status: http.StatusInternalServerError}
		}
		data.CurrentAuthMethod = session.AuthMethod
		data.ErrorKey = "account.password_invalid"
		return authPageOutput{kind: authPageAccount, status: http.StatusUnprocessableEntity, data: data}
	}
	if enrolled, err := h.identity.MFAEnrolled(request.Context(), session.UserID); err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	} else if enrolled {
		challenge, err := h.identity.BeginMFALogin(request.Context(), session.UserID, auditContext(request))
		if err != nil {
			return authPageOutput{status: http.StatusInternalServerError}
		}
		return authPageOutput{kind: authPageMFALogin, challenge: challenge, target: "/account?saved=1"}
	}
	newSession, err := h.identity.CreateSession(request.Context(), session.UserID, "local", "", 0, auditContext(request))
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	return authPageOutput{kind: authPageAccountSession, session: newSession}
}

func (h *Handler) accountDeleteMutation(request *http.Request) authPageOutput {
	if err := request.ParseForm(); err != nil {
		return authPageOutput{status: http.StatusBadRequest}
	}
	session, status := h.checkCSRFSession(request)
	if status != http.StatusOK {
		return authSessionFailure(status)
	}
	if request.FormValue("confirm_delete") != "yes" {
		data, loaded := h.accountData(request, session.UserID, cookieValue(request, "csrf"))
		if !loaded {
			return authPageOutput{status: http.StatusInternalServerError}
		}
		data.CurrentAuthMethod = session.AuthMethod
		data.ErrorKey = "account.delete_confirmation_required"
		return authPageOutput{kind: authPageAccount, status: http.StatusUnprocessableEntity, data: data}
	}
	err := h.identity.DeleteAccount(request.Context(), session.UserID, request.FormValue("password"), auditContext(request))
	if err != nil {
		data, loaded := h.accountData(request, session.UserID, cookieValue(request, "csrf"))
		if !loaded {
			return authPageOutput{status: http.StatusInternalServerError}
		}
		data.CurrentAuthMethod = session.AuthMethod
		if errors.Is(err, identity.ErrOwnedWorkspace) {
			data.ErrorKey = "account.transfer_ownership_required"
		} else {
			data.ErrorKey = "account.delete_password_required"
		}
		return authPageOutput{kind: authPageAccount, status: http.StatusUnprocessableEntity, data: data}
	}
	return authPageOutput{kind: authPageAccountDeleted}
}

func (h *Handler) emailVerificationMutation(request *http.Request) authPageOutput {
	session, status := h.checkCSRFSession(request)
	if status != http.StatusOK {
		return authSessionFailure(status)
	}
	csrf := cookieValue(request, "csrf")
	verification, err := h.identity.BeginEmailVerification(request.Context(), session.UserID, auditContext(request))
	if errors.Is(err, identity.ErrEmailAlreadyVerified) {
		return authPageOutput{kind: authPageRedirect, target: "/account?saved=1"}
	}
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	link := accountURL(h.publicURL, "/account/verify-email/", verification.Secret)
	if !appmail.Enabled(h.mailer) {
		data, loaded := h.accountData(request, session.UserID, csrf)
		if !loaded {
			return authPageOutput{status: http.StatusInternalServerError}
		}
		data.DeliveryWarning = "Email delivery is not configured."
		return authPageOutput{kind: authPageAccount, status: http.StatusServiceUnavailable, data: data}
	}
	locale := h.accountLocale(request, session.UserID)
	if err := h.mailer.Send(request.Context(), verification.Email,
		h.catalog.Translate(locale, "email.verification_subject", nil),
		h.catalog.Translate(locale, "email.verification_body", map[string]any{"link": link})); err != nil {
		data, loaded := h.accountData(request, session.UserID, csrf)
		if !loaded {
			return authPageOutput{status: http.StatusInternalServerError}
		}
		data.DeliveryWarning = "Verification email could not be delivered. Try again later."
		return authPageOutput{kind: authPageAccount, status: http.StatusServiceUnavailable, data: data}
	}
	return authPageOutput{kind: authPageRedirect, target: "/account?saved=1"}
}

func (h *Handler) emailVerification(request *http.Request) authPageOutput {
	err := h.identity.VerifyEmail(request.Context(), request.PathValue("secret"), auditContext(request))
	data := viewmodel.PageData{Title: "Email verification"}
	if err != nil {
		data.Error = "The email verification link is invalid or expired."
		return authPageOutput{kind: authPageVerification, status: http.StatusUnprocessableEntity, data: data}
	}
	data.Saved = true
	return authPageOutput{kind: authPageVerification, status: http.StatusOK, data: data}
}

func (h *Handler) passwordResetPage(request *http.Request) authPageOutput {
	token, err := id.New()
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	return resetPageOutput(http.StatusOK, viewmodel.PageData{Title: "Reset password", CSRF: token}, resetCookieRefresh)
}

func (h *Handler) passwordResetRequest(request *http.Request) authPageOutput {
	if err := request.ParseForm(); err != nil || !validResetCSRF(request) {
		return resetPageOutput(http.StatusForbidden, viewmodel.PageData{Title: "Reset password", ErrorKey: "account.password_reset_form_expired"}, resetCookieUnchanged)
	}
	reset, err := h.identity.BeginPasswordReset(request.Context(), request.FormValue("email"), auditContext(request))
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	if reset.Secret != "" && appmail.Enabled(h.mailer) {
		link := accountURL(h.publicURL, "/password-reset/", reset.Secret)
		locale := h.resolvePageLocale(request.Context(), viewmodel.PageData{})
		_ = h.mailer.Send(request.Context(), reset.Email,
			h.catalog.Translate(locale, "email.password_reset_subject", nil),
			h.catalog.Translate(locale, "email.password_reset_body", map[string]any{"link": link}))
	}
	return resetPageOutput(http.StatusOK, viewmodel.PageData{Title: "Reset password", Saved: true}, resetCookieClear)
}

func (h *Handler) passwordResetTokenPage(request *http.Request) authPageOutput {
	token, err := id.New()
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	return resetPageOutput(http.StatusOK, viewmodel.PageData{Title: "Choose a new password", CSRF: token, PasswordResetSecret: request.PathValue("secret")}, resetCookieRefresh)
}

func (h *Handler) passwordResetComplete(request *http.Request) authPageOutput {
	if err := request.ParseForm(); err != nil || !validResetCSRF(request) {
		return resetPageOutput(http.StatusForbidden, viewmodel.PageData{Title: "Choose a new password", ErrorKey: "account.password_reset_form_expired"}, resetCookieUnchanged)
	}
	if request.FormValue("password") != request.FormValue("password_confirmation") {
		return resetPageOutput(http.StatusUnprocessableEntity, viewmodel.PageData{
			Title: "Choose a new password", ErrorKey: "account.password_reset_mismatch",
			CSRF: request.FormValue("csrf"), PasswordResetSecret: request.PathValue("secret"),
		}, resetCookieUnchanged)
	}
	if err := h.identity.CompletePasswordReset(request.Context(), request.PathValue("secret"), request.FormValue("password"), auditContext(request)); err != nil {
		csrf, tokenErr := id.New()
		if tokenErr != nil {
			return authPageOutput{status: http.StatusInternalServerError}
		}
		return resetPageOutput(http.StatusUnprocessableEntity, viewmodel.PageData{
			Title: "Reset password", ErrorKey: "account.password_reset_invalid", CSRF: csrf,
		}, resetCookieRefresh)
	}
	return resetPageOutput(http.StatusOK, viewmodel.PageData{Title: "Password reset", Saved: true, PasswordResetCompleted: true}, resetCookieClearSession)
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

func (h *Handler) accountLocale(request *http.Request, userID identity.UserID) i18n.Locale {
	profile, err := h.identity.GetUserProfile(request.Context(), userID)
	if err == nil {
		if locale, ok := i18n.ParseLocale(profile.Locale); ok {
			return locale
		}
	}
	return i18n.English
}
