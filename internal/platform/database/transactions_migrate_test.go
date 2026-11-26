package database

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestTransactionsClientErasureMigrationPreservesEntries(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"), MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	legacy := fstest.MapFS{}
	files, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Name() >= "000058" {
			continue
		}
		path := "migrations/" + file.Name()
		data, err := fs.ReadFile(migrationFiles, path)
		if err != nil {
			t.Fatal(err)
		}
		legacy[path] = &fstest.MapFile{Data: data}
	}
	if err := migrateFS(ctx, db, config.SQLite, legacy); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO workspaces (id, name, created_at) VALUES (1, 'Workspace', '2026-01-01')`,
		`INSERT INTO clients (id, workspace_id, display_name, version, created_at, updated_at) VALUES (1, 1, 'Client', 1, '2026-01-01', '2026-01-01')`,
		`INSERT INTO transactions_payment_methods (workspace_id, name) VALUES (1, 'cash')`,
		`INSERT INTO transactions_entries (id, workspace_id, direction, amount_minor, currency, payment_method, occurred_on, client_id, note, created_by, created_at) VALUES (123, 1, 'in', 500, 'USD', 'cash', '2026-01-01', 1, 'sale', 1, '2026-01-01')`,
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM clients WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	var amount, revision int
	var clientID sql.NullInt64
	var note, description string
	if err := db.QueryRowContext(ctx, `SELECT amount_minor, client_id, note, description, revision FROM transactions_entries WHERE id = 123`).Scan(&amount, &clientID, &note, &description, &revision); err != nil {
		t.Fatal(err)
	}
	if amount != 500 || clientID.Valid || note != "sale" || description != "" || revision != 2 {
		t.Fatalf("retained entry: amount=%d client=%v note=%q description=%q revision=%d", amount, clientID, note, description, revision)
	}
}
