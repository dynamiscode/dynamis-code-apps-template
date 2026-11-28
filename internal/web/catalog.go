package web

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/currency"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

type catalogForm struct {
	kind, entryID, priceID string
	name, duration         string
	field, value, version  string
	conflict               bool
	searching              bool
	currencySearch         string
	price                  catalog.PriceOption
}

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
	if request.FormValue("picker_action") == "currency" {
		h.renderCatalog(writer, request, http.StatusOK, false)
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
	variant := strings.TrimSpace(request.FormValue("variant"))
	if variant == "" {
		variant = "Standard"
	}
	mode := request.FormValue("mode")
	if mode == "" {
		mode = "manual"
		if amount != nil {
			mode = "fixed"
		}
	}
	price := catalog.PriceOption{Mode: mode, Amount: amount, Currency: request.FormValue("currency"), Variant: variant, Eligibility: request.FormValue("eligibility")}
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
	includeArchived := request.URL.Query().Get("includeArchived") == "true"
	entryID := request.URL.Query().Get("entry")
	if request.PathValue("entryId") != "" && (status != http.StatusOK || request.FormValue("picker_action") == "currency") {
		entryID = request.PathValue("entryId")
	}
	entries, err := h.catalogService.List(request.Context(), principal, workspaceID, true)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	emptyCatalog := len(entries) == 0
	hasArchived := false
	for _, entry := range entries {
		if entry.Archived {
			hasArchived = true
			break
		}
	}
	filtered := make([]catalog.Entry, 0, len(entries))
	for _, entry := range entries {
		if entryID != "" && entry.ID.String() != entryID {
			continue
		}
		if entryID == "" && entry.Archived != includeArchived {
			continue
		}
		filtered = append(filtered, entry)
	}
	if len(filtered) == 0 && entryID != "" && (status != http.StatusOK || request.FormValue("picker_action") == "currency") {
		entryID = ""
		for _, entry := range entries {
			if !entry.Archived {
				filtered = append(filtered, entry)
			}
		}
	}
	entries = filtered
	sortField := request.URL.Query().Get("sort")
	descending := request.URL.Query().Get("direction") == "desc"
	if sortField == "name" || sortField == "duration" {
		sort.SliceStable(entries, func(i, j int) bool {
			comparison := 0
			if sortField == "name" {
				comparison = strings.Compare(strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name))
			} else if entries[i].DurationMinutes == nil && entries[j].DurationMinutes != nil {
				comparison = -1
			} else if entries[i].DurationMinutes != nil && entries[j].DurationMinutes == nil {
				comparison = 1
			} else if entries[i].DurationMinutes != nil && entries[j].DurationMinutes != nil {
				if *entries[i].DurationMinutes < *entries[j].DurationMinutes {
					comparison = -1
				} else if *entries[i].DurationMinutes > *entries[j].DurationMinutes {
					comparison = 1
				}
			}
			if descending {
				return comparison > 0
			}
			return comparison < 0
		})
	}
	currencies, err := h.currencyService.ListCurrencies(request.Context(), principal, workspaceID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	available := currencies[:0]
	for _, configured := range currencies {
		if canonical, ok := currency.Canonical(configured.Code); ok && canonical.Decimals == configured.Decimals {
			available = append(available, configured)
		}
	}
	currencies = available
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	data := viewmodel.PageData{CSRF: csrf, Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces, NavPage: "catalog", NavSection: "workspace", SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle, FilesEnabled: h.filesEnabled, ClientsEnabled: h.clientsEnabled, ListLive: true}
	data.ListLive = false
	listPath := "/workspaces/" + workspaceID.String() + "/catalog"
	query := request.URL.Query()
	query.Del("entry")
	query.Del("create")
	data.Filter.Path = listPath
	for name, values := range query {
		if name != "includeArchived" {
			for _, value := range values {
				data.Filter.Preserved = append(data.Filter.Preserved, viewmodel.ClientHiddenField{Name: name, Value: value})
			}
		}
	}
	if encoded := query.Encode(); encoded != "" {
		data.ClientQuerySuffix = "?" + encoded
	}
	for _, column := range []viewmodel.ListColumn{{Key: "name", LabelKey: "catalog.name", Visible: true}, {Key: "duration", LabelKey: "catalog.duration_label", Visible: true}, {Key: "prices", LabelKey: "catalog.prices", Visible: true}} {
		if column.Key != "prices" {
			direction := "asc"
			if sortField == column.Key {
				column.SortDirection = "ascending"
				if !descending {
					direction = "desc"
				} else {
					column.SortDirection = "descending"
				}
			}
			query.Set("sort", column.Key)
			query.Set("direction", direction)
			column.SortHref = listPath + "?" + query.Encode()
		}
		data.ListColumns = append(data.ListColumns, column)
	}
	h.localizePage(writer, &data)
	data.Title = data.T("catalog.title")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	form := catalogForm{}
	if request.Method == http.MethodPost && (request.FormValue("picker_action") == "currency" || status == http.StatusUnprocessableEntity || (status == http.StatusConflict && (request.FormValue("field") != "" || strings.HasSuffix(request.URL.Path, "/edit")))) {
		form.field = request.FormValue("field")
		form.value = request.FormValue("value")
		form.version = request.FormValue("version")
		form.conflict = status == http.StatusConflict
		form.searching = request.FormValue("picker_action") == "currency"
		form.currencySearch = request.FormValue("currency_search")
		form.entryID = request.PathValue("entryId")
		form.priceID = request.PathValue("priceId")
		switch {
		case form.field != "":
			form.kind = "inline"
		case form.entryID == "":
			form.kind = "create"
		case form.priceID != "":
			form.kind = "edit-price"
		case strings.HasSuffix(request.URL.Path, "/prices"):
			form.kind = "add-price"
		default:
			form.kind = "edit-entry"
		}
		form.name, form.duration = request.FormValue("name"), request.FormValue("duration_minutes")
		amount := request.FormValue("amount")
		form.price = catalog.PriceOption{Mode: request.FormValue("mode"), Amount: &amount, Currency: request.FormValue("currency"), Variant: request.FormValue("variant"), Eligibility: request.FormValue("eligibility")}
	}
	inUse := status == http.StatusConflict && form.kind != "inline" && form.kind != "edit-entry"
	if request.Header.Get("HX-Request") == "true" && request.Header.Get("HX-History-Restore-Request") != "true" && request.Method == http.MethodGet {
		_ = catalogContent(data, entries, currencies, identity.CanWriteCatalog(principal), invalid, inUse, includeArchived, emptyCatalog, hasArchived, entryID, false, form).Render(request.Context(), writer)
		return
	}
	_ = catalogPage(data, entries, currencies, identity.CanWriteCatalog(principal), invalid, inUse, includeArchived, emptyCatalog, hasArchived, entryID, request.URL.Query().Get("create") == "true", form).Render(request.Context(), writer)
}

func catalogEntryHref(data viewmodel.PageData, entryID string) string {
	href := data.Filter.Path + "?entry=" + entryID
	if data.ClientQuerySuffix != "" {
		href += "&" + strings.TrimPrefix(data.ClientQuerySuffix, "?")
	}
	return href
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
	revision, err := strconv.ParseInt(request.FormValue("version"), 10, 64)
	if err != nil || revision <= 0 {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	if field := request.FormValue("field"); strings.HasSuffix(request.URL.Path, "/field") || field != "" {
		if (field != "name" && field != "duration_minutes") || !request.PostForm.Has("value") || !request.PostForm.Has("version") {
			h.renderError(writer, http.StatusBadRequest)
			return
		}
		err := h.catalogService.UpdateField(request.Context(), principal, workspaceID, catalog.ParseEntryID(request.PathValue("entryId")), field, request.FormValue("value"), revision)
		if errors.Is(err, catalog.ErrConflict) {
			h.renderCatalog(writer, request, http.StatusConflict, false)
			return
		}
		h.catalogEditResult(writer, request, workspaceID, err)
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
	err = h.catalogService.Update(request.Context(), principal, workspaceID, catalog.ParseEntryID(request.PathValue("entryId")), request.FormValue("name"), duration, revision)
	if errors.Is(err, catalog.ErrConflict) {
		h.renderCatalog(writer, request, http.StatusConflict, false)
		return
	}
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
	if !remove && request.FormValue("picker_action") == "currency" {
		if !identity.CanWriteCatalog(principal) {
			h.renderError(writer, http.StatusForbidden)
			return
		}
		h.renderCatalog(writer, request, http.StatusOK, false)
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
		h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/catalog?entry="+request.PathValue("entryId"))
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
	if request.FormValue("picker_action") == "currency" {
		if !identity.CanWriteCatalog(principal) {
			h.renderError(writer, http.StatusForbidden)
			return
		}
		h.renderCatalog(writer, request, http.StatusOK, false)
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
		h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/catalog?entry="+request.PathValue("entryId"))
	}
}
