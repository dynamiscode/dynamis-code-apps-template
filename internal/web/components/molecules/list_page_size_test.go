package molecules

import (
	"strings"
	"testing"

	"github.com/a-h/templ"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/web/components/atoms"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func TestListPageSizePreservesMenuHTML(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	sizes := []int{10, 20, 0}
	const limit = 20
	for _, live := range []bool{false, true} {
		data := viewmodel.PageData{Catalog: catalog, Locale: "en", CurrentPath: "/clients", ListLive: live}
		var items []atoms.DropdownItem
		for _, size := range sizes {
			items = append(items, atoms.DropdownItem{Label: ListPageSizeLabel(data, size), Attrs: listPageSizeAttrs(data, limit, size)})
		}
		original := atoms.Dropdown(atoms.DropdownProps{
			Class: "list-menu list-page-size", SummaryClass: "btn btn-sm list-page-size-trigger",
			SummaryAttrs: templ.Attributes{"aria-label": data.T("clients.page_size") + ": " + ListPageSizeLabel(data, limit)},
			Label:        ListPageSizeLabel(data, limit), Summary: pageSizeChevron(), Items: items,
			ListClass: "list-page-size-options bg-base-100 rounded-box shadow-lg",
		})
		var got, want strings.Builder
		if err := ListPageSize(data, limit, sizes).Render(t.Context(), &got); err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{
			`aria-label="Rows per page: 20"`,
			`href="/clients?limit=10"`,
			`href="/clients?limit=20"`,
			`href="/clients?limit=0"`,
			`aria-current="true"`,
			`>All</a>`,
		} {
			if !strings.Contains(got.String(), expected) {
				t.Errorf("live=%t menu missing %q: %s", live, expected, got.String())
			}
		}
		if err := original.Render(t.Context(), &want); err != nil {
			t.Fatal(err)
		}
		if got.String() != want.String() {
			gotHTML, wantHTML := got.String(), want.String()
			i := 0
			for i < min(len(gotHTML), len(wantHTML)) && gotHTML[i] == wantHTML[i] {
				i++
			}
			t.Errorf("live=%t menu differs at byte %d: got %q, want %q", live, i, gotHTML[i:min(i+30, len(gotHTML))], wantHTML[i:min(i+30, len(wantHTML))])
		}
	}
}
