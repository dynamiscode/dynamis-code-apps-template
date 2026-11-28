package database

import (
	"context"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestPostgresStageGroupOrderMigration(t *testing.T) {
	url := os.Getenv("POSTGRES_STAGE_ORDER_UPGRADE_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_STAGE_ORDER_UPGRADE_TEST_URL requires a dedicated empty database")
	}
	ctx := context.Background()
	db, err := Open(ctx, config.Database{Driver: config.Postgres, URL: url, MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var tables int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema()`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatal("upgrade test requires an empty database")
	}
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
	if err := migrateFS(ctx, db, config.Postgres, before); err != nil {
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
		if _, err := db.Exec(`INSERT INTO engagement_stages (workspace_id, name, kind, position, archived_at) VALUES ($1, $2, $3, $4, $5)`, workspace, row.name, row.kind, i+1, row.archived); err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(ctx, db, config.Postgres); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT name, archived_at IS NOT NULL FROM engagement_stages WHERE workspace_id = $1 ORDER BY position`, workspace)
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
