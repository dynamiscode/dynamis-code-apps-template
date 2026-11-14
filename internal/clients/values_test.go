package clients

import (
	"errors"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func defineTextField(t *testing.T, f fixture, label string) Field {
	t.Helper()
	field, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: label, Type: FieldTypeText}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return field
}

func createClientForValues(t *testing.T, f fixture, name string) Client {
	t.Helper()
	client, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: name}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func valueOf(t *testing.T, f fixture, actor identity.Principal, clientID ClientID, fieldID FieldID) (string, bool) {
	t.Helper()
	values, err := f.service.ListValues(f.ctx, actor, f.workspaceID, clientID)
	if err != nil {
		t.Fatalf("ListValues() error = %v", err)
	}
	for _, value := range values {
		if value.FieldID == fieldID {
			if value.Value == nil {
				return "", false
			}
			return *value.Value, true
		}
	}
	t.Fatalf("ListValues() has no active field %s", fieldID)
	return "", false
}

func TestMemberSetsAndClearsTextValueAndViewerReadsIt(t *testing.T) {
	f := newFixture(t)
	field := defineTextField(t, f, "Source")
	client := createClientForValues(t, f, "Ada")

	set, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, "  Friend  ", identity.AuditContext{})
	if err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}
	if set.Version != client.Version+1 {
		t.Fatalf("SetValue() version = %d, want %d", set.Version, client.Version+1)
	}
	if got, ok := valueOf(t, f, f.viewer, client.ID, field.ID); !ok || got != "Friend" {
		t.Fatalf("viewer sees value %q (set=%v), want Friend", got, ok)
	}

	cleared, err := f.service.ClearValue(f.ctx, f.member, f.workspaceID, client.ID, set.Version, field.ID, identity.AuditContext{})
	if err != nil {
		t.Fatalf("ClearValue() error = %v", err)
	}
	if cleared.Version != set.Version+1 {
		t.Fatalf("ClearValue() version = %d, want %d", cleared.Version, set.Version+1)
	}
	if _, ok := valueOf(t, f, f.viewer, client.ID, field.ID); ok {
		t.Fatal("ClearValue() left a value behind")
	}
}

func TestViewerCannotWriteValues(t *testing.T) {
	f := newFixture(t)
	field := defineTextField(t, f, "Source")
	client := createClientForValues(t, f, "Ada")
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, "Friend", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SetValue(f.ctx, f.viewer, f.workspaceID, client.ID, client.Version+1, field.ID, "Blocked", identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer SetValue() error = %v, want ErrForbidden", err)
	}
	if _, err := f.service.ClearValue(f.ctx, f.viewer, f.workspaceID, client.ID, client.Version+1, field.ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer ClearValue() error = %v, want ErrForbidden", err)
	}
	if got, _ := valueOf(t, f, f.viewer, client.ID, field.ID); got != "Friend" {
		t.Fatalf("value after refused writes = %q, want Friend", got)
	}
}

func TestValueWritesRequireCurrentClientVersion(t *testing.T) {
	f := newFixture(t)
	field := defineTextField(t, f, "Source")
	client := createClientForValues(t, f, "Ada")
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version+5, field.ID, "Stale", identity.AuditContext{}); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("stale SetValue() error = %v, want ErrPreconditionFailed", err)
	}
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, 0, field.ID, "Zero", identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero-version SetValue() error = %v, want ErrInvalidInput", err)
	}
}

func TestTextValuesAreValidatedAsText(t *testing.T) {
	f := newFixture(t)
	field := defineTextField(t, f, "Source")
	client := createClientForValues(t, f, "Ada")
	cases := map[string]string{
		"blank":       "   ",
		"too long":    strings.Repeat("x", maxFieldValueRunes+1),
		"invalid utf": string([]byte{0xff, 'a'}),
	}
	for name, value := range cases {
		if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, value, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s: SetValue() error = %v, want ErrInvalidInput", name, err)
		}
	}
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, "   ", identity.AuditContext{}); !errors.Is(err, ErrValueRequired) {
		t.Fatalf("blank SetValue() error = %v, want ErrValueRequired", err)
	}
	if _, ok := valueOf(t, f, f.viewer, client.ID, field.ID); ok {
		t.Fatal("refused values were stored")
	}
}

func TestValuesRefuseArchivedAndUnknownFields(t *testing.T) {
	f := newFixture(t)
	field := defineTextField(t, f, "Source")
	client := createClientForValues(t, f, "Ada")
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, "Friend", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ArchiveField(f.ctx, f.owner, f.workspaceID, field.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	version := client.Version + 1
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, version, field.ID, "Again", identity.AuditContext{}); !errors.Is(err, ErrFieldArchived) {
		t.Fatalf("archived SetValue() error = %v, want ErrFieldArchived", err)
	}
	if _, err := f.service.ClearValue(f.ctx, f.member, f.workspaceID, client.ID, version, field.ID, identity.AuditContext{}); !errors.Is(err, ErrFieldArchived) {
		t.Fatalf("archived ClearValue() error = %v, want ErrFieldArchived", err)
	}
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, version, 999999, "Ghost", identity.AuditContext{}); !errors.Is(err, ErrFieldNotFound) {
		t.Fatalf("unknown field SetValue() error = %v, want ErrFieldNotFound", err)
	}
	if values, err := f.service.ListValues(f.ctx, f.viewer, f.workspaceID, client.ID); err != nil || len(values) != 0 {
		t.Fatalf("ListValues() after archive = %+v, %v; want no active fields", values, err)
	}
}

func TestFieldTypeLocksOnceAnyClientHoldsAValue(t *testing.T) {
	f := newFixture(t)
	field := defineTextField(t, f, "Source")
	client := createClientForValues(t, f, "Ada")

	if err := f.service.setFieldType(f.ctx, f.owner, f.workspaceID, field.ID, "multi_choice", identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unlocked change to unsupported type error = %v, want ErrInvalidInput", err)
	}
	if err := f.service.setFieldType(f.ctx, f.member, f.workspaceID, field.ID, "number", identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member setFieldType() error = %v, want ErrForbidden", err)
	}

	set, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, "Friend", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.setFieldType(f.ctx, f.owner, f.workspaceID, field.ID, "number", identity.AuditContext{}); !errors.Is(err, ErrFieldTypeLocked) {
		t.Fatalf("locked change error = %v, want ErrFieldTypeLocked", err)
	}

	if _, err := f.service.ClearValue(f.ctx, f.member, f.workspaceID, client.ID, set.Version, field.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.setFieldType(f.ctx, f.owner, f.workspaceID, field.ID, FieldTypeDate, identity.AuditContext{}); err != nil {
		t.Fatalf("change after clearing error = %v, want nil (lock lifted)", err)
	}
}

func TestValueWritesAreAuditedWithoutTheValue(t *testing.T) {
	f := newFixture(t)
	field := defineTextField(t, f, "Source")
	client := createClientForValues(t, f, "Ada")
	set, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, "Secret friend", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ClearValue(f.ctx, f.member, f.workspaceID, client.ID, set.Version, field.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"client.value.set", "client.value.clear"} {
		var events int
		if err := f.db.QueryRowContext(f.ctx,
			`SELECT COUNT(*) FROM audit_events WHERE action = ? AND workspace_id = ? AND target_type = ? AND target_key = ?`,
			action, f.workspaceID, "client", int64(client.ID),
		).Scan(&events); err != nil || events != 1 {
			t.Fatalf("%s audit events = %d, %v; want 1", action, events, err)
		}
	}
	var leaked int
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT COUNT(*) FROM audit_events WHERE metadata LIKE ?`, "%Secret friend%",
	).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("audit metadata contains the value: count=%d, err=%v", leaked, err)
	}
}
