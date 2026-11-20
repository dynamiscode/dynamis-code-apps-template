package listviews

import (
	"context"
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func newTestService(t *testing.T) (*Service, identity.Principal, identity.WorkspaceID) {
	t.Helper()
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
	boot, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, boot.UserID, boot.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	return NewService(db, config.SQLite, auth), actor, boot.WorkspaceID
}

// TestSavedViewsAreListedMadeDefaultAndChanged checks the life of a view: a personal view is created, listed, made
// the default, renamed, and deleted, and clearing the default returns the list to Active.
func TestSavedViewsAreListedMadeDefaultAndChanged(t *testing.T) {
	ctx := context.Background()
	service, actor, workspace := newTestService(t)
	view, err := service.Create(ctx, actor, workspace, ListClients, "Big deals", false, State{Query: "sort=displayName&order=asc"})
	if err != nil {
		t.Fatal(err)
	}
	views, defaultID, err := service.List(ctx, actor, workspace, ListClients)
	if err != nil || len(views) != 1 || views[0].Name != "Big deals" || defaultID != 0 {
		t.Fatalf("list = %+v default %d, %v", views, defaultID, err)
	}
	if err := service.SetDefault(ctx, actor, workspace, ListClients, view.ID); err != nil {
		t.Fatal(err)
	}
	if _, defaultID, _ = service.List(ctx, actor, workspace, ListClients); defaultID != view.ID {
		t.Fatalf("default = %d, want %d", defaultID, view.ID)
	}
	if err := service.Update(ctx, actor, workspace, view.ID, "Renamed", State{Query: "sort=email"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := service.Get(ctx, actor, workspace, view.ID); got.Name != "Renamed" || got.State.Query != "sort=email" {
		t.Fatalf("after update = %+v", got)
	}
	if err := service.SetDefault(ctx, actor, workspace, ListClients, 0); err != nil {
		t.Fatal(err)
	}
	if _, defaultID, _ = service.List(ctx, actor, workspace, ListClients); defaultID != 0 {
		t.Fatalf("cleared default = %d", defaultID)
	}
	if err := service.Delete(ctx, actor, workspace, view.ID); err != nil {
		t.Fatal(err)
	}
	if views, _, _ = service.List(ctx, actor, workspace, ListClients); len(views) != 0 {
		t.Fatalf("deleted view still listed: %+v", views)
	}
}

// TestSharedViewsAreSharedAndRefusedOutsideTheRules checks that a shared view is listed with its name, that a list
// key outside the four lists is refused, and that names are required.
func TestSharedViewsAreSharedAndRefusedOutsideTheRules(t *testing.T) {
	ctx := context.Background()
	service, actor, workspace := newTestService(t)
	if _, err := service.Create(ctx, actor, workspace, ListFiles, "PDFs", true, State{Query: "mime=application/pdf"}); err != nil {
		t.Fatal(err)
	}
	if views, _, _ := service.List(ctx, actor, workspace, ListFiles); len(views) != 1 || !views[0].Shared {
		t.Fatalf("shared view not listed as shared: %+v", views)
	}
	if _, err := service.Create(ctx, actor, workspace, "unknown", "x", false, State{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown list error = %v", err)
	}
	if _, err := service.Create(ctx, actor, workspace, ListFiles, "   ", false, State{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank name error = %v", err)
	}
}

// TestPersonalViewsStopAtTheLimit checks that the per-list limit refuses the view past it.
func TestPersonalViewsStopAtTheLimit(t *testing.T) {
	ctx := context.Background()
	service, actor, workspace := newTestService(t)
	for i := 0; i < MaxPersonalPerList; i++ {
		if _, err := service.Create(ctx, actor, workspace, ListMembers, "View", false, State{}); err != nil {
			t.Fatalf("view %d: %v", i, err)
		}
	}
	if _, err := service.Create(ctx, actor, workspace, ListMembers, "One too many", false, State{}); !errors.Is(err, ErrLimit) {
		t.Fatalf("limit error = %v", err)
	}
}
