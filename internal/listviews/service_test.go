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
	if _, defaultID, err = service.List(ctx, actor, workspace, ListClients); err != nil || defaultID != view.ID {
		t.Fatalf("default = %d, err = %v, want %d", defaultID, err, view.ID)
	}
	if err := service.Update(ctx, actor, workspace, view.ID, "Renamed", State{Query: "sort=email"}); err != nil {
		t.Fatal(err)
	}
	if got, err := service.Get(ctx, actor, workspace, view.ID); err != nil || got.Name != "Renamed" || got.State.Query != "sort=email" {
		t.Fatalf("after update = %+v, %v", got, err)
	}
	if err := service.SetDefault(ctx, actor, workspace, ListClients, 0); err != nil {
		t.Fatal(err)
	}
	if _, defaultID, err = service.List(ctx, actor, workspace, ListClients); err != nil || defaultID != 0 {
		t.Fatalf("cleared default = %d, %v", defaultID, err)
	}
	if err := service.Delete(ctx, actor, workspace, view.ID); err != nil {
		t.Fatal(err)
	}
	if views, _, err = service.List(ctx, actor, workspace, ListClients); err != nil || len(views) != 0 {
		t.Fatalf("views after delete = %+v, %v", views, err)
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
	if views, _, err := service.List(ctx, actor, workspace, ListFiles); err != nil || len(views) != 1 || !views[0].Shared || views[0].Name != "PDFs" {
		t.Fatalf("shared view = %+v, %v", views, err)
	}
	transactionsView, err := service.Create(ctx, actor, workspace, ListTransactions, "Receipts", false, State{Query: "filter=direction%3Aequals%3Ain", Visible: map[string]bool{"amount": true}})
	if err != nil {
		t.Fatal(err)
	}
	transactionsViews, _, err := service.List(ctx, actor, workspace, ListTransactions)
	if err != nil || len(transactionsViews) != 1 || transactionsViews[0].ID != transactionsView.ID || !transactionsViews[0].State.Visible["amount"] {
		t.Fatalf("transactions views = %+v, %v", transactionsViews, err)
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

func TestTransactionsViewsRequireTransactionsReadPermission(t *testing.T) {
	ctx := context.Background()
	service, owner, workspace := newTestService(t)
	view, err := service.Create(ctx, owner, workspace, ListTransactions, "Shared", true, State{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.ExecContext(ctx, `INSERT INTO users (email, created_at) VALUES ('viewer@example.com', 'now')`); err != nil {
		t.Fatal(err)
	}
	var userID identity.UserID
	if err := service.db.QueryRowContext(ctx, `SELECT id FROM users WHERE email = 'viewer@example.com'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := service.auth.AddMember(ctx, owner, userID, identity.Viewer, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	viewer, err := service.auth.Authorize(ctx, userID, workspace, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.List(ctx, viewer, workspace, ListTransactions)
	if !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("list: %v", err)
	}
	_, err = service.Create(ctx, viewer, workspace, ListTransactions, "Private", false, State{})
	if !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("create: %v", err)
	}
	_, err = service.Get(ctx, viewer, workspace, view.ID)
	if !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("get: %v", err)
	}
	for name, err := range map[string]error{
		"update":  service.Update(ctx, viewer, workspace, view.ID, "Changed", State{}),
		"delete":  service.Delete(ctx, viewer, workspace, view.ID),
		"default": service.SetDefault(ctx, viewer, workspace, ListTransactions, view.ID),
	} {
		if !errors.Is(err, identity.ErrForbidden) {
			t.Errorf("%s: %v", name, err)
		}
	}
	views, defaultID, err := service.List(ctx, owner, workspace, ListTransactions)
	if err != nil || len(views) != 1 || views[0].ID != view.ID || views[0].Name != "Shared" || defaultID != 0 {
		t.Fatalf("owner views after forbidden changes = %+v, default %d, %v", views, defaultID, err)
	}
	if err := service.SetDefault(ctx, owner, workspace, ListTransactions, view.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.SetDefault(ctx, viewer, workspace, ListTransactions, 0); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("clear default: %v", err)
	}
	if _, defaultID, err := service.List(ctx, owner, workspace, ListTransactions); err != nil || defaultID != view.ID {
		t.Fatalf("default after forbidden clear = %d, %v; want %d", defaultID, err, view.ID)
	}
}
