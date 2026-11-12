package database

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// Migration 000017 rebuilds files with integer keys. Existing rows must keep
// their object keys, and a deleted key must never be reused.
func TestSQLiteFileShortIDMigrationPreservesRowsAndNeverReusesKeys(t *testing.T) {
	t.Parallel()

	db, err := Open(context.Background(), config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	before := fstest.MapFS{}
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == "000017_file_short_ids.sql" {
			continue
		}
		data, err := fs.ReadFile(migrationFiles, "migrations/"+entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		before["migrations/"+entry.Name()] = &fstest.MapFile{Data: data}
	}
	if err := migrateFS(context.Background(), db, config.SQLite, before); err != nil {
		t.Fatal(err)
	}
	const stamp = "2026-11-11T00:00:00Z"
	var workspaceID int64
	if err := db.QueryRow("INSERT INTO workspaces (name, created_at) VALUES (?, ?) RETURNING id", "Keys", stamp).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO files (id, workspace_id, object_key, original_name, size, status, created_at, updated_at)
		VALUES ('0123456789abcdef0123456789abcdef', ?, 'ws/object', 'a.txt', 5, 'ready', ?, ?)`, workspaceID, stamp, stamp); err != nil {
		t.Fatal(err)
	}

	if err := Migrate(context.Background(), db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	var key int64
	var objectKey string
	if err := db.QueryRow("SELECT id, object_key FROM files").Scan(&key, &objectKey); err != nil {
		t.Fatal(err)
	}
	if key == 0 || objectKey != "ws/object" {
		t.Fatalf("rebuilt row = (%d, %q), want an integer key and the original object key", key, objectKey)
	}
	if _, err := db.Exec("DELETE FROM files WHERE id = ?", key); err != nil {
		t.Fatal(err)
	}
	var next int64
	if err := db.QueryRow(`INSERT INTO files (workspace_id, object_key, original_name, size, status, created_at, updated_at)
		VALUES (?, 'ws/next', 'b.txt', 5, 'ready', ?, ?) RETURNING id`, workspaceID, stamp, stamp).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if next == key {
		t.Fatalf("deleted file key %d was reused", key)
	}
}
