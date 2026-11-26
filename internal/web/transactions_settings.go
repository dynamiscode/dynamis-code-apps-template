package web

import (
	"net/http"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/transactions"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"github.com/a-h/templ"
)

func (h *Handler) transactionsSettingsPage(w http.ResponseWriter, r *http.Request) {
	h.renderTransactionsSettings(w, r, http.StatusOK, transactionsDraft{}, "")
}

func (h *Handler) renderTransactionsSettings(w http.ResponseWriter, r *http.Request, status int, draft transactionsDraft, methodError string) {
	if h.transactions == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(w, r, workspace, identity.ResourcesWrite)
	if !ok {
		return
	}
	if !identity.CanManageTransactions(principal) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	page := "transactions-settings"
	if strings.Contains(r.URL.Path, "/transactions/categories") {
		page = "transactions-categories"
	} else if strings.Contains(r.URL.Path, "/transactions/methods") {
		page = "transactions-methods"
	}
	var categories []transactions.Category
	var methods []transactions.Method
	var err error
	switch page {
	case "transactions-categories":
		categories, err = h.transactions.Categories(r.Context(), principal, workspace)
	case "transactions-methods":
		methods, err = h.transactions.ListMethods(r.Context(), principal, workspace)
	}
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(r.Context(), session.UserID)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	data := viewmodel.PageData{CSRF: csrf, Workspace: workspaceByID(workspaces, workspace), Workspaces: workspaces, NavPage: page, NavSection: "settings"}
	categoryError := ""
	if page == "transactions-categories" && status == http.StatusUnprocessableEntity {
		categoryError = "transactions.category_invalid"
		if draft.categoryDuplicate {
			categoryError = "transactions.category_duplicate"
		}
	}
	h.renderSettingsPage(w, r, status, data, func(data viewmodel.PageData) templ.Component {
		title := "transactions.manage"
		if page == "transactions-categories" {
			title = "transactions.manage_categories"
		} else if page == "transactions-methods" {
			title = "transactions.methods"
		}
		data.Title = data.T(title)
		return transactionsSettingsView(data, page, categories, methods, draft, categoryError, methodError, r.FormValue("id"), r.FormValue("name"), r.FormValue("enabled") == "true", r.URL.Query().Get("category_saved") == "1", r.URL.Query().Get("method_saved") == "1")
	})
}
