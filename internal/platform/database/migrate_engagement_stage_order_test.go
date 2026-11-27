package database

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestSQLiteStageGroupOrderMigration(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"), MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	before := fstest.MapFS{}
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() >= "000066" {
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
	var workspace int64
	if err := db.QueryRow(`INSERT INTO workspaces (name, created_at) VALUES ('Workspace', '2026-01-01T00:00:00Z') RETURNING id`).Scan(&workspace); err != nil {
		t.Fatal(err)
	}
	for i, row := range []struct {
		name, kind string
		archived   any
	}{{"lost", "lost", nil}, {"open-a", "open", nil}, {"won", "won", nil}, {"open-b", "open", "2026-01-01T00:00:00Z"}} {
		if _, err := db.Exec(`INSERT INTO engagement_stages (workspace_id, name, kind, position, archived_at) VALUES (?, ?, ?, ?, ?)`, workspace, row.name, row.kind, i+1, row.archived); err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT name, archived_at IS NOT NULL FROM engagement_stages WHERE workspace_id = ? ORDER BY position`, workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := []string{"open-a", "open-b", "won", "lost"}
	i := 0
	for rows.Next() {
		var name string
		var archived bool
		if err := rows.Scan(&name, &archived); err != nil {
			t.Fatal(err)
		}
		if i >= len(want) || name != want[i] || archived != (name == "open-b") {
			t.Fatalf("row %d: %s archived=%v", i, name, archived)
		}
		i++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if i != len(want) {
		t.Fatalf("got %d stages", i)
	}
}
