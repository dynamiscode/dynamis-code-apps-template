package web

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// applyColumnOrder puts a list's columns in the member's saved order. The first column is the list's name and
// always stays first. A column with no saved position keeps its declared place, after the ordered ones.
func applyColumnOrder(columns []viewmodel.ListColumn, order []string) []viewmodel.ListColumn {
	if len(columns) < 3 || len(order) == 0 {
		return columns
	}
	rank := make(map[string]int, len(order))
	for position, key := range order {
		rank[key] = position
	}
	unranked := len(order)
	position := func(key string) int {
		if at, ok := rank[key]; ok {
			return at
		}
		return unranked
	}
	movable := append([]viewmodel.ListColumn(nil), columns[1:]...)
	sort.SliceStable(movable, func(i, j int) bool {
		return position(movable[i].Key) < position(movable[j].Key)
	})
	return append([]viewmodel.ListColumn{columns[0]}, movable...)
}

// columnDefaults maps each column of a list to its declared visibility, the shape the preference store validates.
func columnDefaults(columns []viewmodel.ListColumn) map[string]bool {
	defaults := make(map[string]bool, len(columns))
	for _, column := range columns {
		defaults[column.Key] = column.DefaultVisible
	}
	return defaults
}

// orderListColumns returns a list's columns in the member's saved order. Without a preference store the declared
// order stays.
func (h *Handler) orderListColumns(
	request *http.Request,
	principal identity.Principal,
	workspaceID identity.WorkspaceID,
	listKey string,
	columns []viewmodel.ListColumn,
) ([]viewmodel.ListColumn, error) {
	if h.listPreferences == nil {
		return columns, nil
	}
	order, err := h.listPreferences.GetOrder(request.Context(), principal, workspaceID, listKey, columnDefaults(columns))
	if err != nil {
		return nil, err
	}
	return applyColumnOrder(columns, order), nil
}

// storeColumnOrder saves the order named by the order field, a comma-separated list of column keys, and then
// redirects to the list. columnsFor reads the list's declared columns for the member.
func (h *Handler) storeColumnOrder(
	writer http.ResponseWriter,
	request *http.Request,
	listKey string,
	columnsFor func(principal identity.Principal, workspaceID identity.WorkspaceID) ([]viewmodel.ListColumn, error),
	redirect string,
	rerender func(http.ResponseWriter, *http.Request),
) {
	if h.listPreferences == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.clientPreferencePrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	columns, err := columnsFor(principal, workspaceID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	var order []string
	for _, key := range strings.Split(request.FormValue("order"), ",") {
		if key = strings.TrimSpace(key); key != "" {
			order = append(order, key)
		}
	}
	if err := h.listPreferences.SetOrder(request.Context(), principal, workspaceID, listKey, order, columnDefaults(columns)); err != nil {
		switch {
		case errors.Is(err, listprefs.ErrInvalidInput):
			h.renderError(writer, http.StatusBadRequest)
		case errors.Is(err, identity.ErrForbidden):
			h.renderError(writer, http.StatusForbidden)
		default:
			h.renderError(writer, http.StatusInternalServerError)
		}
		return
	}
	h.afterListChange(writer, request, redirect, rerender)
}

func (h *Handler) setItemColumnOrder(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	h.storeColumnOrder(writer, request, listprefs.ListItems,
		func(identity.Principal, identity.WorkspaceID) ([]viewmodel.ListColumn, error) {
			return itemListColumns(), nil
		},
		h.itemsPath(workspaceID)+clientQuerySuffix(request.URL.Query()), h.rerenderItems)
}

func (h *Handler) setMemberColumnOrder(writer http.ResponseWriter, request *http.Request) {
	h.storeColumnOrder(writer, request, listprefs.ListMembers,
		func(identity.Principal, identity.WorkspaceID) ([]viewmodel.ListColumn, error) {
			return memberListColumns(), nil
		},
		request.URL.Path, h.rerenderMembers)
}

// afterListChange finishes a column change. An htmx request for the results fragment gets the list again, so its
// table and columns menu update in place. Any other request is redirected to the list.
func (h *Handler) afterListChange(
	writer http.ResponseWriter,
	request *http.Request,
	redirect string,
	rerender func(http.ResponseWriter, *http.Request),
) {
	if listResultsFragment(request) {
		rerender(writer, request)
		return
	}
	h.redirect(writer, request, redirect)
}

func (h *Handler) rerenderItems(writer http.ResponseWriter, request *http.Request) {
	h.renderItems(writer, request, http.StatusOK, "")
}

func (h *Handler) rerenderMembers(writer http.ResponseWriter, request *http.Request) {
	h.membersPage(writer, request)
}
