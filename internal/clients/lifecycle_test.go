package clients

import (
	"errors"
	"slices"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func createLifecycleClient(t *testing.T, f fixture, actor identity.Principal, name string) Client {
	t.Helper()
	client, err := f.service.Create(f.ctx, actor, f.workspaceID, Input{DisplayName: name}, identity.AuditContext{})
	if err != nil {
		t.Fatalf("Create(%s) error = %v", name, err)
	}
	return client
}

// clientActions returns the change-feed actions for one client, oldest first.
func clientActions(t *testing.T, f fixture, clientID ClientID) []string {
	t.Helper()
	page, err := f.service.Changes(f.ctx, f.viewer, f.workspaceID, changeOrigin, 100)
	if err != nil {
		t.Fatalf("Changes() error = %v", err)
	}
	var actions []string
	for _, change := range page.Changes {
		if change.ClientID == clientID {
			actions = append(actions, change.Action)
		}
	}
	return actions
}

func countAudit(t *testing.T, f fixture, eventType string, clientID ClientID) int {
	t.Helper()
	var count int
	if err := f.db.QueryRowContext(f.ctx,
		database.Rebind(f.service.driver,
			`SELECT COUNT(*) FROM audit_events WHERE event_type = ? AND target_key = ?`),
		eventType, int64(clientID),
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestArchiveSetsAndUnarchiveClearsArchivedAt(t *testing.T) {
	f := newFixture(t)
	created := createLifecycleClient(t, f, f.member, "Ada")

	if _, err := f.service.Archive(f.ctx, f.viewer, f.workspaceID, created.ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer archive error = %v, want ErrForbidden", err)
	}
	archived, err := f.service.Archive(f.ctx, f.member, f.workspaceID, created.ID, identity.AuditContext{})
	if err != nil || archived.ArchivedAt == nil || archived.Version != created.Version+1 {
		t.Fatalf("Archive() = %+v, %v; want ArchivedAt set and version %d", archived, err, created.Version+1)
	}
	if got, err := f.service.Get(f.ctx, f.viewer, f.workspaceID, created.ID); err != nil || got.ArchivedAt == nil {
		t.Fatalf("Get() of an archived client = %+v, %v; want it still readable with ArchivedAt", got, err)
	}
	again, err := f.service.Archive(f.ctx, f.member, f.workspaceID, created.ID, identity.AuditContext{})
	if err != nil || again.Version != archived.Version {
		t.Fatalf("repeat Archive() = %+v, %v; want no change", again, err)
	}

	if _, err := f.service.Unarchive(f.ctx, f.viewer, f.workspaceID, created.ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer unarchive error = %v, want ErrForbidden", err)
	}
	restored, err := f.service.Unarchive(f.ctx, f.member, f.workspaceID, created.ID, identity.AuditContext{})
	if err != nil || restored.ArchivedAt != nil || restored.Version != archived.Version+1 {
		t.Fatalf("Unarchive() = %+v, %v; want ArchivedAt cleared and version %d", restored, err, archived.Version+1)
	}
	if again, err := f.service.Unarchive(f.ctx, f.member, f.workspaceID, created.ID, identity.AuditContext{}); err != nil || again.Version != restored.Version {
		t.Fatalf("repeat Unarchive() = %+v, %v; want no change", again, err)
	}

	if got := countAudit(t, f, "client.archived", created.ID); got != 1 {
		t.Fatalf("client.archived audit events = %d, want 1", got)
	}
	if got := countAudit(t, f, "client.unarchived", created.ID); got != 1 {
		t.Fatalf("client.unarchived audit events = %d, want 1", got)
	}
	want := []string{"created", "archived", "unarchived"}
	if got := clientActions(t, f, created.ID); !slices.Equal(got, want) {
		t.Fatalf("change feed actions = %v, want %v", got, want)
	}
}

func TestArchivedFilterSelectsTheListStates(t *testing.T) {
	f := newFixture(t)
	ada := createLifecycleClient(t, f, f.member, "Ada")
	createLifecycleClient(t, f, f.member, "Bea")
	if _, err := f.service.Archive(f.ctx, f.member, f.workspaceID, ada.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}

	names := func(archived string) []string {
		t.Helper()
		page, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 10, Archived: archived})
		if err != nil {
			t.Fatalf("List(%q) error = %v", archived, err)
		}
		var got []string
		for _, client := range page.Clients {
			got = append(got, client.DisplayName)
		}
		return got
	}
	if got := names(""); !slices.Equal(got, []string{"Bea"}) {
		t.Fatalf("default list = %v, want active only", got)
	}
	if got := names(ArchivedActive); !slices.Equal(got, []string{"Bea"}) {
		t.Fatalf("active list = %v, want active only", got)
	}
	if got := names(ArchivedOnly); !slices.Equal(got, []string{"Ada"}) {
		t.Fatalf("archived list = %v, want archived only", got)
	}
	if got := names(ArchivedAll); !slices.Equal(got, []string{"Bea", "Ada"}) {
		t.Fatalf("all list = %v, want both, newest first", got)
	}
	if _, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 10, Archived: "bogus"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown archived value error = %v, want ErrInvalidInput", err)
	}

	page, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 1, Archived: ArchivedAll})
	if err != nil || page.NextCursor == "" {
		t.Fatalf("first page = %+v, %v; want a next cursor", page, err)
	}
	if _, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{
		Limit: 1, Archived: ArchivedActive, Cursor: page.NextCursor,
	}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cursor from another archived selector error = %v, want ErrInvalidCursor", err)
	}
}

func TestDeleteIsOwnerAndAdminOnlyAndRemovesTheClientAndValues(t *testing.T) {
	f := newFixture(t)
	admin := addMember(t, f.ctx, f.db, f.auth, f.owner, f.workspaceID, "admin@example.com", identity.Admin)
	created := createLifecycleClient(t, f, f.member, "Ada")
	field, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Source", Type: FieldTypeText}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, created.ID, created.Version, field.ID, "Referral", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}

	if err := f.service.Delete(f.ctx, f.member, f.workspaceID, created.ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member delete error = %v, want ErrForbidden", err)
	}
	if err := f.service.Delete(f.ctx, f.viewer, f.workspaceID, created.ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer delete error = %v, want ErrForbidden", err)
	}
	if err := f.service.Delete(f.ctx, f.owner, f.workspaceID, 0, identity.AuditContext{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("zero key delete error = %v, want ErrNotFound", err)
	}
	if err := f.service.Delete(f.ctx, admin, f.workspaceID, created.ID, identity.AuditContext{}); err != nil {
		t.Fatalf("admin delete error = %v", err)
	}

	if _, err := f.service.Get(f.ctx, f.viewer, f.workspaceID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}
	var rows int
	if err := f.db.QueryRowContext(f.ctx,
		database.Rebind(f.service.driver, `SELECT COUNT(*) FROM clients WHERE id = ?`), int64(created.ID),
	).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("client rows after delete = %d, %v; want 0", rows, err)
	}
	if err := f.service.Delete(f.ctx, admin, f.workspaceID, created.ID, identity.AuditContext{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete error = %v, want ErrNotFound", err)
	}

	if got := countAudit(t, f, "client.deleted", created.ID); got != 1 {
		t.Fatalf("client.deleted audit events = %d, want 1", got)
	}
	var metadata string
	if err := f.db.QueryRowContext(f.ctx,
		database.Rebind(f.service.driver,
			`SELECT metadata FROM audit_events WHERE event_type = ? AND target_key = ?`),
		"client.deleted", int64(created.ID),
	).Scan(&metadata); err != nil || metadata != "{}" {
		t.Fatalf("client.deleted metadata = %q, %v; want no values", metadata, err)
	}
	want := []string{"created", "deleted"}
	if got := clientActions(t, f, created.ID); !slices.Equal(got, want) {
		t.Fatalf("change feed actions = %v, want %v", got, want)
	}
}
