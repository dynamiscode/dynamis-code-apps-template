package web

import (
	"net/http"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func (h *Handler) oidcLogin(writer http.ResponseWriter, request *http.Request) {
	if h.oidc == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	browserSession := cookieValue(request, "oidc_browser")
	if browserSession == "" {
		var err error
		browserSession, err = id.New()
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		h.setCookie(writer, "oidc_browser", browserSession, timeNow().Add(10*time.Minute), true)
	}
	transaction, loginURL, err := h.oidc.Begin(
		request.Context(), h.identity, request.PathValue("providerId"), browserSession,
	)
	if err != nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	h.setOIDCCookies(writer, "login", transaction)
	h.redirectURL(writer, request, loginURL)
}

func (h *Handler) oidcCallback(writer http.ResponseWriter, request *http.Request) {
	if h.oidc == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	flow := cookieValue(request, "oidc_flow")
	browserSession := cookieValue(request, "oidc_browser")
	if flow == "link" {
		session, err := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session"))
		if err != nil {
			h.oidcError(writer, request)
			return
		}
		browserSession = session.ID
	}
	if flow != "login" && flow != "link" || browserSession == "" ||
		cookieValue(request, "oidc_state") == "" || cookieValue(request, "oidc_state") != request.URL.Query().Get("state") ||
		cookieValue(request, "oidc_verifier") == "" || cookieValue(request, "oidc_nonce") == "" || request.URL.Query().Get("code") == "" {
		h.oidcError(writer, request)
		return
	}
	redirectURI, ok := h.oidc.RedirectURL(request.PathValue("providerId"))
	if !ok {
		h.oidcError(writer, request)
		return
	}
	completion, err := h.oidc.CompleteFlow(
		request.Context(), h.identity, request.PathValue("providerId"), browserSession,
		request.URL.Query().Get("state"), cookieValue(request, "oidc_verifier"),
		cookieValue(request, "oidc_nonce"), redirectURI, request.URL.Query().Get("code"),
	)
	if err != nil {
		h.clearOIDCCookies(writer)
		h.oidcError(writer, request)
		return
	}
	h.clearOIDCCookies(writer)
	if completion.Purpose == "link" {
		session, err := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session"))
		if err != nil || session.UserID != completion.UserID {
			h.oidcError(writer, request)
			return
		}
		if err := h.identity.LinkOIDCIdentity(request.Context(), identity.Principal{
			UserID: session.UserID, AuthMethod: session.AuthMethod,
		}, completion.Claims, true, auditContext(request)); err != nil {
			h.oidcError(writer, request)
			return
		}
		h.redirect(writer, request, "/security")
		return
	}
	userID, err := h.identity.AuthenticateOIDC(request.Context(), completion.Claims, auditContext(request))
	if err != nil {
		h.oidcError(writer, request)
		return
	}
	mfaEnrolled, err := h.identity.MFAEnrolled(request.Context(), userID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	if mfaEnrolled {
		challenge, err := h.identity.BeginMFALoginWithMethod(request.Context(), userID, "oidc", completion.Claims.ProviderID, auditContext(request))
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		h.setCookie(writer, "mfa_challenge", challenge.Token, challenge.ExpiresAt, true)
		h.clearCookie(writer, "oidc_browser")
		h.redirect(writer, request, "/mfa")
		return
	}
	session, err := h.identity.CreateSession(request.Context(), userID, "oidc", completion.Claims.ProviderID, 0, auditContext(request))
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.setCookie(writer, "session", session.Secret, session.ExpiresAt, true)
	h.setCookie(writer, "csrf", session.CSRFSecret, session.ExpiresAt, true)
	h.clearCookie(writer, "oidc_browser")
	h.redirect(writer, request, "/")
}

func (h *Handler) oidcError(writer http.ResponseWriter, request *http.Request) {
	h.render(writer, http.StatusUnauthorized, "login.html", pageData{
		Title: "Sign in", Error: "The identity provider sign-in could not be completed.",
		OIDCProviders: h.providers(),
	})
}

func (h *Handler) clearOIDCCookies(writer http.ResponseWriter) {
	for _, name := range []string{"oidc_flow", "oidc_browser", "oidc_state", "oidc_verifier", "oidc_nonce"} {
		h.clearCookie(writer, name)
	}
}

func timeNow() time.Time { return time.Now() }
