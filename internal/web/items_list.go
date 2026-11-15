package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/listprefs"
)

type itemListQuery struct {
	Search string
	Status string
	Sort   string
	Limit  int
}

func itemListColumns() []clientListColumn {
	return []clientListColumn{
		{Key: "title", LabelKey: "items.title", ValueType: "text", DefaultVisible: true},
		{Key: "status", LabelKey: "items.status", ValueType: "text", DefaultVisible: true},
		{Key: "created_at", LabelKey: "clients.created_at", ValueType: "timestamp"},
		{Key: "updated_at", LabelKey: "clients.updated_at", ValueType: "timestamp"},
	}
}

func parseItemListQuery(query url.Values, defaultLimit int) (itemListQuery, bool) {
	form := itemListQuery{Search: query.Get("search"), Status: query.Get("status"), Sort: query.Get("sort")}
	for _, key := range []string{"search", "status", "sort", "limit", "cursor"} {
		if len(query[key]) > 1 {
			return form, false
		}
	}
	for key := range query {
		switch key {
		case "search", "status", "sort", "limit", "cursor":
		default:
			return form, false
		}
	}
	if form.Status != "" && form.Status != string(items.Active) && form.Status != string(items.Complete) {
		return form, false
	}
	if form.Sort == "" {
		form.Sort = "-created_at"
	}
	if form.Sort != "created_at" && form.Sort != "-created_at" {
		return form, false
	}
	form.Limit = defaultLimit
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return form, false
		}
		form.Limit = limit
	}
	return form, true
}

func itemSortHref(path string, query url.Values, sort string) string {
	copy := cloneClientQuery(query)
	copy.Del("cursor")
	current := query.Get("sort")
	if current == "" {
		current = "-created_at"
	}
	if current == sort {
		copy.Set("sort", "-created_at")
	} else {
		copy.Set("sort", sort)
	}
	return path + "?" + copy.Encode()
}

func itemListRows(page []items.Item) []clientListRow {
	rows := make([]clientListRow, 0, len(page))
	for index, item := range page {
		rows = append(rows, clientListRow{
			ID: item.ID.String(), Item: item, ActionContext: item.Title, ActionPosition: index + 1,
			Values: map[string]string{
				"title": item.Title, "status": string(item.Status),
				"created_at": item.CreatedAt.Format(time.RFC3339),
				"updated_at": item.UpdatedAt.Format(time.RFC3339),
			},
		})
	}
	return rows
}

func (h *Handler) setItemColumn(writer http.ResponseWriter, request *http.Request) {
	if h.listPreferences == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.clientPreferencePrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	defaults := make(map[string]bool)
	for _, column := range itemListColumns() {
		defaults[column.Key] = column.DefaultVisible
	}
	visible, err := strconv.ParseBool(request.FormValue("visible"))
	if err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	if err := h.listPreferences.Set(request.Context(), principal, workspaceID, listprefs.ListItems,
		request.PathValue("column"), visible, defaults); err != nil {
		h.itemPreferenceError(writer, err)
		return
	}
	h.redirect(writer, request, h.itemsPath(workspaceID)+clientQuerySuffix(request.URL.Query()))
}

func (h *Handler) resetItemColumns(writer http.ResponseWriter, request *http.Request) {
	if h.listPreferences == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.clientPreferencePrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	if err := h.listPreferences.Reset(request.Context(), principal, workspaceID, listprefs.ListItems); err != nil {
		h.itemPreferenceError(writer, err)
		return
	}
	h.redirect(writer, request, h.itemsPath(workspaceID)+clientQuerySuffix(request.URL.Query()))
}

func (h *Handler) itemPreferenceError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, listprefs.ErrInvalidInput):
		h.renderError(writer, http.StatusBadRequest)
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(writer, http.StatusForbidden)
	default:
		h.renderError(writer, http.StatusInternalServerError)
	}
}

func (h *Handler) itemsPath(workspaceID identity.WorkspaceID) string {
	return "/workspaces/" + workspaceID.String() + "/items"
}

func itemClearFiltersPath(path string, query url.Values) string {
	copy := cloneClientQuery(query)
	for _, key := range []string{"search", "status", "cursor"} {
		copy.Del(key)
	}
	if values := copy.Encode(); values != "" {
		return path + "?" + values
	}
	return path
}

func itemFilterChips(form itemListQuery, query url.Values, path string) []clientFilterChip {
	var chips []clientFilterChip
	for _, filter := range []struct{ key, value, label string }{
		{"search", form.Search, "items.search"}, {"status", form.Status, "items.status"},
	} {
		if filter.value == "" {
			continue
		}
		copy := cloneClientQuery(query)
		copy.Del(filter.key)
		copy.Del("cursor")
		remove := path
		if values := copy.Encode(); values != "" {
			remove += "?" + values
		}
		chips = append(chips, clientFilterChip{
			FieldLabelKey: filter.label, OperatorLabelKey: "clients.filter_equals",
			Value: filter.value, RemoveHref: remove,
		})
	}
	return chips
}

func itemSortDirection(sort string) string {
	if strings.HasPrefix(sort, "-") {
		return "descending"
	}
	return "ascending"
}
