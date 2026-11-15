package clients

import (
	"encoding/json"
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// setValueOK writes a value with the client's current version and returns the
// updated client.
func setValueOK(t *testing.T, f fixture, client Client, fieldID FieldID, value string) Client {
	t.Helper()
	updated, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, fieldID, value, identity.AuditContext{})
	if err != nil {
		t.Fatalf("SetValue(%q) error = %v", value, err)
	}
	return updated
}

func archiveOK(t *testing.T, f fixture, fieldID FieldID) {
	t.Helper()
	if _, err := f.service.ArchiveField(f.ctx, f.owner, f.workspaceID, fieldID, identity.AuditContext{}); err != nil {
		t.Fatalf("ArchiveField() error = %v", err)
	}
}

func fieldSlot(t *testing.T, f fixture, fieldID FieldID) int {
	t.Helper()
	var slot int
	if err := f.db.QueryRowContext(f.ctx, "SELECT slot FROM client_fields WHERE id = ?", int64(fieldID)).Scan(&slot); err != nil {
		t.Fatalf("field slot error = %v", err)
	}
	return slot
}

// storedValues decodes the client's custom_values straight from the row.
func storedValues(t *testing.T, f fixture, clientID ClientID) map[string]string {
	t.Helper()
	var raw string
	if err := f.db.QueryRowContext(f.ctx, "SELECT custom_values FROM clients WHERE id = ?", int64(clientID)).Scan(&raw); err != nil {
		t.Fatalf("custom_values error = %v", err)
	}
	values := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		t.Fatalf("custom_values %q: %v", raw, err)
	}
	return values
}

func TestPermanentDeleteRefusesFieldsThatAreNotArchived(t *testing.T) {
	f := newFixture(t)
	field := defineTextField(t, f, "Source")
	if err := f.service.DeleteField(f.ctx, f.owner, f.workspaceID, field.ID, identity.AuditContext{}); !errors.Is(err, ErrFieldNotArchived) {
		t.Fatalf("DeleteField() on an active field error = %v, want ErrFieldNotArchived", err)
	}
	fields, err := f.service.ListFields(f.ctx, f.owner, f.workspaceID, ListFieldsInput{IncludeArchived: true})
	if err != nil || len(fields) != 1 || fields[0].ID != field.ID {
		t.Fatalf("fields after refused delete = %+v, %v; want the field kept", fields, err)
	}
}

func TestPermanentDeleteIsRefusedForMembersAndViewers(t *testing.T) {
	f := newFixture(t)
	field := defineTextField(t, f, "Source")
	archiveOK(t, f, field.ID)
	for name, actor := range map[string]identity.Principal{"member": f.member, "viewer": f.viewer} {
		if err := f.service.DeleteField(f.ctx, actor, f.workspaceID, field.ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
			t.Fatalf("%s DeleteField() error = %v, want ErrForbidden", name, err)
		}
	}
	fields, err := f.service.ListFields(f.ctx, f.owner, f.workspaceID, ListFieldsInput{IncludeArchived: true})
	if err != nil || len(fields) != 1 {
		t.Fatalf("fields after refused deletes = %+v, %v; want the field kept", fields, err)
	}
}

func TestPermanentDeleteClearsEveryTypeAndRemovesTheField(t *testing.T) {
	cases := []struct {
		name      string
		fieldType string
	}{
		{"text", FieldTypeText},
		{"number", FieldTypeNumber},
		{"date", FieldTypeDate},
		{"yes_no", FieldTypeYesNo},
		{"single_choice", FieldTypeSingleChoice},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			field := defineTypedField(t, f, tc.fieldType)
			slot := fieldSlot(t, f, field.ID)
			value := validTypedValues[tc.fieldType].input
			if tc.fieldType == FieldTypeSingleChoice {
				value = addOptionOK(t, f, field, "Gold").ID.String()
			}
			ada := setValueOK(t, f, createClientForValues(t, f, "Ada"), field.ID, value)
			ben := setValueOK(t, f, createClientForValues(t, f, "Ben"), field.ID, value)
			cy := createClientForValues(t, f, "Cy")
			archiveOK(t, f, field.ID)

			if err := f.service.DeleteField(f.ctx, f.owner, f.workspaceID, field.ID, identity.AuditContext{}); err != nil {
				t.Fatalf("DeleteField() error = %v", err)
			}
			for _, client := range []Client{ada, ben, cy} {
				if _, ok := storedValues(t, f, client.ID)[slotKey(slot)]; ok {
					t.Fatalf("client %s still keeps a value under slot %d", client.DisplayName, slot)
				}
			}
			var fields, options int
			if err := f.db.QueryRowContext(f.ctx, "SELECT COUNT(*) FROM client_fields WHERE id = ?", int64(field.ID)).Scan(&fields); err != nil || fields != 0 {
				t.Fatalf("field rows after delete = %d, %v; want 0", fields, err)
			}
			if err := f.db.QueryRowContext(f.ctx, "SELECT COUNT(*) FROM client_field_options WHERE field_id = ?", int64(field.ID)).Scan(&options); err != nil || options != 0 {
				t.Fatalf("option rows after delete = %d, %v; want 0", options, err)
			}
		})
	}
}

func TestPermanentDeleteRemovesVisibilityOverridesForTheField(t *testing.T) {
	f := newFixture(t)
	field := defineTextField(t, f, "Source")
	columnKey := clientFieldColumnKey(field.ID)
	for _, userID := range []identity.UserID{f.owner.UserID, f.member.UserID} {
		if _, err := f.db.ExecContext(f.ctx, `
			INSERT INTO list_column_preferences (workspace_id, user_id, list_key, column_key, visible)
			VALUES (?, ?, 'clients', ?, TRUE)
		`, f.workspaceID, userID, columnKey); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.db.ExecContext(f.ctx, `
		INSERT INTO list_column_preferences (workspace_id, user_id, list_key, column_key, visible)
		VALUES (?, ?, 'items', ?, TRUE)
	`, f.workspaceID, f.owner.UserID, columnKey); err != nil {
		t.Fatal(err)
	}
	archiveOK(t, f, field.ID)
	if err := f.service.DeleteField(f.ctx, f.owner, f.workspaceID, field.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	var clients, otherList int
	if err := f.db.QueryRowContext(f.ctx, `
		SELECT COUNT(*) FROM list_column_preferences WHERE workspace_id = ? AND list_key = 'clients' AND column_key = ?
	`, f.workspaceID, columnKey).Scan(&clients); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRowContext(f.ctx, `
		SELECT COUNT(*) FROM list_column_preferences WHERE workspace_id = ? AND list_key = 'items' AND column_key = ?
	`, f.workspaceID, columnKey).Scan(&otherList); err != nil {
		t.Fatal(err)
	}
	if clients != 0 || otherList != 1 {
		t.Fatalf("remaining field overrides: clients=%d items=%d; want 0 and 1", clients, otherList)
	}
}

func TestPermanentDeleteBumpsEachClearedClientAndRecordsOneAudit(t *testing.T) {
	f := newFixture(t)
	field := defineTextField(t, f, "Source")
	ada := setValueOK(t, f, createClientForValues(t, f, "Ada"), field.ID, "Friend")
	ben := setValueOK(t, f, createClientForValues(t, f, "Ben"), field.ID, "Colleague")
	cy := createClientForValues(t, f, "Cy")
	archiveOK(t, f, field.ID)

	if err := f.service.DeleteField(f.ctx, f.owner, f.workspaceID, field.ID, identity.AuditContext{}); err != nil {
		t.Fatalf("DeleteField() error = %v", err)
	}
	for _, client := range []Client{ada, ben} {
		got, err := f.service.Get(f.ctx, f.viewer, f.workspaceID, client.ID)
		if err != nil || got.Version != client.Version+1 {
			t.Fatalf("client %s version = %d, %v; want %d", client.DisplayName, got.Version, err, client.Version+1)
		}
	}
	if got, err := f.service.Get(f.ctx, f.viewer, f.workspaceID, cy.ID); err != nil || got.Version != cy.Version {
		t.Fatalf("unrelated client version = %d, %v; want %d", got.Version, err, cy.Version)
	}

	page, err := f.service.Changes(f.ctx, f.viewer, f.workspaceID, changeOrigin, 100)
	if err != nil {
		t.Fatal(err)
	}
	cleared := 0
	for _, change := range page.Changes {
		if change.Action == "updated" && change.ClientVersion == ada.Version+1 && (change.ClientID == ada.ID || change.ClientID == ben.ID) {
			cleared++
		}
	}
	if cleared != 2 {
		t.Fatalf("deletion change entries = %d, want one per cleared client (2)", cleared)
	}

	var events int
	var metadata string
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT COUNT(*), MIN(metadata) FROM audit_events WHERE event_type = ? AND workspace_id = ? AND target_type = ? AND target_key = ?`,
		"client_field.deleted", f.workspaceID, "client_field", int64(field.ID),
	).Scan(&events, &metadata); err != nil || events != 1 || metadata != `{"clearedClients":2}` {
		t.Fatalf("client_field.deleted audit = %d events, metadata %q, %v; want 1 with clearedClients 2", events, metadata, err)
	}
}

func TestFreedSlotIsReusedOnlyAfterTheDeleteCommits(t *testing.T) {
	f := newFixture(t)
	gone := defineTextField(t, f, "Gone")
	ada := setValueOK(t, f, createClientForValues(t, f, "Ada"), gone.ID, "Friend")
	archiveOK(t, f, gone.ID)
	if err := f.service.DeleteField(f.ctx, f.owner, f.workspaceID, gone.ID, identity.AuditContext{}); err != nil {
		t.Fatalf("DeleteField() error = %v", err)
	}

	fresh := defineTextField(t, f, "Fresh")
	if slot := fieldSlot(t, f, fresh.ID); slot != 1 {
		t.Fatalf("field defined after delete took slot %d, want the freed slot 1", slot)
	}
	if _, ok := valueOf(t, f, f.viewer, ada.ID, fresh.ID); ok {
		t.Fatal("field in the freed slot shows a stale value from the deleted field")
	}
}

func TestFailedPermanentDeleteKeepsValuesAndSlot(t *testing.T) {
	f := newFixture(t)
	field := defineTextField(t, f, "Source")
	ada := setValueOK(t, f, createClientForValues(t, f, "Ada"), field.ID, "Friend")
	columnKey := clientFieldColumnKey(field.ID)
	if _, err := f.db.ExecContext(f.ctx, `
		INSERT INTO list_column_preferences (workspace_id, user_id, list_key, column_key, visible)
		VALUES (?, ?, 'clients', ?, TRUE)
	`, f.workspaceID, f.member.UserID, columnKey); err != nil {
		t.Fatal(err)
	}
	archiveOK(t, f, field.ID)
	// The field row delete runs after the values are cleared, so this trigger
	// makes the whole transaction fail after the clearing step.
	if _, err := f.db.ExecContext(f.ctx,
		`CREATE TRIGGER block_field_delete BEFORE DELETE ON client_fields BEGIN SELECT RAISE(ABORT, 'blocked for test'); END`,
	); err != nil {
		t.Fatal(err)
	}

	if err := f.service.DeleteField(f.ctx, f.owner, f.workspaceID, field.ID, identity.AuditContext{}); err == nil {
		t.Fatal("DeleteField() succeeded through a blocking trigger")
	}
	if got := storedValues(t, f, ada.ID)[slotKey(fieldSlot(t, f, field.ID))]; got != "Friend" {
		t.Fatalf("value after rolled-back delete = %q, want Friend", got)
	}
	var preferences int
	if err := f.db.QueryRowContext(f.ctx, `
		SELECT COUNT(*) FROM list_column_preferences WHERE workspace_id = ? AND user_id = ? AND list_key = 'clients' AND column_key = ?
	`, f.workspaceID, f.member.UserID, columnKey).Scan(&preferences); err != nil || preferences != 1 {
		t.Fatalf("preference rows after rolled-back delete = %d, %v; want 1", preferences, err)
	}
	if got, err := f.service.Get(f.ctx, f.viewer, f.workspaceID, ada.ID); err != nil || got.Version != ada.Version {
		t.Fatalf("client version after rolled-back delete = %d, %v; want %d", got.Version, err, ada.Version)
	}
	other := defineTextField(t, f, "Other")
	if slot := fieldSlot(t, f, other.ID); slot != 2 {
		t.Fatalf("slot after rolled-back delete = %d, want 2 because slot 1 is still held", slot)
	}
}
