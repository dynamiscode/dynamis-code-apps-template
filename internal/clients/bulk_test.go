package clients

import (
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// TestBulkArchiveIsAllOrNothing checks that a bulk archive changes every listed client or none, and that viewers
// are refused as they are for a single client.
func TestBulkArchiveIsAllOrNothing(t *testing.T) {
	f := newFixture(t)
	first, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "First"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Second"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	missing := ClientID(999999)

	if err := f.service.BulkArchive(f.ctx, f.viewer, f.workspaceID, []ClientID{first.ID}, identity.AuditContext{}, true); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer bulk archive error = %v, want ErrForbidden", err)
	}
	if client, err := f.service.Get(f.ctx, f.member, f.workspaceID, first.ID); err != nil || client.ArchivedAt != nil || client.Version != first.Version {
		t.Fatalf("client after refused viewer bulk archive = %+v, %v; want unchanged", client, err)
	}
	if err := f.service.BulkArchive(f.ctx, f.member, f.workspaceID, []ClientID{first.ID, second.ID, missing}, identity.AuditContext{}, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bulk archive with a missing client error = %v, want ErrNotFound", err)
	}
	for _, id := range []ClientID{first.ID, second.ID} {
		client, err := f.service.Get(f.ctx, f.member, f.workspaceID, id)
		if err != nil {
			t.Fatal(err)
		}
		if client.ArchivedAt != nil {
			t.Fatalf("client %d archived after a failed bulk request; the batch must change nothing", id)
		}
	}

	if err := f.service.BulkArchive(f.ctx, f.member, f.workspaceID, []ClientID{first.ID, second.ID, first.ID}, identity.AuditContext{}, true); err != nil {
		t.Fatalf("bulk archive error = %v", err)
	}
	for _, id := range []ClientID{first.ID, second.ID} {
		client, err := f.service.Get(f.ctx, f.member, f.workspaceID, id)
		if err != nil {
			t.Fatal(err)
		}
		if client.ArchivedAt == nil {
			t.Fatalf("client %d not archived by bulk archive", id)
		}
	}
}

// TestBulkDeleteNeedsOwnerAndIsAllOrNothing checks that only owners and admins can bulk delete, and that a failed
// batch removes nothing.
func TestBulkDeleteNeedsOwnerAndIsAllOrNothing(t *testing.T) {
	f := newFixture(t)
	keep, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Keep"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	gone, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Gone"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}

	if err := f.service.BulkDelete(f.ctx, f.member, f.workspaceID, []ClientID{keep.ID}, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member bulk delete error = %v, want ErrForbidden", err)
	}
	if err := f.service.BulkDelete(f.ctx, f.owner, f.workspaceID, []ClientID{gone.ID, ClientID(999999)}, identity.AuditContext{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("owner bulk delete with a missing client error = %v, want ErrNotFound", err)
	}
	if _, err := f.service.Get(f.ctx, f.owner, f.workspaceID, gone.ID); err != nil {
		t.Fatalf("client removed by a failed bulk delete: %v", err)
	}

	if err := f.service.BulkDelete(f.ctx, f.owner, f.workspaceID, []ClientID{gone.ID}, identity.AuditContext{}); err != nil {
		t.Fatalf("owner bulk delete error = %v", err)
	}
	if _, err := f.service.Get(f.ctx, f.owner, f.workspaceID, gone.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted client Get error = %v, want ErrNotFound", err)
	}
	if _, err := f.service.Get(f.ctx, f.owner, f.workspaceID, keep.ID); err != nil {
		t.Fatalf("unselected client lost: %v", err)
	}
}
