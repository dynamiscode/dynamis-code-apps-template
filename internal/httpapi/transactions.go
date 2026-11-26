package httpapi

import (
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"example.com/dynamis-code/apps-template/internal/transactions"
)

func (h *handler) transactionsTotals(writer http.ResponseWriter, request *http.Request) {
	if h.transactions == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	query := request.URL.Query()
	totals, err := h.transactions.Totals(request.Context(), principal, workspaceID, query.Get("period"), query.Get("start"), query.Get("end"))
	if err != nil {
		h.transactionsError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, totals)
}

func (h *handler) listTransactionsEntries(writer http.ResponseWriter, request *http.Request) {
	if h.transactions == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	page, ok := parsePage(request.URL.Query())
	if !ok {
		h.invalidRequest(writer, request, "The page is invalid.")
		return
	}
	query := request.URL.Query()
	filter := transactions.ListFilter{Page: page, PaymentMethod: query.Get("paymentMethod"), Direction: query.Get("direction"), From: query.Get("from"), To: query.Get("to")}
	var err error
	if value := query.Get("clientId"); value != "" {
		filter.ClientID, err = clients.ParseClientID(value)
		if err != nil {
			h.invalidRequest(writer, request, "The client ID is invalid.")
			return
		}
	}
	if value := query.Get("categoryId"); value != "" {
		filter.CategoryID, err = transactions.ParseCategoryID(value)
		if err != nil {
			h.invalidRequest(writer, request, "The category ID is invalid.")
			return
		}
	}
	entries, err := h.transactions.List(request.Context(), principal, workspaceID, filter)
	if err != nil {
		h.transactionsError(writer, request, err)
		return
	}
	if entries == nil {
		entries = []transactions.Entry{}
	}
	writeJSON(writer, http.StatusOK, entries)
}

func (h *handler) listTransactionsMethods(writer http.ResponseWriter, request *http.Request) {
	if h.transactions == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	methods, err := h.transactions.ListMethods(request.Context(), principal, workspaceID)
	if err != nil {
		h.transactionsError(writer, request, err)
		return
	}
	if methods == nil {
		methods = []transactions.Method{}
	}
	writeJSON(writer, http.StatusOK, methods)
}

func (h *handler) saveTransactionsMethod(writer http.ResponseWriter, request *http.Request) {
	if h.transactions == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	var body struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Enabled *bool  `json:"enabled"`
	}
	if err := decodeJSON(request, &body); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	if (request.Method == http.MethodPost && body.ID != "") || (request.Method == http.MethodPut && (body.ID == "" || body.Enabled == nil)) {
		h.invalidRequest(writer, request, "The payment method is invalid.")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	method, err := h.transactions.SaveMethod(request.Context(), principal, workspaceID, body.ID, body.Name, enabled)
	if err != nil {
		h.transactionsError(writer, request, err)
		return
	}
	status := http.StatusOK
	if request.Method == http.MethodPost {
		status = http.StatusCreated
	}
	writeJSON(writer, status, method)
}

func (h *handler) createTransactionsEntry(writer http.ResponseWriter, request *http.Request) {
	if h.transactions == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	var body struct {
		Description     string                   `json:"description"`
		Direction       string                   `json:"direction"`
		AmountMinor     int64                    `json:"amountMinor"`
		Currency        string                   `json:"currency"`
		PaymentMethod   string                   `json:"paymentMethod"`
		OccurredOn      string                   `json:"occurredOn"`
		ClientID        *clients.ClientID        `json:"clientId"`
		CategoryID      *transactions.CategoryID `json:"categoryId"`
		Note            string                   `json:"note"`
		ReplacesEntryID string                   `json:"replacesEntryId"`
		RefundOfEntryID string                   `json:"refundOfEntryId"`
	}
	if err := decodeJSON(request, &body); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	var replaces, refund int64
	var err error
	if body.ReplacesEntryID != "" {
		replaces, err = id.DecodeKey(id.TypeTransactionsEntry, body.ReplacesEntryID)
		if err != nil {
			h.invalidRequest(writer, request, "The transactions entry is invalid.")
			return
		}
	}
	if body.RefundOfEntryID != "" {
		refund, err = id.DecodeKey(id.TypeTransactionsEntry, body.RefundOfEntryID)
		if err != nil {
			h.invalidRequest(writer, request, "The transactions entry is invalid.")
			return
		}
	}
	entry, err := h.transactions.Create(request.Context(), principal, workspaceID, transactions.Input{
		Description: body.Description, Direction: body.Direction, AmountMinor: body.AmountMinor, Currency: body.Currency,
		PaymentMethod: body.PaymentMethod, OccurredOn: body.OccurredOn, ClientID: body.ClientID, CategoryID: body.CategoryID, Note: body.Note,
		ReplacesEntryID: transactions.EntryID(replaces), RefundOfEntryID: transactions.EntryID(refund),
	})
	if err != nil {
		h.transactionsError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, entry)
}

func (h *handler) voidTransactionsEntry(writer http.ResponseWriter, request *http.Request) {
	if h.transactions == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	entryID, err := id.DecodeKey(id.TypeTransactionsEntry, request.PathValue("entryId"))
	if err != nil {
		notFoundProblem(writer, request)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := decodeJSON(request, &body); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	entry, err := h.transactions.Void(request.Context(), principal, workspaceID, transactions.EntryID(entryID), body.Reason)
	if err != nil {
		h.transactionsError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, entry)
}

func (h *handler) transactionsError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, transactions.ErrDuplicateCategory):
		h.invalidRequest(writer, request, "An active expense category with this name already exists.")
	case errors.Is(err, transactions.ErrInvalidInput):
		h.invalidRequest(writer, request, "The transactions input is invalid.")
	case errors.Is(err, transactions.ErrLastMethod):
		writeProblem(writer, request, http.StatusConflict, "last-payment-method", "At least one payment method must remain enabled.")
	case errors.Is(err, clients.ErrNotFound):
		notFoundProblem(writer, request)
	default:
		h.internal(writer, request)
	}
}

func (h *handler) listTransactionsCategories(w http.ResponseWriter, r *http.Request) {
	if h.transactions == nil {
		notFoundProblem(w, r)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	categories, err := h.transactions.Categories(r.Context(), principal, workspace)
	if err != nil {
		h.transactionsError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, categories)
}

func (h *handler) changeTransactionsCategory(w http.ResponseWriter, r *http.Request) {
	if h.transactions == nil {
		notFoundProblem(w, r)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesWrite)
	if !ok {
		return
	}
	var categoryID transactions.CategoryID
	if raw := r.PathValue("categoryId"); raw != "" {
		var err error
		categoryID, err = transactions.ParseCategoryID(raw)
		if err != nil {
			h.invalidRequest(w, r, "Invalid category.")
			return
		}
	}
	var body struct {
		Name string `json:"name"`
	}
	if r.Method != http.MethodDelete {
		if err := decodeJSON(r, &body); err != nil {
			h.badJSON(w, r, err)
			return
		}
	}
	category, err := h.transactions.ChangeCategory(r.Context(), principal, workspace, categoryID, body.Name, r.Method == http.MethodDelete)
	if err != nil {
		h.transactionsError(w, r, err)
		return
	}
	status := http.StatusOK
	if categoryID == 0 {
		status = http.StatusCreated
	}
	writeJSON(w, status, category)
}
