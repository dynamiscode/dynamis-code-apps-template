package catalog

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestCatalogArchiveAndDelete(t *testing.T) {
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
	inUse := false
	service := NewService(db, auth, func(context.Context, *sql.Tx, identity.WorkspaceID, EntryID) (bool, error) { return inUse, nil })
	entry, err := service.Create(ctx, actor, owner.WorkspaceID, "Consultation", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetArchived(ctx, actor, owner.WorkspaceID, entry.ID, true); err != nil {
		t.Fatal(err)
	}
	active, err := service.List(ctx, actor, owner.WorkspaceID, false)
	if err != nil || len(active) != 0 {
		t.Fatalf("active: %v, %v", active, err)
	}
	all, err := service.List(ctx, actor, owner.WorkspaceID, true)
	if err != nil || len(all) != 1 || !all[0].Archived {
		t.Fatalf("all: %v, %v", all, err)
	}
	if err := service.SetArchived(ctx, actor, owner.WorkspaceID, entry.ID, false); err != nil {
		t.Fatal(err)
	}
	inUse = true
	if err := service.Delete(ctx, actor, owner.WorkspaceID, entry.ID+1000); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing entry: %v", err)
	}
	if err := service.Delete(ctx, actor, owner.WorkspaceID, entry.ID); !errors.Is(err, ErrInUse) {
		t.Fatalf("delete in use: %v", err)
	}
	inUse = false
	if err := service.Delete(ctx, actor, owner.WorkspaceID, entry.ID); err != nil {
		t.Fatal(err)
	}
	active, err = service.List(ctx, actor, owner.WorkspaceID, false)
	if err != nil || len(active) != 0 {
		t.Fatalf("deleted: %v, %v", active, err)
	}
}

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
	service := NewService(db, auth, nil)
	entries, err := service.List(ctx, actor, owner.WorkspaceID, false)
	if err != nil || entries == nil || len(entries) != 0 {
		t.Fatalf("List = %v, %v; want empty list", entries, err)
	}
	minutes := 30
	created, err := service.Create(ctx, actor, owner.WorkspaceID, " Consultation ", &minutes)
	if err != nil || created.Name != "Consultation" || created.DurationMinutes == nil || *created.DurationMinutes != 30 {
		t.Fatalf("Create = %+v, %v", created, err)
	}
	if _, err := service.Create(ctx, actor, owner.WorkspaceID, "  ", nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank name = %v", err)
	}
	entries, err = service.List(ctx, actor, owner.WorkspaceID, false)
	if err != nil || len(entries) != 1 || entries[0].Name != "Consultation" {
		t.Fatalf("saved list = %v, %v", entries, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO catalog_entries (workspace_id, name, archived_at) VALUES (?, ?, ?)`, owner.WorkspaceID, "Archived", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	entries, err = service.List(ctx, actor, owner.WorkspaceID, false)
	if err != nil || len(entries) != 1 || entries[0].Name != "Consultation" {
		t.Fatalf("active entries = %v, %v", entries, err)
	}
	if _, err := service.List(ctx, actor, owner.WorkspaceID+1, false); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("foreign workspace list: %v", err)
	}
	if entry, err := service.Create(ctx, actor, owner.WorkspaceID, "No duration", nil); err != nil || entry.DurationMinutes != nil {
		t.Fatalf("optional duration = %+v, %v", entry, err)
	}
	for _, role := range []identity.Role{identity.Admin, identity.Member, identity.Viewer} {
		invitation, err := auth.CreateInvitation(ctx, actor, string(role)+"@example.com", role, time.Hour, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		userID, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "long-enough-password", identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		principal, err := auth.Authorize(ctx, userID, owner.WorkspaceID, identity.ResourcesRead)
		if err != nil {
			t.Fatal(err)
		}
		if list, err := service.List(ctx, principal, owner.WorkspaceID, false); err != nil || len(list) < 2 {
			t.Fatalf("%s list = %v, %v", role, list, err)
		}
		_, err = service.Create(ctx, principal, owner.WorkspaceID, string(role), nil)
		if role == identity.Admin && err != nil {
			t.Fatalf("admin create: %v", err)
		}
		if role != identity.Admin && !errors.Is(err, identity.ErrForbidden) {
			t.Fatalf("%s create: %v", role, err)
		}
	}
	actor.UserID = 0
	if _, err := service.List(ctx, actor, owner.WorkspaceID, false); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("unauthorized List: %v", err)
	}
}
