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

func (h *Handler) setEngagementAgreement(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	if !h.engagementMatchesClient(w, r, actor, workspace) {
		return
	}
	fileID := r.FormValue("file_id")
	if selected := r.FormValue("selected_file_id"); selected != "" {
		fileID = selected
		r.PostForm.Set("file_id", selected)
		r.Form.Set("file_id", selected)
	}
	err := h.engagement.SetAgreement(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), engagement.Agreement{Recorded: r.FormValue("recorded") == "1", FileID: fileID}, auditContext(r))
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if errors.Is(err, engagement.ErrInvalidInput) {
		key := "engagement.invalid"
		if r.FormValue("return_to") == "detail" {
			key = "engagement.invalid_agreement_file"
		}
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, key, nil)
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
	path := h.clientsPath(workspace) + "/" + r.PathValue("clientId")
	if r.FormValue("return_to") == "detail" {
		path += "/engagements/" + r.PathValue("engagementId") + "?saved=1"
	}
	h.redirect(w, r, path)
}

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
	if !h.engagementMatchesClient(w, r, actor, workspace) {
		return
	}
	if r.FormValue("return_to") == "detail" {
		if _, present := r.PostForm["assignee_id"]; !present {
			h.renderClientDetail(w, r, http.StatusUnprocessableEntity, "engagement.invalid_assignee", nil)
			return
		}
	}
	var assignee identity.UserID
	var err error
	if r.FormValue("assignee_id") != "" {
		assignee, err = identity.ParseUserID(r.FormValue("assignee_id"))
	}
	if err != nil {
		key := "engagement.invalid"
		if r.FormValue("return_to") == "detail" {
			key = "engagement.invalid_assignee"
		}
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, key, nil)
		return
	}
	err = h.engagement.SetAssignee(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), assignee, auditContext(r))
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if errors.Is(err, engagement.ErrInvalidInput) {
		key := "engagement.invalid"
		if r.FormValue("return_to") == "detail" {
			key = "engagement.invalid_assignee"
		}
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, key, nil)
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
	path := h.clientsPath(workspace) + "/" + r.PathValue("clientId")
	if r.FormValue("return_to") == "detail" {
		path += "/engagements/" + r.PathValue("engagementId") + "?saved=1"
	}
	h.redirect(w, r, path)
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
	if !h.engagementMatchesClient(w, r, actor, workspace) {
		return
	}
	err := h.engagement.SetQuote(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), engagement.Quote{Status: r.FormValue("status"), Amount: r.FormValue("amount"), DateSent: r.FormValue("date_sent")}, auditContext(r))
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if errors.Is(err, engagement.ErrInvalidInput) {
		key := "engagement.invalid"
		if r.FormValue("return_to") == "detail" {
			key = "engagement.invalid_quote"
		}
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, key, nil)
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
	path := h.clientsPath(workspace) + "/" + r.PathValue("clientId")
	if r.FormValue("return_to") == "detail" {
		path += "/engagements/" + r.PathValue("engagementId") + "?saved=1"
	}
	h.redirect(w, r, path)
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
	if !h.engagementMatchesClient(w, r, actor, workspace) {
		return
	}
	version, err := strconv.ParseInt(r.FormValue("version"), 10, 64)
	if err != nil || version < 0 {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	if r.FormValue("return_to") == "detail" && r.FormValue("clear") != "1" {
		if _, present := r.PostForm["owner_id"]; !present {
			h.renderClientDetail(w, r, http.StatusUnprocessableEntity, "engagement.invalid_next_action", nil)
			return
		}
	}
	var owner identity.UserID
	if r.FormValue("owner_id") != "" {
		owner, err = identity.ParseUserID(r.FormValue("owner_id"))
		if err != nil {
			key := "engagement.invalid"
			if r.FormValue("return_to") == "detail" {
				key = "engagement.invalid_next_action"
			}
			h.renderClientDetail(w, r, http.StatusUnprocessableEntity, key, nil)
			return
		}
	}
	action := engagement.NextAction{Text: r.FormValue("text"), DueDate: r.FormValue("due_date"), OwnerID: owner}
	if r.FormValue("clear") == "1" {
		action = engagement.NextAction{}
	}
	err = h.engagement.SetNextActionIfVersion(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), action, version, auditContext(r))
	if errors.Is(err, engagement.ErrStaleNextAction) {
		h.renderClientDetail(w, r, http.StatusConflict, "engagement.action_conflict", nil)
		return
	}
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if errors.Is(err, engagement.ErrInvalidInput) {
		key := "engagement.invalid"
		if r.FormValue("return_to") == "detail" {
			key = "engagement.invalid_next_action"
		}
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, key, nil)
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
	path := h.clientsPath(workspace) + "/" + r.PathValue("clientId")
	if r.FormValue("return_to") == "detail" {
		path += "/engagements/" + r.PathValue("engagementId") + "?saved=1"
	}
	h.redirect(w, r, path)
}

func (h *Handler) engagementMatchesClient(w http.ResponseWriter, r *http.Request, actor identity.Principal, workspace identity.WorkspaceID) bool {
	item, err := h.engagement.Get(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")))
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return false
	}
	if errors.Is(err, engagement.ErrNotFound) || (err == nil && item.ClientID != clients.ParseClientIDOrZero(r.PathValue("clientId"))) {
		h.renderError(w, http.StatusNotFound)
		return false
	}
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return false
	}
	return true
}

func (h *Handler) setEngagementName(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	if !h.engagementMatchesClient(w, r, actor, workspace) {
		return
	}
	err := h.engagement.SetName(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), r.FormValue("name"), auditContext(r))
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(w, http.StatusForbidden)
	case errors.Is(err, engagement.ErrNotFound):
		h.renderError(w, http.StatusNotFound)
	case errors.Is(err, engagement.ErrInvalidInput):
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, "engagement.invalid_name", nil)
	case err != nil:
		h.renderError(w, http.StatusInternalServerError)
	default:
		h.redirect(w, r, h.clientsPath(workspace)+"/"+r.PathValue("clientId")+"/engagements/"+r.PathValue("engagementId")+"?saved=1")
	}
}

func (h *Handler) engagementDetail(w http.ResponseWriter, r *http.Request) {
	h.renderClientDetail(w, r, http.StatusOK, "", nil)
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
	query := r.URL.Query()
	page := 1
	if query.Get("page") != "" {
		var err error
		page, err = strconv.Atoi(query.Get("page"))
		if err != nil || page < 1 || page > 1000000 {
			h.renderError(w, http.StatusBadRequest)
			return
		}
	}
	options := engagement.ActionQueueOptions{Filter: engagement.ActionQueueFilter(query.Get("filter")), Page: page, Limit: 20}
	owner := query.Get("owner")
	if owner != "" {
		var id identity.UserID
		var err error
		if owner != "unassigned" {
			id, err = identity.ParseUserID(owner)
			if err != nil || id == 0 {
				h.renderError(w, http.StatusBadRequest)
				return
			}
		}
		options.OwnerID = &id
	}
	items, err := h.engagement.ListActionQueue(r.Context(), actor, workspace, options)
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
	members, err := h.identity.ListMembers(r.Context(), actor)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
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
	data := viewmodel.PageData{CSRF: csrf, Workspace: workspaceByID(workspaces, workspace), Workspaces: workspaces, NavPage: "clients", NavSection: "clients", CurrentPath: "/workspaces/" + workspace.String() + "/engagements/due", ClientsEnabled: h.clients != nil, FilesEnabled: h.filesEnabled, SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle, EngagementLabel: label, CanEditClients: actor.Permissions[identity.ResourcesWrite]}
	if query.Get("conflict") == "1" {
		data.ErrorKey = "queue.conflict"
	} else if query.Get("invalid_date") == "1" {
		data.ErrorKey = "queue.invalid_date"
	}
	status := query.Get("status")
	if status != "completed" && status != "reopened" && status != "rescheduled" {
		status = ""
	}
	if query.Get("rescheduled") == "1" {
		status = "rescheduled"
	}
	h.localizePage(w, &data)
	data.Title = data.T("engagement.due_list")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	previous, next := "", ""
	pageURL := func(n int) string {
		values := url.Values{}
		if options.Filter != "" {
			values.Set("filter", string(options.Filter))
		}
		if owner != "" {
			values.Set("owner", owner)
		}
		values.Set("page", strconv.Itoa(n))
		return data.CurrentPath + "?" + values.Encode()
	}
	if page > 1 {
		previous = pageURL(page - 1)
	}
	if items.HasMore && page < 1000000 {
		next = pageURL(page + 1)
	}
	_ = dueEngagementsPage(data, items, stages, members, string(options.Filter), owner, status, previous, next, h.clientsPath(workspace)).Render(r.Context(), w)
}

func queueOperation(historyID int64) string {
	if historyID != 0 {
		return "reopen"
	}
	return "complete"
}

func (h *Handler) changeQueuedAction(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	version, err := strconv.ParseInt(r.FormValue("version"), 10, 64)
	if err != nil || version < 1 {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	id := engagement.ParseID(r.PathValue("engagementId"))
	switch r.PathValue("operation") {
	case "reschedule":
		if _, present := r.PostForm["due_date"]; !present {
			h.renderError(w, http.StatusBadRequest)
			return
		}
		err = h.engagement.RescheduleNextAction(r.Context(), actor, workspace, id, version, r.FormValue("due_date"), auditContext(r))
	case "complete":
		err = h.engagement.CompleteNextAction(r.Context(), actor, workspace, id, version, auditContext(r))
	case "reopen":
		history, parseErr := strconv.ParseInt(r.FormValue("history_id"), 10, 64)
		if parseErr != nil || history < 1 {
			h.renderError(w, http.StatusBadRequest)
			return
		}
		err = h.engagement.ReopenNextAction(r.Context(), actor, workspace, id, history, version, auditContext(r))
	default:
		h.renderError(w, http.StatusNotFound)
		return
	}
	path := "/workspaces/" + workspace.String() + "/engagements/due"
	values := url.Values{}
	filter := r.FormValue("filter")
	switch filter {
	case "overdue", "today", "upcoming", "undated", "completed":
		values.Set("filter", filter)
	default:
		if filter == "" && r.PathValue("operation") == "reopen" {
			values.Set("filter", "completed")
		}
	}
	owner := r.FormValue("owner")
	if id, parseErr := identity.ParseUserID(owner); owner == "unassigned" || (parseErr == nil && id != 0) {
		values.Set("owner", owner)
	}
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(w, http.StatusForbidden)
		return
	case errors.Is(err, engagement.ErrStaleNextAction):
		values.Set("conflict", "1")
	case errors.Is(err, engagement.ErrInvalidInput):
		values.Set("invalid_date", "1")
	case err != nil:
		h.renderError(w, http.StatusInternalServerError)
		return
	default:
		switch r.PathValue("operation") {
		case "complete":
			values.Set("status", "completed")
		case "reopen":
			values.Set("status", "reopened")
		case "reschedule":
			values.Set("rescheduled", "1")
		}
	}
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	h.redirect(w, r, path)
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
	path := h.clientsPath(workspace) + "/" + clientID.String()
	if r.FormValue("return_to") == "detail" {
		path += "/engagements/" + engagementID.String() + "?saved=1"
	} else if r.FormValue("return_to") == "board" {
		path = "/workspaces/" + workspace.String() + "/engagements/board"
		if kind := r.FormValue("kind"); kind == "won" || kind == "lost" {
			path += "?kind=" + kind
		}
	}
	h.redirect(w, r, path)
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
	stageID := engagement.ParseStageID(r.FormValue("stage"))
	if stageID == 0 {
		stageID = -1
	}
	_, err := h.engagement.Create(r.Context(), actor, workspace, engagement.Input{Name: r.FormValue("name"), ClientID: clientID, StageID: stageID, Notes: r.FormValue("notes")}, auditContext(r))
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if errors.Is(err, engagement.ErrNoOpenStage) {
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, "engagement.no_open_stage", nil)
		return
	}
	if errors.Is(err, engagement.ErrInvalidInput) {
		key := "engagement.invalid_create"
		if errors.Is(err, engagement.ErrInvalidName) {
			key = "engagement.invalid_name"
		} else if errors.Is(err, engagement.ErrInvalidStage) {
			key = "engagement.invalid_stage"
		}
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, key, nil)
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
	if r.FormValue("confirm_archive") != "yes" {
		if _, _, _, ok := h.workspaceSession(w, r, workspace, identity.WorkspaceUpdate); !ok {
			return
		}
		h.renderEngagementStages(w, r, http.StatusUnprocessableEntity, "engagement.archive_confirmation_required")
		return
	}
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
