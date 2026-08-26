package maintenance

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestSQLiteRetention(t *testing.T) {
	db, err := database.Open(context.Background(), config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	testRetention(t, db, config.SQLite)
}

func TestPostgresRetention(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	db, err := database.Open(context.Background(), config.Database{
		Driver: config.Postgres, URL: url, MaxOpenConns: 2, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	testRetention(t, db, config.Postgres)
}

func testRetention(t *testing.T, db *sql.DB, driver config.DatabaseDriver) {
	t.Helper()
	ctx := context.Background()
	if err := database.Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	old := stamp(now.Add(-400 * 24 * time.Hour))
	future := stamp(now.Add(time.Hour))
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, database.Rebind(driver, query), args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO users (id, email, created_at) VALUES (?, ?, ?)", "user-maint", "maint@example.com", old)
	exec("INSERT INTO workspaces (id, name, created_at) VALUES (?, ?, ?)", "workspace-maint", "Maintenance", old)
	exec("INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, ?, ?)", "workspace-maint", "user-maint", "owner", old)
	exec(`INSERT INTO sessions (id, user_id, secret_hash, csrf_hash, auth_method, created_at, expires_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "session-old", "user-maint", "session-secret", "csrf", "local", old, future, old)
	exec(`INSERT INTO invitations (id, workspace_id, invited_by_user_id, email, role, secret_hash, created_at, expires_at, accepted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, "invite-old", "workspace-maint", "user-maint", "old@example.com", "viewer", "invite-secret", old, old, old)
	exec(`INSERT INTO api_tokens (id, user_id, workspace_id, name, secret_hash, scopes, created_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "token-old", "user-maint", "workspace-maint", "old", "token-secret", "items:read", old, old)
	exec(`INSERT INTO oidc_transactions (state_hash, provider_id, browser_session_hash, pkce_verifier_hash, nonce_hash, redirect_uri, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "state-old", "provider", "browser", "pkce", "nonce", "https://example.com/callback", old, old)
	exec(`INSERT INTO idempotency_records (key_hash, principal_id, workspace_id, operation, request_hash, result_json, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "key-old", "user-maint", "workspace-maint", "create", "request", "{}", old, old)
	exec(`INSERT INTO item_events (id, workspace_id, item_id, event_type, item_version, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?)`, "event-old", "workspace-maint", "item-old", "deleted", 1, old)
	exec(`INSERT INTO audit_events (id, event_type, auth_method, target_type, action, outcome, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "audit-old", "old", "system", "instance", "test", "success", "{}", old)

	result, err := Run(ctx, db, driver, now, 365*24*time.Hour)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Sessions < 1 || result.Invitations < 1 || result.APITokens < 1 ||
		result.OIDCTransactions < 1 || result.Idempotency < 1 ||
		result.RealtimeReplay < 1 || result.AuditEvents < 1 {
		t.Fatalf("Run() result = %+v", result)
	}
	for table, key := range map[string]string{
		"sessions": "session-old", "invitations": "invite-old",
		"api_tokens": "token-old", "oidc_transactions": "state-old",
		"idempotency_records": "key-old", "item_events": "event-old",
	} {
		var count int
		column := "id"
		if table == "oidc_transactions" {
			column = "state_hash"
		} else if table == "idempotency_records" {
			column = "key_hash"
		}
		query := database.Rebind(driver, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", table, column))
		if err := db.QueryRowContext(ctx, query, key).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count = %d, error = %v", table, count, err)
		}
	}
	var auditCount int
	if err := db.QueryRowContext(ctx, database.Rebind(driver,
		"SELECT COUNT(*) FROM audit_events WHERE event_type = ? AND created_at = ?"),
		"maintenance.retention_pruned", stamp(now)).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("retention audit count = %d, error = %v", auditCount, err)
	}
}
