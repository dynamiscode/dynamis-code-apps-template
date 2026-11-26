package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"example.com/dynamis-code/apps-template/internal/currency"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"github.com/a-h/templ"
)

type currencySettingsRow struct {
	Code, Decimals                         string
	Default, Remove, Legacy, LegacyDefault bool
}

func (h *Handler) currencySettingsPage(writer http.ResponseWriter, request *http.Request) {
	if h.currencyService == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	currencies, err := h.currencyService.ListCurrencies(request.Context(), principal, workspaceID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	rows := make([]currencySettingsRow, 0, len(currencies))
	for _, c := range currencies {
		canonical, ok := currency.Canonical(c.Code)
		legacy := !ok || canonical.Decimals != c.Decimals
		rows = append(rows, currencySettingsRow{Code: c.Code, Decimals: strconv.Itoa(c.Decimals), Default: c.Default, Legacy: legacy, LegacyDefault: legacy && c.Default})
	}
	h.renderCurrencySettings(writer, request, workspaceID, principal, session, csrf, rows, "", "", request.URL.Query().Get("q"), "", http.StatusOK)
}

func (h *Handler) currencySettingsSave(writer http.ResponseWriter, request *http.Request) {
	if h.currencyService == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.managementPrincipal(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	if !identity.CanWriteCatalog(principal) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	current, err := h.currencyService.ListCurrencies(request.Context(), principal, workspaceID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	rows := make([]currencySettingsRow, 0, len(current))
	values := make([]currency.Currency, 0, len(current)+1)
	defaultCode := request.FormValue("default")
	for _, c := range current {
		canonical, ok := currency.Canonical(c.Code)
		legacy := !ok || canonical.Decimals != c.Decimals
		row := currencySettingsRow{Code: c.Code, Decimals: strconv.Itoa(c.Decimals), Default: defaultCode == c.Code, Remove: request.FormValue("remove_"+c.Code) == "1", Legacy: legacy, LegacyDefault: legacy && c.Default}
		rows = append(rows, row)
		if !row.Remove {
			c.Default = row.Default
			values = append(values, c)
		}
	}
	newCode := request.FormValue("new_code")
	found := newCode == ""
	for _, c := range currency.Available() {
		if c.Code == newCode {
			c.Default = defaultCode == "__new"
			values = append(values, c)
			found = true
			break
		}
	}
	if !found || (newCode == "" && defaultCode == "__new") {
		err = currency.ErrInvalidInput
	} else {
		err = h.currencyService.SetCurrencies(request.Context(), principal, workspaceID, values)
	}
	if err == nil {
		h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/settings/currencies?saved=1")
		return
	}
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	message := "currency.invalid"
	if errors.Is(err, currency.ErrInUse) {
		message = "currency.in_use"
	} else if !errors.Is(err, currency.ErrInvalidInput) {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.renderCurrencySettings(writer, request, workspaceID, principal, session, csrf, rows, newCode, defaultCode, request.FormValue("q"), message, http.StatusUnprocessableEntity)
}

func (h *Handler) renderCurrencySettings(writer http.ResponseWriter, request *http.Request, workspaceID identity.WorkspaceID, principal identity.Principal, session identity.Session, csrf string, rows []currencySettingsRow, newCode, defaultCode, query, message string, status int) {
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	enabled := make(map[string]bool, len(rows))
	for _, row := range rows {
		enabled[row.Code] = true
	}
	query = strings.TrimSpace(query)
	var choices []currency.Currency
	for _, c := range currency.Available() {
		if !enabled[c.Code] && (strings.Contains(strings.ToLower(c.Code), strings.ToLower(query)) || c.Code == newCode) {
			choices = append(choices, c)
		}
	}
	data := viewmodel.PageData{Title: "Currencies", NavPage: "currencies", NavSection: "settings", Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces, CSRF: csrf, Saved: request.URL.Query().Get("saved") == "1"}
	h.renderSettingsPage(writer, request, status, data, func(data viewmodel.PageData) templ.Component {
		return currencySettingsView(data, rows, choices, newCode, query, message, defaultCode == "__new", identity.CanWriteCatalog(principal))
	})
}
