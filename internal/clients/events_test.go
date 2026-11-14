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
	if _, err := fx.service.Changes(fx.ctx, otherOwner, fx.workspaceID, changeOrigin, 10); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("cross-workspace read error = %v, want forbidden", err)
	}
}

func TestChangesRetainOnlyTheLatestThousandPerWorkspace(t *testing.T) {
	fx := newFixture(t)
	for range maximumRetainedChanges + 1 {
		if _, err := fx.service.Create(fx.ctx, fx.member, fx.workspaceID, Input{DisplayName: "Bulk"}, identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
	}
	retained := 0
	cursor := changeOrigin
	for {
		page, err := fx.service.Changes(fx.ctx, fx.viewer, fx.workspaceID, cursor, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Changes) == 0 {
			break
		}
		retained += len(page.Changes)
		cursor = page.Next
	}
	if retained != maximumRetainedChanges {
		t.Fatalf("retained changes = %d, want %d", retained, maximumRetainedChanges)
	}
}
