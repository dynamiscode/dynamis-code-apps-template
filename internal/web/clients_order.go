package web

import (
	"net/http"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// setClientColumnOrder saves the Clients column order from the columns menu. Client fields are part of the order,
// so the declared columns are read from the workspace's fields.
func (h *Handler) setClientColumnOrder(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	h.storeColumnOrder(writer, request, listprefs.ListClients,
		func(principal identity.Principal, workspaceID identity.WorkspaceID) ([]viewmodel.ListColumn, error) {
			fields, err := h.clients.ListFields(request.Context(), principal, workspaceID, clients.ListFieldsInput{})
			if err != nil {
				return nil, err
			}
			return clientColumns(fields), nil
		},
		h.clientsPath(workspaceID)+clientQuerySuffix(request.URL.Query()),
		h.rerenderClients)
}

func (h *Handler) rerenderClients(writer http.ResponseWriter, request *http.Request) {
	h.renderClientList(writer, request, http.StatusOK, "", clients.Input{})
}
