package engagement

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestStagesStartEmptyAndRetainCreationOrder(t *testing.T) {
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
	stages, err := svc.ListStages(ctx, owner, boot.WorkspaceID)
	if err != nil || len(stages) != 0 {
		t.Fatalf("initial stages = %v, %v", stages, err)
	}
	for _, input := range []StageInput{{Name: "New", Kind: "open"}, {Name: "Closed", Kind: "won"}} {
		if _, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, input, identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
	}
	stages, err = svc.ListStages(ctx, owner, boot.WorkspaceID)
	if err != nil || len(stages) != 2 || stages[0].Name != "New" || stages[1].Name != "Closed" {
		t.Fatalf("stages = %v, %v", stages, err)
	}
	other, err := auth.CreateWorkspace(ctx, owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, err := auth.Authorize(ctx, boot.UserID, other, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	stages, err = svc.ListStages(ctx, otherOwner, other)
	if err != nil || len(stages) != 0 {
		t.Fatalf("other workspace stages = %v, %v", stages, err)
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
	if _, err := svc.CreateStage(ctx, member, boot.WorkspaceID, StageInput{Name: "Blocked", Kind: "open"}, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member create = %v", err)
	}
	var adminID identity.UserID
	if err := db.QueryRowContext(ctx, `INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id`, "admin@example.com", time.Now().UTC().Format(time.RFC3339Nano)).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddMember(ctx, owner, adminID, identity.Admin, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	admin, err := auth.Authorize(ctx, adminID, boot.WorkspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateStage(ctx, admin, boot.WorkspaceID, StageInput{Name: "Lost", Kind: "lost"}, identity.AuditContext{}); err != nil {
		t.Fatalf("admin create: %v", err)
	}
	for _, input := range []StageInput{{Name: " ", Kind: "open"}, {Name: "Bad", Kind: "other"}} {
		if _, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, input, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid input %+v = %v", input, err)
		}
	}
	var auditCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE workspace_id = ? AND event_type = ?`, boot.WorkspaceID, "engagement_stage.created").Scan(&auditCount); err != nil || auditCount != 3 {
		t.Fatalf("audit count = %d, %v", auditCount, err)
	}
}
