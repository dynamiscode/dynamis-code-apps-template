package catalog

import (
	"context"
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestListRequiresWorkspaceRead(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(auth)
	entries, err := service.List(ctx, actor, owner.WorkspaceID)
	if err != nil || entries == nil || len(entries) != 0 {
		t.Fatalf("List = %v, %v; want empty list", entries, err)
	}
	actor.UserID = 0
	if _, err := service.List(ctx, actor, owner.WorkspaceID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("unauthorized List: %v", err)
	}
}
