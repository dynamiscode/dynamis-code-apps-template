package web

import (
	"context"
	"net/http"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
)

type localeContextKey struct{}

type localeState struct {
	userLocale     string
	cookieLocale   string
	acceptLanguage string
}

type localeWriter struct {
	http.ResponseWriter
	context context.Context
}

func (w *localeWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (h *Handler) localeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		userLocale := ""
		if session, err := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session")); err == nil {
			userLocale, _ = h.identity.GetUserLocale(request.Context(), session.UserID)
		}
		request = request.WithContext(context.WithValue(request.Context(), localeContextKey{}, localeState{
			userLocale: userLocale, cookieLocale: cookieValue(request, "locale"),
			acceptLanguage: request.Header.Get("Accept-Language"),
		}))
		next.ServeHTTP(&localeWriter{ResponseWriter: writer, context: request.Context()}, request)
	})
}

func (h *Handler) pageLocale(writer http.ResponseWriter, data pageData) i18n.Locale {
	state, _ := writer.(*localeWriter)
	userLocale, cookieLocale, acceptLanguage := "", "", ""
	if state != nil {
		if values, ok := state.context.Value(localeContextKey{}).(localeState); ok {
			userLocale, cookieLocale, acceptLanguage = values.userLocale, values.cookieLocale, values.acceptLanguage
		}
	}
	workspaceLocale := data.Workspace.Locale
	if workspaceLocale == "" {
		workspaceLocale = data.Invitation.WorkspaceLocale
	}
	if data.Invitation.ID != "" {
		userLocale = ""
	}
	locale := h.catalog.Resolve(userLocale, cookieLocale, workspaceLocale, acceptLanguage)
	writer.Header().Set("Content-Language", string(locale))
	return locale
}

func (h *Handler) localizePage(writer http.ResponseWriter, data *pageData) {
	locale := h.pageLocale(writer, *data)
	data.Locale = string(locale)
	data.Catalog = h.catalog
	data.Title = localizedLegacy(h.catalog, locale, data.Title)
	data.Error = localizedLegacy(h.catalog, locale, data.Error)
	data.MFAError = localizedLegacy(h.catalog, locale, data.MFAError)
	data.DeliveryWarning = localizedLegacy(h.catalog, locale, data.DeliveryWarning)
}

func localizedLegacy(catalog *i18n.Catalog, locale i18n.Locale, value string) string {
	keys := map[string]string{
		"Sign in": "account.sign_in", "Set up Dynamis Code": "setup.title",
		"Remote setup required": "errors.remote_setup", "Workspaces": "workspace.workspaces",
		"Workspace home": "workspace.home", "Members": "members.members",
		"Invitations": "invitations.invitations", "API tokens": "tokens.tokens",
		"Sessions": "sessions.sessions", "Security": "security.security", "Multi-factor authentication": "security.mfa_title", "Export": "navigation.export", "Import": "navigation.import", "Audit history": "audit.history",
		"Account": "account.account", "Notifications": "notifications.notifications",
		"Reset password": "account.reset_password", "Choose a new password": "account.new_password",
		"Password reset": "account.reset_password", "Email verification": "account.email_verification",
		"Language settings": "account.language_settings_title", "General workspace settings": "workspace.general_settings", "SCIM provisioning": "scim.title",
		"Workspace invitation": "invitations.workspace_invitation",
		"Shared item":          "sharing.shared_item", "Shared item unavailable": "sharing.unavailable",
		"This shared item is unavailable.":                      "sharing.unavailable_detail",
		"The identity provider sign-in could not be completed.": "errors.oidc_signin",
		"Invitation unavailable":                                "errors.invalid_invitation", "Request failed": "errors.request_failed",
		"The request could not be completed.":                                   "errors.generic",
		"The import file is invalid.":                                           "errors.import_file",
		"The import file exceeds the configured limit or workspace item quota.": "errors.import_limit",
		"The import file exceeds the configured limit.":                         "errors.import_size",
		"Confirm that you understand this bulk import.":                         "errors.import_confirmation",
		"Choose a .json or .csv import file.":                                   "errors.import_format",
		"The sign-in form expired. Reload and try again.":                       "errors.signin_expired",
		"The setup form expired. Reload and try again.":                         "errors.setup_expired",
		"The setup token is invalid.":                                           "errors.setup_token", "The passwords do not match.": "errors.password_mismatch",
		"Enter a valid email, workspace name, and password.": "errors.invalid_bootstrap",
		"The email or password is invalid.":                  "errors.invalid_credentials",
		"Remote browser setup is disabled. Set BOOTSTRAP_SETUP_TOKEN or all BOOTSTRAP_ADMIN_* variables, then restart the server.": "errors.remote_setup",
		"The item version is invalid. Reload and try again.":                                                                       "errors.invalid_item_version",
		"The requested item action is invalid.":                                                                                    "errors.invalid_item_action",
		"This item changed. Review the current value and try again.":                                                               "errors.item_changed",
		"The item values are invalid.":                                                                                             "errors.invalid_item", "The workspace item limit was reached. Delete an item before retrying.": "errors.item_limit",
		"The member role is invalid.": "errors.invalid_role", "The member action could not be completed.": "errors.member_action",
		"The final owner cannot be changed.": "errors.final_owner", "The invitation lifetime is invalid.": "errors.invalid_lifetime",
		"The invitation could not be created.": "errors.invitation_create", "The invitation could not be resent.": "errors.invitation_resend",
		"The invitation could not be revoked.": "errors.invitation_revoke", "The invitation could not be found.": "errors.invitation_not_found",
		"The requested invitation action is invalid.":                                  "errors.invalid_invitation_action",
		"Invitation email could not be delivered. Share the invitation link manually.": "errors.delivery",
		"The token expiration is invalid.":                                             "errors.invalid_token_expiration", "The token could not be created.": "errors.token_create",
		"The token could not be revoked.": "errors.token_revoke", "The requested token action is invalid.": "errors.invalid_token_action",
		"The webhook input is invalid.": "errors.webhook_invalid", "The webhook could not be created.": "errors.webhook_create",
		"The workspace webhook limit was reached. Delete one before retrying.":                            "errors.webhook_limit",
		"Webhook secret encryption is not configured. Set WEBHOOK_ENCRYPTION_KEY and restart the server.": "errors.webhook_secret_key",
		"The webhook could not be rotated.":                                                               "errors.webhook_rotate", "The webhook could not be deleted.": "errors.webhook_delete",
		"The webhook could not be found.": "errors.webhook_not_found", "The requested webhook action is invalid.": "errors.invalid_webhook_action",
		"The SCIM credential could not be created.": "errors.scim_create", "The SCIM credential could not be revoked.": "errors.scim_revoke", "The requested SCIM action is invalid.": "errors.invalid_scim_action",
		"No OIDC provider is configured.": "errors.no_oidc", "Reauthentication failed.": "errors.reauthentication",
		"The OIDC link could not be started.":                         "errors.oidc_link",
		"Enter a workspace name between 1 and 120 characters.":        "errors.workspace_name",
		"The selected language is invalid.":                           "errors.invalid_locale",
		"The account settings are invalid.":                           "errors.account_settings",
		"The email verification link is invalid or expired.":          "errors.email_verification",
		"The password reset link is invalid or expired.":              "errors.password_reset",
		"The current password or new password is invalid.":            "errors.password_change",
		"Transfer workspace ownership before deleting this account.":  "errors.owned_workspace",
		"Account deletion requires the current password.":             "errors.delete_account",
		"Email delivery is not configured.":                           "errors.email_delivery",
		"Verification email could not be delivered. Try again later.": "errors.verification_delivery",
		"The notification preference is invalid.":                     "errors.notification_preference",
		"The password reset form expired. Reload and try again.":      "errors.password_reset_expired",
		"Enter a title between 1 and 200 characters.":                 "items.invalid_title",
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

func (h *Handler) languageSettingsPage(writer http.ResponseWriter, request *http.Request) {
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return
	}
	locale, err := h.identity.GetUserLocale(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusOK, "language.html", pageData{
		Title: "Language settings", NavPage: "language", CSRF: csrf, UserLocale: locale,
		Saved: request.URL.Query().Get("saved") == "1",
	})
}

func (h *Handler) languageSettings(writer http.ResponseWriter, request *http.Request) {
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
	locale := strings.TrimSpace(request.FormValue("locale"))
	if locale != "" {
		if _, ok := i18n.ParseLocale(locale); !ok {
			h.render(writer, http.StatusUnprocessableEntity, "language.html", pageData{Title: "Language settings", CSRF: csrf, Error: "The selected language is invalid."})
			return
		}
	}
	if err := h.identity.SetUserLocale(request.Context(), session.UserID, locale, auditContext(request)); err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	if locale == "" {
		h.clearCookie(writer, "locale")
	}
	h.redirect(writer, request, "/settings/language?saved=1")
}

func (h *Handler) generalSettingsPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
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
	h.render(writer, http.StatusOK, "general-settings.html", pageData{
		Title: "General workspace settings", NavPage: "general", NavSection: "settings", CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		WorkspaceNotificationPreferences: preferences,
		CanManage:                        principal.Permissions[identity.WorkspaceUpdate], Saved: request.URL.Query().Get("saved") == "1",
	})
}

func (h *Handler) generalSettings(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, _, _, ok := h.managementPrincipal(writer, request, workspaceID, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	if err := h.identity.UpdateWorkspaceLocale(request.Context(), principal, request.FormValue("locale"), auditContext(request)); err != nil {
		h.managementError(writer, request, workspaceID, "The selected language is invalid.")
		return
	}
	h.redirect(writer, request, "/workspaces/"+workspaceID+"/settings/general?saved=1")
}

func (h *Handler) workspaceNotificationSettings(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, _, _, ok := h.managementPrincipal(writer, request, workspaceID, identity.WorkspaceRead)
	if !ok {
		return
	}
	if err := h.identity.SetWorkspaceNotificationPreference(request.Context(), principal,
		request.FormValue("notification_type"), request.FormValue("enabled") == "true", auditContext(request)); err != nil {
		h.managementError(writer, request, workspaceID, "The notification preference is invalid.")
		return
	}
	h.redirect(writer, request, "/workspaces/"+workspaceID+"/settings/general?saved=1")
}
