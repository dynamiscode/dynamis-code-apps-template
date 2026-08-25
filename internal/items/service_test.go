package items

import (
	"context"
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestItemLifecycleContracts(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{
		Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1,
	})
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
	service := NewService(db, config.SQLite, auth)

	created, err := service.Create(ctx, principal, owner.WorkspaceID, " First ", "idem-12345678", identity.AuditContext{})
	if err != nil || created.Replay || created.Item.Title != "First" {
		t.Fatalf("Create() = %+v, %v", created, err)
	}
	replay, err := service.Create(ctx, principal, owner.WorkspaceID, "First", "idem-12345678", identity.AuditContext{})
	if err != nil || !replay.Replay || replay.Item.ID != created.Item.ID {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	if _, err := service.Create(ctx, principal, owner.WorkspaceID, "Other", "idem-12345678", identity.AuditContext{}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}
	second, err := service.Create(ctx, principal, owner.WorkspaceID, "Second", "idem-abcdefgh", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	page, err := service.List(ctx, principal, owner.WorkspaceID, ListInput{Sort: "created_at", Limit: 1})
	if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	next, err := service.List(ctx, principal, owner.WorkspaceID, ListInput{Sort: "created_at", Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
		t.Fatalf("next page = %+v, %v", next, err)
	}
	if _, err := service.List(ctx, principal, owner.WorkspaceID, ListInput{Sort: "title", Limit: 1}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unsupported sort error = %v", err)
	}
	if _, err := service.List(ctx, principal, owner.WorkspaceID, ListInput{Sort: "created_at", Limit: 1, Cursor: "bad"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("invalid cursor error = %v", err)
	}
	newTitle := "Done"
	complete := Complete
	updated, err := service.Update(ctx, principal, owner.WorkspaceID, second.Item.ID, 1, UpdateInput{Title: &newTitle, Status: &complete}, identity.AuditContext{})
	if err != nil || updated.Version != 2 || updated.Status != Complete {
		t.Fatalf("Update() = %+v, %v", updated, err)
	}
	if _, err := service.Update(ctx, principal, owner.WorkspaceID, second.Item.ID, 1, UpdateInput{Title: &newTitle}, identity.AuditContext{}); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("stale update error = %v", err)
	}
	checkpoint, err := service.Changes(ctx, principal, owner.WorkspaceID, "", 10)
	if err != nil || !checkpoint.Resync || checkpoint.Next == "" {
		t.Fatalf("initial changes = %+v, %v", checkpoint, err)
	}
	third, err := service.Create(ctx, principal, owner.WorkspaceID, "Third", "idem-third-key", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	changes, err := service.Changes(ctx, principal, owner.WorkspaceID, checkpoint.Next, 10)
	if err != nil || changes.Resync || len(changes.Changes) != 1 || changes.Changes[0].Action != "created" {
		t.Fatalf("incremental changes = %+v, %v", changes, err)
	}
	if err := service.Delete(ctx, principal, owner.WorkspaceID, third.Item.ID, third.Item.Version, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, principal, owner.WorkspaceID, third.Item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted Get() error = %v", err)
	}
	deleted, err := service.Changes(ctx, principal, owner.WorkspaceID, changes.Next, 10)
	if err != nil || len(deleted.Changes) != 1 || deleted.Changes[0].Action != "deleted" {
		t.Fatalf("delete changes = %+v, %v", deleted, err)
	}
	wrong := principal
	wrong.WorkspaceID = "00000000000000000000000000000000"
	if _, err := service.Get(ctx, wrong, wrong.WorkspaceID, created.Item.ID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("cross-workspace get error = %v", err)
	}
	var rawKeys int
	if err := db.QueryRow("SELECT COUNT(*) FROM idempotency_records WHERE key_hash IN (?, ?)", "idem-12345678", "idem-abcdefgh").Scan(&rawKeys); err != nil || rawKeys != 0 {
		t.Fatalf("raw idempotency keys stored = %d, %v", rawKeys, err)
	}
}
