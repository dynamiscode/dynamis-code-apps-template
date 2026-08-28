package sharing

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func TestPostgresPublicSharingLifecycle(t *testing.T) {
	databaseURL := os.Getenv("POSTGRES_TEST_URL")
	if databaseURL == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.Postgres, URL: databaseURL, MaxOpenConns: 4, MaxIdleConns: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.Postgres); err != nil {
		t.Fatal(err)
	}
	userID, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, "INSERT INTO users (id, email, created_at) VALUES ($1, $2, $3)", userID, fmt.Sprintf("share-%s@example.com", userID), now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO workspaces (id, name, created_at) VALUES ($1, $2, $3)", workspaceID, "Sharing PostgreSQL", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES ($1, $2, 'owner', $3)", workspaceID, userID, now); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := auth.Authorize(ctx, userID, workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	principal.AuthMethod = "test"
	item, err := items.NewService(db, config.Postgres, auth, 10000).Create(ctx, principal, workspaceID, "PostgreSQL public", "postgres-public-share-"+userID, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, config.Postgres, auth)
	link, err := service.Create(ctx, principal, workspaceID, item.Item.ID, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	public, err := service.Resolve(ctx, link.Token, identity.AuditContext{})
	if err != nil || public.Title != "PostgreSQL public" {
		t.Fatalf("Resolve() = %+v, %v", public, err)
	}
	if err := service.Revoke(ctx, principal, workspaceID, item.Item.ID, link.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(ctx, link.Token, identity.AuditContext{}); err != ErrUnavailable {
		t.Fatalf("revoked Resolve() error = %v", err)
	}
}
