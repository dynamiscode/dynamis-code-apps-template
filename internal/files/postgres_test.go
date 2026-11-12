package files

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// TestPostgresFileStateTransitions exercises the file transition executor
// against a fresh PostgreSQL database, covering Postgres placeholder rebinding
// and the typed FileStatus scan on the Postgres driver.
func TestPostgresFileStateTransitions(t *testing.T) {
	baseURL := os.Getenv("POSTGRES_TEST_URL")
	if baseURL == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	admin, err := database.Open(ctx, config.Database{Driver: config.Postgres, URL: baseURL, MaxOpenConns: 2, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "files_" + hex.EncodeToString(suffix[:])
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	db, err := database.Open(ctx, config.Database{Driver: config.Postgres, URL: parsed.String(), MaxOpenConns: 4, MaxIdleConns: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.Postgres); err != nil {
		t.Fatal(err)
	}

	auth, err := identity.NewService(db, config.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "files-postgres@example.com", Password: "long-enough-password", WorkspaceName: "Files PostgreSQL",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := newLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, config.Postgres, auth, store, 100, 100, 0, "")
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	initiate := func(originalName string) File {
		t.Helper()
		file, err := service.Initiate(ctx, actor, owner.WorkspaceID, InitiateInput{
			OriginalName: originalName, Size: 10, ContentType: "text/plain",
		}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		return file
	}
	statusOf := func(fileID FileID) FileStatus {
		t.Helper()
		var status FileStatus
		if err := db.QueryRowContext(ctx, "SELECT status FROM files WHERE id = $1", fileID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		return status
	}

	ready := initiate("postgres_ready.txt")
	if err := service.transitionFile(ctx, nil, ready.ID, Pending, Ready); err != nil {
		t.Fatalf("Pending -> Ready on Postgres failed: %v", err)
	}
	if got := statusOf(ready.ID); got != Ready {
		t.Fatalf("status = %q, want %q", got, Ready)
	}
	if err := service.transitionFile(ctx, nil, ready.ID, Pending, Failed); err == nil {
		t.Fatal("stale Pending -> Failed on a Ready file succeeded, want error")
	}

	failed := initiate("postgres_failed.txt")
	service.markFailed(ctx, failed.ID)
	if got := statusOf(failed.ID); got != Failed {
		t.Fatalf("markFailed status = %q, want %q", got, Failed)
	}

	inTx := initiate("postgres_tx.txt")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := service.transitionFile(ctx, tx, inTx.ID, Pending, Ready); err != nil {
		t.Fatalf("transaction Pending -> Ready on Postgres failed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(inTx.ID); got != Ready {
		t.Fatalf("transaction status = %q, want %q", got, Ready)
	}
}
