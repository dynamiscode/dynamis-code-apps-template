package files

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/files/filestest"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

// A streamed S3 upload is written by the application, so the content write
// stores the bytes and leaves the file pending. Only Complete verifies the
// stored object and makes the file ready.
func TestStreamedS3UploadStaysPendingUntilComplete(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "streamed@example.com", Password: "long-enough-password", WorkspaceName: "Streamed",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := newS3Store(ctx, s3TestStorage(filestest.S3Endpoint(t)))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, config.SQLite, auth, store, 16, 1024, 0, "streamed")
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	if !service.RequiresCompletion() {
		t.Fatal("S3 content writes must wait for Complete before the file is ready")
	}
	pending, err := service.Initiate(ctx, actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "notes.txt", Size: 5, ContentType: "text/plain",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	written, err := service.PutContent(ctx, actor, owner.WorkspaceID, pending.ID, strings.NewReader("hello"), identity.AuditContext{})
	if err != nil {
		t.Fatalf("content write = %v", err)
	}
	if written.Status != Pending {
		t.Fatalf("status after content write = %s, want pending", written.Status)
	}
	if _, err := service.Get(ctx, actor, owner.WorkspaceID, pending.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get before complete = %v, want ErrNotFound", err)
	}
	ready, err := service.Complete(ctx, actor, owner.WorkspaceID, pending.ID, identity.AuditContext{})
	if err != nil {
		t.Fatalf("complete = %v", err)
	}
	if ready.Status != Ready || ready.SHA256 == nil || ready.Size != 5 {
		t.Fatalf("completed file = %+v", ready)
	}
	_, reader, err := service.Open(ctx, actor, owner.WorkspaceID, pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || string(content) != "hello" {
		t.Fatalf("content = %q, error = %v", content, err)
	}
}

// A streamed write that fails at the object store releases its reservation so
// the workspace quota is available again.
func TestStreamedS3WriteFailureReleasesQuota(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "streamfail@example.com", Password: "long-enough-password", WorkspaceName: "StreamFail",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := newS3Store(ctx, s3TestStorage(filestest.S3Endpoint(t)))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, config.SQLite, auth, &failingPutStore{ObjectStore: store}, 5, 5, 0, "")
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	file, err := service.Initiate(ctx, actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "one.txt", Size: 5, ContentType: "text/plain",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PutContent(ctx, actor, owner.WorkspaceID, file.ID, strings.NewReader("hello"), identity.AuditContext{}); err == nil {
		t.Fatal("content write error = nil, want the store failure")
	}
	if _, err := service.Initiate(ctx, actor, owner.WorkspaceID, InitiateInput{
		OriginalName: "two.txt", Size: 5, ContentType: "text/plain",
	}, identity.AuditContext{}); err != nil {
		t.Fatalf("quota after failed write = %v, want available", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM files WHERE id = ?", file.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("released file count = %d, want 0", count)
	}
}

// A viewer can read the workspace but cannot stream content into it.
func TestStreamedS3ContentWriteDeniesViewer(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "viewerwrite@example.com", Password: "long-enough-password", WorkspaceName: "ViewerWrite",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := newS3Store(ctx, s3TestStorage(filestest.S3Endpoint(t)))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, config.SQLite, auth, store, 16, 1024, 0, "")
	ownerActor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := service.Initiate(ctx, ownerActor, owner.WorkspaceID, InitiateInput{
		OriginalName: "notes.txt", Size: 5, ContentType: "text/plain",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	viewerID, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec("INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, NULL, ?)", viewerID, "viewer-only@example.com", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, ?, ?)", owner.WorkspaceID, viewerID, "viewer", now); err != nil {
		t.Fatal(err)
	}
	viewer, err := auth.Authorize(ctx, viewerID, owner.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PutContent(ctx, viewer, owner.WorkspaceID, pending.ID, strings.NewReader("hello"), identity.AuditContext{}); err != identity.ErrForbidden {
		t.Fatalf("viewer content write = %v, want forbidden", err)
	}
}

func s3TestStorage(endpoint string) config.Storage {
	return config.Storage{
		Driver: config.StorageS3, S3Endpoint: endpoint, S3Region: "us-east-1",
		S3Bucket: "files", S3ForcePathStyle: true,
	}
}

type failingPutStore struct {
	ObjectStore
}

func (*failingPutStore) Put(context.Context, string, io.Reader, int64, string) error {
	return errors.New("put failed")
}
