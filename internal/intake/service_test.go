package intake

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestLinkLifecycle(t *testing.T) {
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
	owner, err := auth.Authorize(ctx, boot.UserID, boot.WorkspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, config.SQLite, auth)
	token, err := svc.Create(ctx, owner, boot.WorkspaceID, "Consent", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, owner, boot.WorkspaceID, "Consent", identity.AuditContext{}); !errors.Is(err, ErrActive) {
		t.Fatalf("duplicate create: %v", err)
	}
	workspace, consent, err := svc.Resolve(ctx, token)
	if err != nil || workspace != boot.WorkspaceID || consent != "Consent" {
		t.Fatalf("resolve: %v %q %v", workspace, consent, err)
	}
	if err := svc.SetConsent(ctx, owner, boot.WorkspaceID, "New consent", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, consent, err := svc.Resolve(ctx, token); err != nil || consent != "New consent" {
		t.Fatalf("updated consent: %q %v", consent, err)
	}
	rotated, err := svc.Rotate(ctx, owner, boot.WorkspaceID, "Updated", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Resolve(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old token: %v", err)
	}
	if _, _, err := svc.Resolve(ctx, rotated); err != nil {
		t.Fatal(err)
	}
	if err := svc.Disable(ctx, owner, boot.WorkspaceID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Disable(ctx, owner, boot.WorkspaceID, identity.AuditContext{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disable inactive: %v", err)
	}
	if _, _, err := svc.Resolve(ctx, rotated); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled token: %v", err)
	}
	if _, err := svc.Create(ctx, owner, boot.WorkspaceID, "Again", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Rotate(ctx, identity.Principal{}, boot.WorkspaceID, "No", identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("unauthorized: %v", err)
	}
	var memberID identity.UserID
	if err := db.QueryRowContext(ctx, `INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id`, "member@example.com", time.Now().UTC().Format(time.RFC3339Nano)).Scan(&memberID); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddMember(ctx, owner, memberID, identity.Member, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	member, err := auth.Authorize(ctx, memberID, boot.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetSettings(ctx, member, boot.WorkspaceID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member settings: %v", err)
	}
	if err := svc.Disable(ctx, member, boot.WorkspaceID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member disable: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE workspace_id = ? AND event_type LIKE 'public_intake.%'`, boot.WorkspaceID).Scan(&count); err != nil || count != 5 {
		t.Fatalf("audit count: %d, %v", count, err)
	}
}
