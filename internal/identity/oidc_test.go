package identity

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func TestOIDCDiscoveryAndIDTokenValidation(t *testing.T) {
	service, _ := newTestService(t)
	ctx := context.Background()
	bootstrapOwner(t, service)

	key := newRSAKey(t)
	otherKey := newRSAKey(t)
	publicKey := jose.JSONWebKey{
		Key: &key.PublicKey, KeyID: "key-1", Algorithm: string(jose.RS256), Use: "sig",
	}
	var issuer string
	var tokenResponse string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			writeJSON(t, writer, map[string]any{
				"issuer":                                issuer,
				"authorization_endpoint":                issuer + "/authorize",
				"token_endpoint":                        issuer + "/token",
				"jwks_uri":                              issuer + "/keys",
				"end_session_endpoint":                  issuer + "/logout",
				"response_types_supported":              []string{"code"},
				"subject_types_supported":               []string{"public"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/keys":
			writeJSON(t, writer, map[string]any{"keys": []jose.JSONWebKey{publicKey}})
		case "/token":
			if err := request.ParseForm(); err != nil || request.Form.Get("code") != "code" {
				http.Error(writer, "invalid code", http.StatusBadRequest)
				return
			}
			writeJSON(t, writer, map[string]any{
				"access_token": "access", "token_type": "Bearer",
				"id_token": tokenResponse,
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	issuer = server.URL

	cfg := config.OIDC{
		Enabled: true, ProviderID: "company", ProviderName: "Company",
		IssuerURL: issuer, ClientID: "client", ClientSecret: "secret",
		RedirectURL: "https://app.example.com/callback",
	}
	provider, err := buildOIDCProvider(ctx, cfg, server.Client())
	if err != nil {
		t.Fatalf("buildOIDCProvider() error = %v", err)
	}
	registry := &OIDCRegistry{providers: map[string]*oidcProvider{"company": provider}}
	if endpoint, ok := registry.LogoutURL("company"); !ok || endpoint != issuer+"/logout" {
		t.Fatalf("LogoutURL() = %q, %v", endpoint, ok)
	}

	tests := []struct {
		name          string
		issuer        string
		audience      string
		nonce         func(string) string
		expiresAt     time.Time
		emailVerified bool
		key           *rsa.PrivateKey
		code          string
		wantError     bool
	}{
		{
			name: "code mismatch", issuer: issuer, audience: "client",
			nonce:     func(value string) string { return value },
			expiresAt: time.Now().Add(time.Hour), emailVerified: true, key: key,
			code: "wrong", wantError: true,
		},
		{
			name: "valid", issuer: issuer, audience: "client",
			nonce:     func(value string) string { return value },
			expiresAt: time.Now().Add(time.Hour), emailVerified: true, key: key,
		},
		{
			name: "nonce mismatch", issuer: issuer, audience: "client",
			nonce:     func(string) string { return "wrong" },
			expiresAt: time.Now().Add(time.Hour), emailVerified: true, key: key,
			wantError: true,
		},
		{
			name: "issuer mismatch", issuer: "https://other.example.com", audience: "client",
			nonce:     func(value string) string { return value },
			expiresAt: time.Now().Add(time.Hour), emailVerified: true, key: key,
			wantError: true,
		},
		{
			name: "audience mismatch", issuer: issuer, audience: "other",
			nonce:     func(value string) string { return value },
			expiresAt: time.Now().Add(time.Hour), emailVerified: true, key: key,
			wantError: true,
		},
		{
			name: "expired", issuer: issuer, audience: "client",
			nonce:     func(value string) string { return value },
			expiresAt: time.Now().Add(-time.Hour), emailVerified: true, key: key,
			wantError: true,
		},
		{
			name: "unverified email", issuer: issuer, audience: "client",
			nonce:     func(value string) string { return value },
			expiresAt: time.Now().Add(time.Hour), emailVerified: false, key: key,
			wantError: true,
		},
		{
			name: "signature mismatch", issuer: issuer, audience: "client",
			nonce:     func(value string) string { return value },
			expiresAt: time.Now().Add(time.Hour), emailVerified: true, key: otherKey,
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transaction, _, err := registry.Begin(ctx, service, "company", "browser-"+test.name)
			if err != nil {
				t.Fatal(err)
			}
			tokenResponse = signIDToken(t, test.key, map[string]any{
				"iss":            test.issuer,
				"sub":            "subject",
				"aud":            test.audience,
				"exp":            test.expiresAt.Unix(),
				"iat":            time.Now().Add(-time.Minute).Unix(),
				"nonce":          test.nonce(transaction.Nonce),
				"email":          "person@example.com",
				"email_verified": test.emailVerified,
			})
			code := test.code
			if code == "" {
				code = "code"
			}
			claims, err := registry.Complete(
				ctx, service, "company", transaction.BrowserSession,
				transaction.State, transaction.PKCEVerifier, transaction.Nonce,
				transaction.RedirectURI, code,
			)
			if test.wantError {
				if err == nil {
					t.Fatalf("Complete() = %+v, nil; want error", claims)
				}
				return
			}
			if err != nil || claims.Issuer != issuer ||
				claims.Subject != "subject" || claims.Email != "person@example.com" {
				t.Fatalf("Complete() = %+v, %v", claims, err)
			}
		})
	}
}

func TestOIDCRejectsUnsafeDiscoveryDestinations(t *testing.T) {
	t.Parallel()

	unsafe := []string{
		"http://id.example.com",
		"https://localhost",
		"https://127.0.0.1",
		"https://169.254.169.254/latest/meta-data",
		"https://10.0.0.1",
	}
	for _, value := range unsafe {
		if err := validatePublicHTTPSURL(value); err == nil {
			t.Errorf("validatePublicHTTPSURL(%q) error = nil", value)
		}
	}
	if err := validatePublicHTTPSURL("https://id.example.com/tenant"); err != nil {
		t.Fatalf("validatePublicHTTPSURL(public) error = %v", err)
	}
}

func newRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func signIDToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "key-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func writeJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}
