package database

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestSQLiteMergeMigrationPreservesFilesBranch(t *testing.T) {
	t.Parallel()

	db, err := Open(context.Background(), config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	legacy := fstest.MapFS{}
	for _, name := range []string{
		"migrations/000001_foundation.sql", "migrations/000002_identity.sql",
		"migrations/000003_items_http.sql", "migrations/000004_item_events.sql",
		"migrations/000005_oidc_flow.sql", "migrations/000006_localization.sql",
		"migrations/000007_account_notifications.sql", "migrations/000008_webhooks.sql",
		"migrations/000009_nullable_item_creator.sql", "migrations/000010_background_jobs.sql",
		"migrations/000011_public_sharing.sql",
	} {
		data, err := fs.ReadFile(migrationFiles, name)
		if err != nil {
			t.Fatal(err)
		}
		legacy[name] = &fstest.MapFile{Data: data}
	}
	files, err := fs.ReadFile(migrationFiles, "migrations/000014_files.sql")
	if err != nil {
		t.Fatal(err)
	}
	legacy["migrations/000012_files.sql"] = &fstest.MapFile{Data: files}
	if err := migrateFS(context.Background(), db, config.SQLite, legacy); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	assertMigrationVersion(t, db, 14)

	for _, table := range []string{"files", "scim_tokens", "scim_users", "scim_groups", "background_jobs"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("table %q count = %d, want 1", table, count)
		}
	}
}

func TestSQLiteMergeMigrationPreservesMFABranch(t *testing.T) {
	t.Parallel()

	db, err := Open(context.Background(), config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	legacy := fstest.MapFS{}
	for _, name := range []string{
		"migrations/000001_foundation.sql", "migrations/000002_identity.sql",
		"migrations/000003_items_http.sql", "migrations/000004_item_events.sql",
		"migrations/000005_oidc_flow.sql", "migrations/000006_localization.sql",
		"migrations/000007_account_notifications.sql", "migrations/000008_webhooks.sql",
		"migrations/000009_nullable_item_creator.sql", "migrations/000010_background_jobs.sql",
		"migrations/000011_public_sharing.sql",
	} {
		data, err := fs.ReadFile(migrationFiles, name)
		if err != nil {
			t.Fatal(err)
		}
		legacy[name] = &fstest.MapFile{Data: data}
	}
	mfa, err := fs.ReadFile(migrationFiles, "migrations/000015_mfa.sql")
	if err != nil {
		t.Fatal(err)
	}
	legacy["migrations/000012_mfa.sql"] = &fstest.MapFile{Data: mfa}
	if err := migrateFS(context.Background(), db, config.SQLite, legacy); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	assertMigrationVersion(t, db, 15)

	for _, table := range []string{"mfa_challenges", "scim_tokens", "scim_users", "scim_groups", "files"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("table %q count = %d, want 1", table, count)
		}
	}
}
