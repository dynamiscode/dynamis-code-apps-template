package identity

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

const (
	testCodeVerifier = "verifier-with-enough-entropy-0123456789abcdef"
	testRedirectURI  = "https://client.example/callback"
	testResource     = "https://app.example.com/mcp"
)

type oauthFixture struct {
	service     *Service
	db          *sql.DB
	ownerID     string
	workspaceID string
	session     NewSession
	client      OAuthClient
}

func newOAuthFixture(t *testing.T) (oauthFixture, func(time.Time)) {
	t.Helper()
	service, db := newTestService(t)
	owner := bootstrapOwner(t, service)
	session, err := service.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := service.RegisterOAuthClient(context.Background(), "Test client", []string{testRedirectURI}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	setNow := func(value time.Time) { service.now = func() time.Time { return value } }
	return oauthFixture{
		service: service, db: db, ownerID: owner.UserID, workspaceID: owner.WorkspaceID,
		session: session, client: client,
	}, setNow
}

func (f oauthFixture) approve(t *testing.T, scopes []Permission) string {
	t.Helper()
	code, err := f.service.ApproveOAuthConsent(context.Background(), f.session.Session, f.workspaceID,
		f.client.ClientID, scopes, testRedirectURI, pkceChallenge(testCodeVerifier), testResource, AuditContext{})
	if err != nil {
		t.Fatalf("ApproveOAuthConsent() error = %v", err)
	}
	return code
}

func (f oauthFixture) exchange(t *testing.T, code string) OAuthTokenSet {
	t.Helper()
	tokens, err := f.service.ExchangeOAuthCode(context.Background(), code, f.client.ClientID,
		testRedirectURI, testCodeVerifier, testResource, AuditContext{})
	if err != nil {
		t.Fatalf("ExchangeOAuthCode() error = %v", err)
	}
	return tokens
}

func TestOAuthRegisterClientValidatesRedirectURIs(t *testing.T) {
	t.Parallel()
	service, _ := newTestService(t)
	ctx := context.Background()

	client, err := service.RegisterOAuthClient(ctx, "Desktop", []string{
		"https://client.example/callback", "http://127.0.0.1:8765/callback",
	}, AuditContext{})
	if err != nil || client.ClientID == "" || len(client.RedirectURIs) != 2 {
		t.Fatalf("RegisterOAuthClient() = %#v, %v", client, err)
	}
	for name, input := range map[string]struct {
		name string
		uris []string
	}{
		"empty name":        {"", []string{testRedirectURI}},
		"remote plain HTTP": {"Client", []string{"http://client.example/cb"}},
		"fragment":          {"Client", []string{"https://client.example/cb#x"}},
		"no redirects":      {"Client", nil},
		"relative":          {"Client", []string{"/callback"}},
	} {
		if _, err := service.RegisterOAuthClient(ctx, input.name, input.uris, AuditContext{}); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestOAuthPKCECodeAndScopeBindings(t *testing.T) {
	t.Parallel()
	fixture, _ := newOAuthFixture(t)
	ctx := context.Background()

	code := fixture.approve(t, []Permission{ResourcesRead})
	if _, err := fixture.service.ExchangeOAuthCode(ctx, code, fixture.client.ClientID,
		testRedirectURI, "wrong-verifier-value-0123456789abcdef", testResource, AuditContext{}); !errors.Is(err, ErrOAuthInvalidGrant) {
		t.Fatalf("wrong PKCE verifier error = %v", err)
	}
	if _, err := fixture.service.ExchangeOAuthCode(ctx, code, fixture.client.ClientID,
		"https://other.example/cb", testCodeVerifier, testResource, AuditContext{}); !errors.Is(err, ErrOAuthInvalidGrant) {
		t.Fatalf("wrong redirect URI error = %v", err)
	}
	if _, err := fixture.service.ExchangeOAuthCode(ctx, code, fixture.client.ClientID,
		testRedirectURI, testCodeVerifier, "https://other.example/mcp", AuditContext{}); !errors.Is(err, ErrOAuthInvalidGrant) {
		t.Fatalf("wrong resource error = %v", err)
	}
	if _, err := fixture.service.ExchangeOAuthCode(ctx, code, "other-client",
		testRedirectURI, testCodeVerifier, testResource, AuditContext{}); !errors.Is(err, ErrOAuthInvalidClient) {
		t.Fatalf("wrong client error = %v", err)
	}

	tokens := fixture.exchange(t, code)
	principal, err := fixture.service.AuthenticateOAuthAccessToken(ctx, tokens.AccessToken)
	if err != nil {
		t.Fatalf("AuthenticateOAuthAccessToken() error = %v", err)
	}
	if principal.AuthMethod != "mcp_oauth" || principal.UserID != fixture.ownerID ||
		principal.WorkspaceID != fixture.workspaceID || !principal.Permissions[ResourcesRead] ||
		principal.Permissions[ResourcesWrite] {
		t.Fatalf("principal = %#v, want read-only MCP OAuth principal", principal)
	}
	assertDatabaseDoesNotContain(t, fixture.db, code)
	assertDatabaseDoesNotContain(t, fixture.db, tokens.AccessToken)
	assertDatabaseDoesNotContain(t, fixture.db, tokens.RefreshToken)
}

func TestOAuthCodeReuseRevokesGrant(t *testing.T) {
	t.Parallel()
	fixture, _ := newOAuthFixture(t)
	ctx := context.Background()

	code := fixture.approve(t, []Permission{ResourcesRead})
	tokens := fixture.exchange(t, code)
	if _, err := fixture.service.ExchangeOAuthCode(ctx, code, fixture.client.ClientID,
		testRedirectURI, testCodeVerifier, testResource, AuditContext{}); !errors.Is(err, ErrOAuthInvalidGrant) {
		t.Fatalf("code reuse error = %v", err)
	}
	if _, err := fixture.service.AuthenticateOAuthAccessToken(ctx, tokens.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("access token after code reuse = %v, want ErrInvalidToken", err)
	}
}

func TestOAuthRefreshRotatesAndDetectsReuse(t *testing.T) {
	t.Parallel()
	fixture, _ := newOAuthFixture(t)
	ctx := context.Background()

	first := fixture.exchange(t, fixture.approve(t, []Permission{ResourcesRead}))
	second, err := fixture.service.RefreshOAuthToken(ctx, first.RefreshToken, fixture.client.ClientID, AuditContext{})
	if err != nil || second.AccessToken == first.AccessToken || second.RefreshToken == first.RefreshToken {
		t.Fatalf("RefreshOAuthToken() = %#v, %v", second, err)
	}
	if _, err := fixture.service.RefreshOAuthToken(ctx, first.RefreshToken, fixture.client.ClientID, AuditContext{}); !errors.Is(err, ErrOAuthInvalidGrant) {
		t.Fatalf("old refresh token error = %v", err)
	}
	if _, err := fixture.service.AuthenticateOAuthAccessToken(ctx, second.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("rotated access token after reuse = %v, want ErrInvalidToken", err)
	}
}

func TestOAuthAccessFollowsExpiryMembershipAndSession(t *testing.T) {
	t.Parallel()
	fixture, setNow := newOAuthFixture(t)
	ctx := context.Background()
	start := fixture.service.now()

	tokens := fixture.exchange(t, fixture.approve(t, []Permission{ResourcesRead}))
	setNow(start.Add(16 * time.Minute))
	if _, err := fixture.service.AuthenticateOAuthAccessToken(ctx, tokens.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired access token error = %v", err)
	}
	setNow(start)

	fresh := fixture.exchange(t, fixture.approve(t, []Permission{ResourcesRead}))
	if _, err := fixture.service.AuthenticateOAuthAccessToken(ctx, fresh.AccessToken); err != nil {
		t.Fatalf("fresh access token error = %v", err)
	}
	if _, err := fixture.db.Exec(
		"UPDATE sessions SET revoked_at = ? WHERE id = ?", timestamp(start), fixture.session.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.AuthenticateOAuthAccessToken(ctx, fresh.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("access token after session revocation = %v, want ErrInvalidToken", err)
	}
}

func TestOAuthMembershipRemovalDeniesAccess(t *testing.T) {
	t.Parallel()
	fixture, _ := newOAuthFixture(t)
	ctx := context.Background()
	tokens := fixture.exchange(t, fixture.approve(t, []Permission{ResourcesRead}))

	if _, err := fixture.db.Exec(
		"DELETE FROM workspace_members WHERE workspace_id = ? AND user_id = ?",
		fixture.workspaceID, fixture.ownerID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.AuthenticateOAuthAccessToken(ctx, tokens.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("access token after membership removal = %v, want ErrInvalidToken", err)
	}
}

func TestOAuthConsentRequiresWorkspacePermission(t *testing.T) {
	t.Parallel()
	fixture, _ := newOAuthFixture(t)
	ctx := context.Background()
	viewerID := insertUser(t, fixture.service, fixture.db, "viewer@example.com")
	insertMembership(t, fixture.service, fixture.db, fixture.workspaceID, viewerID, Viewer)
	viewerSession, err := fixture.service.CreateSession(ctx, viewerID, "local", "", time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := fixture.service.ApproveOAuthConsent(ctx, viewerSession.Session, fixture.workspaceID,
		fixture.client.ClientID, []Permission{ResourcesWrite}, testRedirectURI,
		pkceChallenge(testCodeVerifier), testResource, AuditContext{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer write consent error = %v, want ErrForbidden", err)
	}
}

func TestOAuthRevokeAndRefreshAfterRevoke(t *testing.T) {
	t.Parallel()
	fixture, _ := newOAuthFixture(t)
	ctx := context.Background()
	tokens := fixture.exchange(t, fixture.approve(t, []Permission{ResourcesRead}))

	if err := fixture.service.RevokeOAuthToken(ctx, tokens.RefreshToken, AuditContext{}); err != nil {
		t.Fatalf("RevokeOAuthToken() error = %v", err)
	}
	if _, err := fixture.service.AuthenticateOAuthAccessToken(ctx, tokens.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("access token after revoke = %v, want ErrInvalidToken", err)
	}
	if _, err := fixture.service.RefreshOAuthToken(ctx, tokens.RefreshToken, fixture.client.ClientID, AuditContext{}); !errors.Is(err, ErrOAuthInvalidGrant) {
		t.Fatalf("refresh after revoke = %v", err)
	}
	if err := fixture.service.RevokeOAuthToken(ctx, "unknown-token-value", AuditContext{}); err != nil {
		t.Fatalf("unknown token revoke = %v, want nil (RFC 7009)", err)
	}
}

func TestOAuthClientLookupAndRegistrationLimit(t *testing.T) {
	t.Parallel()
	service, _ := newTestService(t)
	ctx := context.Background()
	service.oauthClientLimit = 2

	client, err := service.RegisterOAuthClient(ctx, "Lookup", []string{testRedirectURI}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	found, err := service.OAuthClientByID(ctx, client.ClientID)
	if err != nil || found.Name != "Lookup" || found.RedirectURIs[0] != testRedirectURI {
		t.Fatalf("OAuthClientByID() = %#v, %v", found, err)
	}
	if _, err := service.OAuthClientByID(ctx, "missing-client"); !errors.Is(err, ErrOAuthInvalidClient) {
		t.Fatalf("missing client error = %v", err)
	}
	if _, err := service.RegisterOAuthClient(ctx, "Second", []string{testRedirectURI}, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RegisterOAuthClient(ctx, "Third", []string{testRedirectURI}, AuditContext{}); !errors.Is(err, ErrOAuthClientLimit) {
		t.Fatalf("third registration = %v, want ErrOAuthClientLimit", err)
	}
}
