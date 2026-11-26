package web

import (
	"net/http"
	"strings"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
)

// bulkClients archives or deletes the selected clients in one transaction, then returns to the list. Any failure
// changes nothing. The form's return field is the list address with its query; it must stay under the clients list.
func (h *Handler) bulkClients(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	listPath := h.clientsPath(workspaceID)
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	var ids []clients.ClientID
	for _, raw := range request.Form["client_id"] {
		id := clients.ParseClientIDOrZero(raw)
		if id == 0 {
			h.renderError(writer, http.StatusBadRequest)
			return
		}
		ids = append(ids, id)
	}
	back := listPath
	if ret := request.FormValue("return"); ret == listPath || strings.HasPrefix(ret, listPath+"?") {
		back = ret
	}
	if action := request.FormValue("action"); action != "archive" && action != "delete" {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	if len(ids) == 0 {
		h.redirect(writer, request, back)
		return
	}
	if request.FormValue("action") == "delete" && request.FormValue("confirm_delete") != "yes" {
		h.renderClientDeleteConfirmation(writer, request, ids, back, true)
		return
	}
	audit := auditContext(request)
	var err error
	switch request.FormValue("action") {
	case "archive":
		err = h.clients.BulkArchive(request.Context(), principal, workspaceID, ids, audit, true)
	case "delete":
		err = h.clients.BulkDelete(request.Context(), principal, workspaceID, ids, audit)
	default:
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	h.redirect(writer, request, back)
}
