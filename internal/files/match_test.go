package files

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// TestFileListMatchModes checks that a name condition and a size range combine with AND by default and
// with OR when the match mode is any.
func TestFileListMatchModes(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Files",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := newLocalStore(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, config.SQLite, auth, store, 16, 64, 0, "prefix")
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Upload(ctx, actor, owner.WorkspaceID, "report.txt", strings.NewReader("hello"), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Upload(ctx, actor, owner.WorkspaceID, "notes.txt", strings.NewReader("hello world!"), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	// The name holds "report" for report.txt only; the size range 10 to 20 holds notes.txt only.
	minSize, maxSize := int64(10), int64(20)
	count := func(match string) int {
		t.Helper()
		page, err := service.ListPage(ctx, actor, owner.WorkspaceID, ListInput{
			Search: "report", MinSize: &minSize, MaxSize: &maxSize, Match: match, Sort: "created_at", Limit: 10,
		})
		if err != nil {
			t.Fatalf("List(match=%q) error = %v", match, err)
		}
		return len(page.Files)
	}
	// The name search always narrows the list, so "any" cannot widen it past the name match.
	if got := count("all"); got != 0 {
		t.Fatalf("all = %d files, want 0", got)
	}
	if got := count("any"); got != 0 {
		t.Fatalf("any = %d files, want 0: the name search still applies", got)
	}
	page, err := service.ListPage(ctx, actor, owner.WorkspaceID, ListInput{Search: "report", Sort: "created_at", Limit: 10})
	if err != nil || len(page.Files) != 1 {
		t.Fatalf("name search alone = %d files, %v; want 1", len(page.Files), err)
	}
	if got := count(""); got != 0 {
		t.Fatalf("default = %d files, want 0", got)
	}
}
