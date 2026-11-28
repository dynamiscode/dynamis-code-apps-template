package web

import (
	"net/http"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

type authPageKind uint8

const (
	authPageError authPageKind = iota
	authPageLogin
	authPageSetup
	authPageRedirect
	authPageSetupCompleted
	authPageMFALogin
	authPageSignedIn
	authPagePasswordReset
	authPageMFAView
	authPageMFAExpired
	authPageMFAVerified
	authPagePasskeyVerified
	authPageVerification
	authPageOIDCBegin
	authPageOIDCLink
	authPageOIDCMFA
	authPageOIDCSignedIn
	authPageLocalLogout
	authPageInvitationLogout
	authPageProviderLogout
	authPageNotifications
	authPageAccount
	authPageAccountSession
	authPageAccountDeleted
	authPageThemeChanged
	authPageLanguage
	authPageSessions
	authPageSecurity
	authPageLocaleCleared
)

type resetCookieChange uint8

const (
	resetCookieUnchanged resetCookieChange = iota
	resetCookieRefresh
	resetCookieClear
	resetCookieClearSession
)

type authPageOutput struct {
	kind            authPageKind
	status          int
	data            viewmodel.PageData
	target          string
	theme           string
	challenge       identity.MFALoginChallenge
	session         identity.NewSession
	resetCookie     resetCookieChange
	expires         time.Time
	oidcBrowser     string
	clearOIDC       bool
	oidcTransaction identity.OIDCTransaction
}

func resetPageOutput(status int, data viewmodel.PageData, change resetCookieChange) authPageOutput {
	return authPageOutput{kind: authPagePasswordReset, status: status, data: data, resetCookie: change}
}

func authSessionFailure(status int) authPageOutput {
	if status == http.StatusSeeOther {
		return authPageOutput{kind: authPageRedirect, target: "/login"}
	}
	return authPageOutput{status: status}
}

// serveAuthPage owns HTTP output. Its callback selects only the closed set of
// authentication and account responses, with no raw writer or component factory.
func (h *Handler) serveAuthPage(view func(*http.Request) authPageOutput) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		result := view(request)
		if result.oidcBrowser != "" {
			h.setCookie(writer, "oidc_browser", result.oidcBrowser, timeNow().Add(10*time.Minute), true)
		}
		if result.clearOIDC {
			h.clearOIDCCookies(writer)
		}
		switch result.kind {
		case authPageLogin:
			if result.status == http.StatusOK {
				h.setCookie(writer, "login_csrf", result.data.CSRF, time.Now().Add(10*time.Minute), true)
			}
			h.render(writer, result.status, "login.html", result.data)
		case authPageSetup:
			h.setCookie(writer, "setup_csrf", result.data.CSRF, time.Now().Add(10*time.Minute), true)
			h.render(writer, result.status, "setup.html", result.data)
		case authPageRedirect:
			h.redirect(writer, request, result.target)
		case authPageSetupCompleted:
			h.clearCookie(writer, "setup_csrf")
			h.redirect(writer, request, "/login")
		case authPageMFALogin:
			h.setMFAChallengeCookies(writer, result.challenge, result.target)
			h.redirect(writer, request, "/mfa")
		case authPageSignedIn:
			h.setCookie(writer, "session", result.session.Secret, result.session.ExpiresAt, true)
			h.setCookie(writer, "csrf", result.session.CSRFSecret, result.session.ExpiresAt, true)
			h.clearCookie(writer, "login_csrf")
			h.redirect(writer, request, result.target)
		case authPagePasswordReset:
			switch result.resetCookie {
			case resetCookieRefresh:
				h.setCookie(writer, "reset_csrf", result.data.CSRF, time.Now().Add(10*time.Minute), true)
			case resetCookieClear:
				h.clearCookie(writer, "reset_csrf")
			case resetCookieClearSession:
				h.clearCookie(writer, "reset_csrf")
				h.clearCookie(writer, "session")
				h.clearCookie(writer, "csrf")
			}
			h.render(writer, result.status, "password-reset.html", result.data)
		case authPageMFAView:
			if result.status == http.StatusOK {
				h.setMFAFormCookie(writer, result.data.CSRF, result.expires)
			}
			h.render(writer, result.status, "mfa.html", result.data)
		case authPageMFAExpired:
			h.clearMFAChallengeCookies(writer)
			h.redirect(writer, request, "/login")
		case authPageMFAVerified:
			h.redirect(writer, request, h.completeMFASession(writer, request, result.session))
		case authPagePasskeyVerified:
			writer.Header().Set("X-MFA-Return-To", h.completeMFASession(writer, request, result.session))
			writer.WriteHeader(http.StatusNoContent)
		case authPageVerification:
			h.render(writer, result.status, "verification.html", result.data)
		case authPageOIDCBegin:
			h.setOIDCCookies(writer, "login", result.oidcTransaction)
			h.redirectURL(writer, request, result.target)
		case authPageOIDCLink:
			h.setOIDCCookies(writer, "link", result.oidcTransaction)
			h.redirectURL(writer, request, result.target)
		case authPageOIDCMFA:
			h.setMFAChallengeCookies(writer, result.challenge, result.target)
			h.clearCookie(writer, "oidc_browser")
			h.redirect(writer, request, "/mfa")
		case authPageOIDCSignedIn:
			h.setCookie(writer, "session", result.session.Secret, result.session.ExpiresAt, true)
			h.setCookie(writer, "csrf", result.session.CSRFSecret, result.session.ExpiresAt, true)
			h.clearCookie(writer, "oidc_browser")
			h.redirect(writer, request, result.target)
		case authPageLocalLogout, authPageInvitationLogout, authPageProviderLogout:
			h.clearCookie(writer, "session")
			h.clearCookie(writer, "csrf")
			switch result.kind {
			case authPageLocalLogout:
				h.redirect(writer, request, "/login")
			case authPageInvitationLogout:
				setInvitationSecurityHeaders(writer)
				http.Redirect(writer, request, invitationLoginLocation, http.StatusSeeOther)
			case authPageProviderLogout:
				h.redirectURL(writer, request, result.target)
			}
		case authPageNotifications:
			h.localizePage(writer, &result.data)
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			writer.Header().Set("Cache-Control", "no-store")
			writer.WriteHeader(http.StatusOK)
			_ = notificationsPage(result.data).Render(request.Context(), writer)
		case authPageAccount:
			h.render(writer, result.status, "account-page", result.data)
		case authPageAccountSession:
			h.setCookie(writer, "session", result.session.Secret, result.session.ExpiresAt, true)
			h.setCookie(writer, "csrf", result.session.CSRFSecret, result.session.ExpiresAt, true)
			h.redirect(writer, request, "/account?saved=1")
		case authPageAccountDeleted:
			h.clearCookie(writer, "session")
			h.clearCookie(writer, "csrf")
			h.redirect(writer, request, "/login")
		case authPageThemeChanged:
			h.setCookie(writer, themeCookie, result.theme, time.Now().Add(themeCookieTTL), true)
			h.redirect(writer, request, result.target)
		case authPageLanguage:
			h.render(writer, result.status, "language-page", result.data)
		case authPageLocaleCleared:
			h.clearCookie(writer, "locale")
			h.redirect(writer, request, result.target)
		case authPageSessions:
			h.render(writer, http.StatusOK, "sessions-page", result.data)
		case authPageSecurity:
			h.render(writer, result.status, "security-page", result.data)
		case authPageError:
			switch result.status {
			case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict:
				h.renderError(writer, result.status)
			default:
				h.renderError(writer, http.StatusInternalServerError)
			}
		default:
			h.renderError(writer, http.StatusInternalServerError)
		}
	}
}

func (h *Handler) loginView(request *http.Request) authPageOutput {
	available, err := h.setupAvailable(request)
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	if available {
		return authPageOutput{kind: authPageRedirect, target: "/setup"}
	}
	token, err := id.New()
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	return authPageOutput{kind: authPageLogin, status: http.StatusOK, data: viewmodel.PageData{
		Title: "Sign in", CSRF: token, OIDCProviders: h.providers(),
		ReturnTo: h.invitationLoginReturnTo(request, request.URL.Query().Get("return_to")),
	}}
}

func (h *Handler) setupView(request *http.Request) authPageOutput {
	available, err := h.setupAvailable(request)
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	if !available {
		return authPageOutput{status: http.StatusNotFound}
	}
	return h.setupOutput(http.StatusOK, viewmodel.PageData{Title: "Set up Dynamis Code"})
}

func (h *Handler) setupOutput(status int, data viewmodel.PageData) authPageOutput {
	token, err := id.New()
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	data.CSRF, data.SetupTokenRequired = token, h.setupTokenHash != ""
	return authPageOutput{kind: authPageSetup, status: status, data: data}
}
