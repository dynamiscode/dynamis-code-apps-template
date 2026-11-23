package web

import (
	"net/http"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func (h *Handler) catalogList(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	if _, err := h.catalogService.List(request.Context(), principal, workspaceID); err != nil {
		h.renderError(writer, http.StatusForbidden)
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
	_ = catalogPage(data).Render(request.Context(), writer)
}
