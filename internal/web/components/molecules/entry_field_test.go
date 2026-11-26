package molecules

import (
	"context"
	"strings"
	"testing"
)

func TestCurrencyPickerKeepsNativeFallbackAndSearchDraft(t *testing.T) {
	var output strings.Builder
	props := CurrencyPickerProps{
		ID: "currency", Name: "currency", Label: "Currency", SearchLabel: "Search currencies",
		Search: "euro", SearchButton: "Search", ActionName: "picker_action", ActionValue: "currency",
		Selected: "USD", Required: true,
		Options: []CurrencyPickerOption{{Code: "USD", Label: "US dollar"}, {Code: "EUR", Label: "Euro"}, {Code: "JPY", Label: "Yen"}},
	}
	if err := CurrencyPicker(props).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{`data-entry-field`, `for="currency"`, `data-entry-picker`, `name="currency_search"`, `value="euro"`, `name="picker_action"`, `value="currency"`, `value="USD" selected`, `value="JPY" hidden`} {
		if !strings.Contains(got, want) {
			t.Errorf("currency picker missing %q: %s", want, got)
		}
	}
}

func TestEntryEditorKeepsDraftOpenAndAccessible(t *testing.T) {
	var output strings.Builder
	props := EntryEditorProps{Label: "Status", EditLabel: "Edit", EmptyText: "Not set", Draft: true}
	if err := EntryEditor(props).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{`data-entry-field`, `<details`, ` open`, `Not set`, `Edit Status`} {
		if !strings.Contains(got, want) {
			t.Errorf("entry editor missing %q: %s", want, got)
		}
	}
}
