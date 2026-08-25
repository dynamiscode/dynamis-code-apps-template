package database

import (
	"context"
	"os"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestPostgresMigrations(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}

	db, err := Open(context.Background(), config.Database{
		Driver:       config.Postgres,
		URL:          url,
		MaxOpenConns: 4,
		MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := Migrate(context.Background(), db, config.Postgres); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := Migrate(context.Background(), db, config.Postgres); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations WHERE version = $1",
		1,
	).Scan(&count); err != nil {
		t.Fatalf("query migration version: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration version count = %d, want 1", count)
	}
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations WHERE version = $1",
		2,
	).Scan(&count); err != nil {
		t.Fatalf("query identity migration version: %v", err)
	}
	if count != 1 {
		t.Fatalf("identity migration version count = %d, want 1", count)
	}
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations WHERE version = $1",
		3,
	).Scan(&count); err != nil {
		t.Fatalf("query HTTP migration version: %v", err)
	}
	if count != 1 {
		t.Fatalf("HTTP migration version count = %d, want 1", count)
	}
}
