package portability

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func TestSQLiteWorkspaceExport(t *testing.T) {
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
	runExportContract(t, db, config.SQLite)
}

func runExportContract(t *testing.T, db *sql.DB, driver config.DatabaseDriver) {
	t.Helper()
	ctx := context.Background()
	auth, err := identity.NewService(db, driver)
	if err != nil {
		t.Fatal(err)
	}
	userID, _ := id.New()
	workspaceID, _ := id.New()
	viewerID, _ := id.New()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, database.Rebind(driver, query), args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO users (id, email, created_at) VALUES (?, ?, ?)", userID, "portability-owner@example.com", now)
	exec("INSERT INTO users (id, email, created_at) VALUES (?, ?, ?)", viewerID, "portability-viewer@example.com", now)
	exec("INSERT INTO workspaces (id, name, locale, created_at) VALUES (?, ?, ?, ?)", workspaceID, "Portable", "es", now)
	exec("INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, ?, ?)", workspaceID, userID, identity.Owner, now)
	exec("INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, ?, ?)", workspaceID, viewerID, identity.Viewer, now)
	actor, err := auth.Authorize(ctx, userID, workspaceID, identity.WorkspaceExport)
	if err != nil {
		t.Fatal(err)
	}
	actor.AuthMethod = "test"
	token, err := auth.CreateAPIToken(ctx, actor, "export test",
		[]identity.Permission{identity.WorkspaceExport}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	itemService := items.NewService(db, driver, auth, 100)
	created, err := itemService.Create(ctx, actor, workspaceID, "Portable item", "export-item-key",
		identity.AuditContext{RequestID: "export-item", SourceAddress: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, driver, auth, 100, 1024*1024)
	service.now = func() time.Time { return time.Date(2026, 8, 25, 20, 0, 0, 0, time.UTC) }
	encoded, err := service.Export(ctx, actor, workspaceID,
		identity.AuditContext{RequestID: "export-request", SourceAddress: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	var result Export
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.FormatVersion != FormatVersion || result.Workspace.ID != workspaceID ||
		result.Workspace.Locale != "es" ||
		len(result.Members) != 2 || len(result.Items) != 1 || result.Items[0].ID != created.Item.ID ||
		len(result.AuditEvents) == 0 || len(result.Excluded) == 0 {
		t.Fatalf("export = %+v", result)
	}
	if strings.Contains(string(encoded), token.Secret) || strings.Contains(string(encoded), "secret_hash") {
		t.Fatal("export contains credential material")
	}
	viewer, err := auth.Authorize(ctx, viewerID, workspaceID, identity.WorkspaceRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Export(ctx, viewer, workspaceID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer export error = %v", err)
	}
	wrongWorkspace, _ := id.New()
	if _, err := service.Export(ctx, actor, wrongWorkspace, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("wrong-workspace export error = %v", err)
	}
	limited := NewService(db, driver, auth, 1, 1024*1024)
	if _, err := limited.Export(ctx, actor, workspaceID, identity.AuditContext{}); !errors.Is(err, ErrLimit) {
		t.Fatalf("limited export error = %v", err)
	}
	var success, failure int
	if err := db.QueryRowContext(ctx, database.Rebind(driver, `
		SELECT COUNT(*) FROM audit_events
		WHERE workspace_id = ? AND action = 'workspace.export' AND outcome = 'success'
	`), workspaceID).Scan(&success); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, database.Rebind(driver, `
		SELECT COUNT(*) FROM audit_events
		WHERE workspace_id = ? AND action = 'workspace.export' AND outcome = 'failure'
	`), workspaceID).Scan(&failure); err != nil {
		t.Fatal(err)
	}
	if success != 1 || failure != 1 {
		t.Fatalf("export audits = success:%d failure:%d", success, failure)
	}
}
