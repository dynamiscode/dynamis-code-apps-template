package identity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func TestPostgresIdentityLifecycle(t *testing.T) {
	databaseURL := os.Getenv("POSTGRES_TEST_URL")
	if databaseURL == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{
		Driver: config.Postgres, URL: databaseURL,
		MaxOpenConns: 4, MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.Postgres); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(db, config.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	service.passwordParams = passwordParams{
		memory: 64, iterations: 1, parallelism: 1, saltLength: 16, keyLength: 32,
	}
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	bootstrap, err := service.BootstrapFirstOwner(ctx, BootstrapInput{
		Email: "owner@example.com", Password: "postgres-owner-password",
		WorkspaceName: "PostgreSQL",
	}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, InvitationsManage)
	roleUsers := map[Role]string{Owner: bootstrap.UserID}
	for _, role := range []Role{Admin, Member, Viewer} {
		userID := postgresInsertUser(t, service, string(role)+"-matrix@example.com")
		if _, err := service.exec(ctx, db, `
			INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
			VALUES (?, ?, ?, ?)
		`, bootstrap.WorkspaceID, userID, role, timestamp(now)); err != nil {
			t.Fatal(err)
		}
		roleUsers[role] = userID
	}
	for role, userID := range roleUsers {
		for _, permission := range []Permission{
			WorkspaceRead, WorkspaceUpdate, WorkspaceDelete, WorkspaceExport, OwnershipTransfer,
			MembersRead, MembersManage, InvitationsManage,
			ResourcesRead, ResourcesWrite,
		} {
			_, err := service.Authorize(ctx, userID, bootstrap.WorkspaceID, permission)
			if got, want := err == nil, permissionsForRole(role)[permission]; got != want {
				t.Errorf("PostgreSQL role %s permission %s = %v, want %v", role, permission, got, want)
			}
		}
	}

	session, err := service.CreateSession(
		ctx, bootstrap.UserID, "local", "", time.Hour, AuditContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateSession(ctx, session.Secret); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RevokeSession(ctx, bootstrap.UserID, session.ID, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateSession(ctx, session.Secret); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("revoked session error = %v", err)
	}

	invitation, err := service.CreateInvitation(
		ctx, owner, "member@example.com", Member, time.Hour, AuditContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	memberID := postgresInsertUser(t, service, "member@example.com")
	if _, err := service.AcceptInvitation(
		ctx, invitation.Secret, memberID, AuditContext{},
	); err != nil {
		t.Fatal(err)
	}
	member := mustAuthorize(t, service, memberID, bootstrap.WorkspaceID, ResourcesWrite)
	token, err := service.CreateAPIToken(
		ctx, member, "postgres", []Permission{ResourcesRead, ResourcesWrite},
		nil, AuditContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateAPIToken(
		ctx, token.Secret, ResourcesWrite, AuditContext{},
	); err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeAPIToken(ctx, member, token.ID, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateAPIToken(
		ctx, token.Secret, ResourcesRead, AuditContext{},
	); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("revoked token error = %v", err)
	}

	transaction, err := service.beginOIDCTransaction(
		ctx, "company", "browser", "https://app.example.com/callback",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.consumeOIDCTransaction(
		ctx, transaction.ProviderID, transaction.BrowserSession,
		transaction.State, transaction.PKCEVerifier, transaction.Nonce,
		transaction.RedirectURI,
	); err != nil {
		t.Fatal(err)
	}

	for _, secret := range []string{
		"postgres-owner-password", session.Secret, session.CSRFSecret,
		invitation.Secret, token.Secret, transaction.State,
		transaction.PKCEVerifier, transaction.Nonce, transaction.BrowserSession,
	} {
		var count int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM (
				SELECT COALESCE(password_hash, '') AS value FROM users
				UNION ALL SELECT secret_hash || csrf_hash FROM sessions
				UNION ALL SELECT secret_hash FROM invitations
				UNION ALL SELECT secret_hash FROM api_tokens
				UNION ALL SELECT state_hash || browser_session_hash || pkce_verifier_hash || nonce_hash FROM oidc_transactions
				UNION ALL SELECT metadata FROM audit_events
			) values_to_check WHERE value LIKE '%' || $1 || '%'
		`, secret).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("PostgreSQL contains plaintext secret")
		}
	}
}

func TestPostgresMFA(t *testing.T) {
	databaseURL := os.Getenv("POSTGRES_TEST_URL")
	if databaseURL == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.Postgres, URL: databaseURL, MaxOpenConns: 2, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.Postgres); err != nil {
		t.Fatal(err)
	}
	service, err := NewServiceWithMFA(db, config.Postgres, MFAConfig{
		Enabled: true, EncryptionKey: []byte("01234567890123456789012345678901"),
		RelyingPartyID: "localhost", Origins: []string{"http://localhost:8080"}, DisplayName: "Dynamis Code",
	})
	if err != nil {
		t.Fatal(err)
	}
	service.passwordParams = passwordParams{memory: 64, iterations: 1, parallelism: 1, saltLength: 16, keyLength: 32}
	marker, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	userID := postgresInsertUser(t, service, fmt.Sprintf("mfa-%s@example.com", marker))
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID) })
	session, err := service.CreateSession(ctx, userID, "local", "", time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := service.BeginTOTPEnrollment(ctx, userID, session.ID, "postgres-user-password", AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteTOTPEnrollment(ctx, session.ID, enrollment.Challenge, totpTestCode(enrollment.Secret, service.now()), AuditContext{}); err != nil {
		t.Fatal(err)
	}
}

func postgresInsertUser(t *testing.T, service *Service, email string) string {
	t.Helper()
	userID, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := hashPassword("postgres-user-password", service.passwordParams)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.exec(context.Background(), service.db, `
		INSERT INTO users (id, email, password_hash, created_at)
		VALUES (?, ?, ?, ?)
	`, userID, email, passwordHash, timestamp(service.now())); err != nil {
		t.Fatal(err)
	}
	return userID
}
