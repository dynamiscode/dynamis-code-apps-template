package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// Migration 000015 gives mfa_passkeys an integer primary key. Passkey
// credential IDs stay text: they are protocol identifiers, not row keys.
var mfaIntegerKeyTables = []string{"mfa_passkeys"}

func TestSQLiteMFATablesUseIntegerPrimaryKeys(t *testing.T) {
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

	for _, table := range mfaIntegerKeyTables {
		var keyType string
		if err := db.QueryRow("SELECT type FROM pragma_table_info(?) WHERE pk = 1", table).Scan(&keyType); err != nil {
			t.Fatal(err)
		}
		if keyType != "INTEGER" {
			t.Fatalf("%s primary key type = %q, want INTEGER", table, keyType)
		}
	}
	var credentialType string
	if err := db.QueryRow("SELECT type FROM pragma_table_info('mfa_passkeys') WHERE name = 'credential_id'").Scan(&credentialType); err != nil {
		t.Fatal(err)
	}
	if credentialType != "TEXT" {
		t.Fatalf("mfa_passkeys.credential_id type = %q, want TEXT", credentialType)
	}
}

func TestPostgresMFATablesUseIntegerPrimaryKeys(t *testing.T) {
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

	for _, table := range mfaIntegerKeyTables {
		var keyType string
		if err := db.QueryRow(`SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = 'id'`, table).Scan(&keyType); err != nil {
			t.Fatal(err)
		}
		if keyType != "bigint" {
			t.Fatalf("%s id type = %q, want bigint", table, keyType)
		}
	}
	var credentialType string
	if err := db.QueryRow(`SELECT data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'mfa_passkeys' AND column_name = 'credential_id'`).Scan(&credentialType); err != nil {
		t.Fatal(err)
	}
	if credentialType != "text" {
		t.Fatalf("mfa_passkeys.credential_id type = %q, want text", credentialType)
	}
}
