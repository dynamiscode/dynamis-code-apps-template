package httpapi

import (
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func (h *handler) createEngagement(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		notFoundProblem(w, r)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesWrite)
	if !ok {
		return
	}
	var input struct {
		ClientID string `json:"clientId"`
		StageID  string `json:"stageId"`
		Notes    string `json:"notes"`
	}
	if err := decodeJSON(r, &input); err != nil {
		h.badJSON(w, r, err)
		return
	}
	stageID := engagement.ParseStageID(input.StageID)
	if input.StageID != "" && stageID == 0 {
		h.invalidRequest(w, r, "The stage is invalid.")
		return
	}
	value, err := h.engagement.Create(r.Context(), actor, workspace, engagement.Input{ClientID: clients.ParseClientIDOrZero(input.ClientID), StageID: stageID, Notes: input.Notes}, h.auditContext(r))
	if err != nil {
		h.engagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (h *handler) getEngagement(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		notFoundProblem(w, r)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	value, err := h.engagement.Get(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")))
	if err != nil {
		h.engagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (h *handler) listEngagementStages(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		notFoundProblem(w, r)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	stages, err := h.engagement.ListStages(r.Context(), actor, workspace)
	if err != nil {
		h.engagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stages)
}

func (h *handler) createEngagementStage(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		notFoundProblem(w, r)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, ok := h.workspaceBearer(w, r, workspace, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	var input engagement.StageInput
	if err := decodeJSON(r, &input); err != nil {
		h.badJSON(w, r, err)
		return
	}
	stage, err := h.engagement.CreateStage(r.Context(), actor, workspace, input, h.auditContext(r))
	if err != nil {
		h.engagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, stage)
}

func (h *handler) reorderEngagementStages(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		notFoundProblem(w, r)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, ok := h.workspaceBearer(w, r, workspace, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	var input struct {
		IDs []string `json:"ids"`
	}
	if err := decodeJSON(r, &input); err != nil {
		h.badJSON(w, r, err)
		return
	}
	ids := make([]engagement.StageID, len(input.IDs))
	for i, value := range input.IDs {
		ids[i] = engagement.ParseStageID(value)
	}
	if err := h.engagement.ReorderStages(r.Context(), actor, workspace, ids, h.auditContext(r)); err != nil {
		h.engagementError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) archiveEngagementStage(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		notFoundProblem(w, r)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, ok := h.workspaceBearer(w, r, workspace, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	if err := h.engagement.ArchiveStage(r.Context(), actor, workspace, engagement.ParseStageID(r.PathValue("stageId")), h.auditContext(r)); err != nil {
		h.engagementError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) engagementError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(w, r, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, engagement.ErrInvalidInput):
		h.invalidRequest(w, r, "The engagement or stage is invalid.")
	case errors.Is(err, engagement.ErrNoOpenStage):
		h.invalidRequest(w, r, "The workspace has no open stage.")
	case errors.Is(err, engagement.ErrNotFound):
		notFoundProblem(w, r)
	default:
		h.internal(w, r)
	}
}
