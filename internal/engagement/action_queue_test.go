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

func TestActionQueue(t *testing.T) {
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
	owner, err := auth.Authorize(ctx, boot.UserID, boot.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, config.SQLite, auth)
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
	today := time.Now().UTC().Truncate(24 * time.Hour)
	dates := []string{today.AddDate(0, 0, -1).Format("2006-01-02"), today.Format("2006-01-02"), today.AddDate(0, 0, 1).Format("2006-01-02"), "", today.Format("2006-01-02")}
	var ids []ID
	for i, date := range dates {
		e, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "Engagement", ClientID: client.ID, StageID: open.ID}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, e.ID)
		if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, NextAction{Text: "Action", DueDate: date, OwnerID: boot.UserID}, identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
		if i == 2 || i == 3 || i == 4 {
			if err := svc.Move(ctx, owner, boot.WorkspaceID, e.ID, won.ID, identity.AuditContext{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := svc.ArchiveStage(ctx, owner, boot.WorkspaceID, open.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		filter ActionQueueFilter
		want   []ID
	}{{ActionQueueUrgent, ids[:2]}, {ActionQueueOverdue, ids[:1]}, {ActionQueueToday, ids[1:2]}, {ActionQueueUpcoming, ids[2:3]}, {ActionQueueUndated, ids[3:4]}} {
		p, err := svc.ListActionQueue(ctx, owner, boot.WorkspaceID, ActionQueueOptions{Filter: tc.filter, Page: 1, Limit: 10})
		if err != nil || len(p.Entries) != len(tc.want) {
			t.Fatalf("filter %q: %+v %v", tc.filter, p, err)
		}
		for i, id := range tc.want {
			if p.Entries[i].EngagementID != id || p.Entries[i].ClientName != "Client" || p.Entries[i].OwnerEmail != "owner@example.com" {
				t.Fatalf("entry: %+v", p.Entries[i])
			}
		}
	}
	first, err := svc.ListActionQueue(ctx, owner, boot.WorkspaceID, ActionQueueOptions{Page: 1, Limit: 1})
	if err != nil || !first.HasMore || first.Entries[0].EngagementID != ids[0] {
		t.Fatalf("first: %+v %v", first, err)
	}
	second, err := svc.ListActionQueue(ctx, owner, boot.WorkspaceID, ActionQueueOptions{Page: 2, Limit: 1})
	if err != nil || second.HasMore || second.Entries[0].EngagementID != ids[1] {
		t.Fatalf("second: %+v %v", second, err)
	}
	if err := svc.CompleteNextAction(ctx, owner, boot.WorkspaceID, ids[4], 1, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	p, err := svc.ListActionQueue(ctx, owner, boot.WorkspaceID, ActionQueueOptions{Filter: ActionQueueCompleted, OwnerID: &boot.UserID, Page: 1, Limit: 10})
	if err != nil || len(p.Entries) != 1 || p.Entries[0].HistoryID == 0 || p.Entries[0].CompletedAt == "" {
		t.Fatalf("completed: %+v %v", p, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE engagement_action_history SET owner_id = NULL WHERE id = ?`, p.Entries[0].HistoryID); err != nil {
		t.Fatal(err)
	}
	unassigned := identity.UserID(0)
	p, err = svc.ListActionQueue(ctx, owner, boot.WorkspaceID, ActionQueueOptions{Filter: ActionQueueCompleted, OwnerID: &unassigned, Page: 1, Limit: 10})
	if err != nil || len(p.Entries) != 1 || p.Entries[0].OwnerEmail != "" {
		t.Fatalf("unassigned: %+v %v", p, err)
	}
	other, err := auth.CreateWorkspace(ctx, owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, err := auth.Authorize(ctx, boot.UserID, other, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	p, err = svc.ListActionQueue(ctx, otherOwner, other, ActionQueueOptions{Page: 1, Limit: 10})
	if err != nil || len(p.Entries) != 0 {
		t.Fatalf("isolation: %+v %v", p, err)
	}
	if _, err := svc.ListActionQueue(ctx, identity.Principal{}, boot.WorkspaceID, ActionQueueOptions{Page: 1, Limit: 10}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("unauthorized: %v", err)
	}
	for _, opt := range []ActionQueueOptions{{Page: 0, Limit: 1}, {Page: 1, Limit: 101}, {Page: 1, Limit: 1, Filter: "bad"}} {
		if _, err := svc.ListActionQueue(ctx, owner, boot.WorkspaceID, opt); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid %+v: %v", opt, err)
		}
	}
}
