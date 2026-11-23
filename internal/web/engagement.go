package web

import (
	"errors"
	"net/http"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func (h *Handler) setEngagementAssignee(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	var assignee identity.UserID
	var err error
	if r.FormValue("assignee_id") != "" {
		assignee, err = identity.ParseUserID(r.FormValue("assignee_id"))
	}
	if err != nil {
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, "engagement.invalid", nil)
		return
	}
	err = h.engagement.SetAssignee(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), assignee, auditContext(r))
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if errors.Is(err, engagement.ErrInvalidInput) {
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, "engagement.invalid", nil)
		return
	}
	if errors.Is(err, engagement.ErrNotFound) {
		h.renderError(w, http.StatusNotFound)
		return
	}
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	h.redirect(w, r, h.clientsPath(workspace)+"/"+r.PathValue("clientId"))
}

func (h *Handler) setEngagementQuote(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	err := h.engagement.SetQuote(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), engagement.Quote{Status: r.FormValue("status"), Amount: r.FormValue("amount"), DateSent: r.FormValue("date_sent")}, auditContext(r))
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if errors.Is(err, engagement.ErrInvalidInput) {
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, "engagement.invalid", nil)
		return
	}
	if errors.Is(err, engagement.ErrNotFound) {
		h.renderError(w, http.StatusNotFound)
		return
	}
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	h.redirect(w, r, h.clientsPath(workspace)+"/"+r.PathValue("clientId"))
}

func (h *Handler) setEngagementNextAction(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	var owner identity.UserID
	var err error
	if r.FormValue("owner_id") != "" {
		owner, err = identity.ParseUserID(r.FormValue("owner_id"))
		if err != nil {
			h.renderClientDetail(w, r, http.StatusUnprocessableEntity, "engagement.invalid", nil)
			return
		}
	}
	action := engagement.NextAction{Text: r.FormValue("text"), DueDate: r.FormValue("due_date"), OwnerID: owner}
	if r.FormValue("clear") == "1" {
		action = engagement.NextAction{}
	}
	err = h.engagement.SetNextAction(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), action, auditContext(r))
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if errors.Is(err, engagement.ErrInvalidInput) {
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, "engagement.invalid", nil)
		return
	}
	if errors.Is(err, engagement.ErrNotFound) {
		h.renderError(w, http.StatusNotFound)
		return
	}
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	h.redirect(w, r, h.clientsPath(workspace)+"/"+r.PathValue("clientId"))
}

func (h *Handler) dueEngagements(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, session, csrf, ok := h.workspaceSession(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	items, err := h.engagement.ListDue(r.Context(), actor, workspace, time.Now().UTC().Format("2006-01-02"))
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(r.Context(), session.UserID)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	data := viewmodel.PageData{CSRF: csrf, Workspace: workspaceByID(workspaces, workspace), Workspaces: workspaces, NavPage: "clients", NavSection: "clients", CurrentPath: "/workspaces/" + workspace.String() + "/engagements/due", ClientsEnabled: h.clients != nil, FilesEnabled: h.filesEnabled, SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle}
	h.localizePage(w, &data)
	data.Title = data.T("engagement.due_list")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = dueEngagementsPage(data, items, h.clientsPath(workspace)).Render(r.Context(), w)
}

func (h *Handler) EnableEngagement(service *engagement.Service) { h.engagement = service }

func (h *Handler) moveEngagement(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(r.PathValue("clientId"))
	engagementID := engagement.ParseID(r.PathValue("engagementId"))
	item, err := h.engagement.Get(r.Context(), actor, workspace, engagementID)
	if err == nil && item.ClientID != clientID {
		err = engagement.ErrNotFound
	}
	if err == nil {
		err = h.engagement.Move(r.Context(), actor, workspace, engagementID, engagement.ParseStageID(r.FormValue("stage")), auditContext(r))
	}
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if errors.Is(err, engagement.ErrNotFound) {
		h.renderError(w, http.StatusNotFound)
		return
	}
	if errors.Is(err, engagement.ErrInvalidInput) {
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, "engagement.invalid_move", nil)
		return
	}
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	h.redirect(w, r, h.clientsPath(workspace)+"/"+clientID.String())
}

func (h *Handler) setEngagementLabel(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	err := h.engagement.SetLabel(r.Context(), actor, workspace, r.FormValue("label"), auditContext(r))
	if errors.Is(err, engagement.ErrInvalidInput) {
		h.renderEngagementStages(w, r, http.StatusUnprocessableEntity, "engagement.invalid_label")
		return
	}
	h.finishStageMutation(w, r, workspace, err)
}

func (h *Handler) createEngagement(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(r.PathValue("clientId"))
	_, err := h.engagement.Create(r.Context(), actor, workspace, engagement.Input{ClientID: clientID, Notes: r.FormValue("notes")}, auditContext(r))
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if errors.Is(err, engagement.ErrNoOpenStage) {
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, "engagement.no_open_stage", nil)
		return
	}
	if errors.Is(err, engagement.ErrInvalidInput) {
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, "engagement.invalid_create", nil)
		return
	}
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	h.redirect(w, r, h.clientsPath(workspace)+"/"+clientID.String())
}

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
	label, err := h.engagement.Label(r.Context(), actor, workspace)
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
	data := viewmodel.PageData{Title: "Engagement stages", CSRF: csrf, ErrorKey: errorKey, Workspace: workspaceByID(workspaces, workspace), Workspaces: workspaces, NavPage: "clients", NavSection: "clients", CurrentPath: path, FilesEnabled: h.filesEnabled, ClientsEnabled: h.clients != nil, SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle, EngagementLabel: label}
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

func (h *Handler) reorderEngagementStages(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	stages, err := h.engagement.ListStages(r.Context(), actor, workspace)
	if err == nil {
		ids := make([]engagement.StageID, len(stages))
		for i, stage := range stages {
			ids[i] = stage.ID
		}
		stageID := engagement.ParseStageID(r.FormValue("stage"))
		found := false
		for i, stage := range stages {
			if stage.ID == stageID {
				j := -1
				if r.FormValue("direction") == "up" {
					j = i - 1
				}
				if r.FormValue("direction") == "down" {
					j = i + 1
				}
				if j >= 0 && j < len(ids) {
					ids[i], ids[j] = ids[j], ids[i]
					found = true
				}
				break
			}
		}
		if !found {
			err = engagement.ErrInvalidInput
		} else {
			err = h.engagement.ReorderStages(r.Context(), actor, workspace, ids, auditContext(r))
		}
	}
	h.finishStageMutation(w, r, workspace, err)
}

func (h *Handler) archiveEngagementStage(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	err := h.engagement.ArchiveStage(r.Context(), actor, workspace, engagement.ParseStageID(r.PathValue("stageId")), auditContext(r))
	h.finishStageMutation(w, r, workspace, err)
}

func (h *Handler) finishStageMutation(w http.ResponseWriter, r *http.Request, workspace identity.WorkspaceID, err error) {
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if errors.Is(err, engagement.ErrInvalidInput) {
		h.renderEngagementStages(w, r, http.StatusUnprocessableEntity, "engagement.invalid")
		return
	}
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	h.redirect(w, r, "/workspaces/"+workspace.String()+"/engagement-stages")
}
