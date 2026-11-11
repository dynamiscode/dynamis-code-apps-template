package files

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	service := NewService(db, config.SQLite, auth, store, 16, 16, 0, "prefix")
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
	otherService := NewService(db, config.SQLite, auth, store, 16, 32, 0, "changed-prefix")
	_, reader, err = otherService.Open(context.Background(), actor, owner.WorkspaceID, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	content, err = io.ReadAll(reader)
	reader.Close()
	if err != nil || string(content) != "hello" {
		t.Fatalf("content after prefix change = %q, error = %v", content, err)
	}
	if _, err := service.Initiate(context.Background(), actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "payload.exe", Size: 5,
	}, identity.AuditContext{}); err != ErrInvalidInput {
		t.Fatalf("unsupported initiated file error = %v, want ErrInvalidInput", err)
	}
	pending, err := service.Initiate(context.Background(), actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "data.csv", Size: 5, ContentType: "text/csv",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), service.objectKey(pending), strings.NewReader("123456"), 6, "text/plain"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.PresignedPut(context.Background(), actor, strings.Repeat("0", 32), pending.ID); err != identity.ErrForbidden {
		t.Fatalf("cross-workspace presign error = %v, want forbidden", err)
	}
	if _, err := service.Complete(context.Background(), actor, owner.WorkspaceID, pending.ID, identity.AuditContext{}); err != ErrInvalidInput {
		t.Fatalf("mismatched external object error = %v, want ErrInvalidInput", err)
	}
	if _, err := service.Initiate(context.Background(), actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "blocked.txt", Size: 7, ContentType: "text/plain",
	}, identity.AuditContext{}); err != ErrLimit {
		t.Fatalf("failed object quota error = %v, want ErrLimit", err)
	}
	if _, err := service.Upload(context.Background(), actor, owner.WorkspaceID, "bad.html", strings.NewReader("<html>"), identity.AuditContext{}); err != ErrInvalidInput {
		t.Fatalf("HTML upload error = %v, want ErrInvalidInput", err)
	}
	if _, err := service.Upload(context.Background(), actor, owner.WorkspaceID, "too.txt", strings.NewReader("012345678901234567"), identity.AuditContext{}); err != ErrObjectLimit {
		t.Fatalf("oversized upload error = %v, want ErrObjectLimit", err)
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

func TestPresignFailureReleasesPendingQuota(t *testing.T) {
	db := openTestDB(t)
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(context.Background(), identity.BootstrapInput{
		Email: "presign@example.com", Password: "long-enough-password", WorkspaceName: "Presign",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := newLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, config.SQLite, auth, &presignFailureStore{ObjectStore: store}, 5, 5, 0, "")
	actor, err := auth.Authorize(context.Background(), owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	file, err := service.Initiate(context.Background(), actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "one.txt", Size: 5, ContentType: "text/plain",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.PresignedPut(context.Background(), actor, owner.WorkspaceID, file.ID); err == nil {
		t.Fatal("presign error = nil")
	}
	if _, err := service.Initiate(context.Background(), actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "two.txt", Size: 5, ContentType: "text/plain",
	}, identity.AuditContext{}); err != nil {
		t.Fatalf("quota after presign failure = %v, want available", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM files WHERE id = ?", file.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("released file count = %d, want 0", count)
	}
}

type presignFailureStore struct {
	ObjectStore
}

func (*presignFailureStore) SupportsPresignedPut() bool { return true }

func (*presignFailureStore) PresignPut(context.Context, string, int64, string, time.Duration) (PresignedUpload, error) {
	return PresignedUpload{}, errors.New("presign failed")
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

func TestFileStateTransitions(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "transitions@example.com", Password: "long-enough-password", WorkspaceName: "Transitions",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := newLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, config.SQLite, auth, store, 100, 100, 0, "")
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Pending -> Ready (valid transition)
	file1, err := service.Initiate(ctx, actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "valid_ready.txt", Size: 10, ContentType: "text/plain",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.transitionFile(ctx, nil, file1.ID, Pending, Ready); err != nil {
		t.Fatalf("Pending -> Ready transition failed: %v", err)
	}
	var status1 FileStatus
	if err := db.QueryRow("SELECT status FROM files WHERE id = ?", file1.ID).Scan(&status1); err != nil {
		t.Fatal(err)
	}
	if status1 != Ready {
		t.Fatalf("expected status %s, got %s", Ready, status1)
	}

	// 2. Pending -> Failed (valid transition)
	file2, err := service.Initiate(ctx, actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "valid_failed.txt", Size: 10, ContentType: "text/plain",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.transitionFile(ctx, nil, file2.ID, Pending, Failed); err != nil {
		t.Fatalf("Pending -> Failed transition failed: %v", err)
	}
	var status2 FileStatus
	if err := db.QueryRow("SELECT status FROM files WHERE id = ?", file2.ID).Scan(&status2); err != nil {
		t.Fatal(err)
	}
	if status2 != Failed {
		t.Fatalf("expected status %s, got %s", Failed, status2)
	}

	// 3. Failed -> Ready (invalid transition: must fail)
	if err := service.transitionFile(ctx, nil, file2.ID, Failed, Ready); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Failed -> Ready error = %v, want ErrInvalidTransition", err)
	}

	// 4. Ready -> Failed (invalid transition: must fail)
	if err := service.transitionFile(ctx, nil, file1.ID, Ready, Failed); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Ready -> Failed error = %v, want ErrInvalidTransition", err)
	}

	// 5. Ready -> Ready (invalid self transition)
	if err := service.transitionFile(ctx, nil, file1.ID, Ready, Ready); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Ready -> Ready error = %v, want ErrInvalidTransition", err)
	}

	// 6. Failed -> Failed (invalid self transition)
	if err := service.transitionFile(ctx, nil, file2.ID, Failed, Failed); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Failed -> Failed error = %v, want ErrInvalidTransition", err)
	}

	// 7. Pending -> Pending (invalid self transition)
	file3, err := service.Initiate(ctx, actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "pending_pending.txt", Size: 10, ContentType: "text/plain",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.transitionFile(ctx, nil, file3.ID, Pending, Pending); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Pending -> Pending error = %v, want ErrInvalidTransition", err)
	}

	// 8. Transition on non-existent file
	if err := service.transitionFile(ctx, nil, strings.Repeat("a", 32), Pending, Ready); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-existent file transition error = %v, want ErrNotFound", err)
	}

	// 9. FromStatus mismatch when file in DB is already Failed
	if err := service.transitionFile(ctx, nil, file2.ID, Pending, Ready); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("stale fromStatus transition error = %v, want ErrInvalidTransition", err)
	}

	// 10. Within a transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := service.transitionFile(ctx, tx, file3.ID, Pending, Ready); err != nil {
		t.Fatalf("tx transition Pending -> Ready failed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var status3 FileStatus
	if err := db.QueryRow("SELECT status FROM files WHERE id = ?", file3.ID).Scan(&status3); err != nil {
		t.Fatal(err)
	}
	if status3 != Ready {
		t.Fatalf("expected status %s, got %s", Ready, status3)
	}
}

func TestQuotaCalculationCleanupStaleAndFailedFiles(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "quota_cleanup@example.com", Password: "long-enough-password", WorkspaceName: "QuotaCleanup",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := newLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	// Total workspace quota is 10 bytes.
	service := NewService(db, config.SQLite, auth, store, 10, 10, 0, "")
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}

	// File 1: Ready file of 5 bytes.
	_, err = service.Upload(ctx, actor, owner.WorkspaceID, "file1.txt", strings.NewReader("12345"), identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}

	// File 2: Initiate 5 bytes (Pending) -> used is now 10 (quota full).
	file2, err := service.Initiate(ctx, actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "file2.txt", Size: 5, ContentType: "text/plain",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}

	// Attempting file 3 should fail due to quota limit.
	_, err = service.Initiate(ctx, actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "file3.txt", Size: 5, ContentType: "text/plain",
	}, identity.AuditContext{})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("expected ErrLimit, got %v", err)
	}

	// Transition file2 to Failed. It should still count against quota until cleaned up.
	if err := service.transitionFile(ctx, nil, file2.ID, Pending, Failed); err != nil {
		t.Fatal(err)
	}

	// Attempting file 3 should still fail because failed files are quota-reserved.
	_, err = service.Initiate(ctx, actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "file3.txt", Size: 5, ContentType: "text/plain",
	}, identity.AuditContext{})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("expected ErrLimit for failed file still reserving quota, got %v", err)
	}

	// Clean up stale/failed files.
	cleaned, err := service.CleanupStaleFiles(ctx, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("CleanupStaleFiles failed: %v", err)
	}
	if cleaned != 1 {
		t.Fatalf("expected 1 cleaned file, got %d", cleaned)
	}

	// Now file 3 should succeed because quota was released!
	file3, err := service.Initiate(ctx, actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "file3.txt", Size: 5, ContentType: "text/plain",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatalf("expected initiate to succeed after cleanup, got: %v", err)
	}
	if file3.ID == "" {
		t.Fatal("expected valid file ID")
	}
}

func TestMarkFailedLogsTransitionError(t *testing.T) {
	var logs bytes.Buffer
	service := NewService(openTestDB(t), config.SQLite, nil, nil, 0, 0, 0, "")
	service.logger = slog.New(slog.NewTextHandler(&logs, nil))
	missingID := strings.Repeat("a", 32)
	service.markFailed(context.Background(), missingID)

	if !strings.Contains(logs.String(), "file_id="+missingID) || !strings.Contains(logs.String(), ErrNotFound.Error()) {
		t.Fatalf("markFailed did not log its transition error; logs = %q", logs.String())
	}
}
