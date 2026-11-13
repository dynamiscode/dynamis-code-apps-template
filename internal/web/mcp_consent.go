package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// mcpConsentView is the data for the MCP consent page. Every value here is
// re-validated on submission; hidden fields are never trusted on their own.
type mcpConsentView struct {
	ClientName string
	Scopes     []string
	Workspaces []mcpWorkspaceChoice
	Params     mcpAuthorizeParams
}

type mcpWorkspaceChoice struct {
	ID   string
	Name string
}

type mcpAuthorizeParams struct {
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	Resource      string
	Scope         string
	State         string
}

// EnableMCPOAuth turns on the browser consent page for the built-in MCP
// authorization server. It must be called before Routes.
func (h *Handler) EnableMCPOAuth(resourceURL string) {
	h.mcpResource = resourceURL
}

func (h *Handler) mcpAuthorizePage(writer http.ResponseWriter, request *http.Request) {
	params, client, scopes, err := h.validateMCPAuthorize(request.Context(), request.URL.Query())
	if err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	session, csrf, ok := h.consentSession(request)
	if !ok {
		h.redirect(writer, request, "/login?return_to="+url.QueryEscape(request.URL.RequestURI()))
		return
	}
	choices := h.consentWorkspaces(request.Context(), session.UserID, scopes)
	if len(choices) == 0 {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	h.render(writer, http.StatusOK, "mcp-consent.html", pageData{
		Title: "Authorize MCP client", CSRF: csrf,
		MCPConsent: &mcpConsentView{ClientName: client.Name, Scopes: scopeNames(scopes), Workspaces: choices, Params: params},
	})
}

func (h *Handler) mcpAuthorize(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	params, client, scopes, err := h.validateMCPAuthorize(request.Context(), request.Form)
	if err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	session, _, ok := h.consentSession(request)
	if !ok {
		h.redirect(writer, request, "/login")
		return
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	if request.FormValue("decision") == "deny" {
		h.redirectMCPResult(writer, request, client, params.RedirectURI, url.Values{
			"error": {"access_denied"}, "state": {params.State},
		})
		return
	}
	code, err := h.identity.ApproveOAuthConsent(request.Context(), session, identity.ParseWorkspaceIDOrZero(request.FormValue("workspace_id")),
		client.ClientID, scopes, params.RedirectURI, params.CodeChallenge, params.Resource,
		auditContext(request))
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(writer, http.StatusForbidden)
		return
	case errors.Is(err, identity.ErrOAuthInvalidRequest), errors.Is(err, identity.ErrOAuthInvalidClient):
		h.renderError(writer, http.StatusBadRequest)
		return
	case err != nil:
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.redirectMCPResult(writer, request, client, params.RedirectURI, url.Values{
		"code": {code}, "state": {params.State},
	})
}

// validateMCPAuthorize checks the OAuth authorization request. Errors are shown
// on the page rather than redirected, because the redirect target is untrusted
// until the client and redirect URI have been matched.
func (h *Handler) validateMCPAuthorize(
	ctx context.Context,
	values url.Values,
) (mcpAuthorizeParams, identity.OAuthClient, []identity.Permission, error) {
	if h.mcpResource == "" || values.Get("response_type") != "code" ||
		values.Get("code_challenge_method") != "S256" || values.Get("code_challenge") == "" ||
		values.Get("resource") != h.mcpResource || len(values.Get("state")) > 512 {
		return mcpAuthorizeParams{}, identity.OAuthClient{}, nil, identity.ErrOAuthInvalidRequest
	}
	client, err := h.identity.OAuthClientByID(ctx, values.Get("client_id"))
	if err != nil || !slices.Contains(client.RedirectURIs, values.Get("redirect_uri")) {
		return mcpAuthorizeParams{}, identity.OAuthClient{}, nil, identity.ErrOAuthInvalidRequest
	}
	scopes, err := parseMCPScopes(values.Get("scope"))
	if err != nil {
		return mcpAuthorizeParams{}, identity.OAuthClient{}, nil, identity.ErrOAuthInvalidRequest
	}
	params := mcpAuthorizeParams{
		ClientID: client.ClientID, RedirectURI: values.Get("redirect_uri"),
		CodeChallenge: values.Get("code_challenge"), Resource: values.Get("resource"),
		Scope: strings.Join(scopeNames(scopes), " "), State: values.Get("state"),
	}
	return params, client, scopes, nil
}

func parseMCPScopes(value string) ([]identity.Permission, error) {
	if strings.TrimSpace(value) == "" {
		return []identity.Permission{identity.ResourcesRead}, nil
	}
	var scopes []identity.Permission
	for _, field := range strings.Fields(value) {
		scope := identity.Permission(field)
		if scope != identity.ResourcesRead && scope != identity.ResourcesWrite {
			return nil, identity.ErrOAuthInvalidRequest
		}
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	return scopes, nil
}

func scopeNames(scopes []identity.Permission) []string {
	names := make([]string, len(scopes))
	for index, scope := range scopes {
		names[index] = string(scope)
	}
	return names
}

// consentSession authenticates the browser session and returns its CSRF token.
func (h *Handler) consentSession(request *http.Request) (identity.Session, string, bool) {
	secret := cookieValue(request, "session")
	csrf := cookieValue(request, "csrf")
	if secret == "" || csrf == "" {
		return identity.Session{}, "", false
	}
	session, err := h.identity.AuthenticateSession(request.Context(), secret)
	if err != nil {
		return identity.Session{}, "", false
	}
	return session, csrf, true
}

// consentWorkspaces lists workspaces where the user holds every requested scope.
func (h *Handler) consentWorkspaces(ctx context.Context, userID identity.UserID, scopes []identity.Permission) []mcpWorkspaceChoice {
	workspaces, err := h.identity.ListWorkspaces(ctx, userID)
	if err != nil {
		return nil
	}
	var choices []mcpWorkspaceChoice
	for _, workspace := range workspaces {
		allowed := true
		for _, scope := range scopes {
			if _, err := h.identity.Authorize(ctx, userID, workspace.ID, scope); err != nil {
				allowed = false
				break
			}
		}
		if allowed {
			choices = append(choices, mcpWorkspaceChoice{ID: workspace.ID.String(), Name: workspace.Name})
		}
	}
	return choices
}

// redirectMCPResult sends the browser back to the client. The target must be
// one of the client's registered redirect URIs; this is re-checked here so the
// guard is local to the redirect, not only in validation.
func (h *Handler) redirectMCPResult(
	writer http.ResponseWriter,
	request *http.Request,
	client identity.OAuthClient,
	redirectURI string,
	values url.Values,
) {
	// Use the registered value, not the request value, as the redirect target.
	registered := ""
	for _, candidate := range client.RedirectURIs {
		if candidate == redirectURI {
			registered = candidate
			break
		}
	}
	if registered == "" {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	target, err := url.Parse(registered)
	if err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	query := target.Query()
	for key, value := range values {
		if value[0] != "" {
			query.Set(key, value[0])
		}
	}
	target.RawQuery = query.Encode()
	writer.Header().Set("Cache-Control", "no-store")
	http.Redirect(writer, request, target.String(), http.StatusSeeOther)
}
