package database

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// shortIDKeyColumns are the audit, idempotency, and job columns that hold integer
// entity keys or a type discriminator. Each is typed, not text.
type shortIDKeyColumn struct{ table, column, sqliteType, postgresType string }

var shortIDKeyColumns = []shortIDKeyColumn{
	{"audit_events", "target_type", "TEXT", "text"},
	{"audit_events", "target_key", "BIGINT", "bigint"},
	{"idempotency_records", "principal_kind", "TEXT", "text"},
	{"idempotency_records", "principal_key", "BIGINT", "bigint"},
	{"background_jobs", "deduplication_key", "BIGINT", "bigint"},
}

// retiredTextColumns were replaced by the typed columns above and must not exist.
var retiredTextColumns = []userKeyColumn{
	{"audit_events", "target_id"},
	{"idempotency_records", "principal_id"},
}

func TestSQLiteShortIDKeyColumns(t *testing.T) {
	t.Parallel()

	db, err := Open(context.Background(), config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(context.Background(), db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	for _, column := range shortIDKeyColumns {
		var columnType string
		if err := db.QueryRow("SELECT type FROM pragma_table_info(?) WHERE name = ?", column.table, column.column).Scan(&columnType); err != nil {
			t.Fatalf("%s.%s: %v", column.table, column.column, err)
		}
		if columnType != column.sqliteType {
			t.Fatalf("%s.%s type = %q, want %s", column.table, column.column, columnType, column.sqliteType)
		}
	}
	for _, column := range retiredTextColumns {
		var found int
		if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", column.table, column.column).Scan(&found); err != nil {
			t.Fatal(err)
		}
		if found != 0 {
			t.Fatalf("%s.%s still exists; the typed column replaces it", column.table, column.column)
		}
	}
}

func TestPostgresShortIDKeyColumns(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}

	db, err := Open(context.Background(), config.Database{
		Driver: config.Postgres, URL: url, MaxOpenConns: 4, MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(context.Background(), db, config.Postgres); err != nil {
		t.Fatal(err)
	}
	dataType := func(table, column string) (string, error) {
		var value string
		err := db.QueryRow(`SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`,
			table, column).Scan(&value)
		return value, err
	}
	for _, column := range shortIDKeyColumns {
		value, err := dataType(column.table, column.column)
		if err != nil || value != column.postgresType {
			t.Fatalf("%s.%s type = %q, %v, want %s", column.table, column.column, value, err, column.postgresType)
		}
	}
	for _, column := range retiredTextColumns {
		_, err := dataType(column.table, column.column)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("%s.%s still exists or query failed: %v", column.table, column.column, err)
		}
	}
}
