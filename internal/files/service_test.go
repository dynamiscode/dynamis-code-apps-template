package files

import (
	"context"
	"database/sql"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestLocalFileLifecycleAndValidation(t *testing.T) {
	db := openTestDB(t)
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(context.Background(), identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Files",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := newLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, config.SQLite, auth, store, 16, 32, 0, "prefix")
	actor, err := auth.Authorize(context.Background(), owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	file, err := service.Upload(context.Background(), actor, owner.WorkspaceID, "../notes.txt", strings.NewReader("hello"), identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if file.Status != Ready || file.OriginalName != "notes.txt" || file.Size != 5 || file.SHA256 == nil {
		t.Fatalf("file = %+v", file)
	}
	_, reader, err := service.Open(context.Background(), actor, owner.WorkspaceID, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || string(content) != "hello" {
		t.Fatalf("content = %q, error = %v", content, err)
	}
	if _, err := service.Upload(context.Background(), actor, owner.WorkspaceID, "bad.html", strings.NewReader("<html>"), identity.AuditContext{}); err != ErrInvalidInput {
		t.Fatalf("HTML upload error = %v, want ErrInvalidInput", err)
	}
	if _, err := service.Upload(context.Background(), actor, owner.WorkspaceID, "too.txt", strings.NewReader("012345678901234567"), identity.AuditContext{}); err != ErrLimit {
		t.Fatalf("oversized upload error = %v, want ErrLimit", err)
	}
}

func TestWorkspaceQuotaAndIsolation(t *testing.T) {
	db := openTestDB(t)
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	first, err := auth.BootstrapFirstOwner(context.Background(), identity.BootstrapInput{
		Email: "first@example.com", Password: "long-enough-password", WorkspaceName: "First",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := newLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, config.SQLite, auth, store, 16, 5, 0, "")
	actor, err := auth.Authorize(context.Background(), first.UserID, first.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Upload(context.Background(), actor, first.WorkspaceID, "one.txt", strings.NewReader("hello"), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Upload(context.Background(), actor, first.WorkspaceID, "two.txt", strings.NewReader("x"), identity.AuditContext{}); err != ErrLimit {
		t.Fatalf("quota error = %v, want ErrLimit", err)
	}
	if _, err := service.List(context.Background(), actor, strings.Repeat("0", 32), 10); err != identity.ErrForbidden {
		t.Fatalf("cross-workspace list error = %v, want forbidden", err)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(context.Background(), config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(context.Background(), db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	return db
}
