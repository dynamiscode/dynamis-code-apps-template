package engagement

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestStageMoveConfirmationAndClassification(t *testing.T) {
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
	lost, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Lost", Kind: "lost"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	open, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	won, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Won", Kind: "won"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(db, config.SQLite, auth, 50).Create(ctx, owner, boot.WorkspaceID, clients.Input{DisplayName: "Client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	e, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "Work", ClientID: client.ID, StageID: open.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, NextAction{Text: "Call", DueDate: time.Now().UTC().Format("2006-01-02"), OwnerID: boot.UserID}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.ListStages(ctx, owner, boot.WorkspaceID)
	if err != nil || len(snapshot) != 3 || snapshot[0].ID != open.ID || snapshot[1].ID != won.ID || snapshot[2].ID != lost.ID {
		t.Fatalf("group order: %+v %v", snapshot, err)
	}
	ids := []StageID{won.ID, open.ID, lost.ID}
	count, err := svc.StageEngagementCount(ctx, owner, boot.WorkspaceID, open.ID)
	if err != nil || count != 1 {
		t.Fatalf("count: %d %v", count, err)
	}
	other, err := auth.CreateWorkspace(ctx, owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, err := auth.Authorize(ctx, boot.UserID, other, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StageEngagementCount(ctx, otherOwner, other, open.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("foreign count: %v", err)
	}
	if err := svc.MoveStages(ctx, otherOwner, other, snapshot, ids, open.ID, "won", count, true, identity.AuditContext{}); !errors.Is(err, ErrStaleStages) {
		t.Fatalf("foreign move: %v", err)
	}
	if err := svc.MoveStages(ctx, owner, boot.WorkspaceID, snapshot, ids, open.ID, "won", count, false, identity.AuditContext{}); !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("unconfirmed: %v", err)
	}
	if err := svc.MoveStages(ctx, owner, boot.WorkspaceID, snapshot, ids, open.ID, "won", 0, true, identity.AuditContext{}); !errors.Is(err, ErrStaleStages) {
		t.Fatalf("changed count: %v", err)
	}
	second, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "Later", ClientID: client.ID, StageID: open.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveStages(ctx, owner, boot.WorkspaceID, snapshot, ids, open.ID, "won", count, true, identity.AuditContext{}); !errors.Is(err, ErrStaleStages) {
		t.Fatalf("new engagement: %v", err)
	}
	if second.ID == e.ID {
		t.Fatal("engagement IDs must differ")
	}
	count = 2
	if err := svc.MoveStages(ctx, identity.Principal{}, boot.WorkspaceID, snapshot, ids, open.ID, "won", count, true, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("unauthorized: %v", err)
	}
	if err := svc.MoveStages(ctx, owner, boot.WorkspaceID, snapshot, ids, open.ID, "won", count, true, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveStages(ctx, owner, boot.WorkspaceID, snapshot, ids, open.ID, "won", count, true, identity.AuditContext{}); !errors.Is(err, ErrStaleStages) {
		t.Fatalf("stale snapshot: %v", err)
	}
	got, err := svc.Get(ctx, owner, boot.WorkspaceID, e.ID)
	if err != nil || got.StageID != open.ID || len(got.History) != 0 || got.NextAction.Text != "Call" {
		t.Fatalf("engagement changed: %+v %v", got, err)
	}
	dashboard, err := svc.Dashboard(ctx, owner, boot.WorkspaceID, 1, 10)
	if err != nil || dashboard.Today != 0 || len(dashboard.Stages) != 0 {
		t.Fatalf("dashboard: %+v %v", dashboard, err)
	}
	queue, err := svc.ListActionQueue(ctx, owner, boot.WorkspaceID, ActionQueueOptions{Filter: ActionQueueToday, Page: 1, Limit: 10})
	if err != nil || len(queue.Entries) != 0 {
		t.Fatalf("queue: %+v %v", queue, err)
	}
	if err := svc.ArchiveStage(ctx, owner, boot.WorkspaceID, open.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = svc.ListStages(ctx, owner, boot.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveStages(ctx, owner, boot.WorkspaceID, snapshot, ids, open.ID, "won", 0, false, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("archived move: %v", err)
	}
}

func TestStageMoveWithinGroupPreservesArchived(t *testing.T) {
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
	first, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "First", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Archived", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	last, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Last", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ArchiveStage(ctx, owner, boot.WorkspaceID, archived.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := svc.ListStages(ctx, owner, boot.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveStages(ctx, owner, boot.WorkspaceID, snapshot, []StageID{last.ID, last.ID, archived.ID}, last.ID, "open", 0, false, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := svc.MoveStages(ctx, owner, boot.WorkspaceID, snapshot, []StageID{last.ID, first.ID}, last.ID, "open", 0, false, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("incomplete: %v", err)
	}
	order := []StageID{last.ID, first.ID, archived.ID}
	if err := svc.MoveStages(ctx, owner, boot.WorkspaceID, snapshot, order, last.ID, "open", 0, false, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveStages(ctx, owner, boot.WorkspaceID, snapshot, order, last.ID, "open", 0, false, identity.AuditContext{}); !errors.Is(err, ErrStaleStages) {
		t.Fatalf("stale order: %v", err)
	}
	stages, err := svc.ListStages(ctx, owner, boot.WorkspaceID)
	if err != nil || len(stages) != 3 || stages[0].ID != last.ID || stages[1].ID != first.ID || stages[2].ID != archived.ID || !stages[2].Archived {
		t.Fatalf("stages: %+v %v", stages, err)
	}
	results := make(chan error, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			results <- svc.MoveStages(ctx, owner, boot.WorkspaceID, stages, []StageID{first.ID, last.ID, archived.ID}, first.ID, "open", 0, false, identity.AuditContext{})
		}()
	}
	close(start)
	a, b := <-results, <-results
	if !((a == nil && errors.Is(b, ErrStaleStages)) || (b == nil && errors.Is(a, ErrStaleStages))) {
		t.Fatalf("concurrent moves: %v, %v", a, b)
	}
}
