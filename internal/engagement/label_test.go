package engagement

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestWorkspaceLabel(t *testing.T) {
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
	if err := svc.SetLabel(ctx, owner, boot.WorkspaceID, strings.Repeat("x", 101), identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("long label: %v", err)
	}
	if got, err := svc.Label(ctx, owner, boot.WorkspaceID); err != nil || got != "" {
		t.Fatalf("default: %q %v", got, err)
	}
	if err := svc.SetLabel(ctx, owner, boot.WorkspaceID, " Matter ", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if got, err := svc.Label(ctx, owner, boot.WorkspaceID); err != nil || got != "Matter" {
		t.Fatalf("label: %q %v", got, err)
	}
	other, err := auth.CreateWorkspace(ctx, owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, err := auth.Authorize(ctx, boot.UserID, other, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := svc.Label(ctx, otherOwner, other); err != nil || got != "" {
		t.Fatalf("other: %q %v", got, err)
	}
	for _, role := range []identity.Role{identity.Admin, identity.Member, identity.Viewer} {
		var user identity.UserID
		if err := db.QueryRowContext(ctx, `INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id`, string(role)+"@example.com", time.Now().UTC().Format(time.RFC3339Nano)).Scan(&user); err != nil {
			t.Fatal(err)
		}
		if err := auth.AddMember(ctx, owner, user, role, identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
		actor, err := auth.Authorize(ctx, user, boot.WorkspaceID, identity.ResourcesRead)
		if err != nil {
			t.Fatal(err)
		}
		label := "Denied"
		if role == identity.Admin {
			label = "Matter"
		}
		err = svc.SetLabel(ctx, actor, boot.WorkspaceID, label, identity.AuditContext{})
		if role == identity.Admin && err != nil {
			t.Fatalf("admin: %v", err)
		}
		if role != identity.Admin && !errors.Is(err, identity.ErrForbidden) {
			t.Fatalf("%s: %v", role, err)
		}
		if got, err := svc.Label(ctx, owner, boot.WorkspaceID); err != nil || got != "Matter" {
			t.Fatalf("%s changed label: %q %v", role, got, err)
		}
	}
	if err := svc.SetLabel(ctx, owner, boot.WorkspaceID, "", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if got, err := svc.Label(ctx, owner, boot.WorkspaceID); err != nil || got != "" {
		t.Fatalf("reset: %q %v", got, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE workspace_id = ? AND event_type = 'engagement_label.updated'`, boot.WorkspaceID).Scan(&count); err != nil || count != 3 {
		t.Fatalf("audit: %d %v", count, err)
	}
}
