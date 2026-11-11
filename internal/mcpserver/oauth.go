package mcpserver

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// ProtectedResourceMetadataPath is the RFC 9728 well-known path for the /mcp
// resource. It is served without authentication only when OAuth discovery is
// configured.
const ProtectedResourceMetadataPath = "/.well-known/oauth-protected-resource/mcp"

// NewProtectedResourceMetadataHandler serves RFC 9728 metadata that points
// clients at the configured authorization server. It returns nil when discovery
// is disabled. The document contains only the configured resource, issuer,
// documented scope names, and bearer transport.
func NewProtectedResourceMetadataHandler(cfg config.MCP) http.Handler {
	if cfg.OAuthAuthorizationServer == "" || cfg.OAuthResourceURL == "" {
		return nil
	}
	return auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               cfg.OAuthResourceURL,
		AuthorizationServers:   []string{cfg.OAuthAuthorizationServer},
		ScopesSupported:        []string{string(identity.ResourcesRead), string(identity.ResourcesWrite)},
		BearerMethodsSupported: []string{"header"},
	})
}

// protectedResourceMetadataURL returns the absolute metadata URL advertised in
// WWW-Authenticate, or an empty string when discovery is disabled.
func protectedResourceMetadataURL(cfg config.MCP) string {
	if cfg.OAuthAuthorizationServer == "" || cfg.OAuthResourceURL == "" {
		return ""
	}
	resource, err := url.Parse(cfg.OAuthResourceURL)
	if err != nil {
		return ""
	}
	return resource.Scheme + "://" + resource.Host + ProtectedResourceMetadataPath
}

// insufficientScopeChallenge is the 403 challenge for an authenticated token
// that lacks the scope required by the called tool.
func insufficientScopeChallenge(cfg config.MCP) string {
	metadataURL := protectedResourceMetadataURL(cfg)
	if metadataURL == "" {
		return ""
	}
	return fmt.Sprintf(`Bearer error="insufficient_scope", resource_metadata=%q`, metadataURL)
}

// invalidTokenChallengeMiddleware adds error="invalid_token" to the SDK's 401
// challenge when a bearer token was presented and rejected (RFC 6750 section
// 3.1). A missing token keeps the plain challenge, because it has no error code.
func invalidTokenChallengeMiddleware(next http.Handler, cfg config.MCP) http.Handler {
	metadataURL := protectedResourceMetadataURL(cfg)
	if metadataURL == "" {
		return next
	}
	challenge := fmt.Sprintf(`Bearer error="invalid_token", resource_metadata=%q`, metadataURL)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		bearer := strings.HasPrefix(strings.ToLower(request.Header.Get("Authorization")), "bearer ")
		next.ServeHTTP(&challengeWriter{ResponseWriter: writer, bearer: bearer, challenge: challenge}, request)
	})
}

type challengeWriter struct {
	http.ResponseWriter
	bearer    bool
	challenge string
}

func (writer *challengeWriter) WriteHeader(status int) {
	if status == http.StatusUnauthorized && writer.bearer && writer.Header().Get("WWW-Authenticate") != "" {
		writer.Header().Set("WWW-Authenticate", writer.challenge)
	}
	writer.ResponseWriter.WriteHeader(status)
}
