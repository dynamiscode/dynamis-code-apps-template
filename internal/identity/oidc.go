package identity

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"example.com/dynamis-code/apps-template/internal/platform/telemetry"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type ExternalClaims struct {
	ProviderID string
	Issuer     string
	Subject    string
	Email      string
}

type oidcProvider struct {
	id                 string
	name               string
	issuer             string
	redirectURI        string
	oauth              oauth2.Config
	verifier           *oidc.IDTokenVerifier
	endSessionEndpoint string
	client             *http.Client
}

type OIDCRegistry struct {
	providers map[string]*oidcProvider
}

func NewOIDCRegistry(ctx context.Context, cfg config.OIDC) (*OIDCRegistry, error) {
	registry := &OIDCRegistry{providers: make(map[string]*oidcProvider)}
	if !cfg.Enabled {
		return registry, nil
	}
	if err := validatePublicHTTPSURL(cfg.IssuerURL); err != nil {
		return nil, fmt.Errorf("OIDC issuer is unsafe: %w", err)
	}
	client := safeOIDCHTTPClient()
	provider, err := buildOIDCProvider(ctx, cfg, client)
	if err != nil {
		return nil, err
	}
	if err := validatePublicHTTPSURL(provider.oauth.Endpoint.AuthURL); err != nil {
		return nil, fmt.Errorf("OIDC provider %q authorization endpoint is unsafe", cfg.ProviderID)
	}
	if err := validatePublicHTTPSURL(provider.oauth.Endpoint.TokenURL); err != nil {
		return nil, fmt.Errorf("OIDC provider %q token endpoint is unsafe", cfg.ProviderID)
	}
	if provider.endSessionEndpoint != "" {
		if err := validatePublicHTTPSURL(provider.endSessionEndpoint); err != nil {
			return nil, fmt.Errorf("OIDC provider %q logout endpoint is unsafe", cfg.ProviderID)
		}
	}
	registry.providers[provider.id] = provider
	return registry, nil
}

func buildOIDCProvider(
	ctx context.Context,
	cfg config.OIDC,
	client *http.Client,
) (*oidcProvider, error) {
	discoveryContext := oidc.ClientContext(ctx, client)
	discovered, err := oidc.NewProvider(discoveryContext, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider %q: %w", cfg.ProviderID, err)
	}
	var providerMetadata struct {
		Issuer             string `json:"issuer"`
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := discovered.Claims(&providerMetadata); err != nil {
		return nil, fmt.Errorf("read OIDC provider %q metadata", cfg.ProviderID)
	}
	if providerMetadata.Issuer != cfg.IssuerURL {
		return nil, fmt.Errorf("OIDC provider %q issuer mismatch", cfg.ProviderID)
	}
	oauth := oauth2.Config{
		ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret,
		Endpoint: discovered.Endpoint(), RedirectURL: cfg.RedirectURL,
		Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
	}
	return &oidcProvider{
		id: cfg.ProviderID, name: cfg.ProviderName, issuer: cfg.IssuerURL,
		redirectURI: cfg.RedirectURL, oauth: oauth,
		verifier:           discovered.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		endSessionEndpoint: providerMetadata.EndSessionEndpoint,
		client:             client,
	}, nil
}

type OIDCProviderInfo struct {
	ID   string
	Name string
}

func (r *OIDCRegistry) Providers() []OIDCProviderInfo {
	providers := make([]OIDCProviderInfo, 0, len(r.providers))
	for _, provider := range r.providers {
		providers = append(providers, OIDCProviderInfo{
			ID: provider.id, Name: provider.name,
		})
	}
	slices.SortFunc(providers, func(a, b OIDCProviderInfo) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return providers
}

func (r *OIDCRegistry) Begin(
	ctx context.Context,
	service *Service,
	providerID string,
	browserSession string,
) (OIDCTransaction, string, error) {
	provider, ok := r.providers[providerID]
	if !ok {
		return OIDCTransaction{}, "", ErrUnknownOIDCProvider
	}
	transaction, err := service.beginOIDCTransaction(
		ctx, providerID, browserSession, provider.redirectURI,
	)
	if err != nil {
		return OIDCTransaction{}, "", err
	}
	loginURL := provider.oauth.AuthCodeURL(
		transaction.State,
		oauth2.S256ChallengeOption(transaction.PKCEVerifier),
		oauth2.SetAuthURLParam("nonce", transaction.Nonce),
	)
	return transaction, loginURL, nil
}

func (r *OIDCRegistry) BeginLink(
	ctx context.Context,
	service *Service,
	providerID string,
	browserSession string,
	userID string,
) (OIDCTransaction, string, error) {
	provider, ok := r.providers[providerID]
	if !ok {
		return OIDCTransaction{}, "", ErrUnknownOIDCProvider
	}
	transaction, err := service.beginOIDCTransactionFor(
		ctx, providerID, browserSession, "link", userID, provider.redirectURI,
	)
	if err != nil {
		return OIDCTransaction{}, "", err
	}
	return transaction, provider.oauth.AuthCodeURL(
		transaction.State,
		oauth2.S256ChallengeOption(transaction.PKCEVerifier),
		oauth2.SetAuthURLParam("nonce", transaction.Nonce),
	), nil
}

func (r *OIDCRegistry) Complete(
	ctx context.Context,
	service *Service,
	providerID string,
	browserSession string,
	state string,
	pkceVerifier string,
	nonce string,
	redirectURI string,
	code string,
) (ExternalClaims, error) {
	completion, err := r.CompleteFlow(
		ctx, service, providerID, browserSession, state, pkceVerifier,
		nonce, redirectURI, code,
	)
	return completion.Claims, err
}

type OIDCCompletion struct {
	Claims  ExternalClaims
	Purpose string
	UserID  string
}

func (r *OIDCRegistry) CompleteFlow(
	ctx context.Context,
	service *Service,
	providerID string,
	browserSession string,
	state string,
	pkceVerifier string,
	nonce string,
	redirectURI string,
	code string,
) (OIDCCompletion, error) {
	provider, ok := r.providers[providerID]
	if !ok {
		return OIDCCompletion{}, ErrUnknownOIDCProvider
	}
	if redirectURI != provider.redirectURI {
		return OIDCCompletion{}, ErrOIDCTransaction
	}
	transaction, err := service.consumeOIDCTransactionFlow(
		ctx, providerID, browserSession, state, pkceVerifier, nonce, redirectURI,
	)
	if err != nil {
		return OIDCCompletion{}, err
	}
	token, err := provider.oauth.Exchange(
		oidc.ClientContext(ctx, provider.client),
		code,
		oauth2.VerifierOption(pkceVerifier),
	)
	if err != nil {
		return OIDCCompletion{}, errors.New("OIDC code exchange failed")
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return OIDCCompletion{}, errors.New("OIDC response has no ID token")
	}
	idToken, err := provider.verifier.Verify(
		oidc.ClientContext(ctx, provider.client), rawIDToken,
	)
	if err != nil {
		return OIDCCompletion{}, errors.New("OIDC ID token validation failed")
	}
	if idToken.Issuer != provider.issuer || idToken.Nonce != nonce ||
		idToken.Subject == "" {
		return OIDCCompletion{}, errors.New("OIDC ID token claims are invalid")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil || !claims.EmailVerified {
		return OIDCCompletion{}, errors.New("OIDC verified email is required")
	}
	email, err := normalizeEmail(claims.Email)
	if err != nil {
		return OIDCCompletion{}, errors.New("OIDC verified email is invalid")
	}
	return OIDCCompletion{
		Claims: ExternalClaims{
			ProviderID: providerID, Issuer: idToken.Issuer,
			Subject: idToken.Subject, Email: email,
		},
		Purpose: transaction.Purpose, UserID: transaction.UserID,
	}, nil
}

func (r *OIDCRegistry) LogoutURL(providerID string) (string, bool) {
	provider, ok := r.providers[providerID]
	if !ok || provider.endSessionEndpoint == "" {
		return "", false
	}
	return provider.endSessionEndpoint, true
}

func (r *OIDCRegistry) RedirectURL(providerID string) (string, bool) {
	provider, ok := r.providers[providerID]
	if !ok {
		return "", false
	}
	return provider.redirectURI, true
}

func (s *Service) beginOIDCTransaction(
	ctx context.Context,
	providerID string,
	browserSession string,
	redirectURI string,
) (OIDCTransaction, error) {
	return s.beginOIDCTransactionFor(ctx, providerID, browserSession, "login", "", redirectURI)
}

func (s *Service) beginOIDCTransactionFor(
	ctx context.Context,
	providerID string,
	browserSession string,
	purpose string,
	userID string,
	redirectURI string,
) (OIDCTransaction, error) {
	if browserSession == "" || (purpose != "login" && purpose != "link") ||
		(purpose == "link" && userID == "") || (purpose == "login" && userID != "") {
		return OIDCTransaction{}, ErrOIDCTransaction
	}
	state, err := newSecret()
	if err != nil {
		return OIDCTransaction{}, err
	}
	verifier, err := newSecret()
	if err != nil {
		return OIDCTransaction{}, err
	}
	nonce, err := newSecret()
	if err != nil {
		return OIDCTransaction{}, err
	}
	now := s.now().UTC()
	transaction := OIDCTransaction{
		ProviderID: providerID, BrowserSession: browserSession,
		Purpose: purpose, UserID: userID,
		State: state, PKCEVerifier: verifier, Nonce: nonce,
		RedirectURI: redirectURI, ExpiresAt: now.Add(defaultOIDCLifetime),
	}
	_, err = s.exec(ctx, s.db, `
		INSERT INTO oidc_transactions (
			state_hash, provider_id, browser_session_hash, pkce_verifier_hash,
			nonce_hash, redirect_uri, purpose, user_id, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, hashSecret(state), providerID, hashSecret(browserSession),
		hashSecret(verifier), hashSecret(nonce), redirectURI, purpose, nullable(userID),
		timestamp(now), timestamp(transaction.ExpiresAt))
	if err != nil {
		return OIDCTransaction{}, fmt.Errorf("create OIDC transaction: %w", err)
	}
	return transaction, nil
}

func (s *Service) consumeOIDCTransaction(
	ctx context.Context,
	providerID string,
	browserSession string,
	state string,
	verifier string,
	nonce string,
	redirectURI string,
) error {
	_, err := s.consumeOIDCTransactionFlow(
		ctx, providerID, browserSession, state, verifier, nonce, redirectURI,
	)
	return err
}

func (s *Service) consumeOIDCTransactionFlow(
	ctx context.Context,
	providerID string,
	browserSession string,
	state string,
	verifier string,
	nonce string,
	redirectURI string,
) (OIDCTransaction, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return OIDCTransaction{}, err
	}
	defer tx.Rollback()
	var storedProvider, browserHash, verifierHash, nonceHash, storedRedirect string
	var purpose string
	var userID sql.NullString
	var expiresAt string
	var consumedAt sql.NullString
	if err := s.queryRow(ctx, tx, `
	SELECT provider_id, browser_session_hash, pkce_verifier_hash,
		nonce_hash, redirect_uri, purpose, user_id, expires_at, consumed_at
		FROM oidc_transactions WHERE state_hash = ?
	`, hashSecret(state)).Scan(
		&storedProvider, &browserHash, &verifierHash, &nonceHash,
		&storedRedirect, &purpose, &userID, &expiresAt, &consumedAt,
	); err != nil {
		return OIDCTransaction{}, ErrOIDCTransaction
	}
	expires, err := parseTimestamp(expiresAt)
	if err != nil || consumedAt.Valid || !now.Before(expires) ||
		storedProvider != providerID || storedRedirect != redirectURI ||
		!equalSecretHash(browserSession, browserHash) ||
		!equalSecretHash(verifier, verifierHash) ||
		!equalSecretHash(nonce, nonceHash) {
		return OIDCTransaction{}, ErrOIDCTransaction
	}
	result, err := s.exec(ctx, tx, `
		UPDATE oidc_transactions SET consumed_at = ?
		WHERE state_hash = ? AND consumed_at IS NULL
	`, timestamp(now), hashSecret(state))
	if err != nil {
		return OIDCTransaction{}, ErrOIDCTransaction
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return OIDCTransaction{}, ErrOIDCTransaction
	}
	if err := tx.Commit(); err != nil {
		return OIDCTransaction{}, err
	}
	return OIDCTransaction{
		ProviderID: providerID, BrowserSession: browserSession,
		Purpose: purpose, UserID: userID.String, RedirectURI: redirectURI,
		ExpiresAt: expires,
	}, nil
}

func (s *Service) AuthenticateOIDC(
	ctx context.Context,
	claims ExternalClaims,
	audit AuditContext,
) (string, error) {
	if claims.Issuer == "" || claims.Subject == "" || claims.ProviderID == "" {
		return "", ErrInvalidCredentials
	}
	email, err := normalizeEmail(claims.Email)
	if err != nil {
		return "", ErrInvalidCredentials
	}
	var userID string
	err = s.queryRow(ctx, s.db, `
		SELECT user_id FROM external_identities
		WHERE issuer = ? AND subject = ?
	`, claims.Issuer, claims.Subject).Scan(&userID)
	if err == nil {
		return userID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var bootstrapped int
	if err := s.queryRow(ctx, tx,
		"SELECT 1 FROM bootstrap_state WHERE id = 1",
	).Scan(&bootstrapped); err != nil {
		return "", ErrInvalidCredentials
	}
	var existingID string
	var existingPassword sql.NullString
	if err := s.queryRow(ctx, tx,
		"SELECT id, password_hash FROM users WHERE email = ?", email,
	).Scan(&existingID, &existingPassword); err == nil {
		if existingPassword.Valid {
			return "", ErrInvalidCredentials
		}
		var provisioned int
		if err := s.queryRow(ctx, tx,
			"SELECT 1 FROM scim_users WHERE user_id = ? LIMIT 1", existingID,
		).Scan(&provisioned); err != nil {
			return "", ErrInvalidCredentials
		}
		var linked int
		if err := s.queryRow(ctx, tx,
			"SELECT 1 FROM external_identities WHERE user_id = ? LIMIT 1", existingID,
		).Scan(&linked); err == nil {
			return "", ErrInvalidCredentials
		} else if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		identityID, err := id.New()
		if err != nil {
			return "", err
		}
		now := s.now().UTC()
		if _, err := s.exec(ctx, tx, `
			INSERT INTO external_identities (
				id, user_id, provider_id, issuer, subject, email, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`, identityID, existingID, claims.ProviderID, claims.Issuer,
			claims.Subject, email, timestamp(now)); err != nil {
			return "", ErrInvalidCredentials
		}
		if _, err := s.exec(ctx, tx,
			"UPDATE users SET email_verified_at = COALESCE(email_verified_at, ?) WHERE id = ?",
			timestamp(now), existingID,
		); err != nil {
			return "", ErrInvalidCredentials
		}
		if err := s.audit(ctx, tx, AuditEvent{
			EventType: "external_identity.created", ActorUserID: existingID,
			AuthMethod: "oidc", TargetType: "external_identity", TargetID: identityID,
			Action: "external_identity.create", Outcome: "success",
			RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
			Metadata:  metadata(map[string]any{"provider_id": claims.ProviderID}),
			CreatedAt: now,
		}); err != nil {
			return "", err
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return existingID, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	userID, err = id.New()
	if err != nil {
		return "", err
	}
	identityID, err := id.New()
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	if _, err := s.exec(ctx, tx, `
		INSERT INTO users (id, email, password_hash, email_verified_at, created_at)
		VALUES (?, ?, NULL, ?, ?)
	`, userID, email, timestamp(now), timestamp(now)); err != nil {
		return "", ErrInvalidCredentials
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO external_identities (
			id, user_id, provider_id, issuer, subject, email, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, identityID, userID, claims.ProviderID, claims.Issuer,
		claims.Subject, email, timestamp(now)); err != nil {
		return "", ErrInvalidCredentials
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "external_identity.created", ActorUserID: userID,
		AuthMethod: "oidc", TargetType: "external_identity", TargetID: identityID,
		Action: "external_identity.create", Outcome: "success",
		RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
		Metadata:  metadata(map[string]any{"provider_id": claims.ProviderID}),
		CreatedAt: now,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return userID, nil
}

func (s *Service) LinkOIDCIdentity(
	ctx context.Context,
	principal Principal,
	claims ExternalClaims,
	reauthenticated bool,
	audit AuditContext,
) error {
	if !reauthenticated || principal.UserID == "" ||
		claims.Issuer == "" || claims.Subject == "" || claims.ProviderID == "" {
		return ErrForbidden
	}
	email, err := normalizeEmail(claims.Email)
	if err != nil {
		return ErrForbidden
	}
	identityID, err := id.New()
	if err != nil {
		return err
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.exec(ctx, tx, `
		INSERT INTO external_identities (
			id, user_id, provider_id, issuer, subject, email, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, identityID, principal.UserID, claims.ProviderID, claims.Issuer,
		claims.Subject, email, timestamp(now)); err != nil {
		return ErrForbidden
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "external_identity.linked", ActorUserID: principal.UserID,
		AuthMethod: principal.AuthMethod, TargetType: "external_identity",
		TargetID: identityID, Action: "external_identity.link",
		Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress,
		Metadata:      metadata(map[string]any{"provider_id": claims.ProviderID}),
		CreatedAt:     now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func safeOIDCHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, errors.New("invalid OIDC network destination")
			}
			addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil || len(addresses) == 0 {
				return nil, errors.New("resolve OIDC network destination")
			}
			for _, address := range addresses {
				if !publicIP(address.IP) {
					return nil, errors.New("OIDC network destination is not public")
				}
			}
			return dialer.DialContext(
				ctx, network, net.JoinHostPort(addresses[0].IP.String(), port),
			)
		},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
	}
	return &http.Client{
		Transport: telemetry.HTTPClientTransport(transport),
		Timeout:   10 * time.Second,
		CheckRedirect: func(request *http.Request, _ []*http.Request) error {
			return validatePublicHTTPSURL(request.URL.String())
		},
	}
}

func validatePublicHTTPSURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.Fragment != "" {
		return errors.New("URL must use HTTPS without user information or fragment")
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return errors.New("loopback host is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil && !publicIP(ip) {
		return errors.New("non-public address is not allowed")
	}
	return nil
}

func publicIP(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsPrivate() &&
		!ip.IsUnspecified() && !ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() && !ip.IsMulticast()
}
