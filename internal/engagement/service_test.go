package engagement

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/maintenance"
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
	otherStages, err := svc.ListStages(ctx, otherOwner, other)
	if err != nil || len(otherStages) != 0 {
		t.Fatalf("other workspace stages = %v, %v", otherStages, err)
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
	if err := svc.ReorderStages(ctx, member, boot.WorkspaceID, []StageID{stages[1].ID, stages[0].ID}, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member reorder = %v", err)
	}
	if err := svc.ArchiveStage(ctx, member, boot.WorkspaceID, stages[0].ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member archive = %v", err)
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
	lost, err := svc.CreateStage(ctx, admin, boot.WorkspaceID, StageInput{Name: "Lost", Kind: "lost"}, identity.AuditContext{})
	if err != nil {
		t.Fatalf("admin create: %v", err)
	}
	if err := svc.ReorderStages(ctx, admin, boot.WorkspaceID, []StageID{lost.ID, stages[1].ID, stages[0].ID}, identity.AuditContext{}); err != nil {
		t.Fatalf("admin reorder: %v", err)
	}
	if err := svc.ArchiveStage(ctx, admin, boot.WorkspaceID, lost.ID, identity.AuditContext{}); err != nil {
		t.Fatalf("admin archive: %v", err)
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

func TestReorderAndArchiveStages(t *testing.T) {
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
	second, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Second", Kind: "won"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ReorderStages(ctx, owner, boot.WorkspaceID, []StageID{first.ID, first.ID}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duplicate reorder = %v", err)
	}
	if err := svc.ReorderStages(ctx, owner, boot.WorkspaceID, []StageID{first.ID}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("incomplete reorder = %v", err)
	}
	other, err := auth.CreateWorkspace(ctx, owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, err := auth.Authorize(ctx, boot.UserID, other, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ArchiveStage(ctx, otherOwner, other, first.ID, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("foreign archive = %v", err)
	}
	if err := svc.ReorderStages(ctx, owner, boot.WorkspaceID, []StageID{second.ID, first.ID}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ArchiveStage(ctx, owner, boot.WorkspaceID, first.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReorderStages(ctx, owner, boot.WorkspaceID, []StageID{first.ID, second.ID}, identity.AuditContext{}); err != nil {
		t.Fatalf("reorder including archived stage: %v", err)
	}
	if err := svc.ReorderStages(ctx, owner, boot.WorkspaceID, []StageID{second.ID}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reorder excluding archived stage = %v", err)
	}
	stages, err := svc.ListStages(ctx, owner, boot.WorkspaceID)
	if err != nil || len(stages) != 2 || stages[0].ID != first.ID || !stages[0].Archived || stages[1].ID != second.ID {
		t.Fatalf("stages after archived reorder = %+v, %v", stages, err)
	}
	if err := svc.ReorderStages(ctx, owner, boot.WorkspaceID, []StageID{second.ID, first.ID}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"engagement_stage.reordered", "engagement_stage.archived"} {
		var count int
		want := 1
		if event == "engagement_stage.reordered" {
			want = 3
		}
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE workspace_id = ? AND event_type = ?`, boot.WorkspaceID, event).Scan(&count); err != nil || count != want {
			t.Fatalf("audit %s = %d, %v", event, count, err)
		}
	}
	stages, err = svc.ListStages(ctx, owner, boot.WorkspaceID)
	if err != nil || len(stages) != 2 || stages[0].ID != second.ID || stages[0].Kind != "won" || !stages[1].Archived {
		t.Fatalf("stages = %+v, %v", stages, err)
	}
}

func TestCreateEngagementRequiresOpenStageAndClient(t *testing.T) {
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
	clientService := clients.NewService(db, config.SQLite, auth, 50)
	client, err := clientService.Create(ctx, owner, boot.WorkspaceID, clients.Input{DisplayName: "Client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, config.SQLite, auth)
	for _, name := range []string{"", "  ", string([]byte{0xff}), strings.Repeat("界", 201)} {
		if _, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: name, ClientID: client.ID}, identity.AuditContext{}); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("invalid name %q: %v", name, err)
		}
	}
	_, err = svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "Engagement", ClientID: client.ID}, identity.AuditContext{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("no stage: %v", err)
	}
	stage, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "Engagement", ClientID: client.ID}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing stage with open stage available: %v", err)
	}
	_, err = svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "Engagement", StageID: stage.ID}, identity.AuditContext{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing client: %v", err)
	}
	other, err := auth.CreateWorkspace(ctx, owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, err := auth.Authorize(ctx, boot.UserID, other, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := clientService.Create(ctx, otherOwner, other, clients.Input{DisplayName: "Foreign"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "Engagement", ClientID: foreign.ID, StageID: stage.ID}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("foreign client: %v", err)
	}
	for _, kind := range []string{"won", "lost"} {
		closed, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: kind, Kind: kind}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "Engagement", ClientID: client.ID, StageID: closed.ID}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s stage: %v", kind, err)
		}
	}
	created, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "  Évaluation  ", ClientID: client.ID, StageID: stage.ID, Notes: "Other person"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctx, owner, boot.WorkspaceID, created.ID)
	if err != nil || got.Name != "Évaluation" || created.Name != "Évaluation" || got.ClientID != client.ID || got.StageID != stage.ID || got.Notes != "Other person" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	listed, err := svc.ListForClient(ctx, owner, boot.WorkspaceID, client.ID)
	if err != nil || len(listed) != 1 || listed[0].Name != "Évaluation" {
		t.Fatalf("list = %+v, %v", listed, err)
	}
	if got.Quote.Status != "not_sent" {
		t.Fatalf("default quote: %+v", got.Quote)
	}
	quote := Quote{Status: "sent", Amount: "120.50", DateSent: "2026-11-23"}
	if err := svc.SetQuote(ctx, owner, boot.WorkspaceID, created.ID, quote, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Get(ctx, owner, boot.WorkspaceID, created.ID)
	if err != nil || got.Quote != quote {
		t.Fatalf("quote: %+v %v", got.Quote, err)
	}
	if err := svc.SetQuote(ctx, owner, boot.WorkspaceID, created.ID, Quote{Status: "invalid"}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid quote: %v", err)
	}
	var viewerID identity.UserID
	if err := db.QueryRowContext(ctx, `INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id`, "viewer@example.com", time.Now().UTC().Format(time.RFC3339Nano)).Scan(&viewerID); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddMember(ctx, owner, viewerID, identity.Viewer, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	viewer, err := auth.Authorize(ctx, viewerID, boot.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetAssignee(ctx, owner, boot.WorkspaceID, created.ID, boot.UserID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetAssignee(ctx, owner, boot.WorkspaceID, created.ID, viewerID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	assigned, err := svc.Get(ctx, viewer, boot.WorkspaceID, created.ID)
	if err != nil || assigned.AssigneeID != viewerID {
		t.Fatalf("assignment: %+v %v", assigned, err)
	}
	if err := svc.SetName(ctx, viewer, boot.WorkspaceID, created.ID, "Denied", identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer rename: %v", err)
	}
	if err := svc.SetName(ctx, otherOwner, other, created.ID, "Denied", identity.AuditContext{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign rename: %v", err)
	}
	if err := svc.SetName(ctx, owner, boot.WorkspaceID, created.ID, string([]byte{0xff}), identity.AuditContext{}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("invalid UTF-8 rename: %v", err)
	}
	if err := svc.SetAssignee(ctx, viewer, boot.WorkspaceID, created.ID, 0, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer assignment: %v", err)
	}
	if err := svc.SetAssignee(ctx, owner, boot.WorkspaceID, created.ID, identity.UserID(999999), identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nonmember assignment: %v", err)
	}
	if err := svc.SetAssignee(ctx, owner, boot.WorkspaceID, created.ID, 0, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	assigned, err = svc.Get(ctx, owner, boot.WorkspaceID, created.ID)
	if err != nil || assigned.AssigneeID != 0 {
		t.Fatalf("unassigned: %+v %v", assigned, err)
	}
	var assignmentAudits int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE workspace_id = ? AND event_type = 'engagement.assignee_updated'`, boot.WorkspaceID).Scan(&assignmentAudits); err != nil || assignmentAudits != 3 {
		t.Fatalf("assignment audits: %d %v", assignmentAudits, err)
	}
	if err := svc.SetQuote(ctx, viewer, boot.WorkspaceID, created.ID, quote, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer quote: %v", err)
	}
	if _, err := svc.Get(ctx, viewer, boot.WorkspaceID, created.ID); err != nil {
		t.Fatalf("viewer get: %v", err)
	}
	if _, err := svc.Create(ctx, viewer, boot.WorkspaceID, Input{Name: "Engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer create: %v", err)
	}
	var auditCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE workspace_id = ? AND event_type = 'engagement.created'`, boot.WorkspaceID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("audit count = %d, %v", auditCount, err)
	}
	if err := svc.ArchiveStage(ctx, owner, boot.WorkspaceID, stage.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, owner, boot.WorkspaceID, created.ID); err != nil {
		t.Fatalf("archive lost engagement: %v", err)
	}
	if _, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "Engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("archived stage: %v", err)
	}
	if _, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Another", Kind: "open"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "Engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("archived target: %v", err)
	}
	stages, err := svc.ListStages(ctx, owner, boot.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Move(ctx, viewer, boot.WorkspaceID, created.ID, stages[1].ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer move: %v", err)
	}
	if err := svc.Move(ctx, otherOwner, other, created.ID, stages[1].ID, identity.AuditContext{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign engagement move: %v", err)
	}
	if err := svc.Move(ctx, viewer, other, created.ID, stages[1].ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("foreign principal move: %v", err)
	}
	if err := svc.Move(ctx, owner, boot.WorkspaceID, created.ID, stage.ID, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("archived move: %v", err)
	}
	foreignStage, err := svc.CreateStage(ctx, otherOwner, other, StageInput{Name: "Foreign", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Move(ctx, owner, boot.WorkspaceID, created.ID, foreignStage.ID, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("foreign stage: %v", err)
	}
	action := NextAction{Text: "Call", DueDate: "2020-01-01", OwnerID: boot.UserID, Version: 1}
	if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, created.ID, action, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetAssignee(ctx, owner, boot.WorkspaceID, created.ID, viewerID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Move(ctx, owner, boot.WorkspaceID, created.ID, stages[1].ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Move(ctx, owner, boot.WorkspaceID, created.ID, stages[1].ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	unchanged, err := svc.Get(ctx, viewer, boot.WorkspaceID, created.ID)
	if err != nil || len(unchanged.History) != 1 {
		t.Fatalf("same-stage move changed history: %+v, %v", unchanged.History, err)
	}
	if due, err := svc.ListDue(ctx, viewer, boot.WorkspaceID, "2020-01-01"); err != nil || len(due) != 0 {
		t.Fatalf("won due: %+v, %v", due, err)
	}
	if err := svc.Move(ctx, viewer, boot.WorkspaceID, created.ID, stages[len(stages)-1].ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer reopen: %v", err)
	}
	var memberID identity.UserID
	if err := db.QueryRowContext(ctx, `INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id`, "staff@example.com", time.Now().UTC().Format(time.RFC3339Nano)).Scan(&memberID); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddMember(ctx, owner, memberID, identity.Member, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	member, err := auth.Authorize(ctx, memberID, boot.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Move(ctx, member, boot.WorkspaceID, created.ID, stages[len(stages)-1].ID, identity.AuditContext{}); err != nil {
		t.Fatalf("member reopen: %v", err)
	}
	if due, err := svc.ListDue(ctx, viewer, boot.WorkspaceID, "2020-01-01"); err != nil || len(due) != 1 || len(due[0].History) != 2 {
		t.Fatalf("reopened due: %+v, %v", due, err)
	}
	got, err = svc.Get(ctx, viewer, boot.WorkspaceID, created.ID)
	if err != nil || got.StageID != stages[len(stages)-1].ID || got.NextAction != action || got.Quote != quote || got.AssigneeID != viewerID || len(got.History) != 2 || got.History[0].FromStageID != stage.ID || got.History[0].ToStageID != stages[1].ID || got.History[1].FromStageID != stages[1].ID || got.History[1].ToStageID != stages[len(stages)-1].ID {
		t.Fatalf("move history: %+v, %v", got, err)
	}
	for i, actorID := range []identity.UserID{owner.UserID, memberID} {
		if _, err := time.Parse(time.RFC3339Nano, got.History[i].CreatedAt); err != nil || got.History[i].ActorUserID != actorID {
			t.Fatalf("history actor or time: %+v, %v", got.History[i], err)
		}
	}
	if err := svc.Move(ctx, owner, boot.WorkspaceID, created.ID, stages[2].ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if due, err := svc.ListDue(ctx, viewer, boot.WorkspaceID, "2020-01-01"); err != nil || len(due) != 0 {
		t.Fatalf("lost due: %+v, %v", due, err)
	}
	if err := svc.Move(ctx, owner, boot.WorkspaceID, created.ID, stages[3].ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ArchiveStage(ctx, owner, boot.WorkspaceID, stages[3].ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Get(ctx, viewer, boot.WorkspaceID, created.ID)
	if err != nil || got.StageID != stages[3].ID || len(got.History) != 4 || got.History[3].FromStageID != stages[2].ID {
		t.Fatalf("archive history: %+v, %v", got, err)
	}
	duplicate, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Another", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Move(ctx, owner, boot.WorkspaceID, created.ID, duplicate.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Get(ctx, viewer, boot.WorkspaceID, created.ID)
	if err != nil || len(got.History) != 5 || got.History[4].FromStageID != stages[3].ID || got.History[4].ToStageID != duplicate.ID {
		t.Fatalf("same name history: %+v, %v", got, err)
	}
	var moveAudits int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE workspace_id = ? AND event_type = 'engagement.moved'`, boot.WorkspaceID).Scan(&moveAudits); err != nil || moveAudits != 5 {
		t.Fatalf("move audits: %d, %v", moveAudits, err)
	}
	if _, err := maintenance.Run(ctx, db, config.SQLite, time.Now().Add(400*24*time.Hour), 365*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE workspace_id = ? AND event_type = 'engagement.moved'`, boot.WorkspaceID).Scan(&moveAudits); err != nil || moveAudits != 0 {
		t.Fatalf("pruned move audits: %d, %v", moveAudits, err)
	}
	got, err = svc.Get(ctx, viewer, boot.WorkspaceID, created.ID)
	if err != nil || len(got.History) != 5 {
		t.Fatalf("history after retention: %+v, %v", got, err)
	}
	if err := clientService.Delete(ctx, owner, boot.WorkspaceID, client.ID, identity.AuditContext{}); err != nil {
		t.Fatalf("delete client: %v", err)
	}
	for _, table := range []string{"engagements", "engagement_next_actions", "engagement_stage_history"} {
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE workspace_id = ?", boot.WorkspaceID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s after deletion: %d, %v", table, count, err)
		}
	}
}

func TestCompleteNextAction(t *testing.T) {
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
	stage, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(db, config.SQLite, auth, 50).Create(ctx, owner, boot.WorkspaceID, clients.Input{DisplayName: "Client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	e, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "Engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	action := NextAction{Text: "Call", DueDate: "2020-01-01", OwnerID: boot.UserID}
	if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, action, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	first, err := svc.Get(ctx, owner, boot.WorkspaceID, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, action, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	current, err := svc.Get(ctx, owner, boot.WorkspaceID, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteNextAction(ctx, owner, boot.WorkspaceID, e.ID, first.NextAction.Version, identity.AuditContext{}); !errors.Is(err, ErrStaleNextAction) {
		t.Fatalf("stale: %v", err)
	}
	var viewerID identity.UserID
	if err := db.QueryRowContext(ctx, `INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id`, "viewer@example.com", time.Now().UTC().Format(time.RFC3339Nano)).Scan(&viewerID); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddMember(ctx, owner, viewerID, identity.Viewer, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	viewer, err := auth.Authorize(ctx, viewerID, boot.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteNextAction(ctx, viewer, boot.WorkspaceID, e.ID, current.NextAction.Version, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_completion BEFORE INSERT ON engagement_action_history WHEN NEW.outcome = 'completed' BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteNextAction(ctx, owner, boot.WorkspaceID, e.ID, current.NextAction.Version, identity.AuditContext{}); err == nil {
		t.Fatal("expected history failure")
	}
	after, err := svc.Get(ctx, owner, boot.WorkspaceID, e.ID)
	if err != nil || after.NextAction != current.NextAction {
		t.Fatalf("rollback: %+v, %v", after.NextAction, err)
	}
	var counter int64
	if err := db.QueryRowContext(ctx, `SELECT action_version FROM engagements WHERE id = ?`, e.ID).Scan(&counter); err != nil || counter != current.NextAction.Version {
		t.Fatalf("rollback counter: %d, %v", counter, err)
	}
	if _, err := db.ExecContext(ctx, `DROP TRIGGER fail_completion`); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteNextAction(ctx, owner, boot.WorkspaceID, e.ID, current.NextAction.Version, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteNextAction(ctx, owner, boot.WorkspaceID, e.ID, current.NextAction.Version, identity.AuditContext{}); !errors.Is(err, ErrStaleNextAction) {
		t.Fatalf("duplicate: %v", err)
	}
	after, err = svc.Get(ctx, owner, boot.WorkspaceID, e.ID)
	if err != nil || after.NextAction.Text != "" {
		t.Fatalf("completed: %+v, %v", after.NextAction, err)
	}
	var text, due, outcome, created string
	var ownerID, actorID identity.UserID
	var count int
	if err := db.QueryRowContext(ctx, `SELECT action_text, due_date, owner_id, outcome, actor_user_id, created_at FROM engagement_action_history WHERE engagement_id = ? AND outcome = 'completed'`, e.ID).Scan(&text, &due, &ownerID, &outcome, &actorID, &created); err != nil {
		t.Fatal(err)
	}
	if text != action.Text || due != action.DueDate || ownerID != boot.UserID || actorID != boot.UserID || outcome != "completed" || created == "" {
		t.Fatalf("history: %q %q %d %q %d %q", text, due, ownerID, outcome, actorID, created)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM engagement_action_history WHERE engagement_id = ? AND outcome = 'completed'`, e.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("history count: %d, %v", count, err)
	}
	var historyID, completedVersion int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM engagement_action_history WHERE engagement_id = ? AND outcome = 'completed'`, e.ID).Scan(&historyID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT action_version FROM engagements WHERE id = ?`, e.ID).Scan(&completedVersion); err != nil {
		t.Fatal(err)
	}
	reopen := func(principal identity.Principal, id, version int64) error {
		return svc.ReopenNextAction(ctx, principal, boot.WorkspaceID, e.ID, id, version, identity.AuditContext{})
	}
	if err := reopen(viewer, historyID, completedVersion); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("reopen viewer: %v", err)
	}
	for _, tc := range []struct{ id, version int64 }{{historyID, completedVersion - 1}, {historyID + 10000, completedVersion}} {
		if err := reopen(owner, tc.id, tc.version); !errors.Is(err, ErrStaleNextAction) {
			t.Fatalf("invalid reopen: %v", err)
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_reopen BEFORE INSERT ON engagement_action_history WHEN NEW.outcome = 'reopened' BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := reopen(owner, historyID, completedVersion); err == nil {
		t.Fatal("expected reopen history failure")
	}
	if err := db.QueryRowContext(ctx, `SELECT action_version FROM engagements WHERE id = ?`, e.ID).Scan(&counter); err != nil || counter != completedVersion {
		t.Fatalf("reopen rollback counter: %d, %v", counter, err)
	}
	if got, err := svc.Get(ctx, owner, boot.WorkspaceID, e.ID); err != nil || got.NextAction.Text != "" {
		t.Fatalf("reopen rollback action: %+v, %v", got.NextAction, err)
	}
	if _, err := db.ExecContext(ctx, `DROP TRIGGER fail_reopen`); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- reopen(owner, historyID, completedVersion) }()
	}
	var successes, stale int
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, ErrStaleNextAction) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("concurrent reopen: successes=%d stale=%d", successes, stale)
	}
	reopened, err := svc.Get(ctx, owner, boot.WorkspaceID, e.ID)
	if err != nil || reopened.NextAction.Text != action.Text || reopened.NextAction.DueDate != action.DueDate || reopened.NextAction.OwnerID != action.OwnerID || reopened.NextAction.Version != completedVersion+1 {
		t.Fatalf("reopened: %+v, %v", reopened.NextAction, err)
	}
	if err := reopen(owner, historyID, reopened.NextAction.Version); !errors.Is(err, ErrStaleNextAction) {
		t.Fatalf("occupied: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM engagement_action_history WHERE engagement_id = ? AND outcome IN ('completed', 'reopened')`, e.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("reopen history count: %d, %v", count, err)
	}
	var reopenedID int64
	if err := db.QueryRowContext(ctx, `SELECT id, action_text, due_date, owner_id, actor_user_id, created_at FROM engagement_action_history WHERE engagement_id = ? AND outcome = 'reopened'`, e.ID).Scan(&reopenedID, &text, &due, &ownerID, &actorID, &created); err != nil || text != action.Text || due != action.DueDate || ownerID != boot.UserID || actorID != boot.UserID || created == "" {
		t.Fatalf("reopened history: %q %q %d %d %q, %v", text, due, ownerID, actorID, created, err)
	}
	if err := svc.CompleteNextAction(ctx, owner, boot.WorkspaceID, e.ID, reopened.NextAction.Version, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := reopen(owner, historyID, completedVersion); !errors.Is(err, ErrStaleNextAction) {
		t.Fatalf("old replay after completion: %v", err)
	}
	if err := reopen(owner, reopenedID, reopened.NextAction.Version+1); !errors.Is(err, ErrStaleNextAction) {
		t.Fatalf("non-completed history: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM engagement_action_history WHERE engagement_id = ? AND outcome = 'completed'`, e.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("completion cycle history: %d, %v", count, err)
	}
	if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, action, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	recreated, err := svc.Get(ctx, owner, boot.WorkspaceID, e.ID)
	if err != nil || recreated.NextAction.Version <= current.NextAction.Version {
		t.Fatalf("recreated: %+v, %v", recreated.NextAction, err)
	}
	if err := svc.CompleteNextAction(ctx, owner, boot.WorkspaceID, e.ID, current.NextAction.Version, identity.AuditContext{}); !errors.Is(err, ErrStaleNextAction) {
		t.Fatalf("old version after recreation: %v", err)
	}
}

func TestNextActionDueList(t *testing.T) {
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
	stage, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(db, config.SQLite, auth, 50).Create(ctx, owner, boot.WorkspaceID, clients.Input{DisplayName: "Client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	e, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "Engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, NextAction{Text: "Call", DueDate: "2020-01-01", OwnerID: boot.UserID}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctx, owner, boot.WorkspaceID, e.ID)
	if err != nil || got.NextAction.Text != "Call" || got.NextAction.Version != 1 {
		t.Fatalf("get = %+v, %v", got, err)
	}
	listed, err := svc.ListForClient(ctx, owner, boot.WorkspaceID, client.ID)
	if err != nil || len(listed) != 1 || listed[0].NextAction.Version != got.NextAction.Version {
		t.Fatalf("list = %+v, %v", listed, err)
	}
	due, err := svc.ListDue(ctx, owner, boot.WorkspaceID, "2020-01-01")
	if err != nil || len(due) != 1 || due[0].ID != e.ID || due[0].NextAction.Version != got.NextAction.Version {
		t.Fatalf("due = %+v, %v", due, err)
	}
	for _, invalid := range []NextAction{{Text: "Call", OwnerID: 999999}, {Text: "Call", OwnerID: boot.UserID, DueDate: "2020-02-30"}} {
		if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, invalid, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid action %+v: %v", invalid, err)
		}
	}
	var viewerID identity.UserID
	if err := db.QueryRowContext(ctx, `INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id`, "viewer@example.com", time.Now().UTC().Format(time.RFC3339Nano)).Scan(&viewerID); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddMember(ctx, owner, viewerID, identity.Viewer, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	viewer, err := auth.Authorize(ctx, viewerID, boot.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetNextAction(ctx, viewer, boot.WorkspaceID, e.ID, NextAction{}, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer change: %v", err)
	}
	if got, err := svc.Get(ctx, viewer, boot.WorkspaceID, e.ID); err != nil || got.NextAction.Text != "Call" {
		t.Fatalf("viewer read: %+v %v", got, err)
	}
	other, err := auth.CreateWorkspace(ctx, owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, err := auth.Authorize(ctx, boot.UserID, other, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	if due, err := svc.ListDue(ctx, otherOwner, other, "2020-01-01"); err != nil || len(due) != 0 {
		t.Fatalf("foreign due: %+v %v", due, err)
	}
	if _, err := svc.Get(ctx, otherOwner, other, e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign get: %v", err)
	}
	if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, NextAction{Text: "Call", DueDate: "2020-01-01", OwnerID: boot.UserID}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, NextAction{Text: "Rejected", OwnerID: identity.UserID(999999999)}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nonmember owner: %v", err)
	}
	if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, NextAction{Text: "Later", DueDate: "2020-01-02", OwnerID: viewerID}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if due, err := svc.ListDue(ctx, owner, boot.WorkspaceID, "2020-01-01"); err != nil || len(due) != 0 {
		t.Fatalf("future due: %+v %v", due, err)
	}
	replaced, err := svc.Get(ctx, owner, boot.WorkspaceID, e.ID)
	if err != nil || replaced.NextAction.Version <= got.NextAction.Version {
		t.Fatalf("replacement version: %+v, %v", replaced.NextAction, err)
	}
	if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, NextAction{}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, NextAction{}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	due, err = svc.ListDue(ctx, owner, boot.WorkspaceID, "2020-01-01")
	if err != nil || len(due) != 0 {
		t.Fatalf("cleared due = %+v, %v", due, err)
	}
	if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, NextAction{Text: "Again", OwnerID: boot.UserID}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	recreated, err := svc.Get(ctx, owner, boot.WorkspaceID, e.ID)
	if err != nil || recreated.NextAction.Version <= replaced.NextAction.Version {
		t.Fatalf("recreated version: %+v, %v", recreated.NextAction, err)
	}
	rows, err := db.QueryContext(ctx, `SELECT action_text, due_date, owner_id, outcome, actor_user_id, created_at FROM engagement_action_history WHERE workspace_id = ? AND engagement_id = ? ORDER BY id`, boot.WorkspaceID, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for _, want := range []struct {
		text, date, outcome string
		owner               identity.UserID
	}{{"Call", "2020-01-01", "replaced", boot.UserID}, {"Later", "2020-01-02", "cleared", viewerID}} {
		if !rows.Next() {
			t.Fatal("missing history row")
		}
		var text, date, outcome, created string
		var ownerID, actorID identity.UserID
		if err := rows.Scan(&text, &date, &ownerID, &outcome, &actorID, &created); err != nil {
			t.Fatal(err)
		}
		if text != want.text || date != want.date || ownerID != want.owner || outcome != want.outcome || actorID != boot.UserID {
			t.Fatalf("unexpected history: %q %q %d %q %d", text, date, ownerID, outcome, actorID)
		}
		if parsed, err := time.Parse(time.RFC3339Nano, created); err != nil || parsed.Location() != time.UTC {
			t.Fatalf("invalid UTC time %q", created)
		}
	}
	if rows.Next() || rows.Err() != nil {
		t.Fatalf("unexpected extra history: %v", rows.Err())
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetNextAction(ctx, owner, boot.WorkspaceID, e.ID, NextAction{Text: "Unassigned"}, identity.AuditContext{}); err != nil {
		t.Fatalf("set unassigned action: %v", err)
	}
	if unassigned, err := svc.Get(ctx, owner, boot.WorkspaceID, e.ID); err != nil || unassigned.NextAction.Text != "Unassigned" || unassigned.NextAction.OwnerID != 0 {
		t.Fatalf("unassigned action: %+v, %v", unassigned.NextAction, err)
	}
}
