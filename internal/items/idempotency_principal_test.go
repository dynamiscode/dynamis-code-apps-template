package items

import (
	"context"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// TestIdempotencyPrincipalIsTypedKey pins the stored principal to a type code and
// the integer key of the user or API token that created the item. Replays still
// deduplicate under the same principal, and a token principal is a separate scope.
func TestIdempotencyPrincipalIsTypedKey(t *testing.T) {
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
	var kind string
	var key int64
	if err := db.QueryRow("SELECT principal_kind, principal_key FROM idempotency_records WHERE key_hash = ?", hash("idem-principal-1")).Scan(&kind, &key); err != nil {
		t.Fatal(err)
	}
	if kind != "user" || key != int64(owner.UserID) {
		t.Fatalf("principal = %q/%d; want user/%d", kind, key, owner.UserID)
	}
	replay, err := service.Create(ctx, principal, owner.WorkspaceID, "Keyed", "idem-principal-1", identity.AuditContext{})
	if err != nil || !replay.Replay || replay.Item.ID != created.Item.ID {
		t.Fatalf("replay under user principal = %+v, %v", replay, err)
	}

	token, err := auth.CreateAPIToken(ctx, principal, "items", []identity.Permission{identity.ResourcesWrite}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	tokenPrincipal, err := auth.AuthenticateAPIToken(ctx, token.Secret, identity.ResourcesWrite, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	tokenPrincipal.AuthMethod = "test"
	tokenCreated, err := service.Create(ctx, tokenPrincipal, owner.WorkspaceID, "Keyed", "idem-principal-1", identity.AuditContext{})
	if err != nil || tokenCreated.Replay || tokenCreated.Item.ID == created.Item.ID {
		t.Fatalf("token principal shared the user scope: %+v, %v", tokenCreated, err)
	}
	if err := db.QueryRow("SELECT principal_kind, principal_key FROM idempotency_records WHERE key_hash = ? AND principal_kind = 'token'", hash("idem-principal-1")).Scan(&kind, &key); err != nil {
		t.Fatal(err)
	}
	if kind != "token" || key != int64(tokenPrincipal.TokenKey) || tokenPrincipal.TokenShortID() != token.ID.String() {
		t.Fatalf("token principal = %q/%d; want token/%d", kind, key, tokenPrincipal.TokenKey)
	}
}
