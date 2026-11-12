package identity

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

// OAuth lifetimes for MCP grants. Codes are single-use; access tokens are short
// and refresh tokens rotate on every use.
const (
	oauthCodeLifetime    = 60 * time.Second
	oauthRefreshLifetime = 30 * 24 * time.Hour
	oauthMaxRedirectURIs = 5

	// OAuthAccessLifetime is the MCP access-token lifetime advertised as expires_in.
	OAuthAccessLifetime = 15 * time.Minute
)

var (
	ErrOAuthInvalidRequest = errors.New("oauth request is invalid")
	ErrOAuthInvalidClient  = errors.New("oauth client is invalid")
	ErrOAuthInvalidGrant   = errors.New("oauth grant is invalid or expired")
	ErrOAuthClientLimit    = errors.New("oauth client registration limit reached")
)

// defaultOAuthClientLimit bounds dynamically registered clients per instance.
const defaultOAuthClientLimit = 1000

// OAuthClient is a dynamically registered MCP client. Clients are public and
// authenticate with PKCE rather than a client secret.
type OAuthClient struct {
	ID           int64
	ClientID     string
	Name         string
	RedirectURIs []string
}

// OAuthTokenSet is returned once per exchange or refresh. The raw secrets are
// never stored; only their SHA-256 hashes are persisted.
type OAuthTokenSet struct {
	AccessToken     string
	RefreshToken    string
	AccessExpiresAt time.Time
	Scopes          []Permission
}

func (s *Service) RegisterOAuthClient(
	ctx context.Context,
	name string,
	redirectURIs []string,
	audit AuditContext,
) (OAuthClient, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return OAuthClient{}, ErrOAuthInvalidRequest
	}
	if len(redirectURIs) == 0 || len(redirectURIs) > oauthMaxRedirectURIs {
		return OAuthClient{}, ErrOAuthInvalidRequest
	}
	for _, redirect := range redirectURIs {
		if !validOAuthRedirectURI(redirect) {
			return OAuthClient{}, ErrOAuthInvalidRequest
		}
	}
	clientID, err := id.New()
	if err != nil {
		return OAuthClient{}, err
	}
	now := s.now().UTC()
	encoded := strings.Join(redirectURIs, "\n")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OAuthClient{}, err
	}
	defer tx.Rollback()
	var registered int
	if err := s.queryRow(ctx, tx, `
		SELECT COUNT(*) FROM oauth_clients WHERE deleted_at IS NULL
	`).Scan(&registered); err != nil {
		return OAuthClient{}, err
	}
	if registered >= s.oauthClientLimit {
		return OAuthClient{}, ErrOAuthClientLimit
	}
	var rowID int64
	if err := s.queryRow(ctx, tx, `
		INSERT INTO oauth_clients (client_id, name, redirect_uris, created_at)
		VALUES (?, ?, ?, ?)
		RETURNING id
	`, clientID, name, encoded, timestamp(now)).Scan(&rowID); err != nil {
		return OAuthClient{}, err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "oauth.client.registered", AuthMethod: "oauth",
		TargetType: "oauth_client", TargetID: id.EncodeKey(id.TypeOAuthClient, rowID), Action: "oauth.client.register",
		Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
		Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return OAuthClient{}, err
	}
	if err := tx.Commit(); err != nil {
		return OAuthClient{}, err
	}
	return OAuthClient{ID: rowID, ClientID: clientID, Name: name, RedirectURIs: redirectURIs}, nil
}

// ApproveOAuthConsent records the user's consent to one workspace and a set of
// MCP scopes, then returns a single-use authorization code. The caller must have
// already authenticated the browser session and verified CSRF.
func (s *Service) ApproveOAuthConsent(
	ctx context.Context,
	session Session,
	workspaceID WorkspaceID,
	clientID string,
	scopes []Permission,
	redirectURI string,
	codeChallenge string,
	resource string,
	audit AuditContext,
) (string, error) {
	now := s.now().UTC()
	if session.RevokedAt != nil || !now.Before(session.ExpiresAt) || session.UserID == "" {
		return "", ErrForbidden
	}
	if len(scopes) == 0 || !mcpScopesOnly(scopes) || codeChallenge == "" || !validOAuthResource(resource) {
		return "", ErrOAuthInvalidRequest
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var clientRowID int64
	var redirectsEncoded string
	if err := s.queryRow(ctx, tx, `
		SELECT id, redirect_uris FROM oauth_clients
		WHERE client_id = ? AND deleted_at IS NULL
	`, clientID).Scan(&clientRowID, &redirectsEncoded); err != nil {
		return "", ErrOAuthInvalidClient
	}
	if !containsLine(redirectsEncoded, redirectURI) {
		return "", ErrOAuthInvalidRequest
	}
	for _, scope := range scopes {
		if _, err := s.authorize(ctx, tx, session.UserID, workspaceID, scope); err != nil {
			return "", ErrForbidden
		}
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO oauth_grants (user_id, workspace_id, client_id, session_id, scopes, auth_level, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, workspace_id, client_id) DO UPDATE SET
			session_id = excluded.session_id,
			scopes = excluded.scopes,
			auth_level = excluded.auth_level,
			created_at = excluded.created_at,
			revoked_at = NULL
	`, session.UserID, workspaceID, clientRowID, session.ID,
		encodeScopes(scopes), int(session.AuthLevel), timestamp(now)); err != nil {
		return "", err
	}
	var storedGrantID int64
	if err := s.queryRow(ctx, tx, `
		SELECT id FROM oauth_grants WHERE user_id = ? AND workspace_id = ? AND client_id = ?
	`, session.UserID, workspaceID, clientRowID).Scan(&storedGrantID); err != nil {
		return "", err
	}
	// Re-consent replaces any earlier tokens for the same grant.
	if _, err := s.exec(ctx, tx, `
		UPDATE oauth_tokens SET revoked_at = ? WHERE grant_id = ? AND revoked_at IS NULL
	`, timestamp(now), storedGrantID); err != nil {
		return "", err
	}
	code, err := newSecret()
	if err != nil {
		return "", err
	}
	codeID, err := id.New()
	if err != nil {
		return "", err
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO oauth_tokens (
			id, grant_id, kind, secret_hash, client_id, redirect_uri, code_challenge, resource,
			created_at, expires_at
		) VALUES (?, ?, 'code', ?, ?, ?, ?, ?, ?, ?)
	`, codeID, storedGrantID, hashSecret(code), clientRowID, redirectURI, codeChallenge, resource,
		timestamp(now), timestamp(now.Add(oauthCodeLifetime))); err != nil {
		return "", err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "oauth.grant.created", ActorUserID: session.UserID, AuthMethod: "oauth",
		WorkspaceID: workspaceID, TargetType: "oauth_grant", TargetID: id.EncodeKey(id.TypeOAuthGrant, storedGrantID),
		Action: "oauth.grant.create", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return code, nil
}

// ExchangeOAuthCode redeems an authorization code. A failed binding check leaves
// the code unused; a second redemption of a consumed code revokes its grant.
func (s *Service) ExchangeOAuthCode(
	ctx context.Context,
	code string,
	clientID string,
	redirectURI string,
	codeVerifier string,
	resource string,
	audit AuditContext,
) (OAuthTokenSet, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OAuthTokenSet{}, err
	}
	defer tx.Rollback()
	row, err := s.loadOAuthSecret(ctx, tx, code, "code")
	if err != nil {
		return OAuthTokenSet{}, ErrOAuthInvalidGrant
	}
	if row.clientPublicID != clientID {
		return OAuthTokenSet{}, ErrOAuthInvalidClient
	}
	if row.consumed.Valid {
		if err := s.revokeOAuthGrant(ctx, tx, row.grantID, now, audit, "oauth.code.reuse"); err != nil {
			return OAuthTokenSet{}, err
		}
		if err := tx.Commit(); err != nil {
			return OAuthTokenSet{}, err
		}
		return OAuthTokenSet{}, ErrOAuthInvalidGrant
	}
	if !row.usable(now) || row.redirectURI != redirectURI || row.resource != resource ||
		!pkceMatches(codeVerifier, row.codeChallenge) {
		return OAuthTokenSet{}, ErrOAuthInvalidGrant
	}
	if err := s.consumeOAuthSecret(ctx, tx, row.tokenID, now); err != nil {
		return OAuthTokenSet{}, err
	}
	tokens, err := s.issueOAuthTokens(ctx, tx, row.grantID, row.clientRowID, "", row.scopes, now)
	if err != nil {
		return OAuthTokenSet{}, err
	}
	if err := tx.Commit(); err != nil {
		return OAuthTokenSet{}, err
	}
	return tokens, nil
}

// RefreshOAuthToken rotates a refresh token. Presenting an already-rotated
// refresh token revokes the whole grant.
func (s *Service) RefreshOAuthToken(
	ctx context.Context,
	refresh string,
	clientID string,
	audit AuditContext,
) (OAuthTokenSet, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OAuthTokenSet{}, err
	}
	defer tx.Rollback()
	row, err := s.loadOAuthSecret(ctx, tx, refresh, "refresh")
	if err != nil {
		return OAuthTokenSet{}, ErrOAuthInvalidGrant
	}
	if row.clientPublicID != clientID {
		return OAuthTokenSet{}, ErrOAuthInvalidClient
	}
	if row.consumed.Valid {
		if err := s.revokeOAuthGrant(ctx, tx, row.grantID, now, audit, "oauth.refresh.reuse"); err != nil {
			return OAuthTokenSet{}, err
		}
		if err := tx.Commit(); err != nil {
			return OAuthTokenSet{}, err
		}
		return OAuthTokenSet{}, ErrOAuthInvalidGrant
	}
	if !row.usable(now) {
		return OAuthTokenSet{}, ErrOAuthInvalidGrant
	}
	if err := s.consumeOAuthSecret(ctx, tx, row.tokenID, now); err != nil {
		return OAuthTokenSet{}, err
	}
	tokens, err := s.issueOAuthTokens(ctx, tx, row.grantID, row.clientRowID, row.tokenID, row.scopes, now)
	if err != nil {
		return OAuthTokenSet{}, err
	}
	if err := tx.Commit(); err != nil {
		return OAuthTokenSet{}, err
	}
	return tokens, nil
}

// AuthenticateOAuthAccessToken resolves an MCP access token to a principal. It
// checks the live session, membership, role, and MFA policy on every use, so
// session revocation, password change, or membership removal ends access
// without separate revocation hooks.
func (s *Service) AuthenticateOAuthAccessToken(ctx context.Context, token string) (Principal, error) {
	now := s.now().UTC()
	var tokenID, userID, encodedScopes string
	var workspaceID WorkspaceID
	var tokenExpires, tokenRevoked, grantRevoked, sessionRevoked, sessionExpires sql.NullString
	var authLevel int
	if err := s.queryRow(ctx, s.db, `
		SELECT t.id, g.user_id, g.workspace_id, g.scopes, g.auth_level,
			t.expires_at, t.revoked_at, g.revoked_at, s.revoked_at, s.expires_at
		FROM oauth_tokens t
		JOIN oauth_grants g ON g.id = t.grant_id
		JOIN sessions s ON s.id = g.session_id
		WHERE t.secret_hash = ? AND t.kind = 'access'
	`, hashSecret(token)).Scan(
		&tokenID, &userID, &workspaceID, &encodedScopes, &authLevel,
		&tokenExpires, &tokenRevoked, &grantRevoked, &sessionRevoked, &sessionExpires,
	); err != nil {
		return Principal{}, ErrInvalidToken
	}
	if tokenRevoked.Valid || grantRevoked.Valid || sessionRevoked.Valid ||
		isExpired(now, tokenExpires) || isExpired(now, sessionExpires) {
		return Principal{}, ErrInvalidToken
	}
	var role Role
	if err := s.queryRow(ctx, s.db, `
		SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?
	`, workspaceID, userID).Scan(&role); err != nil {
		return Principal{}, ErrInvalidToken
	}
	if required, err := s.mfaRequiredForRoleQuery(ctx, s.db, userID, role); err != nil {
		return Principal{}, err
	} else if required {
		return Principal{}, ErrMFARequired
	}
	allowed := permissionsForRole(role)
	permissions := make(map[Permission]bool)
	for _, scope := range decodeScopes(encodedScopes) {
		if allowed[scope] {
			permissions[scope] = true
		}
	}
	return Principal{
		UserID: userID, WorkspaceID: workspaceID, Role: role,
		Permissions: permissions, AuthMethod: "mcp_oauth", TokenID: tokenID,
		AuthLevel: AuthLevel(authLevel),
	}, nil
}

// RevokeOAuthToken revokes the grant that owns a token (RFC 7009). Unknown tokens
// succeed silently so callers cannot probe for valid secrets.
func (s *Service) RevokeOAuthToken(ctx context.Context, token string, audit AuditContext) error {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var grantID int64
	if err := s.queryRow(ctx, tx, `
		SELECT grant_id FROM oauth_tokens WHERE secret_hash = ? AND kind IN ('access', 'refresh')
	`, hashSecret(token)).Scan(&grantID); err != nil {
		return nil
	}
	if err := s.revokeOAuthGrant(ctx, tx, grantID, now, audit, "oauth.grant.revoke"); err != nil {
		return err
	}
	return tx.Commit()
}

type oauthSecretRow struct {
	tokenID        string
	grantID        int64
	clientRowID    int64
	clientPublicID string
	redirectURI    string
	codeChallenge  string
	resource       string
	userID         string
	workspaceID    WorkspaceID
	scopes         []Permission
	expiresAt      sql.NullString
	consumed       sql.NullString
	revoked        sql.NullString
	grantRevoked   sql.NullString
	sessionRevoked sql.NullString
	sessionExpires sql.NullString
}

func (row oauthSecretRow) usable(now time.Time) bool {
	return !row.revoked.Valid && !row.grantRevoked.Valid && !row.sessionRevoked.Valid &&
		!isExpired(now, row.expiresAt) && !isExpired(now, row.sessionExpires)
}

func (s *Service) loadOAuthSecret(
	ctx context.Context,
	queryer rowQueryer,
	secret string,
	kind string,
) (oauthSecretRow, error) {
	var row oauthSecretRow
	var redirect, challenge, resource sql.NullString
	var encodedScopes string
	if err := s.queryRow(ctx, queryer, `
		SELECT t.id, t.grant_id, t.client_id, c.client_id, t.redirect_uri, t.code_challenge, t.resource,
			g.user_id, g.workspace_id, g.scopes, t.expires_at, t.consumed_at, t.revoked_at,
			g.revoked_at, s.revoked_at, s.expires_at
		FROM oauth_tokens t
		JOIN oauth_grants g ON g.id = t.grant_id
		JOIN oauth_clients c ON c.id = t.client_id
		JOIN sessions s ON s.id = g.session_id
		WHERE t.secret_hash = ? AND t.kind = ?
	`, hashSecret(secret), kind).Scan(
		&row.tokenID, &row.grantID, &row.clientRowID, &row.clientPublicID, &redirect, &challenge, &resource,
		&row.userID, &row.workspaceID, &encodedScopes, &row.expiresAt, &row.consumed, &row.revoked,
		&row.grantRevoked, &row.sessionRevoked, &row.sessionExpires,
	); err != nil {
		return oauthSecretRow{}, err
	}
	row.redirectURI = redirect.String
	row.codeChallenge = challenge.String
	row.resource = resource.String
	row.scopes = decodeScopes(encodedScopes)
	return row, nil
}

func (s *Service) consumeOAuthSecret(ctx context.Context, tx *sql.Tx, tokenID string, now time.Time) error {
	result, err := s.exec(ctx, tx, `
		UPDATE oauth_tokens SET consumed_at = ? WHERE id = ? AND consumed_at IS NULL
	`, timestamp(now), tokenID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ErrOAuthInvalidGrant
	}
	return nil
}

func (s *Service) issueOAuthTokens(
	ctx context.Context,
	tx *sql.Tx,
	grantID int64,
	clientRowID int64,
	parentID string,
	scopes []Permission,
	now time.Time,
) (OAuthTokenSet, error) {
	access, err := newSecret()
	if err != nil {
		return OAuthTokenSet{}, err
	}
	refresh, err := newSecret()
	if err != nil {
		return OAuthTokenSet{}, err
	}
	accessID, err := id.New()
	if err != nil {
		return OAuthTokenSet{}, err
	}
	refreshID, err := id.New()
	if err != nil {
		return OAuthTokenSet{}, err
	}
	var parent any
	if parentID != "" {
		parent = parentID
	}
	accessExpires := now.Add(OAuthAccessLifetime)
	if _, err := s.exec(ctx, tx, `
		INSERT INTO oauth_tokens (id, grant_id, kind, secret_hash, client_id, parent_id, created_at, expires_at)
		VALUES (?, ?, 'access', ?, ?, ?, ?, ?)
	`, accessID, grantID, hashSecret(access), clientRowID, parent, timestamp(now), timestamp(accessExpires)); err != nil {
		return OAuthTokenSet{}, err
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO oauth_tokens (id, grant_id, kind, secret_hash, client_id, parent_id, created_at, expires_at)
		VALUES (?, ?, 'refresh', ?, ?, ?, ?, ?)
	`, refreshID, grantID, hashSecret(refresh), clientRowID, parent, timestamp(now),
		timestamp(now.Add(oauthRefreshLifetime))); err != nil {
		return OAuthTokenSet{}, err
	}
	return OAuthTokenSet{
		AccessToken: access, RefreshToken: refresh, AccessExpiresAt: accessExpires, Scopes: scopes,
	}, nil
}

func (s *Service) revokeOAuthGrant(
	ctx context.Context,
	tx *sql.Tx,
	grantID int64,
	now time.Time,
	audit AuditContext,
	action string,
) error {
	if _, err := s.exec(ctx, tx, `
		UPDATE oauth_grants SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL
	`, timestamp(now), grantID); err != nil {
		return err
	}
	if _, err := s.exec(ctx, tx, `
		UPDATE oauth_tokens SET revoked_at = ? WHERE grant_id = ? AND revoked_at IS NULL
	`, timestamp(now), grantID); err != nil {
		return err
	}
	return s.audit(ctx, tx, AuditEvent{
		EventType: action, AuthMethod: "oauth", TargetType: "oauth_grant", TargetID: id.EncodeKey(id.TypeOAuthGrant, grantID),
		Action: action, Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	})
}

func pkceMatches(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(pkceChallenge(verifier)), []byte(challenge)) == 1
}

func mcpScopesOnly(scopes []Permission) bool {
	for _, scope := range scopes {
		if scope != ResourcesRead && scope != ResourcesWrite {
			return false
		}
	}
	return true
}

func containsLine(encoded, value string) bool {
	for _, line := range strings.Split(encoded, "\n") {
		if line == value {
			return true
		}
	}
	return false
}

func validOAuthResource(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" &&
		parsed.User == nil && parsed.Fragment == ""
}

func validOAuthRedirectURI(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" ||
		strings.ContainsAny(value, "\n\r\t ") {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(parsed.Hostname())
	return parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || (ip != nil && ip.IsLoopback()))
}

// OAuthClientByID returns an active registered client for the consent page and
// token exchange checks. Unknown or deleted clients return ErrOAuthInvalidClient.
func (s *Service) OAuthClientByID(ctx context.Context, clientID string) (OAuthClient, error) {
	var client OAuthClient
	var encoded string
	if err := s.queryRow(ctx, s.db, `
		SELECT id, client_id, name, redirect_uris FROM oauth_clients
		WHERE client_id = ? AND deleted_at IS NULL
	`, clientID).Scan(&client.ID, &client.ClientID, &client.Name, &encoded); err != nil {
		return OAuthClient{}, ErrOAuthInvalidClient
	}
	client.RedirectURIs = strings.Split(encoded, "\n")
	return client, nil
}
