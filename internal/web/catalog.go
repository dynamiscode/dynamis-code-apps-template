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
	var amount *string
	if value := request.FormValue("amount"); value != "" {
		amount = &value
	}
	price := catalog.PriceOption{Mode: request.FormValue("mode"), Amount: amount, Currency: request.FormValue("currency"), Variant: request.FormValue("variant"), Eligibility: request.FormValue("eligibility")}
	_, err := h.catalogService.Create(request.Context(), principal, workspaceID, request.FormValue("name"), duration, []catalog.PriceOption{price})
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

func catalogDuration(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func catalogAmount(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (h *Handler) catalogEdit(writer http.ResponseWriter, request *http.Request) {
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
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	var duration *int
	if raw := request.FormValue("duration_minutes"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			h.renderCatalog(writer, request, http.StatusUnprocessableEntity, true)
			return
		}
		duration = &value
	}
	err := h.catalogService.Update(request.Context(), principal, workspaceID, catalog.ParseEntryID(request.PathValue("entryId")), request.FormValue("name"), duration)
	h.catalogEditResult(writer, request, workspaceID, err)
}

func (h *Handler) catalogEditPrice(writer http.ResponseWriter, request *http.Request) {
	h.catalogChangePrice(writer, request, false)
}

func (h *Handler) catalogDeletePrice(writer http.ResponseWriter, request *http.Request) {
	h.catalogChangePrice(writer, request, true)
}

func (h *Handler) catalogChangePrice(writer http.ResponseWriter, request *http.Request, remove bool) {
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
	priceID := catalog.ParsePriceID(request.PathValue("priceId"))
	var err error
	if remove {
		err = h.catalogService.DeletePrice(request.Context(), principal, workspaceID, entryID, priceID)
	} else {
		var amount *string
		if value := request.FormValue("amount"); value != "" {
			amount = &value
		}
		option := catalog.PriceOption{Mode: request.FormValue("mode"), Amount: amount, Currency: request.FormValue("currency"), Variant: request.FormValue("variant"), Eligibility: request.FormValue("eligibility")}
		err = h.catalogService.UpdatePrice(request.Context(), principal, workspaceID, entryID, priceID, option)
	}
	h.catalogEditResult(writer, request, workspaceID, err)
}

func (h *Handler) catalogEditResult(writer http.ResponseWriter, request *http.Request, workspaceID identity.WorkspaceID, err error) {
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(writer, http.StatusForbidden)
	case errors.Is(err, catalog.ErrNotFound):
		h.renderError(writer, http.StatusNotFound)
	case errors.Is(err, catalog.ErrInvalidInput):
		h.renderCatalog(writer, request, http.StatusUnprocessableEntity, true)
	case errors.Is(err, catalog.ErrLastPrice):
		h.renderCatalog(writer, request, http.StatusConflict, true)
	case err != nil:
		h.renderError(writer, http.StatusInternalServerError)
	default:
		h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/catalog")
	}
}

func (h *Handler) catalogCreatePrice(writer http.ResponseWriter, request *http.Request) {
	if h.catalogService == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	var amount *string
	if value := request.FormValue("amount"); value != "" {
		amount = &value
	}
	option := catalog.PriceOption{Mode: request.FormValue("mode"), Amount: amount, Currency: request.FormValue("currency"), Variant: request.FormValue("variant"), Eligibility: request.FormValue("eligibility")}
	_, err := h.catalogService.AddPrice(request.Context(), principal, workspaceID, request.PathValue("entryId"), option)
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
