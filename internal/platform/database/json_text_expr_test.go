package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// The {{json_text_expr:COLUMN:KEY}} placeholder is generic: it stays in every
// generated application, unlike the Clients slot-index tests beside it.

func TestSQLiteJSONTextExprReadsSlotValue(t *testing.T) {
	t.Parallel()

	db, err := Open(context.Background(), config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var got sql.NullString
	query := renderMigration(
		`SELECT {{json_text_expr:custom_values:f01}} FROM (SELECT '{"f01":"Ada"}' AS custom_values)`,
		config.SQLite,
	)
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("rendered %q: %v", query, err)
	}
	if got.String != "Ada" {
		t.Fatalf("slot value = %q, want Ada", got.String)
	}
}

func TestPostgresJSONTextExprReadsSlotValue(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	db, err := Open(ctx, config.Database{
		Driver: config.Postgres, URL: url, MaxOpenConns: 4, MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var got sql.NullString
	query := renderMigration(
		`SELECT {{json_text_expr:custom_values:f01}} FROM (SELECT '{"f01":"Ada"}'::text AS custom_values) AS s`,
		config.Postgres,
	)
	if err := db.QueryRowContext(ctx, query).Scan(&got); err != nil {
		t.Fatalf("rendered %q: %v", query, err)
	}
	if got.String != "Ada" {
		t.Fatalf("slot value = %q, want Ada", got.String)
	}
}
