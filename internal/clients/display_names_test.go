package clients

import (
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestDisplayNamesByID(t *testing.T) {
	f := newFixture(t)
	audit := identity.AuditContext{}
	active, err := f.service.Create(f.ctx, f.owner, f.workspaceID, Input{DisplayName: "Active"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	archived, err := f.service.Create(f.ctx, f.owner, f.workspaceID, Input{DisplayName: "Archived"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Archive(f.ctx, f.owner, f.workspaceID, archived.ID, audit); err != nil {
		t.Fatal(err)
	}
	deleted, err := f.service.Create(f.ctx, f.owner, f.workspaceID, Input{DisplayName: "Deleted"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Delete(f.ctx, f.owner, f.workspaceID, deleted.ID, audit); err != nil {
		t.Fatal(err)
	}
	other, err := f.auth.CreateWorkspace(f.ctx, f.owner, identity.WorkspaceCreateInput{Name: "Other"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	otherActor := principalFor(t, f.auth, f.owner.UserID, other, identity.ResourcesWrite)
	foreign, err := f.service.Create(f.ctx, otherActor, other, Input{DisplayName: "Foreign"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	ids := []ClientID{active.ID, archived.ID, deleted.ID, foreign.ID, active.ID}
	got, err := f.service.DisplayNamesByID(f.ctx, f.viewer, f.workspaceID, ids)
	if err != nil || len(got) != 2 || got[active.ID] != "Active" || got[archived.ID] != "Archived" {
		t.Fatalf("names = %v, %v", got, err)
	}
	if got, err := f.service.DisplayNamesByID(f.ctx, otherActor, other, ids); err != nil || len(got) != 1 || got[foreign.ID] != "Foreign" {
		t.Fatalf("other workspace names = %v, %v", got, err)
	}
	if got, err := f.service.DisplayNamesByID(f.ctx, f.viewer, f.workspaceID, nil); err != nil || len(got) != 0 {
		t.Fatalf("empty names = %v, %v", got, err)
	}
	hundred := make([]ClientID, 100)
	for i := range hundred {
		hundred[i] = active.ID
	}
	if got, err := f.service.DisplayNamesByID(f.ctx, f.viewer, f.workspaceID, hundred); err != nil || len(got) != 1 {
		t.Fatalf("maximum batch = %v, %v", got, err)
	}
	for _, invalid := range [][]ClientID{{0}, {-1}, make([]ClientID, 101)} {
		if _, err := f.service.DisplayNamesByID(f.ctx, f.owner, f.workspaceID, invalid); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid batch error = %v", err)
		}
	}
	if _, err := f.db.ExecContext(f.ctx, "DELETE FROM workspace_members WHERE workspace_id = ? AND user_id = ?", f.workspaceID, f.viewer.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.DisplayNamesByID(f.ctx, forgedPrincipal(f.viewer), f.workspaceID, []ClientID{active.ID}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("revoked viewer error = %v", err)
	}
	if _, err := f.service.DisplayNamesByID(f.ctx, forgedPrincipal(f.viewer), f.workspaceID, nil); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("revoked viewer empty batch error = %v", err)
	}
}
