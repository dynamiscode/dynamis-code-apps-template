package engagement

import (
	"context"
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func TestAgreementCanBeRecordedAndCleared(t *testing.T) {
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
	actor, err := auth.Authorize(ctx, boot.UserID, boot.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, config.SQLite, auth)
	stage, err := svc.CreateStage(ctx, actor, boot.WorkspaceID, StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(db, config.SQLite, auth, 50).Create(ctx, actor, boot.WorkspaceID, clients.Input{DisplayName: "Client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	e, err := svc.Create(ctx, actor, boot.WorkspaceID, Input{Name: "Engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	var fileID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO files (workspace_id, object_key, original_name, size, status, created_at, updated_at) VALUES (?, 'agreement', 'agreement.pdf', 1, 'ready', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z') RETURNING id`, boot.WorkspaceID).Scan(&fileID); err != nil {
		t.Fatal(err)
	}
	shortID := id.EncodeKey(id.TypeFile, fileID)
	if err := svc.SetAgreement(ctx, actor, boot.WorkspaceID, e.ID, Agreement{Recorded: true, FileID: shortID}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctx, actor, boot.WorkspaceID, e.ID)
	if err != nil || !got.Agreement.Recorded || got.Agreement.FileID != shortID {
		t.Fatalf("recorded agreement = %+v, %v", got.Agreement, err)
	}
	won, err := svc.CreateStage(ctx, actor, boot.WorkspaceID, StageInput{Name: "Won", Kind: "won"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Move(ctx, actor, boot.WorkspaceID, e.ID, won.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Get(ctx, actor, boot.WorkspaceID, e.ID)
	if err != nil || !got.Agreement.Recorded || got.Agreement.FileID != shortID {
		t.Fatalf("agreement after move = %+v, %v", got.Agreement, err)
	}
	if err := svc.Move(ctx, actor, boot.WorkspaceID, e.ID, e.StageID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetNextAction(ctx, actor, boot.WorkspaceID, e.ID, NextAction{Text: "Call", DueDate: "2020-01-01", OwnerID: boot.UserID}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	due, err := svc.ListDue(ctx, actor, boot.WorkspaceID, "2020-01-01")
	if err != nil || len(due) != 1 || due[0].Agreement.FileID != shortID || !due[0].Agreement.Recorded {
		t.Fatalf("due agreement = %+v, %v", due, err)
	}
	other, err := auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	var otherFile int64
	if err := db.QueryRowContext(ctx, `INSERT INTO files (workspace_id, object_key, original_name, size, status, created_at, updated_at) VALUES (?, 'other', 'other.pdf', 1, 'ready', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z') RETURNING id`, other).Scan(&otherFile); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetAgreement(ctx, actor, boot.WorkspaceID, e.ID, Agreement{FileID: id.EncodeKey(id.TypeFile, otherFile)}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("foreign file = %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, fileID); err == nil {
		t.Fatal("referenced file was deleted")
	}
	if err := svc.SetAgreement(ctx, actor, boot.WorkspaceID, e.ID, Agreement{}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Get(ctx, actor, boot.WorkspaceID, e.ID)
	if err != nil || got.Agreement.Recorded || got.Agreement.FileID != "" {
		t.Fatalf("cleared agreement = %+v, %v", got.Agreement, err)
	}
}
