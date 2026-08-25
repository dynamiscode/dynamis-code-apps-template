package identity

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"golang.org/x/oauth2"
)

func TestPasswordHashUsesArgon2idDefaults(t *testing.T) {
	t.Parallel()

	encoded, err := hashPassword("correct horse battery staple", defaultPasswordParams)
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	params, _, _, err := parsePasswordHash(encoded)
	if err != nil {
		t.Fatalf("parsePasswordHash() error = %v", err)
	}
	if params.memory != 19*1024 || params.iterations != 2 || params.parallelism != 1 {
		t.Fatalf("Argon2 parameters = %+v", params)
	}
	if !verifyPassword("correct horse battery staple", encoded) {
		t.Fatal("verifyPassword() = false, want true")
	}
	if verifyPassword("wrong password", encoded) {
		t.Fatal("verifyPassword(wrong) = true, want false")
	}
}

func TestCredentialErrorsDoNotContainInputs(t *testing.T) {
	service, _ := newTestService(t)
	secret := "credential-that-must-not-leak"
	checks := []error{}
	_, err := service.AuthenticateLocal(
		context.Background(), "missing@example.com", secret,
	)
	checks = append(checks, err)
	_, err = service.AuthenticateSession(context.Background(), secret)
	checks = append(checks, err)
	_, err = service.AuthenticateAPIToken(
		context.Background(), secret, ResourcesRead, AuditContext{},
	)
	checks = append(checks, err)
	_, err = service.AcceptInvitation(
		context.Background(), secret, "missing", AuditContext{},
	)
	checks = append(checks, err)
	for _, err := range checks {
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("credential-safe error = %v", err)
		}
	}
}

func TestBootstrapLocalAuthenticationAndInstanceScope(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()

	result, err := service.BootstrapFirstOwner(ctx, BootstrapInput{
		Email: "Owner@Example.com", Password: "long-enough-password",
		WorkspaceName: "Example",
	}, AuditContext{RequestID: "req-1", SourceAddress: "192.0.2.1"})
	if err != nil {
		t.Fatalf("BootstrapFirstOwner() error = %v", err)
	}
	if !service.IsInstanceAdmin(ctx, result.UserID) {
		t.Fatal("first user is not explicit instance administrator")
	}
	if _, err := service.BootstrapFirstOwner(ctx, BootstrapInput{
		Email: "second@example.com", Password: "another-long-password",
		WorkspaceName: "Second",
	}, AuditContext{}); !errors.Is(err, ErrAlreadyBootstrapped) {
		t.Fatalf("second bootstrap error = %v, want ErrAlreadyBootstrapped", err)
	}

	userID, err := service.AuthenticateLocal(ctx, "OWNER@example.com", "long-enough-password")
	if err != nil || userID != result.UserID {
		t.Fatalf("AuthenticateLocal() = %q, %v", userID, err)
	}
	for _, input := range []struct{ email, password string }{
		{"missing@example.com", "long-enough-password"},
		{"owner@example.com", "wrong"},
		{"bad", "wrong"},
	} {
		_, err := service.AuthenticateLocal(ctx, input.email, input.password)
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("AuthenticateLocal(%q) error = %v", input.email, err)
		}
	}

	assertDatabaseDoesNotContain(t, db, "long-enough-password")

	adminWithoutWorkspace := insertUser(t, service, db, "instance@example.com")
	if _, err := db.Exec(
		"INSERT INTO instance_admins (user_id, created_at) VALUES (?, ?)",
		adminWithoutWorkspace, timestamp(service.now()),
	); err != nil {
		t.Fatalf("insert instance admin: %v", err)
	}
	if _, err := service.Authorize(
		ctx, adminWithoutWorkspace, result.WorkspaceID, WorkspaceRead,
	); !errors.Is(err, ErrForbidden) {
		t.Fatalf("instance admin workspace authorization = %v", err)
	}
	createdWorkspace, err := service.CreateWorkspace(ctx, Principal{
		UserID: adminWithoutWorkspace, AuthMethod: "session",
	}, "Second workspace", AuditContext{})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, err := service.Authorize(
		ctx, adminWithoutWorkspace, createdWorkspace, OwnershipTransfer,
	); err != nil {
		t.Fatalf("new workspace owner authorization error = %v", err)
	}
}

func TestAuthorizationMatrixAndOwnershipTransfer(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)

	roles := []Role{Owner, Admin, Member, Viewer}
	users := map[Role]string{Owner: bootstrap.UserID}
	for _, role := range roles[1:] {
		userID := insertUser(t, service, db, string(role)+"@example.com")
		insertMembership(t, service, db, bootstrap.WorkspaceID, userID, role)
		users[role] = userID
	}

	permissions := []Permission{
		WorkspaceRead, WorkspaceUpdate, WorkspaceDelete, OwnershipTransfer,
		MembersRead, MembersManage, InvitationsManage, ResourcesRead, ResourcesWrite,
	}
	for _, role := range roles {
		for _, permission := range permissions {
			_, err := service.Authorize(
				ctx, users[role], bootstrap.WorkspaceID, permission,
			)
			if got, want := err == nil, permissionsForRole(role)[permission]; got != want {
				t.Errorf("role %s permission %s allowed = %v, want %v", role, permission, got, want)
			}
		}
	}
	missing := insertUser(t, service, db, "missing@example.com")
	if _, err := service.Authorize(ctx, missing, bootstrap.WorkspaceID, WorkspaceRead); !errors.Is(err, ErrForbidden) {
		t.Fatalf("missing membership error = %v", err)
	}
	wrongWorkspace := insertWorkspace(t, service, db, "Wrong")
	if _, err := service.Authorize(ctx, users[Member], wrongWorkspace, WorkspaceRead); !errors.Is(err, ErrForbidden) {
		t.Fatalf("wrong workspace error = %v", err)
	}

	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, OwnershipTransfer)
	if err := service.ChangeMemberRole(ctx, owner, bootstrap.UserID, Admin, AuditContext{}); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("owner demotion error = %v", err)
	}
	admin := mustAuthorize(t, service, users[Admin], bootstrap.WorkspaceID, MembersManage)
	if err := service.TransferOwnership(ctx, admin, users[Member], AuditContext{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("admin transfer error = %v", err)
	}
	if err := service.TransferOwnership(ctx, owner, users[Member], AuditContext{}); err != nil {
		t.Fatalf("TransferOwnership() error = %v", err)
	}
	if _, err := service.Authorize(ctx, users[Member], bootstrap.WorkspaceID, OwnershipTransfer); err != nil {
		t.Fatalf("new owner authorization error = %v", err)
	}
	newOwner := mustAuthorize(t, service, users[Member], bootstrap.WorkspaceID, MembersManage)
	if err := service.RemoveMember(ctx, newOwner, users[Member], AuditContext{}); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("remove current owner error = %v", err)
	}
}

func TestSessionLifecycleAndCookiePolicy(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)

	session, err := service.CreateSession(
		ctx, bootstrap.UserID, "local", "", time.Hour, AuditContext{},
	)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if session.Secret == "" || session.CSRFSecret == "" {
		t.Fatal("session secrets are empty")
	}
	authenticated, err := service.AuthenticateSession(ctx, session.Secret)
	if err != nil || authenticated.ID != session.ID {
		t.Fatalf("AuthenticateSession() = %+v, %v", authenticated, err)
	}
	if !service.VerifyCSRF(ctx, session.ID, session.CSRFSecret) ||
		service.VerifyCSRF(ctx, session.ID, "wrong") {
		t.Fatal("CSRF verification mismatch")
	}
	principal, err := service.AuthenticateSessionForWorkspace(
		ctx, session.Secret, bootstrap.WorkspaceID, WorkspaceRead,
	)
	if err != nil || principal.UserID != bootstrap.UserID || principal.AuthMethod != "local" {
		t.Fatalf("AuthenticateSessionForWorkspace() = %+v, %v", principal, err)
	}
	policy := BrowserCookiePolicy(true)
	if !policy.HTTPOnly || !policy.Secure || policy.SameSite == 0 {
		t.Fatalf("BrowserCookiePolicy(true) = %+v", policy)
	}
	assertDatabaseDoesNotContain(t, db, session.Secret)
	assertDatabaseDoesNotContain(t, db, session.CSRFSecret)

	if _, err := service.RevokeSession(ctx, bootstrap.UserID, session.ID, AuditContext{}); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}
	if _, err := service.AuthenticateSession(ctx, session.Secret); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("revoked session error = %v", err)
	}

	expiring, err := service.CreateSession(
		ctx, bootstrap.UserID, "oidc", "company", time.Minute, AuditContext{},
	)
	if err != nil {
		t.Fatalf("CreateSession(expiring) error = %v", err)
	}
	service.now = func() time.Time { return expiring.ExpiresAt }
	if _, err := service.AuthenticateSession(ctx, expiring.Secret); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expired session error = %v", err)
	}
	service.now = func() time.Time { return expiring.CreatedAt }
	oidcSession, err := service.CreateSession(
		ctx, bootstrap.UserID, "oidc", "company", time.Hour, AuditContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	providerID, err := service.RevokeSession(
		ctx, bootstrap.UserID, oidcSession.ID, AuditContext{},
	)
	if err != nil || providerID != "company" {
		t.Fatalf("OIDC session revocation provider = %q, %v", providerID, err)
	}
}

func TestInvitationLifecycle(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, InvitationsManage)

	invitation, err := service.CreateInvitation(
		ctx, owner, "Invitee@Example.com", Member, time.Hour, AuditContext{},
	)
	if err != nil {
		t.Fatalf("CreateInvitation() error = %v", err)
	}
	if _, err := service.CreateInvitation(
		ctx, owner, "invitee@example.com", Viewer, time.Hour, AuditContext{},
	); !errors.Is(err, ErrActiveInvitation) {
		t.Fatalf("duplicate invitation error = %v", err)
	}
	assertDatabaseDoesNotContain(t, db, invitation.Secret)

	rotated, err := service.ResendInvitation(ctx, owner, invitation.ID, time.Hour, AuditContext{})
	if err != nil || rotated == invitation.Secret {
		t.Fatalf("ResendInvitation() = %q, %v", rotated, err)
	}
	userID := insertUser(t, service, db, "invitee@example.com")
	if _, err := service.AcceptInvitation(ctx, invitation.Secret, userID, AuditContext{}); !errors.Is(err, ErrInvalidInvitation) {
		t.Fatalf("old invitation secret error = %v", err)
	}
	workspaceID, err := service.AcceptInvitation(ctx, rotated, userID, AuditContext{})
	if err != nil || workspaceID != bootstrap.WorkspaceID {
		t.Fatalf("AcceptInvitation() = %q, %v", workspaceID, err)
	}
	if _, err := service.AcceptInvitation(ctx, rotated, userID, AuditContext{}); !errors.Is(err, ErrInvalidInvitation) {
		t.Fatalf("replayed invitation error = %v", err)
	}
	invitations, err := service.ListInvitations(ctx, owner)
	if err != nil || len(invitations) != 1 || invitations[0].AcceptedAt == nil {
		t.Fatalf("ListInvitations() = %+v, %v", invitations, err)
	}

	revoked, err := service.CreateInvitation(
		ctx, owner, "revoked@example.com", Viewer, time.Hour, AuditContext{},
	)
	if err != nil {
		t.Fatalf("CreateInvitation(revoked) error = %v", err)
	}
	if err := service.RevokeInvitation(ctx, owner, revoked.ID, AuditContext{}); err != nil {
		t.Fatalf("RevokeInvitation() error = %v", err)
	}
	revokedUser := insertUser(t, service, db, "revoked@example.com")
	if _, err := service.AcceptInvitation(ctx, revoked.Secret, revokedUser, AuditContext{}); !errors.Is(err, ErrInvalidInvitation) {
		t.Fatalf("revoked invitation error = %v", err)
	}

	expired, err := service.CreateInvitation(
		ctx, owner, "expired@example.com", Viewer, time.Minute, AuditContext{},
	)
	if err != nil {
		t.Fatalf("CreateInvitation(expired) error = %v", err)
	}
	expiredUser := insertUser(t, service, db, "expired@example.com")
	service.now = func() time.Time { return expired.ExpiresAt }
	if _, err := service.AcceptInvitation(ctx, expired.Secret, expiredUser, AuditContext{}); !errors.Is(err, ErrInvalidInvitation) {
		t.Fatalf("expired invitation error = %v", err)
	}
	var activeEmail sql.NullString
	if err := db.QueryRow(
		"SELECT active_email FROM invitations WHERE id = ?", expired.ID,
	).Scan(&activeEmail); err != nil || activeEmail.Valid {
		t.Fatalf("expired active_email = %+v, error = %v", activeEmail, err)
	}
	assertAuditEvent(t, db, "invitation.expired")
	if _, err := service.AcceptInvitation(
		ctx, expired.Secret, expiredUser, AuditContext{},
	); !errors.Is(err, ErrInvalidInvitation) {
		t.Fatalf("second expired invitation error = %v", err)
	}
	var expirationEvents int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM audit_events
		WHERE event_type = 'invitation.expired' AND target_id = ?
	`, expired.ID).Scan(&expirationEvents); err != nil || expirationEvents != 1 {
		t.Fatalf("expiration audit count = %d, error = %v", expirationEvents, err)
	}
	revivedSecret, err := service.ResendInvitation(
		ctx, owner, expired.ID, time.Hour, AuditContext{},
	)
	if err != nil {
		t.Fatalf("ResendInvitation(expired) error = %v", err)
	}
	if _, err := service.AcceptInvitation(
		ctx, revivedSecret, expiredUser, AuditContext{},
	); err != nil {
		t.Fatalf("AcceptInvitation(revived) error = %v", err)
	}
}

func TestInvitedLocalUserAndExistingMemberSafety(t *testing.T) {
	service, _ := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, InvitationsManage)

	invitation, err := service.CreateInvitation(
		ctx, owner, "new@example.com", Viewer, time.Hour, AuditContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	userID, workspaceID, err := service.CreateInvitedLocalUser(
		ctx, invitation.Secret, "invited-user-password", AuditContext{},
	)
	if err != nil || userID == "" || workspaceID != bootstrap.WorkspaceID {
		t.Fatalf("CreateInvitedLocalUser() = %q, %q, %v", userID, workspaceID, err)
	}
	if _, err := service.AuthenticateLocal(ctx, "new@example.com", "invited-user-password"); err != nil {
		t.Fatalf("AuthenticateLocal(invited) error = %v", err)
	}
	if _, err := service.CreateInvitation(
		ctx, owner, "new@example.com", Member, time.Hour, AuditContext{},
	); !errors.Is(err, ErrActiveInvitation) {
		t.Fatalf("existing member invitation error = %v", err)
	}
}

func TestMemberListingReauthenticationAndInvitationPreview(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, MembersRead)
	viewer := insertUser(t, service, db, "a-viewer@example.com")
	admin := insertUser(t, service, db, "z-admin@example.com")
	insertMembership(t, service, db, bootstrap.WorkspaceID, viewer, Viewer)
	insertMembership(t, service, db, bootstrap.WorkspaceID, admin, Admin)
	members, err := service.ListMembers(ctx, owner)
	if err != nil || len(members) != 3 || members[0].Email != "a-viewer@example.com" || members[2].Email != "z-admin@example.com" {
		t.Fatalf("ListMembers() = %+v, %v", members, err)
	}
	wrongWorkspace := insertWorkspace(t, service, db, "Wrong")
	wrong := mustAuthorize(t, service, viewer, bootstrap.WorkspaceID, MembersRead)
	wrong.WorkspaceID = wrongWorkspace
	if _, err := service.ListMembers(ctx, wrong); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-workspace member listing error = %v", err)
	}
	if err := service.ReauthenticateLocal(ctx, bootstrap.UserID, "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong reauthentication error = %v", err)
	}
	if err := service.ReauthenticateLocal(ctx, bootstrap.UserID, "owner-long-password"); err != nil {
		t.Fatalf("valid reauthentication error = %v", err)
	}
	manage := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, InvitationsManage)
	invitation, err := service.CreateInvitation(ctx, manage, "preview@example.com", Member, time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.InvitationForSecret(ctx, invitation.Secret)
	if err != nil || preview.ID != invitation.ID || preview.Email != invitation.Email {
		t.Fatalf("InvitationForSecret() = %+v, %v", preview, err)
	}
}

func TestAPITokenScopesRevocationAndRoleReevaluation(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	memberID := insertUser(t, service, db, "member@example.com")
	insertMembership(t, service, db, bootstrap.WorkspaceID, memberID, Member)
	member := mustAuthorize(t, service, memberID, bootstrap.WorkspaceID, ResourcesWrite)

	token, err := service.CreateAPIToken(
		ctx, member, "automation", []Permission{ResourcesRead, ResourcesWrite},
		nil, AuditContext{},
	)
	if err != nil {
		t.Fatalf("CreateAPIToken() error = %v", err)
	}
	assertDatabaseDoesNotContain(t, db, token.Secret)
	principal, err := service.AuthenticateAPIToken(
		ctx, token.Secret, ResourcesRead, AuditContext{},
	)
	if err != nil || principal.TokenID != token.ID {
		t.Fatalf("AuthenticateAPIToken() = %+v, %v", principal, err)
	}
	tokens, err := service.ListAPITokens(ctx, member)
	if err != nil || len(tokens) != 1 || tokens[0].ID != token.ID {
		t.Fatalf("ListAPITokens() = %+v, %v", tokens, err)
	}
	if _, err := service.AuthenticateAPIToken(
		ctx, token.Secret, MembersManage, AuditContext{},
	); !errors.Is(err, ErrForbidden) {
		t.Fatalf("insufficient token scope error = %v", err)
	}
	wrongWorkspace := insertWorkspace(t, service, db, "Wrong token scope")
	if _, err := service.AuthorizePrincipal(
		ctx, principal, wrongWorkspace, ResourcesRead,
	); !errors.Is(err, ErrForbidden) {
		t.Fatalf("wrong-workspace token error = %v", err)
	}
	expiresAt := service.now().Add(time.Minute)
	expiringToken, err := service.CreateAPIToken(
		ctx, member, "expiring", []Permission{ResourcesRead}, &expiresAt,
		AuditContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	originalNow := service.now
	service.now = func() time.Time { return expiresAt }
	if _, err := service.AuthenticateAPIToken(
		ctx, expiringToken.Secret, ResourcesRead, AuditContext{},
	); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token error = %v", err)
	}
	service.now = originalNow
	if _, err := db.Exec(`
		UPDATE workspace_members SET role = 'viewer'
		WHERE workspace_id = ? AND user_id = ?
	`, bootstrap.WorkspaceID, memberID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateAPIToken(
		ctx, token.Secret, ResourcesWrite, AuditContext{},
	); !errors.Is(err, ErrForbidden) {
		t.Fatalf("role-reduced token error = %v", err)
	}
	viewer := mustAuthorize(t, service, memberID, bootstrap.WorkspaceID, ResourcesRead)
	forged := viewer
	forged.Permissions = map[Permission]bool{
		WorkspaceRead: true, ResourcesRead: true, ResourcesWrite: true,
	}
	if _, err := service.CreateAPIToken(
		ctx, forged, "forged", []Permission{ResourcesWrite}, nil, AuditContext{},
	); !errors.Is(err, ErrForbidden) {
		t.Fatalf("forged token authority error = %v", err)
	}
	if err := service.UpdateAPITokenScopes(
		ctx, viewer, token.ID, []Permission{ResourcesWrite}, AuditContext{},
	); !errors.Is(err, ErrForbidden) {
		t.Fatalf("scope escalation error = %v", err)
	}
	if err := service.RevokeAPIToken(ctx, viewer, token.ID, AuditContext{}); err != nil {
		t.Fatalf("RevokeAPIToken() error = %v", err)
	}
	if _, err := service.AuthenticateAPIToken(
		ctx, token.Secret, ResourcesRead, AuditContext{},
	); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("revoked token error = %v", err)
	}
}

func TestOIDCTransactionBindingAndIdentityLinking(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	registry := &OIDCRegistry{providers: map[string]*oidcProvider{
		"company": {
			id: "company", redirectURI: "https://app.example.com/callback",
			oauth: oauth2.Config{
				ClientID: "client", RedirectURL: "https://app.example.com/callback",
				Endpoint: oauth2.Endpoint{AuthURL: "https://id.example.com/auth"},
			},
		},
	}}
	if _, _, err := registry.Begin(ctx, service, "unknown", "browser"); !errors.Is(err, ErrUnknownOIDCProvider) {
		t.Fatalf("unknown provider error = %v", err)
	}
	transaction, loginURL, err := registry.Begin(ctx, service, "company", "browser")
	if err != nil {
		t.Fatalf("OIDC Begin() error = %v", err)
	}
	parsed, err := url.Parse(loginURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("state") != transaction.State ||
		query.Get("nonce") != transaction.Nonce ||
		query.Get("code_challenge_method") != "S256" ||
		query.Get("code_challenge") != pkceChallenge(transaction.PKCEVerifier) {
		t.Fatalf("OIDC authorization query = %v", query)
	}
	assertDatabaseDoesNotContain(t, db, transaction.State)
	assertDatabaseDoesNotContain(t, db, transaction.PKCEVerifier)
	assertDatabaseDoesNotContain(t, db, transaction.Nonce)
	assertDatabaseDoesNotContain(t, db, transaction.BrowserSession)

	for name, mutate := range map[string]func(*OIDCTransaction){
		"provider": func(tx *OIDCTransaction) { tx.ProviderID = "other" },
		"browser":  func(tx *OIDCTransaction) { tx.BrowserSession = "other" },
		"state":    func(tx *OIDCTransaction) { tx.State = "other" },
		"verifier": func(tx *OIDCTransaction) { tx.PKCEVerifier = "other" },
		"nonce":    func(tx *OIDCTransaction) { tx.Nonce = "other" },
		"redirect": func(tx *OIDCTransaction) { tx.RedirectURI = "https://app.example.com/other" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate, err := service.beginOIDCTransaction(
				ctx, "company", "browser", "https://app.example.com/callback",
			)
			if err != nil {
				t.Fatal(err)
			}
			mutate(&candidate)
			err = service.consumeOIDCTransaction(
				ctx, candidate.ProviderID, candidate.BrowserSession,
				candidate.State, candidate.PKCEVerifier, candidate.Nonce,
				candidate.RedirectURI,
			)
			if !errors.Is(err, ErrOIDCTransaction) {
				t.Fatalf("consume mismatch error = %v", err)
			}
		})
	}
	if err := service.consumeOIDCTransaction(
		ctx, transaction.ProviderID, transaction.BrowserSession,
		transaction.State, transaction.PKCEVerifier, transaction.Nonce,
		transaction.RedirectURI,
	); err != nil {
		t.Fatalf("consume valid transaction error = %v", err)
	}
	if err := service.consumeOIDCTransaction(
		ctx, transaction.ProviderID, transaction.BrowserSession,
		transaction.State, transaction.PKCEVerifier, transaction.Nonce,
		transaction.RedirectURI,
	); !errors.Is(err, ErrOIDCTransaction) {
		t.Fatalf("replayed transaction error = %v", err)
	}
	expiredTransaction, err := service.beginOIDCTransaction(
		ctx, "company", "expiring-browser", "https://app.example.com/callback",
	)
	if err != nil {
		t.Fatal(err)
	}
	originalNow := service.now
	service.now = func() time.Time { return expiredTransaction.ExpiresAt }
	if err := service.consumeOIDCTransaction(
		ctx, expiredTransaction.ProviderID, expiredTransaction.BrowserSession,
		expiredTransaction.State, expiredTransaction.PKCEVerifier,
		expiredTransaction.Nonce, expiredTransaction.RedirectURI,
	); !errors.Is(err, ErrOIDCTransaction) {
		t.Fatalf("expired transaction error = %v", err)
	}
	service.now = originalNow

	claims := ExternalClaims{
		ProviderID: "company", Issuer: "https://id.example.com",
		Subject: "subject-1", Email: "oidc@example.com",
	}
	userID, err := service.AuthenticateOIDC(ctx, claims, AuditContext{})
	if err != nil || userID == "" {
		t.Fatalf("AuthenticateOIDC() = %q, %v", userID, err)
	}
	again, err := service.AuthenticateOIDC(ctx, claims, AuditContext{})
	if err != nil || again != userID {
		t.Fatalf("AuthenticateOIDC(existing) = %q, %v", again, err)
	}
	conflict := claims
	conflict.Issuer = "https://other.example.com"
	conflict.Subject = "subject-2"
	if _, err := service.AuthenticateOIDC(ctx, conflict, AuditContext{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("silent email merge error = %v", err)
	}
	principal := Principal{UserID: bootstrap.UserID, AuthMethod: "session"}
	linked := ExternalClaims{
		ProviderID: "company", Issuer: "https://id.example.com",
		Subject: "owner-subject", Email: "different@example.com",
	}
	if err := service.LinkOIDCIdentity(ctx, principal, linked, false, AuditContext{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unreauthenticated link error = %v", err)
	}
	if err := service.LinkOIDCIdentity(ctx, principal, linked, true, AuditContext{}); err != nil {
		t.Fatalf("LinkOIDCIdentity() error = %v", err)
	}
	linkedUser, err := service.AuthenticateOIDC(ctx, linked, AuditContext{})
	if err != nil || linkedUser != bootstrap.UserID {
		t.Fatalf("linked identity user = %q, %v", linkedUser, err)
	}
}

func FuzzParsePasswordHash(f *testing.F) {
	f.Add("$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0c2FsdA$ZmFrZWZha2VmYWtlZmFrZWZha2VmYWtlZmFrZQ")
	f.Add("not-a-hash")
	f.Fuzz(func(t *testing.T, value string) {
		_, _, _, _ = parsePasswordHash(value)
	})
}

func newTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "identity.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	service.passwordParams = passwordParams{
		memory: 64, iterations: 1, parallelism: 1, saltLength: 16, keyLength: 32,
	}
	fixed := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixed }
	return service, db
}

func bootstrapOwner(t *testing.T, service *Service) BootstrapResult {
	t.Helper()
	result, err := service.BootstrapFirstOwner(context.Background(), BootstrapInput{
		Email: "owner@example.com", Password: "owner-long-password",
		WorkspaceName: "Workspace",
	}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func insertUser(t *testing.T, service *Service, db *sql.DB, email string) string {
	t.Helper()
	userID, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := hashPassword("user-long-password", service.passwordParams)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO users (id, email, password_hash, created_at)
		VALUES (?, ?, ?, ?)
	`, userID, email, passwordHash, timestamp(service.now())); err != nil {
		t.Fatal(err)
	}
	return userID
}

func insertWorkspace(t *testing.T, service *Service, db *sql.DB, name string) string {
	t.Helper()
	workspaceID, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"INSERT INTO workspaces (id, name, created_at) VALUES (?, ?, ?)",
		workspaceID, name, timestamp(service.now()),
	); err != nil {
		t.Fatal(err)
	}
	return workspaceID
}

func insertMembership(
	t *testing.T,
	service *Service,
	db *sql.DB,
	workspaceID string,
	userID string,
	role Role,
) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
		VALUES (?, ?, ?, ?)
	`, workspaceID, userID, role, timestamp(service.now())); err != nil {
		t.Fatal(err)
	}
}

func mustAuthorize(
	t *testing.T,
	service *Service,
	userID string,
	workspaceID string,
	permission Permission,
) Principal {
	t.Helper()
	principal, err := service.Authorize(
		context.Background(), userID, workspaceID, permission,
	)
	if err != nil {
		t.Fatal(err)
	}
	return principal
}

func assertDatabaseDoesNotContain(t *testing.T, db *sql.DB, secret string) {
	t.Helper()
	queries := []string{
		"SELECT COALESCE(password_hash, '') FROM users",
		"SELECT secret_hash || csrf_hash FROM sessions",
		"SELECT secret_hash FROM invitations",
		"SELECT secret_hash FROM api_tokens",
		"SELECT state_hash || browser_session_hash || pkce_verifier_hash || nonce_hash FROM oidc_transactions",
		"SELECT metadata FROM audit_events",
	}
	for _, query := range queries {
		rows, err := db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			if strings.Contains(value, secret) {
				rows.Close()
				t.Fatalf("database contains secret %q", secret)
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func assertAuditEvent(t *testing.T, db *sql.DB, eventType string) {
	t.Helper()
	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM audit_events WHERE event_type = ?", eventType,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatalf("audit event %q count = 0", eventType)
	}
}
