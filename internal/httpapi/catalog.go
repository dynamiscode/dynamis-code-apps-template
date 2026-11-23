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
		Name            string                `json:"name"`
		DurationMinutes *int                  `json:"durationMinutes"`
		Prices          []catalog.PriceOption `json:"prices"`
	}
	if err := decodeJSON(request, &body); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	entry, err := h.catalogService.Create(request.Context(), principal, workspaceID, body.Name, body.DurationMinutes, body.Prices)
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

func (h *handler) archiveCatalog(writer http.ResponseWriter, request *http.Request) {
	h.mutateCatalog(writer, request, true)
}
func (h *handler) unarchiveCatalog(writer http.ResponseWriter, request *http.Request) {
	h.mutateCatalog(writer, request, false)
}
func (h *handler) deleteCatalog(writer http.ResponseWriter, request *http.Request) {
	h.mutateCatalog(writer, request, false)
}

func (h *handler) mutateCatalog(writer http.ResponseWriter, request *http.Request, archived bool) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	entryID := catalog.ParseEntryID(request.PathValue("entryId"))
	var err error
	switch request.Method {
	case http.MethodDelete:
		err = h.catalogService.Delete(request.Context(), principal, workspaceID, entryID)
	default:
		err = h.catalogService.SetArchived(request.Context(), principal, workspaceID, entryID, archived)
	}
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, catalog.ErrNotFound):
		writeProblem(writer, request, http.StatusNotFound, "not-found", "Catalog entry not found.")
	case errors.Is(err, catalog.ErrInUse):
		writeProblem(writer, request, http.StatusConflict, "in-use", "Catalog entry is in use; archive it instead.")
	case err != nil:
		h.internal(writer, request)
	default:
		writer.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) listCatalog(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	entries, err := h.catalogService.List(request.Context(), principal, workspaceID, request.URL.Query().Get("includeArchived") == "true")
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

func (h *handler) updateCatalog(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	if !identity.CanWriteCatalog(principal) {
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
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
	err := h.catalogService.Update(request.Context(), principal, workspaceID, catalog.ParseEntryID(request.PathValue("entryId")), body.Name, body.DurationMinutes)
	h.catalogEditResult(writer, request, err)
}

func (h *handler) editCatalogPrice(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	if !identity.CanWriteCatalog(principal) {
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
		return
	}
	entryID := catalog.ParseEntryID(request.PathValue("entryId"))
	priceID := catalog.ParsePriceID(request.PathValue("priceId"))
	var err error
	if request.Method == http.MethodDelete {
		err = h.catalogService.DeletePrice(request.Context(), principal, workspaceID, entryID, priceID)
	} else {
		var option catalog.PriceOption
		if err := decodeJSON(request, &option); err != nil {
			h.badJSON(writer, request, err)
			return
		}
		err = h.catalogService.UpdatePrice(request.Context(), principal, workspaceID, entryID, priceID, option)
	}
	h.catalogEditResult(writer, request, err)
}

func (h *handler) catalogEditResult(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, catalog.ErrInvalidInput):
		writeProblem(writer, request, http.StatusUnprocessableEntity, "invalid-input", "Catalog entry or price is invalid.")
	case errors.Is(err, catalog.ErrNotFound):
		writeProblem(writer, request, http.StatusNotFound, "not-found", "Catalog entry or price not found.")
	case errors.Is(err, catalog.ErrLastPrice):
		writeProblem(writer, request, http.StatusConflict, "last-price", "Catalog entry must have at least one price.")
	case err != nil:
		h.internal(writer, request)
	default:
		writer.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) createCatalogPrice(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	var option catalog.PriceOption
	if err := decodeJSON(request, &option); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	price, err := h.catalogService.AddPrice(request.Context(), principal, workspaceID, request.PathValue("entryId"), option)
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, catalog.ErrInvalidInput):
		writeProblem(writer, request, http.StatusUnprocessableEntity, "invalid-input", "Catalog price is invalid.")
	case err != nil:
		h.internal(writer, request)
	default:
		writeJSON(writer, http.StatusCreated, price)
	}
}
