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
	if err := svc.ReorderStages(ctx, member, boot.WorkspaceID, nil, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member reorder = %v", err)
	}
	if err := svc.ArchiveStage(ctx, member, boot.WorkspaceID, 0, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
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
	for _, event := range []string{"engagement_stage.reordered", "engagement_stage.archived"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE workspace_id = ? AND event_type = ?`, boot.WorkspaceID, event).Scan(&count); err != nil || count != 1 {
			t.Fatalf("audit %s = %d, %v", event, count, err)
		}
	}
	stages, err := svc.ListStages(ctx, owner, boot.WorkspaceID)
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
	_, err = svc.Create(ctx, owner, boot.WorkspaceID, Input{ClientID: client.ID}, identity.AuditContext{})
	if !errors.Is(err, ErrNoOpenStage) {
		t.Fatalf("no stage: %v", err)
	}
	stage, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(ctx, owner, boot.WorkspaceID, Input{StageID: stage.ID}, identity.AuditContext{})
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
	if _, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{ClientID: foreign.ID, StageID: stage.ID}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("foreign client: %v", err)
	}
	for _, kind := range []string{"won", "lost"} {
		closed, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: kind, Kind: kind}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{ClientID: client.ID, StageID: closed.ID}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s stage: %v", kind, err)
		}
	}
	created, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{ClientID: client.ID, StageID: stage.ID, Notes: "Other person"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctx, owner, boot.WorkspaceID, created.ID)
	if err != nil || got.ClientID != client.ID || got.StageID != stage.ID || got.Notes != "Other person" {
		t.Fatalf("get = %+v, %v", got, err)
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
	if _, err := svc.Get(ctx, viewer, boot.WorkspaceID, created.ID); err != nil {
		t.Fatalf("viewer get: %v", err)
	}
	if _, err := svc.Create(ctx, viewer, boot.WorkspaceID, Input{ClientID: client.ID}, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
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
	if _, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{}); !errors.Is(err, ErrNoOpenStage) {
		t.Fatalf("archived stage: %v", err)
	}
	if _, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Another", Kind: "open"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("archived target: %v", err)
	}
	if err := clientService.Delete(ctx, owner, boot.WorkspaceID, client.ID, identity.AuditContext{}); !errors.Is(err, clients.ErrHasEngagements) {
		t.Fatalf("delete client: %v", err)
	}
	if _, err := svc.Get(ctx, owner, boot.WorkspaceID, created.ID); err != nil {
		t.Fatalf("delete lost engagement: %v", err)
	}
	stages, err := svc.ListStages(ctx, owner, boot.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Move(ctx, viewer, boot.WorkspaceID, created.ID, stages[1].ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer move: %v", err)
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
	if err := svc.Move(ctx, owner, boot.WorkspaceID, created.ID, stages[1].ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Move(ctx, owner, boot.WorkspaceID, created.ID, stages[len(stages)-1].ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Get(ctx, viewer, boot.WorkspaceID, created.ID)
	if err != nil || got.StageID != stages[len(stages)-1].ID || len(got.History) != 2 || got.History[0].FromStageID != stage.ID || got.History[0].ToStageID != stages[1].ID {
		t.Fatalf("move history: %+v, %v", got, err)
	}
	if err := svc.Move(ctx, owner, boot.WorkspaceID, created.ID, stages[2].ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
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
	if _, err := maintenance.Run(ctx, db, config.SQLite, time.Now().Add(400*24*time.Hour), 365*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Get(ctx, viewer, boot.WorkspaceID, created.ID)
	if err != nil || len(got.History) != 5 {
		t.Fatalf("history after retention: %+v, %v", got, err)
	}
}
