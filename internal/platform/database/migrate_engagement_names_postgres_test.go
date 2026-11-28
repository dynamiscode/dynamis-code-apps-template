package database

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func TestPostgresEngagementNameUpgrade(t *testing.T) {
	url := os.Getenv("POSTGRES_ENGAGEMENT_NAMES_UPGRADE_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_ENGAGEMENT_NAMES_UPGRADE_TEST_URL requires a dedicated empty database")
	}
	ctx := context.Background()
	db, err := Open(ctx, config.Database{Driver: config.Postgres, URL: url, MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

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
	if err := migrateFS(ctx, db, config.Postgres, before); err != nil {
		t.Fatal(err)
	}

	const stamp = "2026-01-01T00:00:00Z"
	var workspace, stage int64
	if err := db.QueryRowContext(ctx, `INSERT INTO workspaces (name, created_at) VALUES ('Workspace', $1) RETURNING id`, stamp).Scan(&workspace); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO engagement_stages (workspace_id, name, kind, position) VALUES ($1, 'Open', 'open', 1) RETURNING id`, workspace).Scan(&stage); err != nil {
		t.Fatal(err)
	}
	type record struct {
		client, engagement int64
		name, notes        string
	}
	records := []record{{name: "Élan 東京", notes: "first notes"}, {name: strings.Repeat("界", 201), notes: "second notes"}}
	for i := range records {
		r := &records[i]
		if err := db.QueryRowContext(ctx, `INSERT INTO clients (workspace_id, display_name, version, created_at, updated_at) VALUES ($1, $2, 1, $3, $3) RETURNING id`, workspace, r.name, stamp).Scan(&r.client); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `INSERT INTO engagements (workspace_id, client_id, stage_id, notes) VALUES ($1, $2, $3, $4) RETURNING id`, workspace, r.client, stage, r.notes).Scan(&r.engagement); err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(ctx, db, config.Postgres); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE clients SET display_name = 'Renamed' WHERE workspace_id = $1`, workspace); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db, config.Postgres); err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		var got, notes string
		var gotWorkspace, gotClient, gotStage int64
		if err := db.QueryRowContext(ctx, `SELECT name, notes, workspace_id, client_id, stage_id FROM engagements WHERE id = $1`, r.engagement).Scan(&got, &notes, &gotWorkspace, &gotClient, &gotStage); err != nil {
			t.Fatal(err)
		}
		suffix, ok := strings.CutPrefix(got, r.name+" — ")
		decoded, err := id.DecodeKey(id.TypeEngagement, suffix)
		if !ok || err != nil || decoded != r.engagement || notes != r.notes || gotWorkspace != workspace || gotClient != r.client || gotStage != stage {
			t.Errorf("engagement %d = (%q, %q, %d, %d, %d), decoded key = %d, error = %v; want notes %q and keys (%d, %d, %d)", r.engagement, got, notes, gotWorkspace, gotClient, gotStage, decoded, err, r.notes, workspace, r.client, stage)
		}
	}
}
