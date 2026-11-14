package clients

import (
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// forgedPrincipal claims every permission and a chosen role, as a caller-side
// bug or a stale in-memory principal could. The service must still deny, because
// it re-checks the role stored for the user in the workspace.
func forgedPrincipal(actor identity.Principal) identity.Principal {
	actor.Role = identity.Owner
	actor.Permissions = map[identity.Permission]bool{
		identity.WorkspaceRead: true, identity.WorkspaceUpdate: true, identity.WorkspaceDelete: true,
		identity.WorkspaceExport: true, identity.OwnershipTransfer: true, identity.MembersRead: true,
		identity.MembersManage: true, identity.InvitationsManage: true, identity.WebhooksRead: true,
		identity.WebhooksManage: true, identity.ResourcesRead: true, identity.ResourcesWrite: true,
		identity.SCIMManage: true, identity.Permission("clients.unlisted"): true,
	}
	return actor
}

// TestServiceDeniesActionsOutsideStoredRole checks, at the service layer and
// for every mutating Clients action, that a caller whose stored role lacks the
// action is refused even when the principal it presents claims every permission.
func TestServiceDeniesActionsOutsideStoredRole(t *testing.T) {
	f := newFixture(t)
	audit := identity.AuditContext{}
	client, err := f.service.Create(f.ctx, f.owner, f.workspaceID, Input{DisplayName: "Acme"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	archived, err := f.service.Create(f.ctx, f.owner, f.workspaceID, Input{DisplayName: "Old"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Archive(f.ctx, f.owner, f.workspaceID, archived.ID, audit); err != nil {
		t.Fatal(err)
	}
	text, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Source", Type: FieldTypeText}, audit)
	if err != nil {
		t.Fatal(err)
	}
	choice, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Tier", Type: FieldTypeSingleChoice}, audit)
	if err != nil {
		t.Fatal(err)
	}
	option, err := f.service.AddOption(f.ctx, f.owner, f.workspaceID, choice.ID, "Gold", audit)
	if err != nil {
		t.Fatal(err)
	}
	retired, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Retired", Type: FieldTypeText}, audit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ArchiveField(f.ctx, f.owner, f.workspaceID, retired.ID, audit); err != nil {
		t.Fatal(err)
	}
	name := "Renamed"
	actions := []struct {
		name string
		// adminOnly marks actions that need WorkspaceUpdate. Member holds
		// ResourcesWrite, so member is checked only on those.
		adminOnly bool
		call      func(actor identity.Principal) error
	}{
		{"create client", false, func(a identity.Principal) error {
			_, err := f.service.Create(f.ctx, a, f.workspaceID, Input{DisplayName: "New"}, audit)
			return err
		}},
		{"edit client", false, func(a identity.Principal) error {
			_, err := f.service.Update(f.ctx, a, f.workspaceID, client.ID, client.Version, UpdateInput{DisplayName: &name}, audit)
			return err
		}},
		{"set value", false, func(a identity.Principal) error {
			_, err := f.service.SetValue(f.ctx, a, f.workspaceID, client.ID, client.Version, text.ID, "x", audit)
			return err
		}},
		{"clear value", false, func(a identity.Principal) error {
			_, err := f.service.ClearValue(f.ctx, a, f.workspaceID, client.ID, client.Version, text.ID, audit)
			return err
		}},
		{"archive client", false, func(a identity.Principal) error {
			_, err := f.service.Archive(f.ctx, a, f.workspaceID, client.ID, audit)
			return err
		}},
		{"unarchive client", false, func(a identity.Principal) error {
			_, err := f.service.Unarchive(f.ctx, a, f.workspaceID, archived.ID, audit)
			return err
		}},
		{"delete client", true, func(a identity.Principal) error {
			return f.service.Delete(f.ctx, a, f.workspaceID, client.ID, audit)
		}},
		{"define field", true, func(a identity.Principal) error {
			_, err := f.service.DefineField(f.ctx, a, f.workspaceID, FieldInput{Label: "Other", Type: FieldTypeText}, audit)
			return err
		}},
		{"archive field", true, func(a identity.Principal) error {
			_, err := f.service.ArchiveField(f.ctx, a, f.workspaceID, text.ID, audit)
			return err
		}},
		{"restore field", true, func(a identity.Principal) error {
			_, err := f.service.RestoreField(f.ctx, a, f.workspaceID, retired.ID, audit)
			return err
		}},
		{"delete field", true, func(a identity.Principal) error {
			return f.service.DeleteField(f.ctx, a, f.workspaceID, retired.ID, audit)
		}},
		{"add option", true, func(a identity.Principal) error {
			_, err := f.service.AddOption(f.ctx, a, f.workspaceID, choice.ID, "Silver", audit)
			return err
		}},
		{"delete option", true, func(a identity.Principal) error {
			return f.service.DeleteOption(f.ctx, a, f.workspaceID, choice.ID, option.ID, audit)
		}},
	}
	for _, action := range actions {
		t.Run("viewer/"+action.name, func(t *testing.T) {
			if err := action.call(forgedPrincipal(f.viewer)); !errors.Is(err, identity.ErrForbidden) {
				t.Fatalf("viewer %s with forged permissions error = %v, want ErrForbidden", action.name, err)
			}
		})
		if !action.adminOnly {
			continue
		}
		t.Run("member/"+action.name, func(t *testing.T) {
			if err := action.call(forgedPrincipal(f.member)); !errors.Is(err, identity.ErrForbidden) {
				t.Fatalf("member %s with forged permissions error = %v, want ErrForbidden", action.name, err)
			}
		})
	}
}

// assertWorkspaceIsolation runs each record-naming action from workspace B with
// workspace A's keys and with a key that names nothing. The caller is a real
// owner of B, so the answer comes from the data layer. Both answers must match.
func assertWorkspaceIsolation(t *testing.T, f fixture) {
	t.Helper()
	audit := identity.AuditContext{}
	wsB, err := f.auth.CreateWorkspace(f.ctx, f.owner, identity.WorkspaceCreateInput{Name: "Other workspace"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := f.auth.Authorize(f.ctx, f.owner.UserID, wsB, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	mustOK := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	active, err := f.service.Create(f.ctx, f.owner, f.workspaceID, Input{DisplayName: "Acme"}, audit)
	mustOK(err)
	archived, err := f.service.Create(f.ctx, f.owner, f.workspaceID, Input{DisplayName: "Old"}, audit)
	mustOK(err)
	_, err = f.service.Archive(f.ctx, f.owner, f.workspaceID, archived.ID, audit)
	mustOK(err)
	text, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Source", Type: FieldTypeText}, audit)
	mustOK(err)
	choice, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Tier", Type: FieldTypeSingleChoice}, audit)
	mustOK(err)
	option, err := f.service.AddOption(f.ctx, f.owner, f.workspaceID, choice.ID, "Gold", audit)
	mustOK(err)
	restorable, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Legacy", Type: FieldTypeText}, audit)
	mustOK(err)
	_, err = f.service.ArchiveField(f.ctx, f.owner, f.workspaceID, restorable.ID, audit)
	mustOK(err)
	deletable, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Retired", Type: FieldTypeText}, audit)
	mustOK(err)
	_, err = f.service.ArchiveField(f.ctx, f.owner, f.workspaceID, deletable.ID, audit)
	mustOK(err)
	_, err = f.service.SetValue(f.ctx, f.owner, f.workspaceID, active.ID, active.Version, text.ID, "Referral", audit)
	mustOK(err)

	type keys struct {
		client ClientID
		field  FieldID
		option OptionID
	}
	actions := []struct {
		name    string
		foreign keys
		call    func(k keys) error
	}{
		{"get client", keys{client: active.ID}, func(k keys) error {
			_, err := f.service.Get(f.ctx, actor, wsB, k.client)
			return err
		}},
		{"edit client", keys{client: active.ID}, func(k keys) error {
			name := "Renamed"
			_, err := f.service.Update(f.ctx, actor, wsB, k.client, active.Version, UpdateInput{DisplayName: &name}, audit)
			return err
		}},
		{"set value", keys{client: active.ID, field: text.ID}, func(k keys) error {
			_, err := f.service.SetValue(f.ctx, actor, wsB, k.client, active.Version, k.field, "x", audit)
			return err
		}},
		{"clear value", keys{client: active.ID, field: text.ID}, func(k keys) error {
			_, err := f.service.ClearValue(f.ctx, actor, wsB, k.client, active.Version, k.field, audit)
			return err
		}},
		{"list values", keys{client: active.ID}, func(k keys) error {
			_, err := f.service.ListValues(f.ctx, actor, wsB, k.client)
			return err
		}},
		{"archive client", keys{client: active.ID}, func(k keys) error {
			_, err := f.service.Archive(f.ctx, actor, wsB, k.client, audit)
			return err
		}},
		{"unarchive client", keys{client: archived.ID}, func(k keys) error {
			_, err := f.service.Unarchive(f.ctx, actor, wsB, k.client, audit)
			return err
		}},
		{"delete client", keys{client: active.ID}, func(k keys) error {
			return f.service.Delete(f.ctx, actor, wsB, k.client, audit)
		}},
		{"archive field", keys{field: text.ID}, func(k keys) error {
			_, err := f.service.ArchiveField(f.ctx, actor, wsB, k.field, audit)
			return err
		}},
		{"restore field", keys{field: restorable.ID}, func(k keys) error {
			_, err := f.service.RestoreField(f.ctx, actor, wsB, k.field, audit)
			return err
		}},
		{"delete field", keys{field: deletable.ID}, func(k keys) error {
			return f.service.DeleteField(f.ctx, actor, wsB, k.field, audit)
		}},
		{"list options", keys{field: choice.ID}, func(k keys) error {
			_, err := f.service.ListOptions(f.ctx, actor, wsB, k.field)
			return err
		}},
		{"add option", keys{field: choice.ID}, func(k keys) error {
			_, err := f.service.AddOption(f.ctx, actor, wsB, k.field, "Silver", audit)
			return err
		}},
		{"delete option", keys{field: choice.ID, option: option.ID}, func(k keys) error {
			return f.service.DeleteOption(f.ctx, actor, wsB, k.field, k.option, audit)
		}},
	}
	// missing swaps every key the action names for one that names nothing.
	missing := func(k keys) keys {
		if k.client != 0 {
			k.client = 999999
		}
		if k.field != 0 {
			k.field = 999999
		}
		if k.option != 0 {
			k.option = 999999
		}
		return k
	}
	for _, action := range actions {
		foreign := action.call(action.foreign)
		absent := action.call(missing(action.foreign))
		if foreign == nil || absent == nil {
			t.Fatalf("%s from workspace B: foreign = %v, missing = %v; want refusals", action.name, foreign, absent)
		}
		if foreign.Error() != absent.Error() || errors.Is(foreign, identity.ErrForbidden) {
			t.Fatalf("%s from workspace B: foreign = %v, missing = %v; want the same not-found answer", action.name, foreign, absent)
		}
	}
}

func TestServiceIsolatesWorkspaces(t *testing.T) {
	assertWorkspaceIsolation(t, newFixture(t))
}
