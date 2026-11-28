package clients

import (
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// typedFieldTypes lists every custom-field type that accepts values.
var typedFieldTypes = []string{FieldTypeText, FieldTypeNumber, FieldTypeDate, FieldTypeYesNo}

// validTypedValues gives one valid input and its stored form for each type.
var validTypedValues = map[string]struct{ input, stored string }{
	FieldTypeText:   {"  Friend  ", "Friend"},
	FieldTypeNumber: {" 12.50 ", "12.5"},
	FieldTypeDate:   {" 2026-02-28 ", "2026-02-28"},
	FieldTypeYesNo:  {" true ", "true"},
}

func defineTypedField(t *testing.T, f fixture, fieldType string) Field {
	t.Helper()
	field, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Field " + fieldType, Type: fieldType}, identity.AuditContext{})
	if err != nil {
		t.Fatalf("DefineField(%s) error = %v", fieldType, err)
	}
	return field
}

func TestNumberValueAcceptsFiniteNumbersOnly(t *testing.T) {
	f := newFixture(t)
	field := defineTypedField(t, f, FieldTypeNumber)
	valid := map[string]string{"12": "12", "-0.5": "-0.5", "1e3": "1000", "0": "0", "-0": "0", ".5": "0.5"}
	for input, want := range valid {
		client := createClientForValues(t, f, "Ada")
		if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, input, identity.AuditContext{}); err != nil {
			t.Fatalf("SetValue(%q) error = %v", input, err)
		}
		if got, ok := valueOf(t, f, f.viewer, client.ID, field.ID); !ok || got != want {
			t.Fatalf("SetValue(%q) stored %q (set=%v), want %q", input, got, ok, want)
		}
	}
	for _, input := range []string{"abc", "NaN", "Inf", "-Inf", "1,000", "0x1p4", "1e400", "12abc", "1.2.3", "1 2"} {
		client := createClientForValues(t, f, "Ada")
		if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, input, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("SetValue(%q) error = %v, want ErrInvalidInput", input, err)
		}
	}
}

func TestDateValueAcceptsCalendarDatesOnly(t *testing.T) {
	f := newFixture(t)
	field := defineTypedField(t, f, FieldTypeDate)
	for input, want := range map[string]string{"2026-02-28": "2026-02-28", "2024-02-29": "2024-02-29", " 2026-03-01 ": "2026-03-01"} {
		client := createClientForValues(t, f, "Ada")
		if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, input, identity.AuditContext{}); err != nil {
			t.Fatalf("SetValue(%q) error = %v", input, err)
		}
		if got, ok := valueOf(t, f, f.viewer, client.ID, field.ID); !ok || got != want {
			t.Fatalf("SetValue(%q) stored %q (set=%v), want %q", input, got, ok, want)
		}
	}
	for _, input := range []string{"2026-02-30", "2025-02-29", "2026-13-01", "2026-00-10", "26-02-28", "2026-2-3", "2026-02-28T10:00:00Z", "28/02/2026", "tomorrow"} {
		client := createClientForValues(t, f, "Ada")
		if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, input, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("SetValue(%q) error = %v, want ErrInvalidInput", input, err)
		}
	}
}

func TestYesNoValueStoresTrueOrFalseOnly(t *testing.T) {
	f := newFixture(t)
	field := defineTypedField(t, f, FieldTypeYesNo)
	for input, want := range map[string]string{"true": "true", "false": "false", " false ": "false"} {
		client := createClientForValues(t, f, "Ada")
		if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, input, identity.AuditContext{}); err != nil {
			t.Fatalf("SetValue(%q) error = %v", input, err)
		}
		if got, ok := valueOf(t, f, f.viewer, client.ID, field.ID); !ok || got != want {
			t.Fatalf("SetValue(%q) stored %q (set=%v), want %q", input, got, ok, want)
		}
	}
	for _, input := range []string{"yes", "no", "1", "0", "True", "TRUE", "on"} {
		client := createClientForValues(t, f, "Ada")
		if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, input, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("SetValue(%q) error = %v, want ErrInvalidInput", input, err)
		}
	}
}

func TestEveryTypeAllowsUnsetValuesAndClears(t *testing.T) {
	f := newFixture(t)
	for _, fieldType := range typedFieldTypes {
		field := defineTypedField(t, f, fieldType)
		client := createClientForValues(t, f, "Ada")
		if _, ok := valueOf(t, f, f.viewer, client.ID, field.ID); ok {
			t.Fatalf("%s: new client has a value for an unset field", fieldType)
		}
		if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, "   ", identity.AuditContext{}); !errors.Is(err, ErrValueRequired) {
			t.Fatalf("%s: blank SetValue() error = %v, want ErrValueRequired", fieldType, err)
		}
		set, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, validTypedValues[fieldType].input, identity.AuditContext{})
		if err != nil {
			t.Fatalf("%s: SetValue() error = %v", fieldType, err)
		}
		if got, ok := valueOf(t, f, f.viewer, client.ID, field.ID); !ok || got != validTypedValues[fieldType].stored {
			t.Fatalf("%s: stored %q (set=%v), want %q", fieldType, got, ok, validTypedValues[fieldType].stored)
		}
		if _, err := f.service.ClearValue(f.ctx, f.member, f.workspaceID, client.ID, set.Version, field.ID, identity.AuditContext{}); err != nil {
			t.Fatalf("%s: ClearValue() error = %v", fieldType, err)
		}
		if _, ok := valueOf(t, f, f.viewer, client.ID, field.ID); ok {
			t.Fatalf("%s: ClearValue() left a value behind", fieldType)
		}
	}
}

func TestTypeLockAppliesToEachType(t *testing.T) {
	f := newFixture(t)
	for _, fieldType := range typedFieldTypes {
		field := defineTypedField(t, f, fieldType)
		client := createClientForValues(t, f, "Ada")
		set, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, validTypedValues[fieldType].input, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		target := FieldTypeText
		if fieldType == FieldTypeText {
			target = FieldTypeDate
		}
		if err := f.service.setFieldType(f.ctx, f.owner, f.workspaceID, field.ID, target, identity.AuditContext{}); !errors.Is(err, ErrFieldTypeLocked) {
			t.Fatalf("%s: change with a stored value error = %v, want ErrFieldTypeLocked", fieldType, err)
		}
		if _, err := f.service.ClearValue(f.ctx, f.member, f.workspaceID, client.ID, set.Version, field.ID, identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
		if err := f.service.setFieldType(f.ctx, f.owner, f.workspaceID, field.ID, target, identity.AuditContext{}); err != nil {
			t.Fatalf("%s: change after clearing error = %v, want nil", fieldType, err)
		}
	}
}
