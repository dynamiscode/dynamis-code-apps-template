//go:build oidcfixture

package identity

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// NewOIDCRegistryForLoopbackTestFixture builds a registry only for the
// explicitly tagged browser fixture. Production construction remains public-HTTPS-only.
func NewOIDCRegistryForLoopbackTestFixture(
	ctx context.Context,
	cfg config.OIDC,
	client *http.Client,
) (*OIDCRegistry, error) {
	issuer, err := url.Parse(cfg.IssuerURL)
	if !cfg.Enabled || err != nil || !isLoopbackFixtureURL(issuer, nil) || client == nil {
		return nil, errors.New("invalid loopback OIDC fixture configuration")
	}
	fixtureClient := *client
	previousRedirect := fixtureClient.CheckRedirect
	fixtureClient.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if !isLoopbackFixtureURL(request.URL, issuer) {
			return errors.New("OIDC fixture redirect is not loopback")
		}
		if previousRedirect != nil {
			return previousRedirect(request, via)
		}
		if len(via) >= 5 {
			return errors.New("OIDC fixture redirect limit exceeded")
		}
		return nil
	}
	provider, err := buildOIDCProvider(ctx, cfg, &fixtureClient)
	if err != nil {
		return nil, errors.New("build loopback OIDC fixture provider")
	}
	for _, endpoint := range []string{
		provider.oauth.Endpoint.AuthURL,
		provider.oauth.Endpoint.TokenURL,
		provider.endSessionEndpoint,
	} {
		if endpoint != "" {
			parsed, err := url.Parse(endpoint)
			if err != nil || !isLoopbackFixtureURL(parsed, issuer) {
				return nil, errors.New("OIDC fixture endpoint is not loopback")
			}
		}
	}
	return &OIDCRegistry{providers: map[string]*oidcProvider{provider.id: provider}}, nil
}

func isLoopbackFixtureURL(candidate, issuer *url.URL) bool {
	if candidate == nil || candidate.Scheme != "https" || candidate.User != nil || candidate.Fragment != "" {
		return false
	}
	ip := net.ParseIP(candidate.Hostname())
	if ip == nil || !ip.IsLoopback() || candidate.Port() == "" {
		return false
	}
	return issuer == nil || strings.EqualFold(candidate.Host, issuer.Host)
}
