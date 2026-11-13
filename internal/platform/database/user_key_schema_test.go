package database

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// Migrations 000002 through 000020 give users an integer key. Every column that
// references users(id) takes the same BIGINT type. Keys that stay text (session,
// OIDC state, MFA challenge, and external identity IDs) are listed separately.
type userKeyColumn struct{ table, column string }

var userReferenceColumns = []userKeyColumn{
	{"external_identities", "user_id"},
	{"instance_admins", "user_id"},
	{"workspace_members", "user_id"},
	{"sessions", "user_id"},
	{"invitations", "invited_by_user_id"},
	{"api_tokens", "user_id"},
	{"items", "created_by_user_id"},
	{"oidc_transactions", "user_id"},
	{"email_verifications", "user_id"},
	{"password_resets", "user_id"},
	{"user_notification_preferences", "user_id"},
	{"workspace_notification_preferences", "user_id"},
	{"notifications", "user_id"},
	{"scim_tokens", "created_by_user_id"},
	{"scim_users", "user_id"},
	{"files", "owner_user_id"},
	{"mfa_challenges", "user_id"},
	{"mfa_totp", "user_id"},
	{"mfa_recovery_codes", "user_id"},
	{"mfa_passkeys", "user_id"},
	{"oauth_grants", "user_id"},
	{"audit_events", "actor_user_id"},
}

// textKeyColumns stay TEXT: they hold bearer or hashed values, or non-user IDs.
var textKeyColumns = []userKeyColumn{
	{"external_identities", "id"},
	{"oidc_transactions", "state_hash"},
	{"sessions", "id"},
	{"mfa_challenges", "session_id"},
	{"oauth_grants", "session_id"},
}

// profilePrunedTables are tables whose migrations cmd/template-init removes for
// apps generated without the Files profile.
var profilePrunedTables = map[string]bool{"files": true}

// prunedTableAllowed reports whether a missing table is an absent profile table.
// It is allowed only when no embedded migration creates or renames it. In this
// repository the Files migrations exist, so a dropped files table still fails.
func prunedTableAllowed(t *testing.T, table string) bool {
	t.Helper()
	if !profilePrunedTables[table] {
		return false
	}
	create := regexp.MustCompile(`(?i)(CREATE TABLE (IF NOT EXISTS )?|RENAME TO )` + regexp.QuoteMeta(table) + `\b`)
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		raw, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if create.Match(raw) {
			return false
		}
	}
	return true
}

func TestSQLiteUserKeysUseIntegerReferences(t *testing.T) {
	t.Parallel()

	db, err := Open(context.Background(), config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(context.Background(), db, config.SQLite); err != nil {
		t.Fatal(err)
	}

	var keyType string
	if err := db.QueryRow("SELECT type FROM pragma_table_info('users') WHERE pk = 1").Scan(&keyType); err != nil {
		t.Fatal(err)
	}
	if keyType != "INTEGER" {
		t.Fatalf("users primary key type = %q, want INTEGER", keyType)
	}
	for _, column := range userReferenceColumns {
		var tables int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", column.table).Scan(&tables); err != nil {
			t.Fatal(err)
		}
		if tables == 0 {
			if prunedTableAllowed(t, column.table) {
				continue
			}
			t.Fatalf("table %s is missing", column.table)
		}
		var columnType string
		if err := db.QueryRow("SELECT type FROM pragma_table_info(?) WHERE name = ?", column.table, column.column).Scan(&columnType); err != nil {
			t.Fatalf("%s.%s: %v", column.table, column.column, err)
		}
		if columnType != "BIGINT" {
			t.Fatalf("%s.%s type = %q, want BIGINT", column.table, column.column, columnType)
		}
	}
	for _, text := range textKeyColumns {
		var tables int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", text.table).Scan(&tables); err != nil {
			t.Fatal(err)
		}
		if tables == 0 {
			if prunedTableAllowed(t, text.table) {
				continue
			}
			t.Fatalf("table %s is missing", text.table)
		}
		var columnType string
		if err := db.QueryRow("SELECT type FROM pragma_table_info(?) WHERE name = ?", text.table, text.column).Scan(&columnType); err != nil {
			t.Fatal(err)
		}
		if columnType != "TEXT" {
			t.Fatalf("%s.%s type = %q, want TEXT", text.table, text.column, columnType)
		}
	}
}

func TestPostgresUserKeysUseIntegerReferences(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}

	db, err := Open(context.Background(), config.Database{
		Driver: config.Postgres, URL: url, MaxOpenConns: 4, MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(context.Background(), db, config.Postgres); err != nil {
		t.Fatal(err)
	}

	// dataType returns sql.ErrNoRows when the table or column is missing.
	dataType := func(table, column string) (string, error) {
		var value string
		err := db.QueryRow(`SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`,
			table, column).Scan(&value)
		return value, err
	}
	if value, err := dataType("users", "id"); err != nil || value != "bigint" {
		t.Fatalf("users.id type = %q, %v, want bigint", value, err)
	}
	for _, column := range userReferenceColumns {
		value, err := dataType(column.table, column.column)
		if errors.Is(err, sql.ErrNoRows) && prunedTableAllowed(t, column.table) {
			continue
		}
		if err != nil || value != "bigint" {
			t.Fatalf("%s.%s type = %q, %v, want bigint", column.table, column.column, value, err)
		}
	}
	for _, text := range textKeyColumns {
		value, err := dataType(text.table, text.column)
		if errors.Is(err, sql.ErrNoRows) && prunedTableAllowed(t, text.table) {
			continue
		}
		if err != nil || value != "text" {
			t.Fatalf("%s.%s type = %q, %v, want text", text.table, text.column, value, err)
		}
	}
}
