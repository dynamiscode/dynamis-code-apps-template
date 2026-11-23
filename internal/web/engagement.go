package web

import (
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func (h *Handler) EnableEngagement(service *engagement.Service) { h.engagement = service }

func (h *Handler) engagementStages(w http.ResponseWriter, r *http.Request) {
	h.renderEngagementStages(w, r, http.StatusOK, "")
}

func (h *Handler) renderEngagementStages(w http.ResponseWriter, r *http.Request, status int, errorKey string) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, session, csrf, ok := h.workspaceSession(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	stages, err := h.engagement.ListStages(r.Context(), actor, workspace)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(r.Context(), session.UserID)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	path := "/workspaces/" + workspace.String() + "/engagement-stages"
	data := viewmodel.PageData{Title: "Engagement stages", CSRF: csrf, ErrorKey: errorKey, Workspace: workspaceByID(workspaces, workspace), Workspaces: workspaces, NavPage: "clients", NavSection: "clients", CurrentPath: path, FilesEnabled: h.filesEnabled, ClientsEnabled: h.clients != nil, SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle}
	h.localizePage(w, &data)
	data.Title = data.T("engagement.stages")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = engagementStagesPage(data, stages, actor.Permissions[identity.WorkspaceUpdate]).Render(r.Context(), w)
}

func (h *Handler) createEngagementStage(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	_, err := h.engagement.CreateStage(r.Context(), actor, workspace, engagement.StageInput{Name: r.FormValue("name"), Kind: r.FormValue("kind")}, auditContext(r))
	if errors.Is(err, engagement.ErrInvalidInput) {
		h.renderEngagementStages(w, r, http.StatusUnprocessableEntity, "engagement.invalid")
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
	h.redirect(w, r, "/workspaces/"+workspace.String()+"/engagement-stages")
}
