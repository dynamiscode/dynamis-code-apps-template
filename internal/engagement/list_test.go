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

func TestListWorkspaceFiltersAndPages(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(ctx, db, config.SQLite); err != nil {
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
	cs := clients.NewService(db, config.SQLite, auth, 50)
	client, err := cs.Create(ctx, owner, boot.WorkspaceID, clients.Input{DisplayName: "Acme"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	stage, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	stage2, err := svc.CreateStage(ctx, owner, boot.WorkspaceID, StageInput{Name: "Other", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []ID
	for _, in := range []Input{{Name: "Alpha", ClientID: client.ID, StageID: stage.ID}, {Name: "100%", ClientID: client.ID, StageID: stage.ID}, {Name: "Alpha", ClientID: client.ID, StageID: stage2.ID}} {
		e, err := svc.Create(ctx, owner, boot.WorkspaceID, in, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, e.ID)
	}
	if err := svc.SetAssignee(ctx, owner, boot.WorkspaceID, ids[0], boot.UserID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ArchiveStage(ctx, owner, boot.WorkspaceID, stage.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
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
	other, err := auth.CreateWorkspace(ctx, owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, err := auth.Authorize(ctx, boot.UserID, other, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	foreignClient, err := cs.Create(ctx, otherOwner, other, clients.Input{DisplayName: "Acme"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	foreignStage, err := svc.CreateStage(ctx, otherOwner, other, StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := svc.Create(ctx, otherOwner, other, Input{Name: "Alpha", ClientID: foreignClient.ID, StageID: foreignStage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	check := func(in ListInput, total int, want ...ID) {
		t.Helper()
		got, err := svc.List(ctx, viewer, boot.WorkspaceID, in)
		if err != nil || got.Total != total || len(got.Engagements) != len(want) {
			t.Fatalf("list %+v = %+v, %v", in, got, err)
		}
		for i, id := range want {
			if got.Engagements[i].ID != id {
				t.Fatalf("order = %+v", got.Engagements)
			}
		}
	}
	check(ListInput{Page: 1, Limit: 2}, 3, ids[2], ids[1])
	check(ListInput{Page: 2, Limit: 2}, 3, ids[0])
	check(ListInput{Page: 1, Limit: 1, Sort: "name", Direction: "asc"}, 3, ids[1])
	check(ListInput{Page: 2, Limit: 1, Sort: "name", Direction: "asc"}, 3, ids[0])
	check(ListInput{Page: 3, Limit: 1, Sort: "name", Direction: "asc"}, 3, ids[2])
	check(ListInput{Page: 1, Limit: 2, Sort: "name", Direction: "desc"}, 3, ids[2], ids[0])
	check(ListInput{Page: 1, Limit: 3, Sort: "stage", Direction: "asc"}, 3, ids[0], ids[1], ids[2])
	check(ListInput{Page: 1, Limit: 3, Sort: "assignee", Direction: "asc"}, 3, ids[1], ids[2], ids[0])
	check(ListInput{Page: 1, Limit: 3, Sort: "client", Direction: "asc"}, 3, ids[0], ids[1], ids[2])
	check(ListInput{Page: 3, Limit: 2}, 3)
	check(ListInput{Page: 1, Limit: 10, Search: "acME"}, 3, ids[2], ids[1], ids[0])
	check(ListInput{Page: 1, Limit: 10, Search: "%"}, 1, ids[1])
	check(ListInput{Page: 1, Limit: 10, Search: "alpha", StageID: stage.ID, AssigneeID: &boot.UserID}, 1, ids[0])
	unassigned := identity.UserID(0)
	check(ListInput{Page: 1, Limit: 10, StageID: stage.ID, AssigneeID: &unassigned}, 1, ids[1])
	check(ListInput{Page: 1, Limit: 10, StageID: foreignStage.ID}, 0)
	if _, err := svc.List(ctx, viewer, other, ListInput{Page: 1, Limit: 10}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("foreign access: %v", err)
	}
	if err := svc.MoveMany(ctx, viewer, boot.WorkspaceID, []ID{ids[0]}, stage2.ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer bulk move: %v", err)
	}
	if err := svc.MoveMany(ctx, owner, boot.WorkspaceID, []ID{ids[0], 999999999}, stage2.ID, identity.AuditContext{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bulk move with missing engagement: %v", err)
	}
	if err := svc.MoveMany(ctx, owner, boot.WorkspaceID, []ID{ids[0], foreign.ID}, stage2.ID, identity.AuditContext{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bulk move with foreign engagement: %v", err)
	}
	var historyCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM engagement_stage_history WHERE workspace_id = ? AND engagement_id = ?`, boot.WorkspaceID, ids[0]).Scan(&historyCount); err != nil || historyCount != 0 {
		t.Fatalf("failed bulk move retained history: %d, %v", historyCount, err)
	}
	check(ListInput{Page: 1, Limit: 10, StageID: stage.ID}, 2, ids[1], ids[0])
	if err := svc.MoveMany(ctx, owner, boot.WorkspaceID, []ID{ids[0], ids[1]}, foreignStage.ID, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bulk move to foreign stage: %v", err)
	}
	if err := svc.MoveMany(ctx, owner, boot.WorkspaceID, []ID{ids[0], ids[1]}, stage2.ID, identity.AuditContext{}); err != nil {
		t.Fatalf("bulk move: %v", err)
	}
	check(ListInput{Page: 1, Limit: 10, StageID: stage2.ID}, 3, ids[2], ids[1], ids[0])
	accentClient, err := cs.Create(ctx, owner, boot.WorkspaceID, clients.Input{DisplayName: "ÉCOLE!_100%"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	accent, err := svc.Create(ctx, owner, boot.WorkspaceID, Input{Name: "MÜNCHEN!_%", ClientID: accentClient.ID, StageID: stage2.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	check(ListInput{Page: 1, Limit: 10, Search: "mÜnchen"}, 1, accent.ID)
	check(ListInput{Page: 1, Limit: 10, Search: "münchen"}, 0)
	check(ListInput{Page: 1, Limit: 10, Search: "école"}, 0)
	check(ListInput{Page: 1, Limit: 10, Search: "École"}, 1, accent.ID)
	for _, literal := range []string{"!", "_", "!_%"} {
		check(ListInput{Page: 1, Limit: 10, Search: literal}, 1, accent.ID)
	}
	check(ListInput{Page: 1, Limit: 1, Search: "%"}, 2, accent.ID)
	check(ListInput{Page: 2, Limit: 1, Search: "%"}, 2, ids[1])
	for _, in := range []ListInput{{Page: 0, Limit: 1}, {Page: 1, Limit: 101}, {Page: clients.MaxListPage + 1, Limit: 1}, {Page: 1, Limit: 1, Search: string([]byte{0xff})}} {
		if _, err := svc.List(ctx, owner, boot.WorkspaceID, in); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid %+v: %v", in, err)
		}
	}
}
