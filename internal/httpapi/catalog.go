package httpapi

import (
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func (h *handler) createCatalog(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	var body struct {
		Name            string `json:"name"`
		DurationMinutes *int   `json:"durationMinutes"`
	}
	if err := decodeJSON(request, &body); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	entry, err := h.catalogService.Create(request.Context(), principal, workspaceID, body.Name, body.DurationMinutes)
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, catalog.ErrInvalidInput):
		writeProblem(writer, request, http.StatusUnprocessableEntity, "invalid-input", "Catalog entry is invalid.")
	case err != nil:
		h.internal(writer, request)
	default:
		writeJSON(writer, http.StatusCreated, entry)
	}
}

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
