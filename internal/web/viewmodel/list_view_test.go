package viewmodel

import (
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
)

func TestClientListCellValueShowsTypedCells(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	page := PageData{Catalog: catalog, Locale: "en"}
	at := time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC)
	row := ListRow{ID: "row-1", Cells: map[string]ListCell{
		"when":   TimeCell(at),
		"empty":  TimeCell(time.Time{}),
		"yes":    YesNoCell(true),
		"no":     YesNoCell(false),
		"role":   RoleCell(identity.Owner),
		"active": StatusCell(items.Active),
		"done":   StatusCell(items.Complete),
		"other":  StatusCell("archived"),
		"text":   TextCell("Ada"),
	}}
	cases := map[string]string{
		"when":   page.DateTime(at),
		"empty":  "",
		"yes":    page.T("clients.yes"),
		"no":     page.T("clients.no"),
		"role":   page.Role(identity.Owner),
		"active": page.T("items.active"),
		"done":   page.T("items.complete"),
		"other":  "archived",
		"text":   "Ada",
		"id":     "row-1",
	}
	for key, want := range cases {
		column := ListColumn{Key: key}
		if got := page.ClientListCellValue(column, row); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestListRowCellTextIsEmptyForMissingCell(t *testing.T) {
	row := ListRow{Cells: map[string]ListCell{"role": RoleCell(identity.Admin)}}
	if got := row.CellText("role"); got != string(identity.Admin) {
		t.Fatalf("CellText(role) = %q", got)
	}
	if got := row.CellText("missing"); got != "" {
		t.Fatalf("CellText(missing) = %q, want empty", got)
	}
}
