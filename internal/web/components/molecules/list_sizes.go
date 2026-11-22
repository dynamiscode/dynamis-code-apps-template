package molecules

import (
	"net/url"
	"strconv"

	"github.com/a-h/templ"

	atoms "example.com/dynamis-code/apps-template/internal/web/components/atoms"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// listPageSizeLabel is the dropdown text for one size: "All" for 0, otherwise the number.
func ListPageSizeLabel(data viewmodel.PageData, size int) string {
	if size == 0 {
		return data.T("ui.all")
	}
	return strconv.Itoa(size)
}

// ListPageSizeHref is the list's address showing size rows per page. The list's search, filters, sort, and scope
// are kept, and the page starts at the first rows.
func ListPageSizeHref(data viewmodel.PageData, size int) string {
	query := url.Values{}
	for _, field := range data.ListPreservedQuery {
		if field.Name == "limit" || field.Name == "page" {
			continue
		}
		query.Add(field.Name, field.Value)
	}
	query.Set("limit", strconv.Itoa(size))
	return data.CurrentPath + "?" + query.Encode()
}

// listPageSizeItems is one menu entry per size, with the current one marked.
func listPageSizeItems(data viewmodel.PageData, limit int, sizes []int) []atoms.DropdownItem {
	items := make([]atoms.DropdownItem, 0, len(sizes))
	for _, size := range sizes {
		href := ListPageSizeHref(data, size)
		attrs := templ.Attributes{"href": href}
		if size == limit {
			attrs["aria-current"] = "true"
		}
		for name, value := range ListLiveLinkAttributes(data, href) {
			attrs[name] = value
		}
		items = append(items, atoms.DropdownItem{Label: ListPageSizeLabel(data, size), Attrs: attrs})
	}
	return items
}
