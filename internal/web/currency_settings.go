package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"github.com/a-h/templ"
)

type currencySettingsRow struct {
	Code, Decimals  string
	Default, Remove bool
}

func (h *Handler) currencySettingsPage(writer http.ResponseWriter, request *http.Request) {
	if h.catalogService == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	currencies, err := h.catalogService.ListCurrencies(request.Context(), principal, workspaceID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	rows := make([]currencySettingsRow, 0, len(currencies))
	for _, c := range currencies {
		rows = append(rows, currencySettingsRow{Code: c.Code, Decimals: strconv.Itoa(c.Decimals), Default: c.Default})
	}
	h.renderCurrencySettings(writer, request, workspaceID, principal, session, csrf, rows, "", "", "", http.StatusOK)
}

func (h *Handler) currencySettingsSave(writer http.ResponseWriter, request *http.Request) {
	if h.catalogService == nil {
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
	rows := make([]currencySettingsRow, 0, len(request.Form["code"]))
	values := make([]catalog.Currency, 0, len(request.Form["code"])+1)
	invalid := false
	for _, code := range request.Form["code"] {
		row := currencySettingsRow{Code: code, Decimals: request.FormValue("decimals_" + code), Default: request.FormValue("default") == code, Remove: request.FormValue("remove_"+code) == "1"}
		rows = append(rows, row)
		if row.Remove {
			continue
		}
		decimals, err := strconv.Atoi(row.Decimals)
		if err != nil {
			invalid = true
		}
		values = append(values, catalog.Currency{Code: code, Decimals: decimals, Default: row.Default})
	}
	newCode, newDecimals := strings.TrimSpace(request.FormValue("new_code")), request.FormValue("new_decimals")
	if newCode != "" {
		decimals, err := strconv.Atoi(newDecimals)
		if err != nil {
			invalid = true
		}
		values = append(values, catalog.Currency{Code: newCode, Decimals: decimals, Default: request.FormValue("default") == "__new"})
	} else if request.FormValue("default") == "__new" {
		invalid = true
	}
	var err error
	if invalid {
		err = catalog.ErrInvalidInput
	} else {
		err = h.catalogService.SetCurrencies(request.Context(), principal, workspaceID, values)
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
	if errors.Is(err, catalog.ErrInUse) {
		message = "currency.in_use"
	} else if !errors.Is(err, catalog.ErrInvalidInput) {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.renderCurrencySettings(writer, request, workspaceID, principal, session, csrf, rows, newCode, newDecimals, message, http.StatusUnprocessableEntity)
}

func (h *Handler) renderCurrencySettings(writer http.ResponseWriter, request *http.Request, workspaceID identity.WorkspaceID, principal identity.Principal, session identity.Session, csrf string, rows []currencySettingsRow, newCode, newDecimals, message string, status int) {
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	data := viewmodel.PageData{Title: "Currencies", NavPage: "general", NavSection: "settings", Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces, CSRF: csrf, Saved: request.URL.Query().Get("saved") == "1"}
	h.renderSettingsPage(writer, request, status, data, func(data viewmodel.PageData) templ.Component {
		return currencySettingsView(data, rows, newCode, newDecimals, message, request.FormValue("default") == "__new", identity.CanWriteCatalog(principal))
	})
}
