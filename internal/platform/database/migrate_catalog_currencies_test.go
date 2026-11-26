package database

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestCatalogCurrencyMigrationPreservesPrices(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "catalog.db"), MaxOpenConns: 1, MaxIdleConns: 1})
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
		if name >= "migrations/000045" {
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
	if _, err := db.ExecContext(ctx, `INSERT INTO workspaces (id, name, created_at) VALUES (1, 'Legacy', '2026-01-01'); INSERT INTO catalog_entries (id, workspace_id, name) VALUES (1, 1, 'Entry'); INSERT INTO catalog_prices (entry_id, mode, amount, currency, variant, eligibility) VALUES (1, 'fixed', '0.123456789012345678', 'BTC', 'Standard', '')`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	var amount string
	if err := db.QueryRowContext(ctx, `SELECT amount FROM catalog_prices WHERE entry_id = 1`).Scan(&amount); err != nil || amount != "0.123456789012345678" {
		t.Fatalf("amount=%q err=%v", amount, err)
	}
	var code string
	var decimals int
	if err := db.QueryRowContext(ctx, `SELECT code, decimals FROM catalog_currencies WHERE workspace_id = 1 AND is_default = TRUE`).Scan(&code, &decimals); err != nil || code != "USD" || decimals != 2 {
		t.Fatalf("default=%s/%d err=%v", code, decimals, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT decimals FROM catalog_currencies WHERE workspace_id = 1 AND code = 'BTC'`).Scan(&decimals); err != nil || decimals != 18 {
		t.Fatalf("BTC precision=%d err=%v", decimals, err)
	}
}

func TestCatalogRevisionMigrationPreservesEntries(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "catalog-revision.db"), MaxOpenConns: 1, MaxIdleConns: 1})
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
		if name >= "migrations/000056" {
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
	if _, err := db.ExecContext(ctx, `INSERT INTO workspaces (id, name, created_at) VALUES (1, 'Legacy', '2026-01-01'); INSERT INTO catalog_entries (id, workspace_id, name, duration_minutes) VALUES (1, 1, 'Entry', 45)`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	var name string
	var duration, revision int64
	if err := db.QueryRowContext(ctx, `SELECT name, duration_minutes, revision FROM catalog_entries WHERE id = 1 AND workspace_id = 1`).Scan(&name, &duration, &revision); err != nil || name != "Entry" || duration != 45 || revision != 1 {
		t.Fatalf("entry=%q duration=%d revision=%d err=%v", name, duration, revision, err)
	}
	var seed int64
	if err := db.QueryRowContext(ctx, `SELECT revision FROM catalog_entry_revision_seeds WHERE entry_id = 1`).Scan(&seed); err != nil || seed != revision {
		t.Fatalf("seed=%d err=%v", seed, err)
	}
}
