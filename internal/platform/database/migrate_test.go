package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestSQLiteConfigurationAndMigrations(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "app.db")
	db, err := Open(context.Background(), config.Database{
		Driver:       config.SQLite,
		SQLitePath:   path,
		MaxOpenConns: 4,
		MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := Migrate(context.Background(), db, config.SQLite); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if err := Migrate(context.Background(), db, config.SQLite); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	var foreignKeys int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}

	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	assertMigrationVersion(t, db, 1)
	assertMigrationVersion(t, db, 2)
	assertMigrationVersion(t, db, 3)
	assertMigrationVersion(t, db, 4)
	assertMigrationVersion(t, db, 5)
	assertMigrationVersion(t, db, 6)
	assertMigrationVersion(t, db, 7)
}

func TestLoadMigrationsRejectsDuplicateVersion(t *testing.T) {
	t.Parallel()

	_, err := loadMigrations(fstest.MapFS{
		"migrations/000001_first.sql":  {Data: []byte("SELECT 1")},
		"migrations/000001_second.sql": {Data: []byte("SELECT 2")},
	})
	if err == nil {
		t.Fatal("loadMigrations() error = nil, want duplicate version error")
	}
}

func TestSQLiteMigrationFailureRollsBack(t *testing.T) {
	t.Parallel()

	db, err := Open(context.Background(), config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	err = migrateFS(context.Background(), db, config.SQLite, fstest.MapFS{
		"migrations/000001_interrupted.sql": {Data: []byte(
			"CREATE TABLE interrupted (id TEXT); INSERT INTO missing_table VALUES (1);",
		)},
	})
	if err == nil {
		t.Fatal("migrateFS() error = nil, want failure")
	}
	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'interrupted'",
	).Scan(&count); err != nil {
		t.Fatalf("query interrupted table: %v", err)
	}
	if count != 0 {
		t.Fatalf("interrupted table count = %d, want 0", count)
	}
}

func TestOpenRejectsPostgresURLWithoutLeakingSecret(t *testing.T) {
	t.Parallel()

	secret := "super-secret"
	_, err := Open(context.Background(), config.Database{
		Driver:       config.Postgres,
		URL:          "postgres://user:" + secret + "@%",
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err == nil {
		t.Fatal("Open() error = nil, want validation error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("Open() error leaked DATABASE_URL secret: %v", err)
	}
}

func assertMigrationVersion(t *testing.T, queryer interface {
	QueryRow(string, ...any) *sql.Row
}, version int64) {
	t.Helper()

	var count int
	if err := queryer.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations WHERE version = ?",
		version,
	).Scan(&count); err != nil {
		t.Fatalf("query migration version: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration version %d count = %d, want 1", version, count)
	}
}
