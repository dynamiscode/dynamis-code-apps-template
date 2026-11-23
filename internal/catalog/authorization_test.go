package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestCatalogAuthorizationAndIsolation(t *testing.T) {
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
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "A"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, auth, nil)
	for _, role := range []identity.Role{identity.Owner, identity.Admin, identity.Member, identity.Viewer} {
		t.Run(string(role), func(t *testing.T) {
			user := owner.UserID
			if role != identity.Owner {
				inv, err := auth.CreateInvitation(ctx, actor, string(role)+"@example.com", role, time.Hour, identity.AuditContext{})
				if err != nil {
					t.Fatal(err)
				}
				user, _, err = auth.CreateInvitedLocalUser(ctx, inv.Secret, "long-enough-password", identity.AuditContext{})
				if err != nil {
					t.Fatal(err)
				}
			}
			principal, err := auth.Authorize(ctx, user, owner.WorkspaceID, identity.ResourcesRead)
			if err != nil {
				t.Fatal(err)
			}
			entry, err := service.Create(ctx, actor, owner.WorkspaceID, "Seed", nil, testPrices())
			if err != nil {
				t.Fatal(err)
			}
			row, err := service.AddPrice(ctx, actor, owner.WorkspaceID, entry.ID.String(), PriceOption{Mode: "manual", Currency: "USD", Variant: "Visit"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.List(ctx, principal, owner.WorkspaceID, false); err != nil {
				t.Fatal(err)
			}
			writable := role == identity.Owner || role == identity.Admin
			check := func(name string, err error) {
				t.Helper()
				if writable && err != nil || !writable && !errors.Is(err, identity.ErrForbidden) {
					t.Errorf("%s: %v", name, err)
				}
			}
			_, err = service.Create(ctx, principal, owner.WorkspaceID, "New", nil, testPrices())
			check("create", err)
			check("edit", service.Update(ctx, principal, owner.WorkspaceID, entry.ID, "Edited", nil))
			_, err = service.AddPrice(ctx, principal, owner.WorkspaceID, entry.ID.String(), PriceOption{Mode: "manual", Currency: "USD", Variant: "Other"})
			check("add price", err)
			check("edit price", service.UpdatePrice(ctx, principal, owner.WorkspaceID, entry.ID, row.ID, PriceOption{Mode: "manual", Currency: "USD", Variant: "Edited"}))
			check("delete price", service.DeletePrice(ctx, principal, owner.WorkspaceID, entry.ID, row.ID))
			check("archive", service.SetArchived(ctx, principal, owner.WorkspaceID, entry.ID, true))
			check("unarchive", service.SetArchived(ctx, principal, owner.WorkspaceID, entry.ID, false))
			check("delete", service.Delete(ctx, principal, owner.WorkspaceID, entry.ID))
		})
	}
	other, err := auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "B"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherActor, err := auth.Authorize(ctx, owner.UserID, other, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := service.Create(ctx, actor, owner.WorkspaceID, "Private", nil, testPrices())
	if err != nil {
		t.Fatal(err)
	}
	row, err := service.AddPrice(ctx, actor, owner.WorkspaceID, entry.ID.String(), PriceOption{Mode: "manual", Currency: "USD", Variant: "Private"})
	if err != nil {
		t.Fatal(err)
	}
	local, err := service.Create(ctx, otherActor, other, "Local", nil, testPrices())
	if err != nil {
		t.Fatal(err)
	}
	list, err := service.List(ctx, otherActor, other, true)
	if err != nil || len(list) != 1 || list[0].Name != "Local" {
		t.Fatalf("foreign list: %v %v", list, err)
	}
	for name, call := range map[string]func() error{
		"entry edit":    func() error { return service.Update(ctx, otherActor, other, entry.ID, "stolen", nil) },
		"entry archive": func() error { return service.SetArchived(ctx, otherActor, other, entry.ID, true) },
		"entry delete":  func() error { return service.Delete(ctx, otherActor, other, entry.ID) },
		"price add": func() error {
			_, err := service.AddPrice(ctx, otherActor, other, entry.ID.String(), PriceOption{Mode: "manual", Currency: "USD", Variant: "x"})
			return err
		},
		"price edit": func() error {
			return service.UpdatePrice(ctx, otherActor, other, entry.ID, row.ID, PriceOption{Mode: "manual", Currency: "USD", Variant: "x"})
		},
		"price delete":                 func() error { return service.DeletePrice(ctx, otherActor, other, entry.ID, row.ID) },
		"foreign price on local entry": func() error { return service.DeletePrice(ctx, otherActor, other, local.ID, row.ID) },
	} {
		if err := call(); !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: %v", name, err)
		}
	}
	list, err = service.List(ctx, actor, owner.WorkspaceID, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range list {
		if e.ID == entry.ID {
			found = e.Name == "Private" && !e.Archived && len(e.Prices) == 2 && e.Prices[1].ID == row.ID
		}
	}
	if !found {
		t.Fatal("foreign mutations changed entry or row")
	}
}
