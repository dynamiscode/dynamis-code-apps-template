package listprefs

import (
	"context"
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestPreferencesMergeIndependentlyAndResetByUserWorkspaceAndList(t *testing.T) {
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
	boot, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, boot.UserID, boot.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, config.SQLite, auth)
	defaults := map[string]bool{"name": true, "email": false}

	got, err := service.Get(ctx, actor, boot.WorkspaceID, "clients", defaults)
	if err != nil || !got["name"] || got["email"] {
		t.Fatalf("first visit = %v, %v; want declared defaults", got, err)
	}
	if err := service.Set(ctx, actor, boot.WorkspaceID, "clients", "name", false, defaults); err != nil {
		t.Fatal(err)
	}
	if err := service.Set(ctx, actor, boot.WorkspaceID, "clients", "email", true, defaults); err != nil {
		t.Fatal(err)
	}
	got, err = service.Get(ctx, actor, boot.WorkspaceID, "clients", defaults)
	if err != nil || got["name"] || !got["email"] {
		t.Fatalf("independent changes = %v, %v; want name hidden and email shown", got, err)
	}
	if err := service.Set(ctx, actor, boot.WorkspaceID, "clients", "name", true, defaults); err != nil {
		t.Fatal(err)
	}
	got, err = service.Get(ctx, actor, boot.WorkspaceID, "clients", defaults)
	if err != nil || !got["name"] || !got["email"] {
		t.Fatalf("default restored for one column = %v, %v", got, err)
	}
	if err := service.Reset(ctx, actor, boot.WorkspaceID, "clients"); err != nil {
		t.Fatal(err)
	}
	got, err = service.Get(ctx, actor, boot.WorkspaceID, "clients", defaults)
	if err != nil || !got["name"] || got["email"] {
		t.Fatalf("reset = %v, %v; want all defaults", got, err)
	}
	if err := service.Set(ctx, actor, boot.WorkspaceID, ListTransactions, "name", false, defaults); err != nil {
		t.Fatal(err)
	}
	got, err = service.Get(ctx, actor, boot.WorkspaceID, ListTransactions, defaults)
	if err != nil || got["name"] {
		t.Fatalf("transactions preference = %v, %v", got, err)
	}
	got, err = service.Get(ctx, actor, boot.WorkspaceID, ListClients, defaults)
	if err != nil || !got["name"] {
		t.Fatalf("client preference changed = %v, %v", got, err)
	}
}

func TestPreferencesCannotBeReadOrChangedOutsideAuthenticatedScope(t *testing.T) {
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
	boot, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authorize(ctx, boot.UserID, boot.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO users (email, created_at) VALUES ('other@example.com', 'now')`); err != nil {
		t.Fatal(err)
	}
	var userID identity.UserID
	if err := db.QueryRowContext(ctx, "SELECT id FROM users WHERE email = 'other@example.com'").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddMember(ctx, owner, userID, identity.Member, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	member, err := auth.Authorize(ctx, userID, boot.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, config.SQLite, auth)
	defaults := map[string]bool{"name": true}
	if err := service.Set(ctx, owner, boot.WorkspaceID, "clients", "name", false, defaults); err != nil {
		t.Fatal(err)
	}
	got, err := service.Get(ctx, member, boot.WorkspaceID, "clients", defaults)
	if err != nil || !got["name"] {
		t.Fatalf("other member preferences = %v, %v; want their default", got, err)
	}
	if err := service.Set(ctx, member, boot.WorkspaceID, "clients", "name", false, defaults); err != nil {
		t.Fatal(err)
	}
	got, err = service.Get(ctx, owner, boot.WorkspaceID, "clients", defaults)
	if err != nil || got["name"] {
		t.Fatalf("owner preferences changed by another member = %v, %v; want hidden", got, err)
	}
	if err := service.Set(ctx, owner, boot.WorkspaceID, ListItems, "name", false, defaults); err != nil {
		t.Fatal(err)
	}
	otherWorkspaceID, err := auth.CreateWorkspace(ctx, owner, identity.WorkspaceCreateInput{Name: "Other workspace"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherWorkspaceActor, err := auth.Authorize(ctx, owner.UserID, otherWorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Set(ctx, otherWorkspaceActor, otherWorkspaceID, ListClients, "name", false, defaults); err != nil {
		t.Fatal(err)
	}
	if err := service.Reset(ctx, owner, boot.WorkspaceID, ListClients); err != nil {
		t.Fatal(err)
	}
	got, err = service.Get(ctx, owner, boot.WorkspaceID, ListClients, defaults)
	if err != nil || !got["name"] {
		t.Fatalf("owner preferences after reset = %v, %v; want default", got, err)
	}
	got, err = service.Get(ctx, member, boot.WorkspaceID, ListClients, defaults)
	if err != nil || got["name"] {
		t.Fatalf("other member preferences after reset = %v, %v; want hidden", got, err)
	}
	got, err = service.Get(ctx, owner, boot.WorkspaceID, ListItems, defaults)
	if err != nil || got["name"] {
		t.Fatalf("other list preferences after reset = %v, %v; want hidden", got, err)
	}
	got, err = service.Get(ctx, otherWorkspaceActor, otherWorkspaceID, ListClients, defaults)
	if err != nil || got["name"] {
		t.Fatalf("other workspace preferences after reset = %v, %v; want hidden", got, err)
	}
	if _, err := service.Get(ctx, owner, 0, "clients", defaults); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("wrong workspace Get() error = %v, want forbidden", err)
	}
	if err := service.Set(ctx, owner, boot.WorkspaceID, "clients", "unknown", false, defaults); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown column Set() error = %v, want invalid input", err)
	}
	if err := service.Reset(ctx, owner, boot.WorkspaceID, "unknown"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown list Reset() error = %v, want invalid input", err)
	}
}
