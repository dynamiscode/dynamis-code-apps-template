package clients

import (
	"errors"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestOwnerAndAdminDefineTextFieldsMembersAndViewersAreRefused(t *testing.T) {
	f := newFixture(t)
	admin := addMember(t, f.ctx, f.db, f.auth, f.owner, f.workspaceID, "admin@example.com", identity.Admin)

	field, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "  Referral source  ", Type: FieldTypeText}, identity.AuditContext{})
	if err != nil {
		t.Fatalf("owner DefineField() error = %v", err)
	}
	if field.Label != "Referral source" || field.Type != FieldTypeText || field.ArchivedAt != nil || field.ID == 0 {
		t.Fatalf("DefineField() = %+v", field)
	}
	if _, err := f.service.DefineField(f.ctx, admin, f.workspaceID, FieldInput{Label: "Admin field", Type: FieldTypeText}, identity.AuditContext{}); err != nil {
		t.Fatalf("admin DefineField() error = %v", err)
	}
	for name, actor := range map[string]identity.Principal{"member": f.member, "viewer": f.viewer} {
		if _, err := f.service.DefineField(f.ctx, actor, f.workspaceID, FieldInput{Label: "Blocked", Type: FieldTypeText}, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
			t.Fatalf("%s DefineField() error = %v, want ErrForbidden", name, err)
		}
	}
}

func TestDefinedFieldsTakeDistinctSlotsInOrder(t *testing.T) {
	f := newFixture(t)
	var ids []FieldID
	for _, label := range []string{"First", "Second", "Third"} {
		field, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: label, Type: FieldTypeText}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, field.ID)
	}
	want := []int{1, 2, 3}
	for i, id := range ids {
		var slot int
		if err := f.db.QueryRowContext(f.ctx, "SELECT slot FROM client_fields WHERE id = ?", int64(id)).Scan(&slot); err != nil {
			t.Fatal(err)
		}
		if slot != want[i] {
			t.Fatalf("field %d slot = %d, want %d", i, slot, want[i])
		}
	}
}

func TestDefineFieldRefusesBlankLongAndUnsupportedTypes(t *testing.T) {
	f := newFixture(t)
	cases := map[string]FieldInput{
		"blank label":      {Label: "   ", Type: FieldTypeText},
		"long label":       {Label: strings.Repeat("x", 101), Type: FieldTypeText},
		"unsupported type": {Label: "Age", Type: "single_choice"},
	}
	for name, input := range cases {
		if _, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, input, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s: DefineField() error = %v, want ErrInvalidInput", name, err)
		}
	}
	if _, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "  ", Type: FieldTypeText}, identity.AuditContext{}); !errors.Is(err, ErrFieldLabelRequired) {
		t.Fatalf("blank label error = %v, want ErrFieldLabelRequired", err)
	}
}

func TestDefineFieldStopsAtConfiguredLimitCountingArchivedFields(t *testing.T) {
	f := newFixture(t)
	limited := NewService(f.db, config.SQLite, f.auth, 2)
	first, err := limited.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "One", Type: FieldTypeText}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := limited.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Two", Type: FieldTypeText}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := limited.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Three", Type: FieldTypeText}, identity.AuditContext{}); !errors.Is(err, ErrLimit) {
		t.Fatalf("third DefineField() error = %v, want ErrLimit", err)
	}
	if _, err := limited.ArchiveField(f.ctx, f.owner, f.workspaceID, first.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := limited.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Three", Type: FieldTypeText}, identity.AuditContext{}); !errors.Is(err, ErrLimit) {
		t.Fatalf("archived fields must count: DefineField() error = %v, want ErrLimit", err)
	}
}

func TestArchiveHidesFieldFromFormsKeepsValuesAndRestoreBringsItBack(t *testing.T) {
	f := newFixture(t)
	field, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Source", Type: FieldTypeText}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Ada"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(f.ctx, `UPDATE clients SET custom_values = ? WHERE id = ?`, `{"f01":"Friend"}`, int64(client.ID)); err != nil {
		t.Fatal(err)
	}

	if _, err := f.service.ArchiveField(f.ctx, f.member, f.workspaceID, field.ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member ArchiveField() error = %v, want ErrForbidden", err)
	}
	archived, err := f.service.ArchiveField(f.ctx, f.owner, f.workspaceID, field.ID, identity.AuditContext{})
	if err != nil {
		t.Fatalf("ArchiveField() error = %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatalf("ArchiveField() left ArchivedAt nil: %+v", archived)
	}
	forms, err := f.service.ListFields(f.ctx, f.member, f.workspaceID, ListFieldsInput{})
	if err != nil || len(forms) != 0 {
		t.Fatalf("form fields after archive = %+v, %v; want none", forms, err)
	}
	all, err := f.service.ListFields(f.ctx, f.owner, f.workspaceID, ListFieldsInput{IncludeArchived: true})
	if err != nil || len(all) != 1 || all[0].ArchivedAt == nil {
		t.Fatalf("management fields after archive = %+v, %v", all, err)
	}
	var values string
	if err := f.db.QueryRowContext(f.ctx, `SELECT custom_values FROM clients WHERE id = ?`, int64(client.ID)).Scan(&values); err != nil || values != `{"f01":"Friend"}` {
		t.Fatalf("custom_values after archive = %q, %v; want untouched", values, err)
	}

	restored, err := f.service.RestoreField(f.ctx, f.owner, f.workspaceID, field.ID, identity.AuditContext{})
	if err != nil || restored.ArchivedAt != nil {
		t.Fatalf("RestoreField() = %+v, %v", restored, err)
	}
	forms, err = f.service.ListFields(f.ctx, f.member, f.workspaceID, ListFieldsInput{})
	if err != nil || len(forms) != 1 || forms[0].ID != field.ID {
		t.Fatalf("form fields after restore = %+v, %v", forms, err)
	}
}

func TestFieldWritesAreAudited(t *testing.T) {
	f := newFixture(t)
	field, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Audited", Type: FieldTypeText}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ArchiveField(f.ctx, f.owner, f.workspaceID, field.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RestoreField(f.ctx, f.owner, f.workspaceID, field.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	for _, eventType := range []string{"client_field.created", "client_field.archived", "client_field.restored"} {
		var events int
		if err := f.db.QueryRowContext(f.ctx,
			`SELECT COUNT(*) FROM audit_events WHERE event_type = ? AND workspace_id = ? AND target_type = ? AND target_key = ?`,
			eventType, f.workspaceID, "client_field", int64(field.ID),
		).Scan(&events); err != nil || events != 1 {
			t.Fatalf("%s audit events = %d, %v; want 1", eventType, events, err)
		}
	}
}
