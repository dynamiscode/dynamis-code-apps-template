package items

import (
	"context"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// TestItemListMatchModes checks that status and title conditions combine with AND by default and with
// OR when the match mode is any.
func TestItemListMatchModes(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	principal.AuthMethod = "test"
	service := NewService(db, config.SQLite, auth, 3)
	for _, title := range []string{"First", "Second"} {
		if _, err := service.Create(ctx, principal, owner.WorkspaceID, title, "idem-"+title+"-12345", identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
	}
	list := func(input ListInput) int {
		t.Helper()
		input.Sort, input.Limit = "created_at", 10
		page, err := service.List(ctx, principal, owner.WorkspaceID, input)
		if err != nil {
			t.Fatalf("List(%+v) error = %v", input, err)
		}
		return len(page.Items)
	}
	// The search always combines with AND, whatever the match mode, so "any" cannot widen it.
	if got := list(ListInput{Status: Complete, Search: "First", Match: "any"}); got != 0 {
		t.Fatalf("any: complete and First = %d rows, want 0", got)
	}
	if got := list(ListInput{Status: Active, Search: "zzz", Match: "any"}); got != 0 {
		t.Fatalf("any: active and zzz = %d rows, want 0", got)
	}
	if got := list(ListInput{Status: Active, Search: "First", Match: "any"}); got != 1 {
		t.Fatalf("any: active and First = %d rows, want 1", got)
	}
	if got := list(ListInput{Status: Active}); got != 2 {
		t.Fatalf("status alone: active = %d rows, want 2", got)
	}
}
