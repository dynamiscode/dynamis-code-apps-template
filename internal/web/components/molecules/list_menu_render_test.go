package molecules

import (
	"strings"
	"testing"

	"github.com/a-h/templ"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/web/components/atoms"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func TestClosedListMenusPreserveDropdownHTML(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	data := viewmodel.PageData{Catalog: catalog, Locale: "en"}
	const wide = "bg-base-100 rounded-box shadow-lg border border-base-200 p-3 w-96 max-w-[90vw]"
	const narrow = "bg-base-100 rounded-box shadow-lg border border-base-200 p-3 w-72 max-w-[90vw]"
	rowLabel := data.T("ui.row_actions")
	ctx := templ.WithChildren(t.Context(), ListFilterIcon())
	for _, tc := range []struct {
		name     string
		menu     templ.Component
		original atoms.DropdownProps
	}{
		{"add", ListAddMenu("Filters", true, true), atoms.DropdownProps{
			SummaryClass: "btn btn-xs", Summary: listAddSummary("Filters", true), Open: true,
			SummaryAttrs: templ.Attributes{"aria-label": "Filters", "title": "Filters"}, PanelClass: wide,
		}},
		{"filter", ListFilterMenu("Filters", false), atoms.DropdownProps{Label: "Filters", SummaryClass: "btn btn-sm", PanelClass: wide}},
		{"row", ListRowMenu(data), atoms.DropdownProps{
			SummaryClass: "btn btn-xs", Summary: RowMenuGlyph(false),
			SummaryAttrs: templ.Attributes{"aria-label": rowLabel, "title": rowLabel}, PanelClass: narrow,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.original.Class, tc.original.PanelChildren = "list-menu", true
			var got, want strings.Builder
			if err := tc.menu.Render(ctx, &got); err != nil {
				t.Fatal(err)
			}
			if err := atoms.Dropdown(tc.original).Render(ctx, &want); err != nil {
				t.Fatal(err)
			}
			if got.String() != want.String() {
				t.Errorf("menu output differs\ngot  %q\nwant %q", got.String(), want.String())
			}
		})
	}
}
