package items

import (
	"context"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// TestIdempotencyPrincipalUsesShortUserID pins the stored principal string to
// the user's short ID and checks that replays still deduplicate under it.
func TestIdempotencyPrincipalUsesShortUserID(t *testing.T) {
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
	service := NewService(db, config.SQLite, auth, 3)

	created, err := service.Create(ctx, principal, owner.WorkspaceID, "Keyed", "idem-principal-1", identity.AuditContext{})
	if err != nil || created.Replay {
		t.Fatalf("Create() = %+v, %v", created, err)
	}
	var stored string
	if err := db.QueryRow("SELECT principal_id FROM idempotency_records WHERE key_hash = ?", hash("idem-principal-1")).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if want := "user:" + owner.UserID.String(); stored != want {
		t.Fatalf("principal_id = %q; want %q", stored, want)
	}
	replay, err := service.Create(ctx, principal, owner.WorkspaceID, "Keyed", "idem-principal-1", identity.AuditContext{})
	if err != nil || !replay.Replay || replay.Item.ID != created.Item.ID {
		t.Fatalf("replay under short-ID principal = %+v, %v", replay, err)
	}
}
