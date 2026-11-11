package mcpserver

import (
	"fmt"
	"net/http"
	"net/url"

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
