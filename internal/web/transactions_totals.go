package web

import (
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/transactions"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func (h *Handler) transactionsTotals(w http.ResponseWriter, r *http.Request) {
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	query := r.URL.Query()
	period, start, end := query.Get("period"), query.Get("start"), query.Get("end")
	totals, err := h.transactions.Totals(r.Context(), principal, workspace, period, start, end)
	invalid := errors.Is(err, transactions.ErrInvalidInput)
	if err != nil && !invalid {
		h.transactionsError(w, err)
		return
	}
	if !invalid && (period == "" || (totals.Period != "custom" && (query.Has("start") || query.Has("end")))) {
		query.Set("period", totals.Period)
		if totals.Period != "custom" {
			query.Del("start")
			query.Del("end")
		}
		http.Redirect(w, r, r.URL.Path+"?"+query.Encode(), http.StatusSeeOther)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(r.Context(), session.UserID)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	data := viewmodel.PageData{CSRF: csrf, Workspace: workspaceByID(workspaces, workspace), Workspaces: workspaces, NavPage: "transactions", NavSection: "workspace", SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle, FilesEnabled: h.filesEnabled, ClientsEnabled: h.clientsEnabled}
	h.localizePage(w, &data)
	data.Title = data.T("transactions.totals")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if invalid {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	_ = transactionsTotalsPage(data, totals, period, start, end, invalid).Render(r.Context(), w)
}
