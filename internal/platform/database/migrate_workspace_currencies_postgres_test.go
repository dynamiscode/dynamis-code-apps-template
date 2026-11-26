package database

import (
	"context"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestPostgresWorkspaceCurrenciesUpgrade(t *testing.T) {
	url := os.Getenv("POSTGRES_WORKSPACE_CURRENCIES_UPGRADE_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_WORKSPACE_CURRENCIES_UPGRADE_TEST_URL requires a dedicated empty database")
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
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name >= "migrations/000063" {
			continue
		}
		data, err := fs.ReadFile(migrationFiles, name)
		if err != nil {
			t.Fatal(err)
		}
		before[name] = &fstest.MapFile{Data: data}
	}
	if err := migrateFS(ctx, db, config.Postgres, before); err != nil {
		t.Fatal(err)
	}
	var workspace int64
	if err := db.QueryRowContext(ctx, `INSERT INTO workspaces (name, created_at) VALUES ('Legacy', '2026-01-01') RETURNING id`).Scan(&workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO catalog_currencies (workspace_id, code, decimals, is_default) VALUES ($1, 'BTC', 18, TRUE), ($1, 'OLD', 7, FALSE)`, workspace); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db, config.Postgres); err != nil {
		t.Fatal(err)
	}
	var code string
	var decimals int
	if err := db.QueryRowContext(ctx, `SELECT code, decimals FROM workspace_currencies WHERE workspace_id = $1 AND is_default = TRUE`, workspace).Scan(&code, &decimals); err != nil || code != "BTC" || decimals != 18 {
		t.Fatalf("default=%s/%d err=%v", code, decimals, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT decimals FROM workspace_currencies WHERE workspace_id = $1 AND code = 'OLD'`, workspace).Scan(&decimals); err != nil || decimals != 7 {
		t.Fatalf("legacy precision=%d err=%v", decimals, err)
	}
}
