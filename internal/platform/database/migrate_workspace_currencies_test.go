package database

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestWorkspaceCurrenciesMigrationPreservesCatalog(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "currencies.db"), MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	legacy := fstest.MapFS{}
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
		legacy[name] = &fstest.MapFile{Data: data}
	}
	if err := migrateFS(ctx, db, config.SQLite, legacy); err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO workspaces (id, name, created_at) VALUES (1, 'Legacy', '2026-01-01');
		INSERT INTO catalog_currencies (workspace_id, code, decimals, is_default) VALUES (1, 'BTC', 18, TRUE), (1, 'OLD', 7, FALSE);
		INSERT INTO catalog_entries (id, workspace_id, name) VALUES (1, 1, 'Entry');
		INSERT INTO catalog_prices (entry_id, mode, amount, currency, variant, eligibility) VALUES (1, 'fixed', '0.123456789012345678', 'BTC', 'Standard', '')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	var code, amount string
	var decimals int
	if err := db.QueryRowContext(ctx, `SELECT code, decimals FROM workspace_currencies WHERE workspace_id = 1 AND is_default = TRUE`).Scan(&code, &decimals); err != nil || code != "BTC" || decimals != 18 {
		t.Fatalf("default=%s/%d err=%v", code, decimals, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT decimals FROM workspace_currencies WHERE workspace_id = 1 AND code = 'OLD'`).Scan(&decimals); err != nil || decimals != 7 {
		t.Fatalf("legacy precision=%d err=%v", decimals, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT amount FROM catalog_prices WHERE entry_id = 1`).Scan(&amount); err != nil || amount != "0.123456789012345678" {
		t.Fatalf("amount=%s err=%v", amount, err)
	}
}

func TestWorkspaceCurrenciesMigrationWithoutCatalog(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "no-catalog.db"), MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	data, err := fs.ReadFile(migrationFiles, "migrations/000063_workspace_currencies.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := fstest.MapFS{
		"migrations/000001_workspaces.sql":           &fstest.MapFile{Data: []byte(`CREATE TABLE workspaces (id BIGINT PRIMARY KEY); INSERT INTO workspaces (id) VALUES (1);`)},
		"migrations/000063_workspace_currencies.sql": &fstest.MapFile{Data: data},
	}
	if err := migrateFS(ctx, db, config.SQLite, source); err != nil {
		t.Fatal(err)
	}
	var code string
	var decimals int
	if err := db.QueryRowContext(ctx, `SELECT code, decimals FROM workspace_currencies WHERE workspace_id = 1 AND is_default = TRUE`).Scan(&code, &decimals); err != nil || code != "USD" || decimals != 2 {
		t.Fatalf("default=%s/%d err=%v", code, decimals, err)
	}
}
