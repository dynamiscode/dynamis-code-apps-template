package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listviews"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// listColumnsFunc reads a list's declared columns for a member. Each list registers its own when it is enabled, so
// the shared view code never depends on one list's package.
type listColumnsFunc func(ctx context.Context, principal identity.Principal, workspaceID identity.WorkspaceID) ([]viewmodel.ListColumn, error)

// listViewState is what a list's view menu needs for one request.
type listViewState struct {
	Views      []viewmodel.ListViewOption
	Query      string
	Base       string
	Route      string
	Active     string
	CanShare   bool
	SaveReason string
}

// EnableListViews attaches the saved views store. Items and Members register their columns here; Clients and Files
// register theirs when they are enabled.
func (h *Handler) EnableListViews(views *listviews.Service) {
	h.listViews = views
	h.registerListColumns(listviews.ListItems, func(context.Context, identity.Principal, identity.WorkspaceID) ([]viewmodel.ListColumn, error) {
		return itemListColumns(), nil
	})
	h.registerListColumns(listviews.ListMembers, func(context.Context, identity.Principal, identity.WorkspaceID) ([]viewmodel.ListColumn, error) {
		return memberListColumns(), nil
	})
}

func (h *Handler) registerListColumns(listKey string, columns listColumnsFunc) {
	if h.listColumns == nil {
		h.listColumns = make(map[string]listColumnsFunc)
	}
	h.listColumns[listKey] = columns
}

// listViewBase is a list's page address. It is built here rather than with the list's own helper, so this shared file
// still builds when a list is pruned from a generated app.
func (h *Handler) listViewBase(workspaceID identity.WorkspaceID, listKey string) string {
	segment := "settings/members"
	switch listKey {
	case listviews.ListClients:
		segment = "clients"
	case listviews.ListItems:
		segment = "items"
	case listviews.ListFiles:
		segment = "files"
	}
	return "/workspaces/" + workspaceID.String() + "/" + segment
}

// viewStateQuery is the part of a list's query a view saves: filters, match, sort and order. The search text, the
// page, the rows per page, the archived scope and the markers of a results request are not part of a view.
func viewStateQuery(values url.Values) string {
	kept := url.Values{}
	for key, entries := range values {
		switch key {
		case "q", "search", "page", "limit", "archived", "part", "all":
			continue
		}
		kept[key] = entries
	}
	return kept.Encode()
}

// sameViewQuery reports whether a saved view's query matches the list on screen, ignoring parameters a view does not
// save and the order the parameters were written in.
func sameViewQuery(saved string, current url.Values) bool {
	parsed, err := url.ParseQuery(saved)
	if err != nil {
		return false
	}
	return viewStateQuery(parsed) == viewStateQuery(current)
}

// listViewState builds the view menu's state: the member's views, the query a new view would save, and whether the
// list can save at all. Archived clients cannot be saved as a view.
func (h *Handler) listViewState(request *http.Request, principal identity.Principal, workspaceID identity.WorkspaceID, listKey string, archived bool) listViewState {
	state := listViewState{
		Base:  h.listViewBase(workspaceID, listKey),
		Route: "/workspaces/" + workspaceID.String() + "/lists/" + listKey + "/views",
		Query: viewStateQuery(request.URL.Query()),
	}
	state.Active = state.Base
	if search := request.URL.Query().Get("q"); search != "" {
		state.Active += "?q=" + url.QueryEscape(search)
	}
	state.CanShare = principal.Permissions[identity.WorkspaceUpdate]
	if archived {
		state.SaveReason = "clients.views_active_only"
	}
	if h.listViews == nil {
		return state
	}
	views, defaultID, err := h.listViews.List(request.Context(), principal, workspaceID, listKey)
	if err != nil {
		return state
	}
	for _, view := range views {
		state.Views = append(state.Views, viewmodel.ListViewOption{
			ID: view.ID, Name: view.Name, Shared: view.Shared, Default: view.ID == defaultID,
			Editable: view.CreatedBy == principal.UserID || (view.Shared && principal.Permissions[identity.WorkspaceUpdate]),
			Current:  sameViewQuery(view.State.Query, request.URL.Query()),
		})
	}
	return state
}

// currentColumnSettings reads the member's column visibility and order for a list, which a new or changed view saves.
func (h *Handler) currentColumnSettings(request *http.Request, principal identity.Principal, workspaceID identity.WorkspaceID, listKey string) (map[string]bool, []string, error) {
	columns, err := h.listColumns[listKey](request.Context(), principal, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	defaults := columnDefaults(columns)
	visible, err := h.listPreferences.Get(request.Context(), principal, workspaceID, listKey, defaults)
	if err != nil {
		return nil, nil, err
	}
	order, err := h.listPreferences.GetOrder(request.Context(), principal, workspaceID, listKey, defaults)
	return visible, order, err
}

// viewRequest checks a saved-view request: the list is one that has views, the member's session and token are
// valid, and the list's columns can be read.
func (h *Handler) viewRequest(writer http.ResponseWriter, request *http.Request) (string, identity.Principal, identity.WorkspaceID, bool) {
	if h.listViews == nil || h.listPreferences == nil {
		h.renderError(writer, http.StatusNotFound)
		return "", identity.Principal{}, 0, false
	}
	listKey := request.PathValue("list")
	if _, ok := h.listColumns[listKey]; !ok {
		h.renderError(writer, http.StatusNotFound)
		return "", identity.Principal{}, 0, false
	}
	principal, ok := h.clientPreferencePrincipal(writer, request)
	if !ok {
		return "", identity.Principal{}, 0, false
	}
	return listKey, principal, identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId")), true
}

// viewError maps a saved-view failure to a response.
func (h *Handler) viewError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, listviews.ErrInvalidInput):
		h.renderError(writer, http.StatusBadRequest)
	case errors.Is(err, listviews.ErrNotFound):
		h.renderError(writer, http.StatusNotFound)
	case errors.Is(err, listviews.ErrLimit):
		h.renderError(writer, http.StatusConflict)
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(writer, http.StatusForbidden)
	default:
		h.renderError(writer, http.StatusInternalServerError)
	}
}

func (h *Handler) createListView(writer http.ResponseWriter, request *http.Request) {
	listKey, principal, workspaceID, ok := h.viewRequest(writer, request)
	if !ok {
		return
	}
	visible, order, err := h.currentColumnSettings(request, principal, workspaceID, listKey)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	state := listviews.State{Query: viewStateQuery(formQuery(request)), Visible: visible, Order: order}
	shared := request.FormValue("shared") == "1"
	if _, err := h.listViews.Create(request.Context(), principal, workspaceID, listKey, request.FormValue("name"), shared, state); err != nil {
		h.viewError(writer, err)
		return
	}
	h.redirect(writer, request, h.listViewBase(workspaceID, listKey)+"?"+state.Query)
}

// formQuery is the list query a view form was sent from, read from its hidden query field.
func formQuery(request *http.Request) url.Values {
	values, err := url.ParseQuery(request.FormValue("query"))
	if err != nil {
		return url.Values{}
	}
	return values
}

func (h *Handler) updateListView(writer http.ResponseWriter, request *http.Request) {
	listKey, principal, workspaceID, ok := h.viewRequest(writer, request)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(request.PathValue("viewId"), 10, 64)
	if err != nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	visible, order, err := h.currentColumnSettings(request, principal, workspaceID, listKey)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	current, err := h.listViews.Get(request.Context(), principal, workspaceID, id)
	if err != nil {
		h.viewError(writer, err)
		return
	}
	state := listviews.State{Query: viewStateQuery(formQuery(request)), Visible: visible, Order: order}
	if err := h.listViews.Update(request.Context(), principal, workspaceID, id, current.Name, state); err != nil {
		h.viewError(writer, err)
		return
	}
	h.redirect(writer, request, h.listViewBase(workspaceID, listKey)+"?"+state.Query)
}

func (h *Handler) applyListView(writer http.ResponseWriter, request *http.Request) {
	listKey, principal, workspaceID, ok := h.viewRequest(writer, request)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(request.PathValue("viewId"), 10, 64)
	if err != nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	view, err := h.listViews.Get(request.Context(), principal, workspaceID, id)
	if err != nil {
		h.viewError(writer, err)
		return
	}
	columns, err := h.listColumns[listKey](request.Context(), principal, workspaceID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	defaults := columnDefaults(columns)
	// The view's columns and order land in one transaction: all of it applies, or none of it does.
	var order []string
	for _, key := range view.State.Order {
		if _, known := defaults[key]; known {
			order = append(order, key)
		}
	}
	if err := h.listPreferences.ApplyView(request.Context(), principal, workspaceID, listKey, view.State.Visible, order, defaults); err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	query, err := url.ParseQuery(view.State.Query)
	if err != nil {
		query = url.Values{}
	}
	if search := request.FormValue("q"); search != "" {
		query.Set("q", search)
	}
	target := h.listViewBase(workspaceID, listKey)
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	h.redirect(writer, request, target)
}

// renameListView changes a view's name and keeps what it saves.
func (h *Handler) renameListView(writer http.ResponseWriter, request *http.Request) {
	listKey, principal, workspaceID, ok := h.viewRequest(writer, request)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(request.PathValue("viewId"), 10, 64)
	if err != nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	current, err := h.listViews.Get(request.Context(), principal, workspaceID, id)
	if err != nil {
		h.viewError(writer, err)
		return
	}
	if err := h.listViews.Update(request.Context(), principal, workspaceID, id, request.FormValue("name"), current.State); err != nil {
		h.viewError(writer, err)
		return
	}
	h.redirect(writer, request, h.listViewBase(workspaceID, listKey))
}

func (h *Handler) deleteListView(writer http.ResponseWriter, request *http.Request) {
	listKey, principal, workspaceID, ok := h.viewRequest(writer, request)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(request.PathValue("viewId"), 10, 64)
	if err != nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	if err := h.listViews.Delete(request.Context(), principal, workspaceID, id); err != nil {
		h.viewError(writer, err)
		return
	}
	h.redirect(writer, request, h.listViewBase(workspaceID, listKey))
}

// setListViewDefault makes a view the member's default for the list. The clear route passes zero.
func (h *Handler) setListViewDefault(writer http.ResponseWriter, request *http.Request) {
	listKey, principal, workspaceID, ok := h.viewRequest(writer, request)
	if !ok {
		return
	}
	var id int64
	if raw := request.PathValue("viewId"); raw != "clear" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			h.renderError(writer, http.StatusNotFound)
			return
		}
		id = parsed
	}
	if err := h.listViews.SetDefault(request.Context(), principal, workspaceID, listKey, id); err != nil {
		h.viewError(writer, err)
		return
	}
	h.redirect(writer, request, h.listViewBase(workspaceID, listKey))
}

// openDefaultListView sends a member who opens a list with no query to the list's default view. It reports whether
// it redirected, so the caller stops rendering.
func (h *Handler) openDefaultListView(writer http.ResponseWriter, request *http.Request, listKey string, principal identity.Principal, workspaceID identity.WorkspaceID) bool {
	// Only a bare list address opens the default view. Any parameter, including the results marker an htmx swap
	// sends, must render as it was asked for.
	if h.listViews == nil || request.Method != http.MethodGet || len(request.URL.Query()) > 0 {
		return false
	}
	views, defaultID, err := h.listViews.List(request.Context(), principal, workspaceID, listKey)
	if err != nil || defaultID == 0 {
		return false
	}
	for _, view := range views {
		if view.ID == defaultID && view.State.Query != "" {
			h.redirect(writer, request, h.listViewBase(workspaceID, listKey)+"?"+view.State.Query)
			return true
		}
	}
	return false
}
