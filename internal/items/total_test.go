package items

import (
	"context"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// TestItemListTotalIgnoresPage checks that Total counts every row the filters match, whatever the page size
// and cursor, so the list footer can say "Showing N of M rows".
func TestItemListTotalIgnoresPage(t *testing.T) {
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
	for _, title := range []string{"Alpha", "Beta", "Gamma"} {
		if _, err := service.Create(ctx, principal, owner.WorkspaceID, title, "idem-"+title+"-12345", identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
	}
	list := func(input ListInput) Page {
		t.Helper()
		input.Sort = "created_at"
		page, err := service.List(ctx, principal, owner.WorkspaceID, input)
		if err != nil {
			t.Fatalf("List(%+v) error = %v", input, err)
		}
		return page
	}
	first := list(ListInput{Page: 1, Limit: 2})
	if len(first.Items) != 2 || first.Total != 3 {
		t.Fatalf("first page = %d rows, total %d; want 2 rows, total 3", len(first.Items), first.Total)
	}
	second := list(ListInput{Page: 2, Limit: 2})
	if len(second.Items) != 1 || second.Total != 3 {
		t.Fatalf("second page = %d rows, total %d; want 1 row, total 3", len(second.Items), second.Total)
	}
	filtered := list(ListInput{Limit: 10, Search: "Alpha"})
	if len(filtered.Items) != 1 || filtered.Total != 1 {
		t.Fatalf("filtered = %d rows, total %d; want 1 and 1", len(filtered.Items), filtered.Total)
	}
}
