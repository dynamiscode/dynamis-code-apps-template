package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
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
	input := engagement.ListInput{Search: q.Get("q"), Page: 1, Limit: 25}
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
	stages, err := h.engagement.ListStages(r.Context(), actor, workspace)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
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
	data := viewmodel.PageData{CSRF: csrf, Workspace: workspaceByID(workspaces, workspace), Workspaces: workspaces, NavPage: "clients", NavSection: "clients", CurrentPath: path, ClientsEnabled: h.clients != nil, FilesEnabled: h.filesEnabled, SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle}
	h.localizePage(w, &data)
	data.Title = data.T("engagement.list")
	prev, next := "", ""
	link := func(n int) string {
		values := url.Values{}
		for _, key := range []string{"q", "stage", "assignee"} {
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
	_ = engagementListPage(data, page, stages, members, q.Get("q"), q.Get("stage"), q.Get("assignee"), prev, next, h.clientsPath(workspace)).Render(r.Context(), w)
}
