package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/web/components/organisms"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func (h *Handler) listEngagements(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, session, csrf, ok := h.workspaceSession(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	q := r.URL.Query()
	view := q.Get("view")
	if view == "" {
		view = "list"
	}
	if view != "list" && view != "board" {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	input := engagement.ListInput{Search: q.Get("q"), Page: 1, Limit: 25, Sort: q.Get("sort"), Direction: q.Get("direction")}
	if q.Get("page") != "" {
		var err error
		input.Page, err = strconv.Atoi(q.Get("page"))
		if err != nil {
			h.renderError(w, http.StatusBadRequest)
			return
		}
	}
	if q.Get("stage") != "" {
		input.StageID = engagement.ParseStageID(q.Get("stage"))
		if input.StageID == 0 {
			h.renderError(w, http.StatusBadRequest)
			return
		}
	}
	if q.Get("assignee") != "" {
		var user identity.UserID
		var err error
		if q.Get("assignee") != "unassigned" {
			user, err = identity.ParseUserID(q.Get("assignee"))
			if err != nil {
				h.renderError(w, http.StatusBadRequest)
				return
			}
		}
		input.AssigneeID = &user
	}
	page, err := h.engagement.List(r.Context(), actor, workspace, input)
	if errors.Is(err, engagement.ErrInvalidInput) {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	empty := page.Total == 0
	if empty && (input.Search != "" || input.StageID != 0 || input.AssigneeID != nil) {
		all, err := h.engagement.List(r.Context(), actor, workspace, engagement.ListInput{Page: 1, Limit: 1})
		if err != nil {
			if errors.Is(err, identity.ErrForbidden) {
				h.renderError(w, http.StatusForbidden)
			} else {
				h.renderError(w, http.StatusInternalServerError)
			}
			return
		}
		empty = all.Total == 0
	}
	stages, err := h.engagement.ListStages(r.Context(), actor, workspace)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	var columns []engagementBoardColumn
	if view == "board" {
		columns = make([]engagementBoardColumn, 0, len(stages))
		for _, stage := range stages {
			if input.StageID != 0 && input.StageID != stage.ID {
				continue
			}
			boardInput := input
			boardInput.StageID = stage.ID
			boardInput.Page = 1
			boardPage, err := h.engagement.List(r.Context(), actor, workspace, boardInput)
			if err != nil {
				h.renderError(w, http.StatusInternalServerError)
				return
			}
			columns = append(columns, engagementBoardColumn{Stage: stage, Page: boardPage})
		}
	}
	members, err := h.identity.ListMembers(r.Context(), actor)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(r.Context(), session.UserID)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	path := "/workspaces/" + workspace.String() + "/engagements"
	data := viewmodel.PageData{CSRF: csrf, Workspace: workspaceByID(workspaces, workspace), Workspaces: workspaces, NavPage: "clients", NavSection: "clients", CurrentPath: path, ClientsEnabled: h.clients != nil, FilesEnabled: h.filesEnabled, SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle, CanEditClients: actor.Permissions[identity.ResourcesWrite], CanManageFields: actor.Permissions[identity.WorkspaceUpdate]}
	data.ClientQuerySuffix = "?" + q.Encode()
	for _, stage := range stages {
		if !stage.Archived && data.CanEditClients {
			data.ListSelectable = true
			break
		}
	}
	h.localizePage(w, &data)
	data.Title = data.T("engagement.title")
	for _, key := range []string{"name", "client", "stage", "assignee"} {
		values := url.Values{}
		for _, filter := range []string{"q", "stage", "assignee", "view"} {
			if q.Get(filter) != "" {
				values.Set(filter, q.Get(filter))
			}
		}
		direction := "asc"
		sortDirection := ""
		if input.Sort == key {
			if input.Direction == "asc" {
				direction, sortDirection = "desc", "ascending"
			} else {
				sortDirection = "descending"
			}
		}
		values.Set("sort", key)
		values.Set("direction", direction)
		data.ListColumns = append(data.ListColumns, viewmodel.ListColumn{
			Key: key, LabelKey: "engagement." + key, Visible: true,
			SortHref: path + "?" + values.Encode(), SortDirection: sortDirection,
		})
	}
	prev, next := "", ""
	link := func(n int) string {
		values := url.Values{}
		for _, key := range []string{"q", "stage", "assignee", "sort", "direction", "view"} {
			if q.Get(key) != "" {
				values.Set(key, q.Get(key))
			}
		}
		values.Set("page", strconv.Itoa(n))
		return path + "?" + values.Encode()
	}
	if input.Page > 1 {
		prev = link(input.Page - 1)
	}
	if input.Page < clients.MaxListPage && input.Page*input.Limit < page.Total {
		next = link(input.Page + 1)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("HX-Request") == "true" {
		_ = engagementListView(data, page, stages, members, q.Get("q"), q.Get("stage"), q.Get("assignee"), prev, next, h.clientsPath(workspace), columns, view, empty).Render(r.Context(), w)
		return
	}
	_ = engagementListPage(data, page, stages, members, q.Get("q"), q.Get("stage"), q.Get("assignee"), prev, next, h.clientsPath(workspace), columns, view, empty).Render(r.Context(), w)
}

func engagementListItemURL(data viewmodel.PageData, item engagement.ListItem, clientsPath string) string {
	values := url.Values{"engagement_return": {data.CurrentPath + data.ClientQuerySuffix}}
	return clientsPath + "/" + item.ClientID.String() + "/engagements/" + item.ID.String() + "?" + values.Encode()
}

func engagementListActions(data viewmodel.PageData, item engagement.ListItem, clientsPath string) []organisms.ListAction {
	return []organisms.ListAction{{
		Label: data.T("ui.open"),
		Icon:  "open",
		Href:  engagementListItemURL(data, item, clientsPath),
	}}
}

func (h *Handler) bulkMoveEngagements(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	if err := r.ParseForm(); err != nil || r.FormValue("confirm") != "yes" {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	rawIDs := r.Form["engagement_id"]
	if r.FormValue("selection") == "all" {
		rawIDs = r.Form["page_id"]
	} else if r.FormValue("selection") != "" {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	ids := make([]engagement.ID, 0, len(rawIDs))
	seen := map[engagement.ID]bool{}
	for _, raw := range rawIDs {
		id := engagement.ParseID(raw)
		if id == 0 {
			h.renderError(w, http.StatusBadRequest)
			return
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	err := h.engagement.MoveMany(r.Context(), actor, workspace, ids, engagement.ParseStageID(r.FormValue("stage")), auditContext(r))
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(w, http.StatusForbidden)
		return
	case errors.Is(err, engagement.ErrNotFound):
		h.renderError(w, http.StatusNotFound)
		return
	case errors.Is(err, engagement.ErrInvalidInput):
		h.renderError(w, http.StatusUnprocessableEntity)
		return
	case err != nil:
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	path := "/workspaces/" + workspace.String() + "/engagements"
	back := r.FormValue("return")
	if back == path || strings.HasPrefix(back, path+"?") {
		path = back
	}
	h.redirect(w, r, path)
}
