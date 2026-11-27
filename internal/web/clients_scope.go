package web

import (
	"net/url"
	"strconv"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func clientDetailHref(data viewmodel.PageData, id string) string {
	href := data.CurrentPath + "/" + id
	if data.ClientReturnQuery != "" {
		href += "?return=" + url.QueryEscape(data.ClientReturnQuery)
	}
	return href
}

func clientContextURL(path, raw string) string {
	if raw == "" || clientReturnHref("/clients", raw) == "/clients" {
		return path
	}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "return=" + url.QueryEscape(raw)
}

func engagementReturnPath(workspace identity.WorkspaceID, raw string) string {
	if len(raw) > 4096 {
		return ""
	}
	u, err := url.Parse(raw)
	listPath := "/workspaces/" + workspace.String() + "/engagements"
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || u.Opaque != "" || u.Fragment != "" || (u.Path != listPath && u.Path != listPath+"/due") {
		return ""
	}
	return u.RequestURI()
}

func engagementContextURL(path, raw string) string {
	if raw == "" {
		return path
	}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "engagement_return=" + url.QueryEscape(raw)
}

func clientTimelineURL(data viewmodel.PageData, page int) string {
	path := data.CurrentPath + "/" + data.Client.ID.String() + "?timeline_page=" + strconv.Itoa(page)
	return clientContextURL(path, data.ClientReturnQuery)
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
