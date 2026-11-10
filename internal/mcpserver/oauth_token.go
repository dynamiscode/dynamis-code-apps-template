package mcpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
)

const maxOAuthFormBytes = 16 << 10

// NewOAuthTokenEndpoint serves RFC 6749 token requests for the built-in MCP
// authorization server: authorization_code (PKCE) and refresh_token grants. Errors
// use the OAuth error format, never Problem Details.
func NewOAuthTokenEndpoint(auth *identity.Service) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.Header().Set("Allow", http.MethodPost)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		setOAuthNoStore(writer)
		request.Body = http.MaxBytesReader(writer, request.Body, maxOAuthFormBytes)
		if err := request.ParseForm(); err != nil {
			writeOAuthError(writer, http.StatusBadRequest, "invalid_request")
			return
		}
		audit := oauthAuditContext(request)
		var tokens identity.OAuthTokenSet
		var err error
		switch request.PostForm.Get("grant_type") {
		case "authorization_code":
			tokens, err = auth.ExchangeOAuthCode(request.Context(),
				request.PostForm.Get("code"), request.PostForm.Get("client_id"),
				request.PostForm.Get("redirect_uri"), request.PostForm.Get("code_verifier"),
				request.PostForm.Get("resource"), audit)
		case "refresh_token":
			tokens, err = auth.RefreshOAuthToken(request.Context(),
				request.PostForm.Get("refresh_token"), request.PostForm.Get("client_id"), audit)
		default:
			writeOAuthError(writer, http.StatusBadRequest, "unsupported_grant_type")
			return
		}
		switch {
		case errors.Is(err, identity.ErrOAuthInvalidClient):
			writeOAuthError(writer, http.StatusUnauthorized, "invalid_client")
			return
		case errors.Is(err, identity.ErrOAuthInvalidGrant):
			writeOAuthError(writer, http.StatusBadRequest, "invalid_grant")
			return
		case errors.Is(err, identity.ErrOAuthInvalidRequest):
			writeOAuthError(writer, http.StatusBadRequest, "invalid_request")
			return
		case err != nil:
			writeOAuthError(writer, http.StatusInternalServerError, "server_error")
			return
		}
		scopes := make([]string, len(tokens.Scopes))
		for index, scope := range tokens.Scopes {
			scopes[index] = string(scope)
		}
		writeOAuthJSON(writer, http.StatusOK, map[string]any{
			"access_token":  tokens.AccessToken,
			"token_type":    "Bearer",
			"expires_in":    int(identity.OAuthAccessLifetime.Seconds()),
			"refresh_token": tokens.RefreshToken,
			"scope":         strings.Join(scopes, " "),
		})
	})
}

// NewOAuthRevocationEndpoint serves RFC 7009 revocation. Valid requests succeed
// whether or not the token exists, so the endpoint cannot be used to probe secrets.
func NewOAuthRevocationEndpoint(auth *identity.Service) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.Header().Set("Allow", http.MethodPost)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		setOAuthNoStore(writer)
		request.Body = http.MaxBytesReader(writer, request.Body, maxOAuthFormBytes)
		if err := request.ParseForm(); err != nil || request.PostForm.Get("token") == "" {
			writeOAuthError(writer, http.StatusBadRequest, "invalid_request")
			return
		}
		if err := auth.RevokeOAuthToken(request.Context(), request.PostForm.Get("token"), oauthAuditContext(request)); err != nil {
			writeOAuthError(writer, http.StatusInternalServerError, "server_error")
			return
		}
		writer.WriteHeader(http.StatusOK)
	})
}

func oauthAuditContext(request *http.Request) identity.AuditContext {
	return identity.AuditContext{
		RequestID: request.Header.Get("X-Request-ID"), SourceAddress: sourceHost(request.RemoteAddr),
	}
}

func setOAuthNoStore(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Pragma", "no-cache")
}

func writeOAuthError(writer http.ResponseWriter, status int, code string) {
	writeOAuthJSON(writer, status, map[string]string{"error": code})
}

func writeOAuthJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
