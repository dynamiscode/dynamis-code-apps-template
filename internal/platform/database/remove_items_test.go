package database

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestSQLiteRemoveItemsUpgrade(t *testing.T) {
	testRemoveItemsUpgrade(t, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"), MaxOpenConns: 1, MaxIdleConns: 1})
}

func TestPostgresRemoveItemsUpgrade(t *testing.T) {
	url := os.Getenv("POSTGRES_ITEMS_UPGRADE_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_ITEMS_UPGRADE_TEST_URL requires a dedicated empty database")
	}
	testRemoveItemsUpgrade(t, config.Database{Driver: config.Postgres, URL: url, MaxOpenConns: 1, MaxIdleConns: 1})
}

func testRemoveItemsUpgrade(t *testing.T, cfg config.Database) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if cfg.Driver == config.Postgres {
		var tables int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema()`).Scan(&tables); err != nil {
			t.Fatal(err)
		}
		if tables != 0 {
			t.Fatal("upgrade test requires an empty database")
		}
	}
	legacy := fstest.MapFS{}
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "000030_") {
			continue
		}
		name := "migrations/" + entry.Name()
		data, err := fs.ReadFile(migrationFiles, name)
		if err != nil {
			t.Fatal(err)
		}
		legacy[name] = &fstest.MapFile{Data: data}
	}
	if err := migrateFS(ctx, db, cfg.Driver, legacy); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO workspaces (id, name, created_at) VALUES (1, 'Workspace', '2026-01-01')`,
		`INSERT INTO items (id, workspace_id, title, status, version, created_at, updated_at) VALUES (1, 1, 'Legacy', 'active', 1, '2026-01-01', '2026-01-01')`,
		`INSERT INTO item_events (workspace_id, item_id, event_type, item_version, occurred_at) VALUES (1, 1, 'created', 1, '2026-01-01')`,
		`INSERT INTO clients (workspace_id, display_name, version, created_at, updated_at) VALUES (1, 'Retained client', 1, '2026-01-01', '2026-01-01')`,
		`INSERT INTO public_links (workspace_id, item_id, token_hash, created_at, expires_at) VALUES (1, 1, 'hash', '2026-01-01', '2027-01-01')`,
		`INSERT INTO audit_events (event_type, auth_method, target_type, action, outcome, metadata, created_at) VALUES ('item.created', 'system', 'item', 'create', 'success', '{}', '2026-01-01')`,
	} {
		if cfg.Driver == config.Postgres && (strings.HasPrefix(query, "INSERT INTO workspaces ") || strings.HasPrefix(query, "INSERT INTO items ")) {
			query = strings.Replace(query, " VALUES ", " OVERRIDING SYSTEM VALUE VALUES ", 1)
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(ctx, db, cfg.Driver); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"items", "item_events", "public_links"} {
		var count int
		query := `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`
		if cfg.Driver == config.Postgres {
			query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ?`
		}
		if err := db.QueryRowContext(ctx, Rebind(cfg.Driver, query), table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s exists: count %d, error %v", table, count, err)
		}
	}
	for _, table := range []string{"workspaces", "clients", "audit_events"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s retained: count %d, error %v", table, count, err)
		}
	}
}
