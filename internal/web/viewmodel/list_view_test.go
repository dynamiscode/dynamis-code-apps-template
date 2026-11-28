package viewmodel

import (
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestClientListCellValueShowsTypedCells(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	page := PageData{Catalog: catalog, Locale: "en"}
	at := time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC)
	row := ListRow{ID: "row-1", Cells: map[string]ListCell{
		"when":  TimeCell(at),
		"empty": TimeCell(time.Time{}),
		"yes":   YesNoCell(true),
		"no":    YesNoCell(false),
		"role":  RoleCell(identity.Owner),
		"text":  TextCell("Ada"),
	}}
	cases := map[string]string{
		"when":  "Nov 20, 2026 9:30 AM",
		"empty": "",
		"yes":   "Yes",
		"no":    "No",
		"role":  "Owner",
		"text":  "Ada",
		"id":    "row-1",
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
