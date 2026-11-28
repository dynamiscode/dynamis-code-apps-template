package database

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func TestSQLiteEngagementNameMigrationPreservesSnapshot(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"), MaxOpenConns: 1, MaxIdleConns: 1})
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
		if entry.Name() >= "000042" {
			continue
		}
		path := "migrations/" + entry.Name()
		data, err := fs.ReadFile(migrationFiles, path)
		if err != nil {
			t.Fatal(err)
		}
		before[path] = &fstest.MapFile{Data: data}
	}
	if err := migrateFS(ctx, db, config.SQLite, before); err != nil {
		t.Fatal(err)
	}
	const stamp = "2026-01-01T00:00:00Z"
	var workspace, client, stage, first, second int64
	if err := db.QueryRow(`INSERT INTO workspaces (name, created_at) VALUES ('Workspace', ?) RETURNING id`, stamp).Scan(&workspace); err != nil {
		t.Fatal(err)
	}
	name := strings.Repeat("界", 201)
	if err := db.QueryRow(`INSERT INTO clients (workspace_id, display_name, version, created_at, updated_at) VALUES (?, ?, 1, ?, ?) RETURNING id`, workspace, name, stamp, stamp).Scan(&client); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO engagement_stages (workspace_id, name, kind, position) VALUES (?, 'Open', 'open', 1) RETURNING id`, workspace).Scan(&stage); err != nil {
		t.Fatal(err)
	}
	for _, key := range []*int64{&first, &second} {
		if err := db.QueryRow(`INSERT INTO engagements (workspace_id, client_id, stage_id, notes) VALUES (?, ?, ?, 'keep') RETURNING id`, workspace, client, stage).Scan(key); err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE clients SET display_name = 'Renamed' WHERE id = ?`, client); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	for _, key := range []int64{first, second} {
		var got, notes string
		if err := db.QueryRow(`SELECT name, notes FROM engagements WHERE id = ?`, key).Scan(&got, &notes); err != nil {
			t.Fatal(err)
		}
		suffix, ok := strings.CutPrefix(got, name+" — ")
		decoded, err := id.DecodeKey(id.TypeEngagement, suffix)
		if !ok || err != nil || decoded != key || notes != "keep" {
			t.Fatalf("engagement %d = (%q, %q), decoded key = %d, error = %v", key, got, notes, decoded, err)
		}
	}
}
