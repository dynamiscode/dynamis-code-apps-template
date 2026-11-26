package web

import (
	"net/url"

	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

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
