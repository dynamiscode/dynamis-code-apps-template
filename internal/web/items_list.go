package web

import (
	"errors"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func itemPageData(data viewmodel.PageData) viewmodel.PageData {
	data.Title = data.T("workspace.items_title", map[string]any{"name": data.Workspace.Name})
	return data
}

func itemListColumns() []viewmodel.ListColumn {
	return []viewmodel.ListColumn{
		{Key: "title", LabelKey: "items.title", DefaultVisible: true},
		{Key: "id", LabelKey: "ui.id"},
		{Key: "status", LabelKey: "items.status", DefaultVisible: true},
		{Key: "created_at", LabelKey: "clients.created_at"},
		{Key: "updated_at", LabelKey: "clients.updated_at"},
	}
}

func itemSortHref(path string, query url.Values, sort string) string {
	copy := cloneClientQuery(query)
	copy.Del("page")
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

func itemListRows(page []items.Item) []viewmodel.ListRow {
	rows := make([]viewmodel.ListRow, 0, len(page))
	for index, item := range page {
		rows = append(rows, viewmodel.ListRow{
			ID: item.ID.String(), Item: item, ActionContext: item.Title, ActionPosition: index + 1,
			Cells: map[string]viewmodel.ListCell{
				"title": viewmodel.TextCell(item.Title), "status": viewmodel.StatusCell(item.Status),
				"created_at": viewmodel.TimeCell(item.CreatedAt),
				"updated_at": viewmodel.TimeCell(item.UpdatedAt),
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
	h.afterListChange(writer, request, h.itemsPath(workspaceID)+clientQuerySuffix(request.URL.Query()), h.rerenderItems)
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
	h.afterListChange(writer, request, h.itemsPath(workspaceID)+clientQuerySuffix(request.URL.Query()), h.rerenderItems)
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
	for _, key := range []string{"search", "status", "page"} {
		copy.Del(key)
	}
	if values := copy.Encode(); values != "" {
		return path + "?" + values
	}
	return path
}

func itemSortDirection(sort string) string {
	if strings.HasPrefix(sort, "-") {
		return "descending"
	}
	return "ascending"
}
