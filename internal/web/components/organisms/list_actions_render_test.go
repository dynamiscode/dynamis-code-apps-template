package organisms

import (
	"strings"
	"testing"

	"github.com/a-h/templ"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/web/components/atoms"
	"example.com/dynamis-code/apps-template/internal/web/components/molecules"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func TestListActionsMenuPreservesDropdownHTML(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	data := viewmodel.PageData{Catalog: catalog, Locale: "en", CSRF: "fixture"}
	open := ListAction{Label: "Open", Href: "/clients", Icon: "open"}
	archive := ListAction{Label: "Archive", PostPath: "/clients/archive", Danger: true, Confirm: "Archive it?"}
	label := data.T("ui.row_actions")
	for _, vertical := range []bool{false, true} {
		original := atoms.Dropdown(atoms.DropdownProps{
			Class: "list-menu", SummaryClass: "btn btn-sm list-row-menu-button",
			SummaryAttrs: templ.Attributes{"aria-label": label, "title": label},
			Summary:      molecules.RowMenuGlyph(vertical), ListClass: "list-action-items bg-base-100 rounded-box shadow-lg",
			Items: []atoms.DropdownItem{{Item: listActionLink(open)}, {Divider: true}, {Item: listActionForm(data, archive)}},
		})
		var got, want strings.Builder
		if err := ListActionsMenu(data, []ListAction{open, archive}, vertical).Render(t.Context(), &got); err != nil {
			t.Fatal(err)
		}
		if err := original.Render(t.Context(), &want); err != nil {
			t.Fatal(err)
		}
		if got.String() != want.String() {
			t.Errorf("vertical=%t menu differs\ngot  %q\nwant %q", vertical, got.String(), want.String())
		}
	}
}
