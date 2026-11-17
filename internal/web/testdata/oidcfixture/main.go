//go:build oidcfixture

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/web"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func main() {
	ctx := context.Background()
	address := os.Getenv("OIDC_FIXTURE_ADDRESS")
	databasePath := os.Getenv("OIDC_FIXTURE_DATABASE")
	values := map[string]string{
		"DATABASE_DRIVER": "sqlite", "SQLITE_PATH": databasePath,
		"HTTP_ADDRESS": address, "MULTI_WORKSPACE_ENABLED": "true",
		"HTTP_REQUESTS_PER_MINUTE": "10000", "HTTP_AUTH_REQUESTS_PER_MINUTE": "1000",
	}
	cfg, err := config.LoadFrom(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	if err != nil {
		log.Fatal("load OIDC fixture configuration")
	}
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		log.Fatal("open OIDC fixture database")
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		log.Fatal("migrate OIDC fixture database")
	}
	identityService, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		log.Fatal("initialize OIDC fixture identity")
	}
	if _, err := identityService.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "oidc-fixture@example.com", Password: "oidc-fixture-password-123", WorkspaceName: "OIDC Fixture",
	}, identity.AuditContext{}); err != nil {
		log.Fatal("bootstrap OIDC fixture account")
	}
	callbackURL := "http://" + address + "/auth/oidc/company/callback"
	provider := newProviderFixture(callbackURL)
	defer provider.Close()
	issuer := provider.URL
	oidcRegistry, err := identity.NewOIDCRegistryForLoopbackTestFixture(ctx, config.OIDC{
		Enabled: true, ProviderID: "company", ProviderName: "Company SSO",
		IssuerURL: issuer, ClientID: "fixture-client", ClientSecret: "fixture-secret",
		RedirectURL: callbackURL,
	}, provider.Client())
	if err != nil {
		log.Fatal("initialize local OIDC provider fixture")
	}
	itemService := items.NewService(db, cfg.Database.Driver, identityService, cfg.Data.ItemsMaxPerWorkspace)
	handler, err := web.NewHandlerWithServices(
		identityService, itemService, nil, nil, oidcRegistry, cfg.HTTP, "", "", nil,
	)
	if err != nil {
		log.Fatal("initialize OIDC fixture web handler")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/ready", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/", handler.Routes())
	server := &http.Server{Addr: address, Handler: mux}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("serve OIDC fixture")
	}
}

func newProviderFixture(callbackURL string) *httptest.Server {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal("generate OIDC fixture signing key")
	}
	publicKey := jose.JSONWebKey{
		Key: &key.PublicKey, KeyID: "fixture-key", Algorithm: string(jose.RS256), Use: "sig",
	}
	var issuer string
	var mu sync.Mutex
	txns := make(map[string]struct{ nonce, challenge string })
	provider := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
				"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys",
				"response_types_supported":              []string{"code"},
				"subject_types_supported":               []string{"public"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/authorize":
			query := request.URL.Query()
			state := query.Get("state")
			if query.Get("response_type") != "code" || query.Get("client_id") != "fixture-client" ||
				query.Get("redirect_uri") != callbackURL || query.Get("nonce") == "" ||
				query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" || state == "" {
				http.Error(writer, "invalid authorization request", http.StatusBadRequest)
				return
			}
			code := "fixture-code-" + state
			mu.Lock()
			txns[code] = struct{ nonce, challenge string }{query.Get("nonce"), query.Get("code_challenge")}
			mu.Unlock()
			redirect, err := url.Parse(callbackURL)
			if err != nil {
				http.Error(writer, "invalid callback", http.StatusBadRequest)
				return
			}
			params := redirect.Query()
			params.Set("code", code)
			params.Set("state", state)
			redirect.RawQuery = params.Encode()
			http.Redirect(writer, request, redirect.String(), http.StatusFound)
		case "/keys":
			_ = json.NewEncoder(writer).Encode(map[string]any{"keys": []jose.JSONWebKey{publicKey}})
		case "/token":
			if err := request.ParseForm(); err != nil || request.Form.Get("grant_type") != "authorization_code" ||
				request.Form.Get("redirect_uri") != callbackURL {
				http.Error(writer, "invalid token request", http.StatusBadRequest)
				return
			}
			code := request.Form.Get("code")
			mu.Lock()
			txn, ok := txns[code]
			delete(txns, code)
			mu.Unlock()
			verifier := request.Form.Get("code_verifier")
			digest := sha256.Sum256([]byte(verifier))
			if !ok || verifier == "" || txn.challenge != base64.RawURLEncoding.EncodeToString(digest[:]) {
				http.Error(writer, "invalid authorization code", http.StatusBadRequest)
				return
			}
			token, err := signIDToken(key, issuer, txn.nonce)
			if err != nil {
				http.Error(writer, "could not sign fixture token", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"access_token": "fixture-access-token", "token_type": "Bearer", "id_token": token,
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	issuer = provider.URL
	return provider
}

func signIDToken(key *rsa.PrivateKey, issuer, nonce string) (string, error) {
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "fixture-key"),
	)
	if err != nil {
		return "", err
	}
	now := time.Now()
	return jwt.Signed(signer).Claims(map[string]any{
		"iss": issuer, "sub": "fixture-user", "aud": "fixture-client",
		"exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix(), "nonce": nonce,
		"email": "oidc-user@example.com", "email_verified": true,
	}).Serialize()
}
