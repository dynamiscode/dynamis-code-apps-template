package web

import (
	"crypto/subtle"
	"errors"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/http"
	"time"
)

var (
	ErrInvalidFlow           = errors.New("flow is not 'login' or 'link'")
	ErrMissingBrowserSession = errors.New("browser session cookie empty")
	ErrStateMismatch         = errors.New("state query param empty or does not match state cookie")
	ErrMissingCryptoParams   = errors.New("verifier or nonce cookie empty")
	ErrMissingAuthCode       = errors.New("authorization code query param empty")
)

type OIDCCallbackParams struct {
	Flow           string
	BrowserSession string
	StateCookie    string
	StateQuery     string
	State          string
	Verifier       string
	PKCEVerifier   string
	Nonce          string
	Code           string
}

func validateOIDCCallbackParams(p OIDCCallbackParams) error {
	if p.Flow != "login" && p.Flow != "link" {
		return ErrInvalidFlow
	}
	if p.BrowserSession == "" {
		return ErrMissingBrowserSession
	}
	stateQuery := p.StateQuery
	if stateQuery == "" {
		stateQuery = p.State
	}
	if p.StateCookie == "" || stateQuery == "" || subtle.ConstantTimeCompare([]byte(p.StateCookie), []byte(stateQuery)) != 1 {
		return ErrStateMismatch
	}
	verifier := p.Verifier
	if verifier == "" {
		verifier = p.PKCEVerifier
	}
	if verifier == "" || p.Nonce == "" {
		return ErrMissingCryptoParams
	}
	if p.Code == "" {
		return ErrMissingAuthCode
	}
	return nil
}

func (h *Handler) oidcLogin(request *http.Request) (result authPageOutput) {
	if h.oidc == nil {
		result.status = http.StatusNotFound
		return
	}
	browserSession := cookieValue(request, "oidc_browser")
	if browserSession == "" {
		var err error
		browserSession, err = id.New()
		if err != nil {
			result.status = http.StatusInternalServerError
			return
		}
		result.oidcBrowser = browserSession
	}
	transaction, loginURL, err := h.oidc.Begin(
		request.Context(), h.identity, request.PathValue("providerId"), browserSession,
	)
	if err != nil {
		result.status = http.StatusNotFound
		return
	}
	result.kind, result.oidcTransaction, result.target = authPageOIDCBegin, transaction, loginURL
	return
}

func (h *Handler) oidcCallback(request *http.Request) (result authPageOutput) {
	if h.oidc == nil {
		result.status = http.StatusNotFound
		return
	}
	flow := cookieValue(request, "oidc_flow")
	browserSession := cookieValue(request, "oidc_browser")
	if flow == "link" {
		session, err := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session"))
		if err != nil {
			result.kind, result.status, result.data = authPageLogin, http.StatusUnauthorized, h.oidcErrorData()
			return
		}
		browserSession = session.ID
	}
	params := OIDCCallbackParams{
		Flow:           flow,
		BrowserSession: browserSession,
		StateCookie:    cookieValue(request, "oidc_state"),
		StateQuery:     request.URL.Query().Get("state"),
		State:          request.URL.Query().Get("state"),
		Verifier:       cookieValue(request, "oidc_verifier"),
		PKCEVerifier:   cookieValue(request, "oidc_verifier"),
		Nonce:          cookieValue(request, "oidc_nonce"),
		Code:           request.URL.Query().Get("code"),
	}
	if err := validateOIDCCallbackParams(params); err != nil {
		h.logger.WarnContext(request.Context(), "oidc callback validation failed", "error", err)
		result.kind, result.status, result.data = authPageLogin, http.StatusUnauthorized, h.oidcErrorData()
		return
	}
	redirectURI, ok := h.oidc.RedirectURL(request.PathValue("providerId"))
	if !ok {
		result.kind, result.status, result.data = authPageLogin, http.StatusUnauthorized, h.oidcErrorData()
		return
	}
	completion, err := h.oidc.CompleteFlow(
		request.Context(), h.identity, request.PathValue("providerId"), browserSession,
		request.URL.Query().Get("state"), cookieValue(request, "oidc_verifier"),
		cookieValue(request, "oidc_nonce"), redirectURI, request.URL.Query().Get("code"),
	)
	if err != nil {
		result.clearOIDC = true
		result.kind, result.status, result.data = authPageLogin, http.StatusUnauthorized, h.oidcErrorData()
		return
	}
	result.clearOIDC = true
	if completion.Purpose == identity.OIDCPurposeLink {
		session, err := h.identity.AuthenticateSession(request.Context(), cookieValue(request, "session"))
		if err != nil || session.UserID != completion.UserID {
			result.kind, result.status, result.data = authPageLogin, http.StatusUnauthorized, h.oidcErrorData()
			return
		}
		if err := h.identity.LinkOIDCIdentity(request.Context(), identity.Principal{
			UserID: session.UserID, AuthMethod: session.AuthMethod,
		}, completion.Claims, true, auditContext(request)); err != nil {
			result.kind, result.status, result.data = authPageLogin, http.StatusUnauthorized, h.oidcErrorData()
			return
		}
		result.kind, result.target = authPageRedirect, "/security"
		return
	}
	userID, err := h.identity.AuthenticateOIDC(request.Context(), completion.Claims, auditContext(request))
	if err != nil {
		result.kind, result.status, result.data = authPageLogin, http.StatusUnauthorized, h.oidcErrorData()
		return
	}
	mfaEnrolled, err := h.identity.MFAEnrolled(request.Context(), userID)
	if err != nil {
		result.status = http.StatusInternalServerError
		return
	}
	if mfaEnrolled {
		challenge, err := h.identity.BeginMFALoginWithMethod(request.Context(), userID, "oidc", completion.Claims.ProviderID, auditContext(request))
		if err != nil {
			result.status = http.StatusInternalServerError
			return
		}
		returnTo := "/"
		if h.invitationReturnActive(request) {
			returnTo = invitationContinuePath
		}
		result.kind, result.challenge, result.target = authPageOIDCMFA, challenge, returnTo
		return
	}
	session, err := h.identity.CreateSession(request.Context(), userID, "oidc", completion.Claims.ProviderID, 0, auditContext(request))
	if err != nil {
		result.status = http.StatusInternalServerError
		return
	}
	result.kind, result.session, result.target = authPageOIDCSignedIn, session, "/"
	if h.invitationReturnActive(request) {
		result.target = invitationContinuePath
	}
	return
}

func (h *Handler) oidcErrorData() viewmodel.PageData {
	return viewmodel.PageData{
		Title: "Sign in", Error: "The identity provider sign-in could not be completed.",
		OIDCProviders: h.providers(),
	}
}

func (h *Handler) clearOIDCCookies(writer http.ResponseWriter) {
	for _, name := range []string{"oidc_flow", "oidc_browser", "oidc_state", "oidc_verifier", "oidc_nonce"} {
		h.clearCookie(writer, name)
	}
}

func timeNow() time.Time { return time.Now() }
