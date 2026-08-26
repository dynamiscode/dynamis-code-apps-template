package backup

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func TestSQLiteBackupRestoreAndEvidence(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	cfg := config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(directory, "source.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	}
	db, err := database.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO workspaces (id, name, created_at) VALUES (?, ?, ?)",
		"backup-workspace", "Known backup record", "2026-08-25T12:00:00Z"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 25, 13, 0, 0, 0, time.UTC)
	backupPath := filepath.Join(directory, "app.backup")
	if _, err := Create(ctx, db, cfg, backupPath, now); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	db.Close()

	restored := filepath.Join(directory, "restored.db")
	if err := Restore(ctx, cfg, backupPath, restored, now.Add(time.Hour), 24*time.Hour); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	restoredDB, err := database.Open(ctx, config.Database{
		Driver: config.SQLite, SQLitePath: restored, MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer restoredDB.Close()
	var name string
	if err := restoredDB.QueryRow("SELECT name FROM workspaces WHERE id = ?", "backup-workspace").Scan(&name); err != nil || name != "Known backup record" {
		t.Fatalf("restored name = %q, error = %v", name, err)
	}

	if _, err := Verify(backupPath, config.SQLite, now.Add(48*time.Hour), 24*time.Hour); !errors.Is(err, ErrStale) {
		t.Fatalf("Verify(stale) error = %v", err)
	}
	file, err := os.OpenFile(backupPath, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte("corrupt"), 0); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := Verify(backupPath, config.SQLite, now.Add(time.Hour), 24*time.Hour); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Verify(corrupt) error = %v", err)
	}
}

func TestPostgresBackupRestoreAndEvidence(t *testing.T) {
	rawURL := os.Getenv("POSTGRES_TEST_URL")
	if rawURL == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump is not installed")
	}
	if _, err := exec.LookPath("pg_restore"); err != nil {
		t.Skip("pg_restore is not installed")
	}
	ctx := context.Background()
	sourceCfg := config.Database{
		Driver: config.Postgres, URL: rawURL, MaxOpenConns: 2, MaxIdleConns: 1,
	}
	source, err := database.Open(ctx, sourceCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { source.Close() })
	if err := database.Migrate(ctx, source, config.Postgres); err != nil {
		t.Fatal(err)
	}
	workspaceID := fmt.Sprintf("backup-%d", time.Now().UnixNano())
	if _, err := source.Exec(
		"INSERT INTO workspaces (id, name, created_at) VALUES ($1, $2, $3)",
		workspaceID, "Known PostgreSQL backup record", "2026-08-25T12:00:00Z",
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = source.Exec("DELETE FROM workspaces WHERE id = $1", workspaceID) })

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	targetName := fmt.Sprintf("dynamis_code_restore_%d", time.Now().UnixNano())
	adminURL := *parsed
	adminURL.Path = "/postgres"
	admin, err := database.Open(ctx, config.Database{
		Driver: config.Postgres, URL: adminURL.String(), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	quotedTarget := pgx.Identifier{targetName}.Sanitize()
	if _, err := admin.Exec("CREATE DATABASE " + quotedTarget); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE " + quotedTarget + " WITH (FORCE)") })
	targetURL := *parsed
	targetURL.Path = "/" + targetName
	targetCfg := config.Database{
		Driver: config.Postgres, URL: targetURL.String(), MaxOpenConns: 2, MaxIdleConns: 1,
	}

	now := time.Date(2026, 8, 25, 13, 0, 0, 0, time.UTC)
	backupPath := filepath.Join(t.TempDir(), "postgres.dump")
	if _, err := Create(ctx, source, sourceCfg, backupPath, now); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := Restore(ctx, targetCfg, backupPath, "", now.Add(time.Hour), 24*time.Hour); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	target, err := database.Open(ctx, targetCfg)
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := target.QueryRow("SELECT name FROM workspaces WHERE id = $1", workspaceID).Scan(&name); err != nil || name != "Known PostgreSQL backup record" {
		t.Fatalf("restored name = %q, error = %v", name, err)
	}
	target.Close()
	if _, err := Verify(backupPath, config.Postgres, now.Add(48*time.Hour), 24*time.Hour); !errors.Is(err, ErrStale) {
		t.Fatalf("Verify(stale) error = %v", err)
	}
	file, err := os.OpenFile(backupPath, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte("corrupt"), 0); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := Verify(backupPath, config.Postgres, now.Add(time.Hour), 24*time.Hour); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Verify(corrupt) error = %v", err)
	}
}
