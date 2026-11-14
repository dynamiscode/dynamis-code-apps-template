package httpapi

import (
	"net/http"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func (h *handler) archiveClient(writer http.ResponseWriter, request *http.Request) {
	h.changeClient(writer, request, true)
}

func (h *handler) unarchiveClient(writer http.ResponseWriter, request *http.Request) {
	h.changeClient(writer, request, false)
}

// changeClient archives or unarchives a client. Members and above may change
// it, so the request needs write scope, as client edits do.
func (h *handler) changeClient(writer http.ResponseWriter, request *http.Request, archive bool) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(request.PathValue("clientId"))
	if len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	var (
		client clients.Client
		err    error
	)
	if archive {
		client, err = h.clients.Archive(request.Context(), principal, workspaceID, clientID, h.auditContext(request))
	} else {
		client, err = h.clients.Unarchive(request.Context(), principal, workspaceID, clientID, h.auditContext(request))
	}
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writer.Header().Set("ETag", etag(client.Version))
	writeJSON(writer, http.StatusOK, client)
}

// deleteClient permanently removes a client. Owners and admins only, so the
// request needs the workspace-update grant that field deletes need.
func (h *handler) deleteClient(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(request.PathValue("clientId"))
	if len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	if err := h.clients.Delete(request.Context(), principal, workspaceID, clientID, h.auditContext(request)); err != nil {
		h.clientError(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
