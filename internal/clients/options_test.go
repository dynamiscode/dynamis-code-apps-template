package clients

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func defineChoiceField(t *testing.T, f fixture, label string) Field {
	t.Helper()
	field, err := f.service.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: label, Type: FieldTypeSingleChoice}, identity.AuditContext{})
	if err != nil {
		t.Fatalf("DefineField(single_choice) error = %v", err)
	}
	return field
}

func addOptionOK(t *testing.T, f fixture, field Field, label string) Option {
	t.Helper()
	option, err := f.service.AddOption(f.ctx, f.owner, f.workspaceID, field.ID, label, identity.AuditContext{})
	if err != nil {
		t.Fatalf("AddOption(%q) error = %v", label, err)
	}
	return option
}

func TestOwnerAddsOptionsMembersChooseViewersRead(t *testing.T) {
	f := newFixture(t)
	field := defineChoiceField(t, f, "Tier")
	gold := addOptionOK(t, f, field, "Gold")
	silver := addOptionOK(t, f, field, "Silver")

	if _, err := f.service.AddOption(f.ctx, f.member, f.workspaceID, field.ID, "Blocked", identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member AddOption() error = %v, want ErrForbidden", err)
	}
	if err := f.service.DeleteOption(f.ctx, f.member, f.workspaceID, field.ID, gold.ID, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member DeleteOption() error = %v, want ErrForbidden", err)
	}
	options, err := f.service.ListOptions(f.ctx, f.viewer, f.workspaceID, field.ID)
	if err != nil || len(options) != 2 || options[0].ID != gold.ID || options[1].ID != silver.ID || options[1].Label != "Silver" {
		t.Fatalf("viewer ListOptions() = %+v, %v; want Gold then Silver", options, err)
	}

	client := createClientForValues(t, f, "Ada")
	set, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, gold.ID.String(), identity.AuditContext{})
	if err != nil {
		t.Fatalf("member SetValue(option) error = %v", err)
	}
	if got, ok := valueOf(t, f, f.viewer, client.ID, field.ID); !ok || got != gold.ID.String() {
		t.Fatalf("viewer sees value %q (set=%v), want the Gold option ID", got, ok)
	}
	if _, err := f.service.SetValue(f.ctx, f.viewer, f.workspaceID, client.ID, set.Version, field.ID, silver.ID.String(), identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer SetValue() error = %v, want ErrForbidden", err)
	}
	if err := f.service.setFieldType(f.ctx, f.owner, f.workspaceID, field.ID, FieldTypeText, identity.AuditContext{}); !errors.Is(err, ErrFieldTypeLocked) {
		t.Fatalf("setFieldType() with a stored choice error = %v, want ErrFieldTypeLocked", err)
	}
}

func TestOptionLimitIsOneHundredPerField(t *testing.T) {
	f := newFixture(t)
	field := defineChoiceField(t, f, "Tier")
	for i := 1; i <= maxFieldOptions; i++ {
		addOptionOK(t, f, field, fmt.Sprintf("Option %03d", i))
	}
	if _, err := f.service.AddOption(f.ctx, f.owner, f.workspaceID, field.ID, "Over", identity.AuditContext{}); !errors.Is(err, ErrOptionLimit) {
		t.Fatalf("AddOption() over the limit error = %v, want ErrOptionLimit", err)
	}
	other := defineChoiceField(t, f, "Other")
	addOptionOK(t, f, other, "Allowed")
}

func TestOptionRefusesBlankLongNonChoiceArchivedAndUnknownFields(t *testing.T) {
	f := newFixture(t)
	choice := defineChoiceField(t, f, "Tier")
	text := defineTextField(t, f, "Source")
	cases := []struct {
		name    string
		fieldID FieldID
		label   string
		want    error
	}{
		{"blank label", choice.ID, "   ", ErrOptionLabelRequired},
		{"long label", choice.ID, strings.Repeat("x", maxOptionLabelRunes+1), ErrInvalidInput},
		{"text field", text.ID, "Gold", ErrNotChoiceField},
		{"unknown field", 999999, "Gold", ErrFieldNotFound},
	}
	for _, tc := range cases {
		if _, err := f.service.AddOption(f.ctx, f.owner, f.workspaceID, tc.fieldID, tc.label, identity.AuditContext{}); !errors.Is(err, tc.want) {
			t.Fatalf("%s: AddOption() error = %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, err := f.service.ArchiveField(f.ctx, f.owner, f.workspaceID, choice.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AddOption(f.ctx, f.owner, f.workspaceID, choice.ID, "Gold", identity.AuditContext{}); !errors.Is(err, ErrFieldArchived) {
		t.Fatalf("archived AddOption() error = %v, want ErrFieldArchived", err)
	}
}

func TestValueMustReferenceAnOptionOfItsOwnField(t *testing.T) {
	f := newFixture(t)
	field := defineChoiceField(t, f, "Tier")
	other := defineChoiceField(t, f, "Other")
	gold := addOptionOK(t, f, field, "Gold")
	elsewhere := addOptionOK(t, f, other, "Elsewhere")
	client := createClientForValues(t, f, "Ada")

	refused := map[string]string{
		"integer key":       strconv.FormatInt(int64(gold.ID), 10),
		"option of another": elsewhere.ID.String(),
		"label text":        "Gold",
		"client short ID":   ClientID(1).String(),
		"unknown option":    OptionID(999999).String(),
	}
	for name, value := range refused {
		if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, value, identity.AuditContext{}); !errors.Is(err, ErrInvalidOption) {
			t.Fatalf("%s: SetValue() error = %v, want ErrInvalidOption", name, err)
		}
	}
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, "   ", identity.AuditContext{}); !errors.Is(err, ErrValueRequired) {
		t.Fatalf("blank SetValue() error = %v, want ErrValueRequired", err)
	}
	if _, ok := valueOf(t, f, f.viewer, client.ID, field.ID); ok {
		t.Fatal("refused option values were stored")
	}
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, gold.ID.String(), identity.AuditContext{}); err != nil {
		t.Fatalf("own option SetValue() error = %v", err)
	}
}

func TestDeletingOptionClearsItFromEveryClientAndStopsChoice(t *testing.T) {
	f := newFixture(t)
	field := defineChoiceField(t, f, "Tier")
	gold := addOptionOK(t, f, field, "Gold")
	silver := addOptionOK(t, f, field, "Silver")
	ada := createClientForValues(t, f, "Ada")
	ben := createClientForValues(t, f, "Ben")
	cy := createClientForValues(t, f, "Cy")
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, ada.ID, ada.Version, field.ID, gold.ID.String(), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, ben.ID, ben.Version, field.ID, gold.ID.String(), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, cy.ID, cy.Version, field.ID, silver.ID.String(), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}

	if err := f.service.DeleteOption(f.ctx, f.owner, f.workspaceID, field.ID, gold.ID, identity.AuditContext{}); err != nil {
		t.Fatalf("DeleteOption() error = %v", err)
	}
	for _, client := range []Client{ada, ben} {
		if _, ok := valueOf(t, f, f.viewer, client.ID, field.ID); ok {
			t.Fatalf("client %s still holds the deleted option", client.DisplayName)
		}
		if got, err := f.service.Get(f.ctx, f.viewer, f.workspaceID, client.ID); err != nil || got.Version != client.Version+2 {
			t.Fatalf("client %s version = %d, %v; want %d", client.DisplayName, got.Version, err, client.Version+2)
		}
	}
	if got, ok := valueOf(t, f, f.viewer, cy.ID, field.ID); !ok || got != silver.ID.String() {
		t.Fatalf("unrelated client value = %q (set=%v), want Silver", got, ok)
	}
	options, err := f.service.ListOptions(f.ctx, f.viewer, f.workspaceID, field.ID)
	if err != nil || len(options) != 1 || options[0].ID != silver.ID {
		t.Fatalf("ListOptions() after delete = %+v, %v; want Silver only", options, err)
	}
	if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, ada.ID, ada.Version+2, field.ID, gold.ID.String(), identity.AuditContext{}); !errors.Is(err, ErrInvalidOption) {
		t.Fatalf("SetValue() with the deleted option error = %v, want ErrInvalidOption", err)
	}
	if err := f.service.DeleteOption(f.ctx, f.owner, f.workspaceID, field.ID, gold.ID, identity.AuditContext{}); !errors.Is(err, ErrOptionNotFound) {
		t.Fatalf("second DeleteOption() error = %v, want ErrOptionNotFound", err)
	}
	if err := f.service.DeleteOption(f.ctx, f.owner, f.workspaceID, defineChoiceField(t, f, "Other").ID, silver.ID, identity.AuditContext{}); !errors.Is(err, ErrOptionNotFound) {
		t.Fatalf("DeleteOption() under another field error = %v, want ErrOptionNotFound", err)
	}
	if err := f.service.DeleteOption(f.ctx, f.owner, f.workspaceID, field.ID, 0, identity.AuditContext{}); !errors.Is(err, ErrOptionNotFound) {
		t.Fatalf("DeleteOption(0) error = %v, want ErrOptionNotFound", err)
	}
}

func TestOptionWritesAreAudited(t *testing.T) {
	f := newFixture(t)
	field := defineChoiceField(t, f, "Tier")
	gold := addOptionOK(t, f, field, "Gold")
	if err := f.service.DeleteOption(f.ctx, f.owner, f.workspaceID, field.ID, gold.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	for _, eventType := range []string{"client_field_option.created", "client_field_option.deleted"} {
		var events int
		if err := f.db.QueryRowContext(f.ctx,
			`SELECT COUNT(*) FROM audit_events WHERE event_type = ? AND workspace_id = ? AND target_type = ? AND target_key = ?`,
			eventType, f.workspaceID, "client_field_option", int64(gold.ID),
		).Scan(&events); err != nil || events != 1 {
			t.Fatalf("%s audit events = %d, %v; want 1", eventType, events, err)
		}
	}
}
