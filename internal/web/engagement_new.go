package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func (h *Handler) newEngagement(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil || h.clients == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, session, csrf, ok := h.workspaceSession(w, r, workspace, identity.ResourcesWrite)
	if !ok {
		return
	}
	status, errorKey := http.StatusOK, ""
	clientValue := r.URL.Query().Get("client")
	pickerAction := ""
	if r.Method == http.MethodPost {
		actor, ok = h.mutationPrincipal(w, r)
		if !ok {
			return
		}
		clientValue = r.FormValue("client")
		pickerAction = r.FormValue("picker_action")
		if strings.HasPrefix(pickerAction, "select:") {
			clientValue = strings.TrimPrefix(pickerAction, "select:")
		}
		if pickerAction == "" {
			clientID := clients.ParseClientIDOrZero(clientValue)
			stageID := engagement.ParseStageID(r.FormValue("stage"))
			created, err := h.engagement.Create(r.Context(), actor, workspace, engagement.Input{Name: r.FormValue("name"), ClientID: clientID, StageID: stageID, Notes: r.FormValue("notes")}, auditContext(r))
			switch {
			case err == nil:
				h.redirect(w, r, h.clientsPath(workspace)+"/"+clientID.String()+"/engagements/"+created.ID.String())
				return
			case errors.Is(err, identity.ErrForbidden):
				h.renderError(w, http.StatusForbidden)
				return
			case errors.Is(err, engagement.ErrNoOpenStage):
				status, errorKey = http.StatusUnprocessableEntity, "engagement.no_open_stage"
			case errors.Is(err, engagement.ErrInvalidInput):
				status, errorKey = http.StatusUnprocessableEntity, "engagement.invalid_create"
				if errors.Is(err, engagement.ErrInvalidName) {
					errorKey = "engagement.invalid_name"
				}
				if errors.Is(err, engagement.ErrInvalidStage) {
					errorKey = "engagement.invalid_stage"
				}
				if errors.Is(err, engagement.ErrInvalidNotes) {
					errorKey = "engagement.invalid_notes"
				}
				if clientID == 0 && !errors.Is(err, engagement.ErrInvalidName) {
					errorKey = "engagement.invalid_client"
				}
			default:
				h.renderError(w, http.StatusInternalServerError)
				return
			}
		}
	}
	workspaces, err := h.identity.ListWorkspaces(r.Context(), session.UserID)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	stages, err := h.engagement.ListStages(r.Context(), actor, workspace)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	openStages := make([]engagement.Stage, 0, len(stages))
	for _, stage := range stages {
		if stage.Kind == "open" && !stage.Archived {
			openStages = append(openStages, stage)
		}
	}
	var selected clients.Client
	if clientValue != "" {
		selected, err = h.clients.Get(r.Context(), actor, workspace, clients.ParseClientIDOrZero(clientValue))
		if err != nil && !errors.Is(err, clients.ErrNotFound) {
			h.renderError(w, http.StatusInternalServerError)
			return
		}
		if err != nil || selected.ArchivedAt != nil {
			selected = clients.Client{}
			status, errorKey = http.StatusUnprocessableEntity, "engagement.invalid_client"
		}
	}
	pageNumber := 1
	pageValue := r.URL.Query().Get("page")
	if strings.HasPrefix(pickerAction, "page:") {
		pageValue = strings.TrimPrefix(pickerAction, "page:")
	}
	if value := pageValue; value != "" {
		pageNumber, err = strconv.Atoi(value)
		if err != nil || pageNumber < 1 || pageNumber > clients.MaxListPage {
			h.renderError(w, http.StatusBadRequest)
			return
		}
	}
	search := r.FormValue("q")
	page, err := h.clients.List(r.Context(), actor, workspace, clients.ListInput{Page: pageNumber, Limit: 25, Search: search})
	if errors.Is(err, clients.ErrInvalidInput) {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	path := "/workspaces/" + workspace.String() + "/engagements"
	prev, next := "", ""
	if pageNumber > 1 {
		prev = strconv.Itoa(pageNumber - 1)
	}
	if pageNumber < clients.MaxListPage && pageNumber*25 < page.Total {
		next = strconv.Itoa(pageNumber + 1)
	}
	data := viewmodel.PageData{CSRF: csrf, Workspace: workspaceByID(workspaces, workspace), Workspaces: workspaces, NavPage: "clients", NavSection: "clients", CurrentPath: path, ClientsEnabled: true, FilesEnabled: h.filesEnabled, SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle, CanManageFields: actor.Permissions[identity.WorkspaceUpdate], ErrorKey: errorKey, EngagementName: r.FormValue("name"), EngagementStage: r.FormValue("stage"), EngagementNotes: r.FormValue("notes"), Client: clients.Client{ID: clients.ParseClientIDOrZero(r.FormValue("from_client"))}}
	h.localizePage(w, &data)
	data.Title = data.T("engagement.create")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Header.Get("HX-Request") == "true" && r.Method == http.MethodGet && r.URL.Query().Get("part") == "client-results" {
		if len(openStages) == 0 && len(page.Clients) > 0 {
			_ = engagementNoStages(data).Render(r.Context(), w)
		} else {
			_ = engagementClientResults(data, page.Clients, search, prev, next).Render(r.Context(), w)
		}
	} else if r.Header.Get("HX-Request") == "true" && r.Method == http.MethodGet {
		_ = engagementNewDialog(data, page.Clients, selected, openStages, search, prev, next, pickerAction != "" && !strings.HasPrefix(pickerAction, "select:")).Render(r.Context(), w)
	} else {
		_ = engagementNewPage(data, page.Clients, selected, openStages, search, prev, next, pickerAction != "" && !strings.HasPrefix(pickerAction, "select:")).Render(r.Context(), w)
	}
}
