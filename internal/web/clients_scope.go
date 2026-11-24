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

func clientDetailHref(data viewmodel.PageData, id string) string {
	href := data.CurrentPath + "/" + id
	if data.ClientReturnQuery != "" {
		href += "?return=" + url.QueryEscape(data.ClientReturnQuery)
	}
	return href
}

func clientReturnHref(path, raw string) string {
	query, err := url.ParseQuery(raw)
	if err != nil || len(raw) > 4096 {
		return path
	}
	for key := range query {
		switch key {
		case "q", "sort", "order", "archived", "limit", "page", "filter", "match", "filter_field", "filter_op", "filter_value", "filter_value_to", "filter_error", listPartKey, listFullKey:
		default:
			return path
		}
	}
	query = cloneClientQuery(query)
	if encoded := query.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}
