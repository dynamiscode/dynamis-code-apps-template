package database

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestEngagementActionCompletionUpgrade(t *testing.T) {
	for _, driver := range []config.DatabaseDriver{config.SQLite, config.Postgres} {
		t.Run(string(driver), func(t *testing.T) {
			ctx := context.Background()
			cfg := config.Database{Driver: driver, MaxOpenConns: 1, MaxIdleConns: 1}
			if driver == config.SQLite {
				cfg.SQLitePath = filepath.Join(t.TempDir(), "app.db")
			} else {
				cfg.URL = os.Getenv("POSTGRES_ENGAGEMENT_ACTION_UPGRADE_TEST_URL")
				if cfg.URL == "" {
					t.Skip("POSTGRES_ENGAGEMENT_ACTION_UPGRADE_TEST_URL requires a dedicated empty database")
				}
			}
			db, err := Open(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if driver == config.Postgres {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema()`).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatal("upgrade test requires an empty database")
				}
			}
			before := fstest.MapFS{}
			entries, err := fs.ReadDir(migrationFiles, "migrations")
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.Name() >= "000044" {
					continue
				}
				path := "migrations/" + entry.Name()
				data, err := fs.ReadFile(migrationFiles, path)
				if err != nil {
					t.Fatal(err)
				}
				before[path] = &fstest.MapFile{Data: data}
			}
			if err := migrateFS(ctx, db, driver, before); err != nil {
				t.Fatal(err)
			}
			q := func(s string) string { return Rebind(driver, s) }
			var workspace, client, stage, engagement, history int64
			if err := db.QueryRow(q(`INSERT INTO workspaces (name, created_at) VALUES ('Workspace', ?) RETURNING id`), "2026-01-01T00:00:00Z").Scan(&workspace); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(q(`INSERT INTO clients (workspace_id, display_name, version, created_at, updated_at) VALUES (?, 'Client', 1, 't', 't') RETURNING id`), workspace).Scan(&client); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(q(`INSERT INTO engagement_stages (workspace_id, name, kind, position) VALUES (?, 'Open', 'open', 1) RETURNING id`), workspace).Scan(&stage); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(q(`INSERT INTO engagements (workspace_id, client_id, stage_id, notes) VALUES (?, ?, ?, 'notes') RETURNING id`), workspace, client, stage).Scan(&engagement); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(q(`INSERT INTO engagement_next_actions (workspace_id, engagement_id, action_text, due_date) VALUES (?, ?, 'Active', '2026-02-01')`), workspace, engagement); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(q(`INSERT INTO engagement_action_history (workspace_id, engagement_id, action_text, due_date, outcome, created_at) VALUES (?, ?, 'Previous', '2026-01-01', 'replaced', 'stamp') RETURNING id`), workspace, engagement).Scan(&history); err != nil {
				t.Fatal(err)
			}
			if err := Migrate(ctx, db, driver); err != nil {
				t.Fatal(err)
			}
			if err := Migrate(ctx, db, driver); err != nil {
				t.Fatal(err)
			}
			var text, date, outcome, stamp string
			var version int64
			if err := db.QueryRow(q(`SELECT action_text, due_date, version FROM engagement_next_actions WHERE workspace_id = ? AND engagement_id = ?`), workspace, engagement).Scan(&text, &date, &version); err != nil {
				t.Fatal(err)
			}
			if text != "Active" || date != "2026-02-01" || version != 1 {
				t.Fatalf("active = %q %q %d", text, date, version)
			}
			if err := db.QueryRow(q(`SELECT action_text, due_date, outcome, created_at FROM engagement_action_history WHERE id = ? AND workspace_id = ? AND engagement_id = ?`), history, workspace, engagement).Scan(&text, &date, &outcome, &stamp); err != nil {
				t.Fatal(err)
			}
			if text != "Previous" || date != "2026-01-01" || outcome != "replaced" || stamp != "stamp" {
				t.Fatalf("history = %q %q %q %q", text, date, outcome, stamp)
			}
			for _, value := range []string{"completed", "reopened", "cleared"} {
				var id int64
				if err := db.QueryRow(q(`INSERT INTO engagement_action_history (workspace_id, engagement_id, action_text, outcome, created_at) VALUES (?, ?, 'Action', ?, 'stamp') RETURNING id`), workspace, engagement, value).Scan(&id); err != nil {
					t.Fatal(err)
				}
				if id <= history {
					t.Fatalf("new history id %d <= %d", id, history)
				}
				history = id
			}
			if _, err := db.Exec(q(`INSERT INTO engagement_action_history (workspace_id, engagement_id, action_text, outcome, created_at) VALUES (?, ?, 'Action', 'invalid', 'stamp')`), workspace, engagement); err == nil {
				t.Fatal("invalid outcome accepted")
			}
			if _, err := db.Exec(q(`DELETE FROM engagements WHERE workspace_id = ? AND id = ?`), workspace, engagement); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(q(`SELECT COUNT(*) FROM engagement_action_history WHERE workspace_id = ?`), workspace).Scan(&count); err != nil || count != 0 {
				t.Fatalf("history after cascade = %d, %v", count, err)
			}
		})
	}
}
