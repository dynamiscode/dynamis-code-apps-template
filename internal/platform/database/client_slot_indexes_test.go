package database

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// clientSlotCount is the number of custom-field slots in ADR 0012.
const clientSlotCount = 50

func TestSQLiteClientSlotIndexesExistForEverySlot(t *testing.T) {
	t.Parallel()

	db := migratedSQLite(t)
	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name LIKE 'clients_custom_slot_%_idx'",
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != clientSlotCount {
		t.Fatalf("slot indexes = %d, want %d", count, clientSlotCount)
	}
	for slot := 1; slot <= clientSlotCount; slot++ {
		name := fmt.Sprintf("clients_custom_slot_%02d_idx", slot)
		var found int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", name).Scan(&found); err != nil {
			t.Fatal(err)
		}
		if found != 1 {
			t.Fatalf("index %s missing", name)
		}
	}
}

func TestSQLiteClientSlotIndexServesWorkspaceScopedSlotFilter(t *testing.T) {
	t.Parallel()

	db := migratedSQLite(t)
	rows, err := db.Query(`
		EXPLAIN QUERY PLAN
		SELECT id FROM clients
		WHERE workspace_id = ? AND json_extract(custom_values, '$.f07') = ?
	`, 1, "x")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "clients_custom_slot_07_idx") {
		t.Fatalf("query plan does not use the slot index:\n%s", plan.String())
	}
}

func migratedSQLite(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	return db
}
