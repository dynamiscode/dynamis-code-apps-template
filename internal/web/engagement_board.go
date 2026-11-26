package web

import (
	"net/http"
	"net/url"

	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

type engagementBoardColumn struct {
	Stage engagement.Stage
	Page  engagement.Page
}

func engagementBoardListURL(data viewmodel.PageData, path, stage, search, assignee string) string {
	values := url.Values{"stage": {stage}}
	if search != "" {
		values.Set("q", search)
	}
	if assignee != "" {
		values.Set("assignee", assignee)
	}
	for _, column := range data.ListColumns {
		if column.SortDirection == "ascending" {
			values.Set("sort", column.Key)
			values.Set("direction", "asc")
		} else if column.SortDirection == "descending" {
			values.Set("sort", column.Key)
			values.Set("direction", "desc")
		}
	}
	return path + "?" + values.Encode()
}

func boardHasMoveTarget(stages []engagement.Stage, current engagement.StageID) bool {
	for _, stage := range stages {
		if !stage.Archived && stage.ID != current {
			return true
		}
	}
	return false
}

func (h *Handler) engagementBoard(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, session, csrf, ok := h.workspaceSession(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "open"
	}
	if kind != "open" && kind != "won" && kind != "lost" {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	stages, err := h.engagement.ListStages(r.Context(), actor, workspace)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	columns := make([]engagementBoardColumn, 0, len(stages))
	for _, stage := range stages {
		if stage.Kind != kind {
			continue
		}
		page, err := h.engagement.List(r.Context(), actor, workspace, engagement.ListInput{StageID: stage.ID, Page: 1, Limit: 25})
		if err != nil {
			h.renderError(w, http.StatusInternalServerError)
			return
		}
		columns = append(columns, engagementBoardColumn{Stage: stage, Page: page})
	}
	workspaces, err := h.identity.ListWorkspaces(r.Context(), session.UserID)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	path := "/workspaces/" + workspace.String() + "/engagements"
	data := viewmodel.PageData{CanEditClients: actor.Permissions[identity.ResourcesWrite], CSRF: csrf, Workspace: workspaceByID(workspaces, workspace), Workspaces: workspaces, NavPage: "clients", NavSection: "clients", CurrentPath: path + "/board", ClientsEnabled: h.clients != nil, FilesEnabled: h.filesEnabled, SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle}
	h.localizePage(w, &data)
	data.Title = data.T("engagement.board")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = engagementBoardPage(data, columns, stages, kind, path, h.clientsPath(workspace)).Render(r.Context(), w)
}
