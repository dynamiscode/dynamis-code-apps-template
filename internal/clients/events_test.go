package clients

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestChangesRecordCreateAndUpdateForViewers(t *testing.T) {
	fx := newFixture(t)
	created, err := fx.service.Create(fx.ctx, fx.member, fx.workspaceID, Input{DisplayName: "Ada Lovelace"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	page, err := fx.service.Changes(fx.ctx, fx.viewer, fx.workspaceID, changeOrigin, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Changes) != 1 {
		t.Fatalf("changes after create = %d, want 1", len(page.Changes))
	}
	change := page.Changes[0]
	if change.ClientID != created.ID || change.Action != "created" || change.ClientVersion != 1 || change.SchemaVersion != 1 {
		t.Fatalf("create change = %+v", change)
	}
	encoded, err := json.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "Ada Lovelace") || !strings.Contains(string(encoded), created.ID.String()) {
		t.Fatalf("change payload = %s, want the short ID and no display name", encoded)
	}

	phone := "555-0100"
	if _, err := fx.service.Update(fx.ctx, fx.member, fx.workspaceID, created.ID, 1, UpdateInput{Phone: &phone}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	page, err = fx.service.Changes(fx.ctx, fx.viewer, fx.workspaceID, page.Next, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Changes) != 1 || page.Changes[0].Action != "updated" || page.Changes[0].ClientVersion != 2 {
		t.Fatalf("update changes = %+v", page.Changes)
	}

	tier, err := fx.service.DefineField(fx.ctx, fx.owner, fx.workspaceID, FieldInput{Label: "Tier", Type: FieldTypeText}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	set, err := fx.service.SetValue(fx.ctx, fx.member, fx.workspaceID, created.ID, 2, tier.ID, "gold", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.service.ClearValue(fx.ctx, fx.member, fx.workspaceID, created.ID, set.Version, tier.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	page, err = fx.service.Changes(fx.ctx, fx.viewer, fx.workspaceID, page.Next, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Changes) != 2 || page.Changes[0].Action != "updated" || page.Changes[0].ClientVersion != 3 ||
		page.Changes[1].Action != "updated" || page.Changes[1].ClientVersion != 4 {
		t.Fatalf("value write changes = %+v, want two updated changes at versions 3 and 4", page.Changes)
	}
}

func TestChangesAreScopedToTheirWorkspace(t *testing.T) {
	fx := newFixture(t)
	if _, err := fx.service.Create(fx.ctx, fx.member, fx.workspaceID, Input{DisplayName: "Grace Hopper"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	otherWorkspace, err := fx.auth.CreateWorkspace(fx.ctx, fx.owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherOwner := principalFor(t, fx.auth, fx.owner.UserID, otherWorkspace, identity.ResourcesRead)
	page, err := fx.service.Changes(fx.ctx, otherOwner, otherWorkspace, changeOrigin, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Changes) != 0 {
		t.Fatalf("other workspace received %d changes", len(page.Changes))
	}
	// A principal without membership in the first workspace is refused outright.
	otherMember := addMember(t, fx.ctx, fx.db, fx.auth, otherOwner, otherWorkspace, "other-member@example.com", identity.Member)
	if _, err := fx.service.Changes(fx.ctx, otherMember, fx.workspaceID, changeOrigin, 10); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("cross-workspace read error = %v, want forbidden", err)
	}
}

func TestChangesRetainOnlyTheLatestThousandPerWorkspace(t *testing.T) {
	fx := newFixture(t)
	var first, last ClientID
	for i := range 1001 {
		client, err := fx.service.Create(fx.ctx, fx.member, fx.workspaceID, Input{DisplayName: "Bulk"}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = client.ID
		}
		last = client.ID
	}
	retained := 0
	var sawFirst, sawLast bool
	cursor := changeOrigin
	for {
		page, err := fx.service.Changes(fx.ctx, fx.viewer, fx.workspaceID, cursor, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Changes) == 0 {
			break
		}
		for _, change := range page.Changes {
			sawFirst = sawFirst || change.ClientID == first
			sawLast = sawLast || change.ClientID == last
		}
		retained += len(page.Changes)
		cursor = page.Next
	}
	if retained != 1000 || sawFirst || !sawLast {
		t.Fatalf("retained changes = %d, first present = %v, last present = %v; want 1000 latest", retained, sawFirst, sawLast)
	}
}
