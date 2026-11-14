package clients

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// TestPostgresClientLifecycle runs the same client rules on PostgreSQL. It is
// skipped unless POSTGRES_TEST_URL is set.
func TestPostgresClientLifecycle(t *testing.T) {
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
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var ownerID, viewerID identity.UserID
	var workspaceID identity.WorkspaceID
	if err := db.QueryRowContext(ctx,
		"INSERT INTO users (email, created_at) VALUES ($1, $2) RETURNING id",
		"clients-owner-"+suffix+"@example.com", now,
	).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx,
		"INSERT INTO users (email, created_at) VALUES ($1, $2) RETURNING id",
		"clients-viewer-"+suffix+"@example.com", now,
	).Scan(&viewerID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx,
		"INSERT INTO workspaces (name, created_at) VALUES ($1, $2) RETURNING id",
		"Clients PostgreSQL", now,
	).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
		VALUES ($1, $2, 'owner', $3), ($1, $4, 'viewer', $3)
	`, workspaceID, ownerID, now, viewerID); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authorize(ctx, ownerID, workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	owner.AuthMethod = "test"
	viewer, err := auth.Authorize(ctx, viewerID, workspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	viewer.AuthMethod = "test"
	service := NewService(db, config.Postgres, auth, 50)

	created, err := service.Create(ctx, owner, workspaceID, Input{DisplayName: "PostgreSQL", Email: "pg@example.com"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := service.Get(ctx, viewer, workspaceID, created.ID); err != nil || got.DisplayName != "PostgreSQL" {
		t.Fatalf("Get() = %+v, %v", got, err)
	}
	if _, err := service.Create(ctx, viewer, workspaceID, Input{DisplayName: "Blocked"}, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer create error = %v, want ErrForbidden", err)
	}
	if _, err := service.Create(ctx, owner, workspaceID, Input{}, identity.AuditContext{}); !errors.Is(err, ErrDisplayNameRequired) {
		t.Fatalf("blank create error = %v, want ErrDisplayNameRequired", err)
	}
	name := "PostgreSQL edited"
	updated, err := service.Update(ctx, owner, workspaceID, created.ID, 1, UpdateInput{DisplayName: &name}, identity.AuditContext{})
	if err != nil || updated.Version != 2 || updated.DisplayName != name || updated.Email != "pg@example.com" {
		t.Fatalf("Update() = %+v, %v", updated, err)
	}
	if _, err := service.Update(ctx, owner, workspaceID, created.ID, 1, UpdateInput{DisplayName: &name}, identity.AuditContext{}); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("stale update error = %v, want ErrPreconditionFailed", err)
	}
	page, err := service.List(ctx, viewer, workspaceID, ListInput{Limit: 10})
	if err != nil || len(page.Clients) != 1 || page.Clients[0].ID != created.ID {
		t.Fatalf("List() = %+v, %v", page, err)
	}
	if _, err := service.Get(ctx, viewer, workspaceID, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("zero key get error = %v, want ErrNotFound", err)
	}

	field, err := service.DefineField(ctx, owner, workspaceID, FieldInput{Label: "Source", Type: FieldTypeText}, identity.AuditContext{})
	if err != nil || field.ID == 0 {
		t.Fatalf("DefineField() = %+v, %v", field, err)
	}
	if _, err := service.DefineField(ctx, viewer, workspaceID, FieldInput{Label: "Blocked", Type: FieldTypeText}, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer DefineField() error = %v, want ErrForbidden", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE clients SET custom_values = $1 WHERE id = $2`, `{"f01":"Friend"}`, int64(created.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ArchiveField(ctx, owner, workspaceID, field.ID, identity.AuditContext{}); err != nil {
		t.Fatalf("ArchiveField() error = %v", err)
	}
	if forms, err := service.ListFields(ctx, viewer, workspaceID, ListFieldsInput{}); err != nil || len(forms) != 0 {
		t.Fatalf("form fields after archive = %+v, %v; want none", forms, err)
	}
	var values string
	if err := db.QueryRowContext(ctx, `SELECT custom_values FROM clients WHERE id = $1`, int64(created.ID)).Scan(&values); err != nil || values != `{"f01":"Friend"}` {
		t.Fatalf("custom_values after archive = %q, %v; want untouched", values, err)
	}
	if _, err := service.RestoreField(ctx, owner, workspaceID, field.ID, identity.AuditContext{}); err != nil {
		t.Fatalf("RestoreField() error = %v", err)
	}
	if forms, err := service.ListFields(ctx, viewer, workspaceID, ListFieldsInput{}); err != nil || len(forms) != 1 {
		t.Fatalf("form fields after restore = %+v, %v; want one", forms, err)
	}
	limited := NewService(db, config.Postgres, auth, 1)
	if _, err := limited.DefineField(ctx, owner, workspaceID, FieldInput{Label: "Over", Type: FieldTypeText}, identity.AuditContext{}); !errors.Is(err, ErrLimit) {
		t.Fatalf("DefineField() over limit error = %v, want ErrLimit", err)
	}
}
