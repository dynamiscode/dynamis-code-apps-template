package web

import (
	"context"
	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/http"
	"strings"
	"time"
)

type localeContextKey struct{}

type localeState struct {
	userLocale     string
	cookieLocale   string
	acceptLanguage string
	userTheme      string
	cookieTheme    string
	userName       string
	userEmail      string
	returnPath     string
}

type localeWriter struct {
	http.ResponseWriter
	context             context.Context
	engagementWorkspace identity.WorkspaceID
	engagementLabel     string
	engagementLoaded    bool
}

func (w *localeWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (h *Handler) localeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		userLocale, userTheme, userName, userEmail := "", "", "", ""
		if session, err := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session")); err == nil {
			if profile, err := h.identity.GetUserProfile(request.Context(), session.UserID); err == nil {
				userLocale, userTheme = profile.Locale, profile.Theme
				userName, userEmail = profile.DisplayName, profile.Email
			}
		}
		request = request.WithContext(context.WithValue(request.Context(), localeContextKey{}, localeState{
			userLocale: userLocale, cookieLocale: cookieValue(request, "locale"),
			acceptLanguage: request.Header.Get("Accept-Language"),
			userTheme:      userTheme, cookieTheme: cookieValue(request, themeCookie),
			userName: userName, userEmail: userEmail, returnPath: request.URL.Path,
		}))
		next.ServeHTTP(&localeWriter{ResponseWriter: writer, context: request.Context()}, request)
	})
}

func (h *Handler) pageLocale(writer http.ResponseWriter, data viewmodel.PageData) i18n.Locale {
	state, _ := writer.(*localeWriter)
	ctx := context.Background()
	if state != nil {
		ctx = state.context
	}
	locale := h.resolvePageLocale(ctx, data)
	writer.Header().Set("Content-Language", string(locale))
	return locale
}

func (h *Handler) resolvePageLocale(ctx context.Context, data viewmodel.PageData) i18n.Locale {
	userLocale, cookieLocale, acceptLanguage := "", "", ""
	if values, ok := ctx.Value(localeContextKey{}).(localeState); ok {
		userLocale, cookieLocale, acceptLanguage = values.userLocale, values.cookieLocale, values.acceptLanguage
	}
	workspaceLocale := data.Workspace.Locale
	if workspaceLocale == "" {
		workspaceLocale = data.Invitation.WorkspaceLocale
	}
	if data.Invitation.ID != 0 {
		userLocale = ""
	}
	return h.catalog.Resolve(userLocale, cookieLocale, workspaceLocale, acceptLanguage)
}

func (h *Handler) localizePage(writer http.ResponseWriter, data *viewmodel.PageData) {
	locale := h.pageLocale(writer, *data)
	data.Locale = string(locale)
	data.Theme = h.pageTheme(writer)
	data.Catalog = h.catalog
	data.CatalogEnabled = h.catalogService != nil
	data.TransactionsEnabled = h.transactions != nil
	data.EngagementEnabled = h.engagement != nil
	if state, ok := writer.(*localeWriter); ok {
		if state.engagementLoaded && state.engagementWorkspace == data.Workspace.ID {
			data.EngagementLabel = state.engagementLabel
		}
		if values, ok := state.context.Value(localeContextKey{}).(localeState); ok {
			data.ReturnPath = values.returnPath
			if data.Profile.Email == "" && values.userEmail != "" {
				data.Profile = identity.UserProfile{DisplayName: values.userName, Email: values.userEmail}
			}
		}
	}
	data.Title = localizedLegacy(h.catalog, locale, data.Title)
	data.Error = localizedLegacy(h.catalog, locale, data.Error)
	data.MFAError = localizedLegacy(h.catalog, locale, data.MFAError)
	data.DeliveryWarning = localizedLegacy(h.catalog, locale, data.DeliveryWarning)
}

func localizedLegacy(catalog *i18n.Catalog, locale i18n.Locale, value string) string {
	keys := map[string]string{
		"Sign in": "account.sign_in", "Set up Dynamis Code": "setup.title",
		"Workspaces":     "workspace.workspaces",
		"Workspace home": "workspace.home", "Members": "members.members",
		"Invitations": "invitations.invitations", "API tokens": "tokens.tokens",
		"Sessions": "sessions.sessions", "Security": "security.security", "Multi-factor authentication": "security.mfa_title", "Export": "navigation.export", "Audit history": "audit.history",
		"Account": "account.account", "Notifications": "notifications.notifications",
		"Reset password": "account.reset_password", "Choose a new password": "account.new_password",
		"Password reset": "account.reset_password", "Email verification": "account.email_verification",
		"Language settings": "account.language_settings_title", "General workspace settings": "workspace.general_settings", "SCIM provisioning": "scim.title",
		"Workspace invitation": "invitations.workspace_invitation",
		"The identity provider sign-in could not be completed.": "errors.oidc_signin",
		"Invitation unavailable":                                "errors.invalid_invitation", "Request failed": "errors.request_failed",
		"The request could not be completed.":             "errors.generic",
		"The sign-in form expired. Reload and try again.": "errors.signin_expired",
		"The setup form expired. Reload and try again.":   "errors.setup_expired",
		"The setup token is invalid.":                     "errors.setup_token", "The passwords do not match.": "errors.password_mismatch",
		"Enter a valid email, workspace name, and password.": "errors.invalid_bootstrap",
		"The email or password is invalid.":                  "errors.invalid_credentials",
		"The member role is invalid.":                        "errors.invalid_role", "The member action could not be completed.": "errors.member_action",
		"The final owner cannot be changed.": "errors.final_owner", "The invitation lifetime is invalid.": "errors.invalid_lifetime",
		"The invitation could not be created.": "errors.invitation_create", "The invitation could not be resent.": "errors.invitation_resend",
		"The invitation could not be revoked.": "errors.invitation_revoke", "The invitation could not be found.": "errors.invitation_not_found",
		"The requested invitation action is invalid.":                                  "errors.invalid_invitation_action",
		"Invitation email could not be delivered. Share the invitation link manually.": "errors.delivery",
		"The token expiration is invalid.":                                             "errors.invalid_token_expiration", "The token could not be created.": "errors.token_create",
		"The token could not be revoked.": "errors.token_revoke", "The requested token action is invalid.": "errors.invalid_token_action",
		"The webhook input is invalid.": "errors.webhook_invalid", "The webhook could not be created.": "errors.webhook_create",
		"The workspace webhook limit was reached. Delete one before retrying.":                     "errors.webhook_limit",
		"Webhook secret encryption is not configured. Set ENCRYPTION_KEY and restart the server.":  "errors.webhook_secret_key",
		"SMTP login test failed. Check the host, port, username, and password. Nothing was saved.": "errors.email_login",
		"Secret encryption is not configured. Set ENCRYPTION_KEY and restart the server.":          "errors.email_secret_key",
		"The email settings are invalid.": "errors.email_invalid", "Enter the password or API key again when the provider, server, port, or username changes.": "errors.email_password_required", "The email settings could not be saved.": "errors.email_save", "Postmark check failed. Check the server and account tokens, then save again. Nothing was saved.": "errors.email_postmark_check", "Enter the Postmark account token on the first save. Nothing was saved.": "errors.email_account_token_required",
		"Resend check failed. Check the API key, then save again. Nothing was saved.": "errors.email_resend_check",
		"The webhook could not be rotated.":                                           "errors.webhook_rotate", "The webhook could not be deleted.": "errors.webhook_delete",
		"The webhook could not be found.": "errors.webhook_not_found", "The requested webhook action is invalid.": "errors.invalid_webhook_action",
		"The SCIM credential could not be created.": "errors.scim_create", "The SCIM credential could not be revoked.": "errors.scim_revoke", "The requested SCIM action is invalid.": "errors.invalid_scim_action",
		"No OIDC provider is configured.": "errors.no_oidc", "Reauthentication failed.": "errors.reauthentication",
		"The OIDC link could not be started.":                         "errors.oidc_link",
		"Enter a workspace name between 1 and 120 characters.":        "errors.workspace_name",
		"The selected language is invalid.":                           "errors.invalid_locale",
		"The account settings are invalid.":                           "errors.account_settings",
		"Confirm permanent account deletion to continue.":             "account.delete_confirmation_required",
		"Transfer workspace ownership before deleting this account.":  "account.transfer_ownership_required",
		"Account deletion requires the current password.":             "account.delete_password_required",
		"The email verification link is invalid or expired.":          "errors.email_verification",
		"The password reset link is invalid or expired.":              "errors.password_reset",
		"The current password or new password is invalid.":            "errors.password_change",
		"Email delivery is not configured.":                           "errors.email_delivery",
		"Verification email could not be delivered. Try again later.": "errors.verification_delivery",
		"The notification preference is invalid.":                     "errors.notification_preference",
		"The password reset form expired. Reload and try again.":      "errors.password_reset_expired",
		"Not Found": "errors.not_found", "Unauthorized": "errors.unauthorized",
		"Forbidden": "errors.forbidden", "Too Many Requests": "errors.too_many_requests",
		"The invitation is invalid or the password could not be accepted.":                         "errors.invitation_password",
		"This invitation is invalid, expired, revoked, already used, or belongs to another email.": "errors.invalid_invitation_email",
		"The verification code is invalid.":                                                        "errors.mfa_verification",
		"The recovery code is invalid.":                                                            "errors.mfa_recovery",
		"The passkey verification failed.":                                                         "errors.mfa_passkey",
		"Fresh authentication failed.":                                                             "errors.mfa_fresh",
		"The authenticator could not be removed.":                                                  "errors.mfa_remove_authenticator",
		"The passkey could not be removed.":                                                        "errors.mfa_remove_passkey",
	}
	key := keys[value]
	if key == "" {
		return value
	}
	return catalog.Translate(locale, key, nil)
}

func (h *Handler) language(writer http.ResponseWriter, request *http.Request) {
	locale, ok := i18n.ParseLocale(request.URL.Query().Get("locale"))
	if ok {
		h.setCookie(writer, "locale", string(locale), time.Now().Add(365*24*time.Hour), true)
	}
	h.redirect(writer, request, safeReturnTo(request.URL.Query().Get("return_to")))
}

func (h *Handler) languageSettingsPage(request *http.Request) authPageOutput {
	session, csrf, ok := h.checkSession(request)
	if !ok {
		return authSessionFailure(http.StatusSeeOther)
	}
	locale, err := h.identity.GetUserLocale(request.Context(), session.UserID)
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	data := h.languageSettingsData(request, session.UserID, csrf, locale)
	data.Saved = request.URL.Query().Get("saved") == "1"
	return authPageOutput{kind: authPageLanguage, status: http.StatusOK, data: data}
}

func (h *Handler) languageSettingsData(request *http.Request, userID identity.UserID, csrf, locale string) viewmodel.PageData {
	return viewmodel.PageData{
		Title: "Language settings", NavPage: "language", CSRF: csrf, UserLocale: locale,
		DefaultWorkspaceHref: h.defaultHomeHref(request.Context(), userID),
	}
}

func (h *Handler) languageSettings(request *http.Request) authPageOutput {
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
	locale := strings.TrimSpace(request.FormValue("locale"))
	if locale != "" {
		if _, ok := i18n.ParseLocale(locale); !ok {
			profile, err := h.identity.GetUserProfile(request.Context(), session.UserID)
			if err != nil {
				return authPageOutput{status: http.StatusInternalServerError}
			}
			data := h.languageSettingsData(request, session.UserID, csrf, profile.Locale)
			data.Theme = profile.Theme
			data.Error = "The selected language is invalid."
			return authPageOutput{kind: authPageLanguage, status: http.StatusUnprocessableEntity, data: data}
		}
	}
	if err := h.identity.SetUserLocale(request.Context(), session.UserID, locale, auditContext(request)); err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	if locale == "" {
		return authPageOutput{kind: authPageLocaleCleared}
	}
	return authPageOutput{kind: authPageRedirect, target: "/settings/language?saved=1"}
}

func (h *Handler) generalSettingsPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceRead)
	if !ok {
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	preferences, err := h.identity.GetNotificationPreferences(request.Context(), session.UserID, workspaceID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.renderSettingsPage(writer, request, http.StatusOK, viewmodel.PageData{
		Title: "General workspace settings", NavPage: "general", NavSection: "settings", CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		WorkspaceNotificationPreferences: preferences,
		CanManage:                        principal.Permissions[identity.WorkspaceUpdate], Saved: request.URL.Query().Get("saved") == "1",
		CatalogEnabled: h.catalogService != nil,
	}, generalSettingsView)
}

func (h *Handler) generalSettings(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, _, _, ok := h.managementPrincipal(writer, request, workspaceID, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	if err := h.identity.UpdateWorkspaceLocale(request.Context(), principal, request.FormValue("locale"), auditContext(request)); err != nil {
		h.managementError(writer, request, workspaceID, "The selected language is invalid.")
		return
	}
	h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/settings/general?saved=1")
}

func (h *Handler) workspaceNotificationSettings(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, _, _, ok := h.managementPrincipal(writer, request, workspaceID, identity.WorkspaceRead)
	if !ok {
		return
	}
	if err := h.identity.SetWorkspaceNotificationPreference(request.Context(), principal,
		request.FormValue("notification_type"), request.FormValue("enabled") == "true", auditContext(request)); err != nil {
		h.managementError(writer, request, workspaceID, "The notification preference is invalid.")
		return
	}
	h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/settings/general?saved=1")
}
