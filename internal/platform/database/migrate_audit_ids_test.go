package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// Migration 000002 gives audit events an integer primary key.
var auditIntegerKeyTables = []string{"audit_events"}

func TestSQLiteAuditTablesUseIntegerPrimaryKeys(t *testing.T) {
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

	for _, table := range auditIntegerKeyTables {
		var keyType string
		if err := db.QueryRow("SELECT type FROM pragma_table_info(?) WHERE pk = 1", table).Scan(&keyType); err != nil {
			t.Fatal(err)
		}
		if keyType != "INTEGER" {
			t.Fatalf("%s primary key type = %q, want INTEGER", table, keyType)
		}
	}
}

// A deleted audit key must never be handed out again, so a stale short ID
// cannot resolve to a newer audit event on SQLite.
func TestSQLiteAuditKeysAreNeverReused(t *testing.T) {
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

	insertAudit := func() int64 {
		var key int64
		if err := db.QueryRowContext(ctx, `INSERT INTO audit_events (event_type, auth_method, target_type, action, outcome, metadata, created_at)
			VALUES ('test', 'system', 'instance', 'test', 'success', '{}', '2026-01-01T00:00:00Z') RETURNING id`).Scan(&key); err != nil {
			t.Fatal(err)
		}
		return key
	}
	first := insertAudit()
	if _, err := db.ExecContext(ctx, "DELETE FROM audit_events"); err != nil {
		t.Fatal(err)
	}
	if second := insertAudit(); second == first {
		t.Fatalf("audit event key %d was reused", first)
	}
}

func TestPostgresAuditTablesUseIntegerPrimaryKeys(t *testing.T) {
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

	for _, table := range auditIntegerKeyTables {
		var keyType string
		if err := db.QueryRow(`SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = 'id'`, table).Scan(&keyType); err != nil {
			t.Fatal(err)
		}
		if keyType != "bigint" {
			t.Fatalf("%s id type = %q, want bigint", table, keyType)
		}
	}
}
