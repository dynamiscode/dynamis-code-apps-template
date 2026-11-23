package web

import (
	"errors"
	"net/http"
	"strconv"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func (h *Handler) EnableCatalog(service *catalog.Service) { h.catalogService = service }

func (h *Handler) catalogList(writer http.ResponseWriter, request *http.Request) {
	h.renderCatalog(writer, request, http.StatusOK, false)
}

func (h *Handler) catalogCreate(writer http.ResponseWriter, request *http.Request) {
	if h.catalogService == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	if !identity.CanWriteCatalog(principal) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	var duration *int
	if raw := request.FormValue("duration_minutes"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			h.renderCatalog(writer, request, http.StatusUnprocessableEntity, true)
			return
		}
		duration = &value
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	_, err := h.catalogService.Create(request.Context(), principal, workspaceID, request.FormValue("name"), duration)
	switch {
	case errors.Is(err, catalog.ErrInvalidInput):
		h.renderCatalog(writer, request, http.StatusUnprocessableEntity, true)
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(writer, http.StatusForbidden)
	case err != nil:
		h.renderError(writer, http.StatusInternalServerError)
	default:
		h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/catalog")
	}
}

func (h *Handler) catalogArchive(writer http.ResponseWriter, request *http.Request) {
	h.catalogMutate(writer, request, "archive")
}
func (h *Handler) catalogUnarchive(writer http.ResponseWriter, request *http.Request) {
	h.catalogMutate(writer, request, "unarchive")
}
func (h *Handler) catalogDelete(writer http.ResponseWriter, request *http.Request) {
	h.catalogMutate(writer, request, "delete")
}

func (h *Handler) catalogMutate(writer http.ResponseWriter, request *http.Request, action string) {
	if h.catalogService == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	entryID := catalog.ParseEntryID(request.PathValue("entryId"))
	var err error
	if action == "delete" {
		err = h.catalogService.Delete(request.Context(), principal, workspaceID, entryID)
	} else {
		err = h.catalogService.SetArchived(request.Context(), principal, workspaceID, entryID, action == "archive")
	}
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(writer, http.StatusForbidden)
	case errors.Is(err, catalog.ErrNotFound):
		h.renderError(writer, http.StatusNotFound)
	case errors.Is(err, catalog.ErrInUse):
		h.renderCatalog(writer, request, http.StatusConflict, false)
	case err != nil:
		h.renderError(writer, http.StatusInternalServerError)
	default:
		h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/catalog")
	}
}

func (h *Handler) renderCatalog(writer http.ResponseWriter, request *http.Request, status int, invalid bool) {
	if h.catalogService == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	entries, err := h.catalogService.List(request.Context(), principal, workspaceID, request.URL.Query().Get("includeArchived") == "true")
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	data := viewmodel.PageData{CSRF: csrf, Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces, NavPage: "workspace", NavSection: "workspace", SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle, FilesEnabled: h.filesEnabled, ClientsEnabled: h.clientsEnabled}
	h.localizePage(writer, &data)
	data.Title = data.T("catalog.title")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = catalogPage(data, entries, identity.CanWriteCatalog(principal), invalid, status == http.StatusConflict, request.URL.Query().Get("includeArchived") == "true", request.FormValue("name"), request.FormValue("duration_minutes")).Render(request.Context(), writer)
}
