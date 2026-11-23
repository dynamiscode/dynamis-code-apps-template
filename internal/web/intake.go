package web

import (
	"errors"
	"net/http"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/intake"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func (h *Handler) EnableIntake(service *intake.Service) { h.intake = service }

func (h *Handler) intakePage(w http.ResponseWriter, r *http.Request) {
	h.renderIntake(w, r, http.StatusOK, "", "")
}

func (h *Handler) renderIntake(w http.ResponseWriter, r *http.Request, status int, message, token string) {
	if h.intake == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	actor, session, csrf, ok := h.workspaceSession(w, r, workspace, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	settings, err := h.intake.GetSettings(r.Context(), actor, workspace)
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(r.Context(), session.UserID)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	path := "/workspaces/" + workspace.String() + "/intake"
	data := viewmodel.PageData{Title: "Public intake", CSRF: csrf, Workspace: workspaceByID(workspaces, workspace), Workspaces: workspaces, NavPage: "clients", NavSection: "clients", CurrentPath: path, FilesEnabled: h.filesEnabled, ClientsEnabled: h.clients != nil, SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle}
	h.localizePage(w, &data)
	link := ""
	if token != "" {
		link = strings.TrimRight(h.publicURL, "/") + "/intake/" + token
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = intakeSettingsPage(data, settings, message, link).Render(r.Context(), w)
}

func (h *Handler) intakeMutation(w http.ResponseWriter, r *http.Request) {
	if h.intake == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	var token string
	var err error
	switch r.FormValue("action") {
	case "create":
		token, err = h.intake.Create(r.Context(), actor, workspace, r.FormValue("consent"), auditContext(r))
	case "rotate":
		token, err = h.intake.Rotate(r.Context(), actor, workspace, r.FormValue("consent"), auditContext(r))
	case "disable":
		err = h.intake.Disable(r.Context(), actor, workspace, auditContext(r))
	case "consent":
		err = h.intake.SetConsent(r.Context(), actor, workspace, r.FormValue("consent"), auditContext(r))
	default:
		h.renderError(w, http.StatusBadRequest)
		return
	}
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if errors.Is(err, intake.ErrInvalidInput) {
		h.renderIntake(w, r, http.StatusUnprocessableEntity, "intake.invalid", "")
		return
	}
	if errors.Is(err, intake.ErrActive) || errors.Is(err, intake.ErrNotFound) {
		h.renderIntake(w, r, http.StatusUnprocessableEntity, "intake.conflict", "")
		return
	}
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	if token != "" {
		h.renderIntake(w, r, http.StatusOK, "", token)
		return
	}
	h.redirect(w, r, "/workspaces/"+workspace.String()+"/intake")
}
