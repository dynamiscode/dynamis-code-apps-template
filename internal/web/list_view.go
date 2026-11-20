package web

import (
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/http"
	"net/url"
	"slices"
)

func hasVisibleClientColumns(visible map[string]bool) bool {
	for _, shown := range visible {
		if shown {
			return true
		}
	}
	return false
}

func (h *Handler) EnableItemsPreferences(preferences *listprefs.Service) {
	h.listPreferences = preferences
}

func clientQuerySuffix(query url.Values) string {
	copy := cloneClientQuery(query)
	copy.Del("page")
	if encoded := copy.Encode(); encoded != "" {
		return "?" + encoded
	}
	return ""
}

func cloneClientQuery(query url.Values) url.Values {
	copy := make(url.Values, len(query))
	for key, values := range query {
		copy[key] = append([]string(nil), values...)
	}
	// The results-fragment markers belong to one request. A link built from it must not carry them into a page.
	delete(copy, listPartKey)
	delete(copy, listFullKey)
	return copy
}

func (h *Handler) clientPreferencePrincipal(writer http.ResponseWriter, request *http.Request) (identity.Principal, bool) {
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return identity.Principal{}, false
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, _, ok := h.workspaceSession(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return identity.Principal{}, false
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return identity.Principal{}, false
	}
	return principal, true
}

// clientPreservedQuery returns the list parameters that every list form keeps, so one control
// does not reset the others. The page and the add-filter inputs are left out.
func clientPreservedQuery(query url.Values) []viewmodel.ClientHiddenField {
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	var fields []viewmodel.ClientHiddenField
	for _, key := range keys {
		switch key {
		case "page", "filter_field", "filter_op", "filter_value", "filter_value_to", "filter_error", listPartKey, listFullKey:
			continue
		}
		for _, value := range query[key] {
			fields = append(fields, viewmodel.ClientHiddenField{Name: key, Value: value})
		}
	}
	return fields
}

func columnVisibilityKey(visible bool) string {
	if visible {
		return "clients.column_hide"
	}
	return "clients.column_show"
}
