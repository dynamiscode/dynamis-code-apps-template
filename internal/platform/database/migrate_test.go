package database

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestSQLiteConfigurationAndMigrations(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "app.db")
	db, err := Open(context.Background(), config.Database{
		Driver:       config.SQLite,
		SQLitePath:   path,
		MaxOpenConns: 4,
		MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := Migrate(context.Background(), db, config.SQLite); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := Migrate(context.Background(), db, config.SQLite); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	var foreignKeys int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}

	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	assertMigrationVersion(t, db, 1)
	assertMigrationVersion(t, db, 2)
	assertMigrationVersion(t, db, 3)
	assertMigrationVersion(t, db, 4)
	assertMigrationVersion(t, db, 5)
	assertMigrationVersion(t, db, 6)
	assertMigrationVersion(t, db, 7)
	assertMigrationVersion(t, db, 8)
	assertMigrationVersion(t, db, 9)
	assertMigrationVersion(t, db, 10)
	assertMigrationVersion(t, db, 11)
	assertMigrationVersion(t, db, 12)
	assertMigrationVersion(t, db, 13)
	assertMigrationVersion(t, db, 15)
}

func TestSQLiteBackgroundJobsMigrationBackfillsPendingWebhook(t *testing.T) {
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
		"migrations/000009_nullable_item_creator.sql",
	} {
		data, err := fs.ReadFile(migrationFiles, name)
		if err != nil {
			t.Fatal(err)
		}
		legacy[name] = &fstest.MapFile{Data: data}
	}
	scim, err := fs.ReadFile(migrationFiles, "migrations/000012_scim.sql")
	if err != nil {
		t.Fatal(err)
	}
	legacy["migrations/000010_scim.sql"] = &fstest.MapFile{Data: scim}
	ctx := context.Background()
	if err := migrateFS(ctx, db, config.SQLite, legacy); err != nil {
		t.Fatal(err)
	}
	var workspaceID int64
	if err := db.QueryRow(
		"INSERT INTO workspaces (name, created_at) VALUES (?, ?) RETURNING id",
		"Migration", "2026-08-28T00:00:00Z",
	).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	var webhookID int64
	if err := db.QueryRow(`
		INSERT INTO webhooks (workspace_id, name, url, secret_ciphertext, events, created_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, workspaceID, "Webhook", "https://example.com/hook", "ciphertext", `["item.created"]`, "2026-08-28T00:00:00Z").Scan(&webhookID); err != nil {
		t.Fatal(err)
	}
	var eventID int64
	if err := db.QueryRow(`
		INSERT INTO webhook_events (workspace_id, event_type, created_at)
		VALUES (?, ?, ?) RETURNING id
	`, workspaceID, "item.created", "2026-08-28T00:00:00Z").Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	var deliveryID int64
	if err := db.QueryRow(`
		INSERT INTO webhook_deliveries (
			webhook_id, event_id, event_type, payload, attempt_count,
			status, next_attempt_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, webhookID, eventID, "item.created", `{}`, 2, "pending", "2026-08-28T00:00:01Z", "2026-08-28T00:00:00Z").Scan(&deliveryID); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	var gotWorkspaceID, jobID int64
	var kind, key, payload, status string
	var attempts int
	wantKey := strconv.FormatInt(deliveryID, 10)
	if err := db.QueryRow(`
		SELECT id, workspace_id, kind, deduplication_key, payload, status, attempt_count
		FROM background_jobs WHERE deduplication_key = ?
	`, wantKey).Scan(&jobID, &gotWorkspaceID, &kind, &key, &payload, &status, &attempts); err != nil {
		t.Fatal(err)
	}
	wantPayload := `{"deliveryId":` + wantKey + `}`
	if jobID < 1 || gotWorkspaceID != workspaceID || kind != "webhook.delivery" || key != wantKey ||
		payload != wantPayload || status != "pending" || attempts != 2 {
		t.Fatalf("backfilled job = %d, %d, %q, %q, %q, %q, %d", jobID, gotWorkspaceID, kind, key, payload, status, attempts)
	}
}

func TestLoadMigrationsRejectsDuplicateVersion(t *testing.T) {
	t.Parallel()

	_, err := loadMigrations(fstest.MapFS{
		"migrations/000001_first.sql":  {Data: []byte("SELECT 1")},
		"migrations/000001_second.sql": {Data: []byte("SELECT 2")},
	})
	if err == nil {
		t.Fatal("loadMigrations() error = nil, want duplicate version error")
	}
}

func TestSQLiteMigrationFailureRollsBack(t *testing.T) {
	t.Parallel()

	db, err := Open(context.Background(), config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	err = migrateFS(context.Background(), db, config.SQLite, fstest.MapFS{
		"migrations/000001_interrupted.sql": {Data: []byte(
			"CREATE TABLE interrupted (id TEXT); INSERT INTO missing_table VALUES (1);",
		)},
	})
	if err == nil {
		t.Fatal("migrateFS() error = nil, want failure")
	}
	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'interrupted'",
	).Scan(&count); err != nil {
		t.Fatalf("query interrupted table: %v", err)
	}
	if count != 0 {
		t.Fatalf("interrupted table count = %d, want 0", count)
	}
}

func TestOpenRejectsPostgresURLWithoutLeakingSecret(t *testing.T) {
	t.Parallel()

	secret := "super-secret"
	_, err := Open(context.Background(), config.Database{
		Driver:       config.Postgres,
		URL:          "postgres://user:" + secret + "@%",
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err == nil {
		t.Fatal("Open() error = nil, want validation error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("Open() error leaked DATABASE_URL secret: %v", err)
	}
}

func assertMigrationVersion(t *testing.T, queryer interface {
	QueryRow(string, ...any) *sql.Row
}, version int64) {
	t.Helper()

	var count int
	if err := queryer.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations WHERE version = ?",
		version,
	).Scan(&count); err != nil {
		t.Fatalf("query migration version: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration version %d count = %d, want 1", version, count)
	}
}

func TestSQLiteItemEventKeysAreNeverReused(t *testing.T) {
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
	var workspaceID int64
	if err := db.QueryRowContext(ctx, "INSERT INTO workspaces (name, created_at) VALUES (?, ?) RETURNING id", "Events", "2026-01-01T00:00:00Z").Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	insert := func() int64 {
		var eventID int64
		if err := db.QueryRowContext(ctx, `INSERT INTO item_events (workspace_id, item_id, event_type, item_version, occurred_at)
			VALUES (?, ?, 'created', 1, ?) RETURNING id`, workspaceID, 1, "2026-01-01T00:00:00Z").Scan(&eventID); err != nil {
			t.Fatal(err)
		}
		return eventID
	}
	first := insert()
	if _, err := db.ExecContext(ctx, "DELETE FROM item_events"); err != nil {
		t.Fatal(err)
	}
	// A cursor for a pruned event must not resolve to the next event.
	if second := insert(); second == first {
		t.Fatalf("item event key %d was reused", first)
	}
}
