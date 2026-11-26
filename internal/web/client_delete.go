package web

import (
	"net/http"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"github.com/a-h/templ"
)

func (h *Handler) renderClientDeleteConfirmation(writer http.ResponseWriter, request *http.Request, ids []clients.ClientID, back string, bulk bool) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	selected := make([]clients.Client, 0, len(ids))
	for _, id := range ids {
		client, err := h.clients.Get(request.Context(), principal, workspaceID, id)
		if err != nil {
			h.clientError(writer, request, err)
			return
		}
		selected = append(selected, client)
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	data := viewmodel.PageData{
		CSRF: csrf, Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		CurrentPath: h.clientsPath(workspaceID), NavPage: "clients", NavSection: "clients",
	}
	h.renderClientPage(writer, request, http.StatusOK, func(data viewmodel.PageData) templ.Component {
		return clientDeletePage(data, request.URL.Path, back, selected, bulk)
	}, data)
}
