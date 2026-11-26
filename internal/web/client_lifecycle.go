package web

import (
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"net/http"
)

func (h *Handler) archiveClient(writer http.ResponseWriter, request *http.Request) {
	h.changeClientArchive(writer, request, true)
}

func (h *Handler) unarchiveClient(writer http.ResponseWriter, request *http.Request) {
	h.changeClientArchive(writer, request, false)
}

// changeClientArchive archives or unarchives a client from its detail page and
// redirects back to it. Members and above may do either.
func (h *Handler) changeClientArchive(writer http.ResponseWriter, request *http.Request, archive bool) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(request.PathValue("clientId"))
	var err error
	if archive {
		_, err = h.clients.Archive(request.Context(), principal, workspaceID, clientID, auditContext(request))
	} else {
		_, err = h.clients.Unarchive(request.Context(), principal, workspaceID, clientID, auditContext(request))
	}
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	h.redirect(writer, request, h.clientsPath(workspaceID)+"/"+clientID.String())
}

// deleteClient permanently removes a client. Owners and admins only. The page
// shows the control behind a confirmation, and the redirect goes to the list.
func (h *Handler) deleteClient(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(request.PathValue("clientId"))
	if request.FormValue("confirm_delete") != "yes" {
		h.renderClientDeleteConfirmation(writer, request, []clients.ClientID{clientID}, h.clientsPath(workspaceID)+"/"+clientID.String(), false)
		return
	}
	if err := h.clients.Delete(request.Context(), principal, workspaceID, clientID, auditContext(request)); err != nil {
		h.clientError(writer, request, err)
		return
	}
	h.redirect(writer, request, h.clientsPath(workspaceID))
}
