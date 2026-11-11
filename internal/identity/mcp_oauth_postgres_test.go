package identity

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// TestPostgresMCPOAuthFlow runs the MCP OAuth grant flow against a fresh
// PostgreSQL database created for this test.
func TestPostgresMCPOAuthFlow(t *testing.T) {
	baseURL := os.Getenv("POSTGRES_TEST_URL")
	if baseURL == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	admin, err := database.Open(ctx, config.Database{Driver: config.Postgres, URL: baseURL, MaxOpenConns: 2, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "mcp_oauth_" + hex.EncodeToString(suffix[:])
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	targetURL := databaseURLWithName(t, baseURL, name)
	db, err := database.Open(ctx, config.Database{Driver: config.Postgres, URL: targetURL, MaxOpenConns: 4, MaxIdleConns: 2})
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
	service.passwordParams = passwordParams{memory: 64, iterations: 1, parallelism: 1, saltLength: 16, keyLength: 32}
	service.now = func() time.Time { return time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC) }
	owner, err := service.BootstrapFirstOwner(ctx, BootstrapInput{
		Email: "owner@example.com", Password: "postgres-owner-password", WorkspaceName: "PostgreSQL MCP",
	}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := service.CreateSession(ctx, owner.UserID, "local", "", time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := service.RegisterOAuthClient(ctx, "PostgreSQL client", []string{testRedirectURI}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	code, err := service.ApproveOAuthConsent(ctx, session.Session, owner.WorkspaceID, client.ClientID,
		[]Permission{ResourcesRead, ResourcesWrite}, testRedirectURI, pkceChallenge(testCodeVerifier), testResource, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := service.ExchangeOAuthCode(ctx, code, client.ClientID, testRedirectURI, testCodeVerifier, testResource, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := service.AuthenticateOAuthAccessToken(ctx, tokens.AccessToken)
	if err != nil || !principal.Permissions[ResourcesWrite] || principal.AuthMethod != "mcp_oauth" {
		t.Fatalf("AuthenticateOAuthAccessToken() = %#v, %v", principal, err)
	}
	rotated, err := service.RefreshOAuthToken(ctx, tokens.RefreshToken, client.ClientID, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RefreshOAuthToken(ctx, tokens.RefreshToken, client.ClientID, AuditContext{}); !errors.Is(err, ErrOAuthInvalidGrant) {
		t.Fatalf("refresh reuse = %v", err)
	}
	if _, err := service.AuthenticateOAuthAccessToken(ctx, rotated.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("access after reuse = %v", err)
	}
	var revoked sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT revoked_at FROM oauth_grants LIMIT 1").Scan(&revoked); err != nil || !revoked.Valid {
		t.Fatalf("grant not revoked after reuse: %v %v", revoked, err)
	}
}

func databaseURLWithName(t *testing.T, raw, name string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	return parsed.String()
}
