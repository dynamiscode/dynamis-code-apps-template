package web

import (
	"net/url"

	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// clientScopeHref is the list's address with the given scope: "archived" for archived clients, "active" for the
// default list. The list's search, filters, sort and columns are kept, and the page starts at the first rows.
func clientScopeHref(data viewmodel.PageData, scope string) string {
	query := url.Values{}
	for _, field := range data.ListPreservedQuery {
		if field.Name != "archived" && field.Name != "page" {
			query.Add(field.Name, field.Value)
		}
	}
	if scope != "" && scope != "active" {
		query.Set("archived", scope)
	}
	if encoded := query.Encode(); encoded != "" {
		return data.CurrentPath + "?" + encoded
	}
	return data.CurrentPath
}
