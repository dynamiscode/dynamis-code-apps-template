package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// Migration 000002 gives users and external identities integer keys. Every
// text users(id) foreign key converts to BIGINT in the same migration history.
var userIntegerKeyTables = []string{"users", "external_identities"}

var userForeignKeyColumns = []struct{ table, column string }{
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
	{"sessions", "user_id"},
	{"external_identities", "user_id"},
	{"instance_admins", "user_id"},
	{"workspace_members", "user_id"},
	{"invitations", "invited_by_user_id"},
	{"api_tokens", "user_id"},
}

func TestSQLiteUserTablesUseIntegerKeysAndForeignKeys(t *testing.T) {
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

	for _, table := range userIntegerKeyTables {
		var keyType string
		if err := db.QueryRow("SELECT type FROM pragma_table_info(?) WHERE pk = 1", table).Scan(&keyType); err != nil {
			t.Fatal(err)
		}
		if keyType != "INTEGER" {
			t.Fatalf("%s primary key type = %q, want INTEGER", table, keyType)
		}
	}
	for _, fk := range userForeignKeyColumns {
		// Generated apps without a profile drop that profile's migrations, so a
		// missing table is not a missing conversion.
		var tableColumns int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?)", fk.table).Scan(&tableColumns); err != nil {
			t.Fatal(err)
		}
		if tableColumns == 0 {
			continue
		}
		var columnType string
		if err := db.QueryRow("SELECT type FROM pragma_table_info(?) WHERE name = ?", fk.table, fk.column).Scan(&columnType); err != nil {
			t.Fatalf("%s.%s: %v", fk.table, fk.column, err)
		}
		if columnType != "BIGINT" {
			t.Fatalf("%s.%s type = %q, want BIGINT", fk.table, fk.column, columnType)
		}
	}
}

// A deleted user or external identity key must never be handed out again, so
// a stale short ID cannot resolve to a newer row on SQLite.
func TestSQLiteUserAndExternalIdentityKeysAreNeverReused(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := Open(ctx, config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	const created = "2026-01-01T00:00:00Z"

	insertUser := func(email string) int64 {
		var key int64
		if err := db.QueryRowContext(ctx, "INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id", email, created).Scan(&key); err != nil {
			t.Fatal(err)
		}
		return key
	}
	firstUser := insertUser("first@example.com")
	if _, err := db.ExecContext(ctx, "DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	if second := insertUser("second@example.com"); second == firstUser {
		t.Fatalf("user key %d was reused", firstUser)
	}

	keep := insertUser("keep@example.com")
	insertIdentity := func(subject string) int64 {
		var key int64
		if err := db.QueryRowContext(ctx, `INSERT INTO external_identities (user_id, provider_id, issuer, subject, email, created_at)
			VALUES (?, 'provider', 'https://issuer.example', ?, 'keep@example.com', ?) RETURNING id`, keep, subject, created).Scan(&key); err != nil {
			t.Fatal(err)
		}
		return key
	}
	firstIdentity := insertIdentity("subject-first")
	if _, err := db.ExecContext(ctx, "DELETE FROM external_identities"); err != nil {
		t.Fatal(err)
	}
	if second := insertIdentity("subject-second"); second == firstIdentity {
		t.Fatalf("external identity key %d was reused", firstIdentity)
	}
}

func TestPostgresUserTablesUseIntegerKeysAndForeignKeys(t *testing.T) {
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

	check := func(table, column, want string) {
		t.Helper()
		var dataType string
		if err := db.QueryRow(`SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`, table, column).Scan(&dataType); err != nil {
			t.Fatalf("%s.%s: %v", table, column, err)
		}
		if dataType != want {
			t.Fatalf("%s.%s data type = %q, want %q", table, column, dataType, want)
		}
	}
	for _, table := range userIntegerKeyTables {
		check(table, "id", "bigint")
	}
	for _, fk := range userForeignKeyColumns {
		var tableColumns int
		if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1`, fk.table).Scan(&tableColumns); err != nil {
			t.Fatal(err)
		}
		if tableColumns == 0 {
			continue
		}
		check(fk.table, fk.column, "bigint")
	}
}
