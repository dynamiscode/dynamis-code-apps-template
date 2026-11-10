package mcpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

type clientRegistration struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

var (
	supportedGrantTypes   = []string{"authorization_code", "refresh_token"}
	supportedResponseType = []string{"code"}
)

// NewOAuthRegistrationEndpoint serves RFC 7591 dynamic registration for public
// PKCE clients. Confidential clients and other grant types are rejected.
func NewOAuthRegistrationEndpoint(auth *identity.Service) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.Header().Set("Allow", http.MethodPost)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		setOAuthNoStore(writer)
		request.Body = http.MaxBytesReader(writer, request.Body, maxOAuthFormBytes)
		var input clientRegistration
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil || !supportedRegistration(input) {
			writeOAuthError(writer, http.StatusBadRequest, "invalid_client_metadata")
			return
		}
		client, err := auth.RegisterOAuthClient(request.Context(), input.ClientName, input.RedirectURIs,
			oauthAuditContext(request))
		switch {
		case errors.Is(err, identity.ErrOAuthClientLimit):
			writeOAuthError(writer, http.StatusTooManyRequests, "temporarily_unavailable")
			return
		case errors.Is(err, identity.ErrOAuthInvalidRequest):
			writeOAuthError(writer, http.StatusBadRequest, "invalid_client_metadata")
			return
		case err != nil:
			writeOAuthError(writer, http.StatusInternalServerError, "server_error")
			return
		}
		writeOAuthJSON(writer, http.StatusCreated, map[string]any{
			"client_id":                  client.ClientID,
			"client_name":                client.Name,
			"redirect_uris":              client.RedirectURIs,
			"grant_types":                supportedGrantTypes,
			"response_types":             supportedResponseType,
			"token_endpoint_auth_method": "none",
		})
	})
}

func supportedRegistration(input clientRegistration) bool {
	if input.TokenEndpointAuthMethod != "" && input.TokenEndpointAuthMethod != "none" {
		return false
	}
	for _, grant := range input.GrantTypes {
		if !slices.Contains(supportedGrantTypes, grant) {
			return false
		}
	}
	for _, response := range input.ResponseTypes {
		if !slices.Contains(supportedResponseType, response) {
			return false
		}
	}
	return true
}

// NewAuthorizationServerMetadataHandler serves RFC 8414 metadata for the
// built-in authorization server. It returns nil when the built-in server is off.
func NewAuthorizationServerMetadataHandler(cfg config.MCP) http.Handler {
	if !cfg.OAuthBuiltIn || cfg.OAuthAuthorizationServer == "" {
		return nil
	}
	issuer := cfg.OAuthAuthorizationServer
	metadata := map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth/authorize",
		"token_endpoint":                        issuer + "/oauth/token",
		"revocation_endpoint":                   issuer + "/oauth/revoke",
		"registration_endpoint":                 issuer + "/oauth/register",
		"scopes_supported":                      []string{string(identity.ResourcesRead), string(identity.ResourcesWrite)},
		"response_types_supported":              supportedResponseType,
		"grant_types_supported":                 supportedGrantTypes,
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writeOAuthJSON(writer, http.StatusOK, metadata)
	})
}
