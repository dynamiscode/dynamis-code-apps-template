package httpapi

import (
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
)

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

func (h *handler) engagementError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(w, r, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, engagement.ErrInvalidInput):
		h.invalidRequest(w, r, "The stage is invalid.")
	default:
		h.internal(w, r)
	}
}
