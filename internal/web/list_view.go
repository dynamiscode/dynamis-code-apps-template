package web

import (
	"net/http"
	"net/url"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/listprefs"
)

type clientListColumn struct {
	Key, Label, LabelKey, SortHref string
	ValueType                      string
	Visible, DefaultVisible        bool
	SortDirection                  string
}

type clientListRow struct {
	ID             string
	Values         map[string]string
	Item           items.Item
	Member         identity.MemberSummary
	ActionContext  string
	ActionPosition int
}

type clientFilterChip struct {
	FieldLabel       string
	FieldLabelKey    string
	OperatorLabelKey string
	Value            string
	RemoveHref       string
	Hidden           bool
}

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
	copy.Del("cursor")
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

func (p pageData) ClientListCellValue(column clientListColumn, row clientListRow) string {
	value := row.Values[column.Key]
	if p.IsItemList && column.Key == "status" {
		if value == string(items.Active) {
			return p.T("items.active")
		}
		if value == string(items.Complete) {
			return p.T("items.complete")
		}
	}
	if column.ValueType == "yes_no" {
		if value == "true" {
			return p.T("clients.yes")
		}
		if value == "false" {
			return p.T("clients.no")
		}
	}
	if p.IsMembersList && column.Key == "role" {
		return p.Role(identity.Role(value))
	}
	return value
}
