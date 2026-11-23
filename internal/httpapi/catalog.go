package httpapi

import (
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func (h *handler) listCatalog(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	entries, err := h.catalogService.List(request.Context(), principal, workspaceID)
	if errors.Is(err, identity.ErrForbidden) {
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
		return
	}
	if err != nil {
		h.internal(writer, request)
		return
	}
	writeJSON(writer, http.StatusOK, entries)
}
