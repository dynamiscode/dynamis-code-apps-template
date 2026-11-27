package httpapi

import (
	"errors"
	"net/http"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func (h *handler) setEngagementAgreement(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		notFoundProblem(w, r)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesWrite)
	if !ok {
		return
	}
	var input engagement.Agreement
	if err := decodeJSON(r, &input); err != nil {
		h.badJSON(w, r, err)
		return
	}
	if err := h.engagement.SetAgreement(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), input, h.auditContext(r)); err != nil {
		h.engagementError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) setEngagementAssignee(w http.ResponseWriter, r *http.Request) {
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
		AssigneeID string `json:"assigneeId"`
	}
	if err := decodeJSON(r, &input); err != nil {
		h.badJSON(w, r, err)
		return
	}
	var assignee identity.UserID
	if input.AssigneeID != "" {
		var err error
		assignee, err = identity.ParseUserID(input.AssigneeID)
		if err != nil {
			h.invalidRequest(w, r, "The assignee is invalid.")
			return
		}
	}
	if err := h.engagement.SetAssignee(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), assignee, h.auditContext(r)); err != nil {
		h.engagementError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) getEngagementLabel(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		notFoundProblem(w, r)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	label, err := h.engagement.Label(r.Context(), actor, workspace)
	if err != nil {
		h.engagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"label": label})
}

func (h *handler) setEngagementLabel(w http.ResponseWriter, r *http.Request) {
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
		Label *string `json:"label"`
	}
	if err := decodeJSON(r, &input); err != nil {
		h.badJSON(w, r, err)
		return
	}
	if input.Label == nil {
		h.invalidRequest(w, r, "Label is required.")
		return
	}
	if err := h.engagement.SetLabel(r.Context(), actor, workspace, *input.Label, h.auditContext(r)); err != nil {
		h.engagementError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) setEngagementQuote(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		notFoundProblem(w, r)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesWrite)
	if !ok {
		return
	}
	var quote engagement.Quote
	if err := decodeJSON(r, &quote); err != nil {
		h.badJSON(w, r, err)
		return
	}
	if err := h.engagement.SetQuote(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), quote, h.auditContext(r)); err != nil {
		h.engagementError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) setEngagementNextAction(w http.ResponseWriter, r *http.Request) {
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
		Text    string `json:"text"`
		DueDate string `json:"dueDate"`
		OwnerID string `json:"ownerId"`
	}
	if err := decodeJSON(r, &input); err != nil {
		h.badJSON(w, r, err)
		return
	}
	var owner identity.UserID
	if input.OwnerID != "" {
		var err error
		owner, err = identity.ParseUserID(input.OwnerID)
		if err != nil {
			h.invalidRequest(w, r, "The owner is invalid.")
			return
		}
	}
	if err := h.engagement.SetNextAction(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), engagement.NextAction{Text: input.Text, DueDate: input.DueDate, OwnerID: owner}, h.auditContext(r)); err != nil {
		h.engagementError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) listDueEngagements(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		notFoundProblem(w, r)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	values, err := h.engagement.ListDue(r.Context(), actor, workspace, time.Now().UTC().Format("2006-01-02"))
	if err != nil {
		h.engagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, values)
}

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
		Name     string `json:"name"`
		ClientID string `json:"clientId"`
		StageID  string `json:"stageId"`
		Notes    string `json:"notes"`
	}
	if err := decodeJSON(r, &input); err != nil {
		h.badJSON(w, r, err)
		return
	}
	stageID := engagement.ParseStageID(input.StageID)
	if stageID == 0 {
		h.invalidRequest(w, r, "The stage is invalid.")
		return
	}
	value, err := h.engagement.Create(r.Context(), actor, workspace, engagement.Input{Name: input.Name, ClientID: clients.ParseClientIDOrZero(input.ClientID), StageID: stageID, Notes: input.Notes}, h.auditContext(r))
	if err != nil {
		h.engagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (h *handler) setEngagementName(w http.ResponseWriter, r *http.Request) {
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
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &input); err != nil {
		h.badJSON(w, r, err)
		return
	}
	if err := h.engagement.SetName(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), input.Name, h.auditContext(r)); err != nil {
		h.engagementError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func (h *handler) moveEngagement(w http.ResponseWriter, r *http.Request) {
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
		StageID string `json:"stageId"`
	}
	if err := decodeJSON(r, &input); err != nil {
		h.badJSON(w, r, err)
		return
	}
	if err := h.engagement.Move(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), engagement.ParseStageID(input.StageID), h.auditContext(r)); err != nil {
		h.engagementError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
		IDs      []string `json:"ids"`
		Snapshot []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Kind     string `json:"kind"`
			Archived *bool  `json:"archived"`
		} `json:"snapshot"`
		MovedID       string `json:"movedId"`
		NewKind       string `json:"newKind"`
		ExpectedCount *int   `json:"expectedCount"`
		Confirmed     *bool  `json:"confirmed"`
	}
	if err := decodeJSON(r, &input); err != nil {
		h.badJSON(w, r, err)
		return
	}
	if input.IDs == nil || input.Snapshot == nil || input.MovedID == "" || input.NewKind == "" || input.ExpectedCount == nil || input.Confirmed == nil {
		h.invalidRequest(w, r, "The stage move requires a snapshot and confirmation details.")
		return
	}
	snapshot := make([]engagement.Stage, len(input.Snapshot))
	for i, stage := range input.Snapshot {
		if stage.ID == "" || stage.Name == "" || stage.Kind == "" || stage.Archived == nil {
			h.invalidRequest(w, r, "The stage snapshot is invalid.")
			return
		}
		snapshot[i] = engagement.Stage{ID: engagement.ParseStageID(stage.ID), Name: stage.Name, Kind: stage.Kind, Archived: *stage.Archived}
	}
	ids := make([]engagement.StageID, len(input.IDs))
	for i, value := range input.IDs {
		ids[i] = engagement.ParseStageID(value)
	}
	if err := h.engagement.MoveStages(r.Context(), actor, workspace, snapshot, ids, engagement.ParseStageID(input.MovedID), input.NewKind, *input.ExpectedCount, *input.Confirmed, h.auditContext(r)); err != nil {
		h.engagementError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) stageEngagementCount(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		notFoundProblem(w, r)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, ok := h.workspaceBearer(w, r, workspace, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	count, err := h.engagement.StageEngagementCount(r.Context(), actor, workspace, engagement.ParseStageID(r.PathValue("stageId")))
	if err != nil {
		h.engagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Count int `json:"count"`
	}{count})
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
	case errors.Is(err, engagement.ErrStaleStages):
		writeProblem(w, r, http.StatusConflict, "stale-stages", "The stages or engagement count changed; reload and confirm again.")
	case errors.Is(err, engagement.ErrConfirmationRequired):
		writeProblem(w, r, http.StatusConflict, "confirmation-required", "Changing a stage kind requires confirmation.")
	case errors.Is(err, engagement.ErrInvalidName):
		h.invalidRequest(w, r, "The engagement name is invalid.")
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
