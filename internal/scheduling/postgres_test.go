package scheduling

import (
	"context"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/testdb"
)

func TestPostgresResources(t *testing.T) {
	ctx := context.Background()
	db := testdb.Postgres(t)
	auth, err := identity.NewService(db, config.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "scheduling@example.com", Password: "long-enough-password", WorkspaceName: "Scheduling"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	r, err := s.Create(ctx, actor, owner.WorkspaceID, "Room", &owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, actor, owner.WorkspaceID, r.ID, "Office", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTimeZone(ctx, actor, owner.WorkspaceID, "America/Bogota"); err != nil {
		t.Fatal(err)
	}
	list, zone, err := s.List(ctx, actor, owner.WorkspaceID)
	if err != nil || zone != "America/Bogota" || len(list) != 1 || list[0].Name != "Office" || list[0].UserID != nil {
		t.Fatalf("list: %v %q %v", list, zone, err)
	}
	if err := s.SetTimeZone(ctx, actor, owner.WorkspaceID, "UTC"); err != nil {
		t.Fatal(err)
	}
	checkConcurrentBookings(t, s, actor, owner.WorkspaceID)
}
