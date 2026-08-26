package items

import (
	"context"
	"os"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func TestPostgresItemLifecycle(t *testing.T) {
	databaseURL := os.Getenv("POSTGRES_TEST_URL")
	if databaseURL == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{
		Driver: config.Postgres, URL: databaseURL, MaxOpenConns: 4, MaxIdleConns: 2,
	})
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
	if _, err := db.ExecContext(ctx,
		"INSERT INTO users (id, email, created_at) VALUES ($1, $2, $3)",
		userID, "items-postgres@example.com", now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO workspaces (id, name, created_at) VALUES ($1, $2, $3)",
		workspaceID, "Items PostgreSQL", now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
		VALUES ($1, $2, 'owner', $3)
	`, workspaceID, userID, now); err != nil {
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
	service := NewService(db, config.Postgres, auth, 10000)
	created, err := service.Create(ctx, principal, workspaceID, "PostgreSQL", "postgres-idem-key", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := service.Get(ctx, principal, workspaceID, created.Item.ID); err != nil || got.Title != "PostgreSQL" {
		t.Fatalf("Get() = %+v, %v", got, err)
	}
	checkpoint, err := service.Changes(ctx, principal, workspaceID, "", 10)
	if err != nil || !checkpoint.Resync {
		t.Fatalf("Changes() = %+v, %v", checkpoint, err)
	}
	complete := Complete
	updated, err := service.Update(
		ctx, principal, workspaceID, created.Item.ID, 1,
		UpdateInput{Status: &complete}, identity.AuditContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := service.Changes(ctx, principal, workspaceID, checkpoint.Next, 10)
	if err != nil || len(changes.Changes) != 1 || changes.Changes[0].Action != "updated" {
		t.Fatalf("incremental Changes() = %+v, %v", changes, err)
	}
	if err := service.Delete(
		ctx, principal, workspaceID, created.Item.ID, updated.Version,
		identity.AuditContext{},
	); err != nil {
		t.Fatal(err)
	}
}
