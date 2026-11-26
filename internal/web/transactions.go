package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/currency"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/listviews"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/transactions"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func (h *Handler) EnableTransactions(service *transactions.Service) {
	h.transactions = service
	if service != nil {
		h.registerListColumns(listviews.ListTransactions, func(context.Context, identity.Principal, identity.WorkspaceID) ([]viewmodel.ListColumn, error) {
			return transactionsColumns(), nil
		})
	}
}

func transactionsColumns() []viewmodel.ListColumn {
	return []viewmodel.ListColumn{
		{Key: "description", LabelKey: "transactions.description", DefaultVisible: true},
		{Key: "date", LabelKey: "transactions.date", DefaultVisible: true},
		{Key: "direction", LabelKey: "transactions.direction", DefaultVisible: true},
		{Key: "amount", LabelKey: "transactions.amount", DefaultVisible: true},
		{Key: "method", LabelKey: "transactions.method", DefaultVisible: true},
		{Key: "category", LabelKey: "transactions.category", DefaultVisible: true},
		{Key: "client", LabelKey: "transactions.client", DefaultVisible: true},
		{Key: "note", LabelKey: "transactions.note", DefaultVisible: true},
		{Key: "status", LabelKey: "transactions.status_voided", DefaultVisible: true},
	}
}

func (h *Handler) transactionsPreferencePrincipal(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	if h.transactions == nil || h.listPreferences == nil {
		h.renderError(w, http.StatusNotFound)
		return identity.Principal{}, false
	}
	principal, ok := h.clientPreferencePrincipal(w, r)
	if ok && !identity.CanReadTransactions(principal) {
		h.renderError(w, http.StatusForbidden)
		return identity.Principal{}, false
	}
	return principal, ok
}

func (h *Handler) setTransactionsColumn(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.transactionsPreferencePrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	visible, err := strconv.ParseBool(listVisibleParam(r))
	if err != nil {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	err = h.listPreferences.Set(r.Context(), principal, workspace, listprefs.ListTransactions, r.PathValue("column"), visible, columnDefaults(transactionsColumns()))
	if err != nil {
		h.clientError(w, r, err)
		return
	}
	h.afterListChange(w, r, "/workspaces/"+workspace.String()+"/transactions"+clientQuerySuffix(r.URL.Query()), h.transactionsList)
}

func (h *Handler) resetTransactionsColumns(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.transactionsPreferencePrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	if err := h.listPreferences.Reset(r.Context(), principal, workspace, listprefs.ListTransactions); err != nil {
		h.clientError(w, r, err)
		return
	}
	h.afterListChange(w, r, "/workspaces/"+workspace.String()+"/transactions"+clientQuerySuffix(r.URL.Query()), h.transactionsList)
}

func (h *Handler) setTransactionsColumnOrder(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.transactionsPreferencePrincipal(w, r); !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	h.storeColumnOrder(w, r, listprefs.ListTransactions, func(identity.Principal, identity.WorkspaceID) ([]viewmodel.ListColumn, error) {
		return transactionsColumns(), nil
	}, "/workspaces/"+workspace.String()+"/transactions"+clientQuerySuffix(r.URL.Query()), h.transactionsList)
}

var errTransactionsAmount = fmt.Errorf("invalid amount: %w", transactions.ErrInvalidInput)

type transactionsDraft struct {
	direction, amount, currency, method, occurredOn, clientID, description, note, categoryID string
	categoryName, categoryEditID, replacesID, refundOfID                                     string
	refundRemaining, voidID, voidReason, voidError                                           string
	amountMode, amountLocale                                                                 string
	currencySearch                                                                           string
	currencies                                                                               []currency.Currency
	editID, editField, editValue, editError, editVersion                                     string
	editMethods                                                                              []transactions.Method
	editCategories                                                                           []transactions.Category
	editClients                                                                              transactionsPicker
	categoryDuplicate, correctionFocus, recordView, directionFocus, amountInvalid            bool
}

type transactionsRow struct {
	entry                                          transactions.Entry
	clientName, authorName, methodName, voiderName string
	refundCount                                    int
}

type transactionsPicker struct {
	choices          []clients.Client
	selected         clients.Client
	search           string
	focus, dateError bool
	page, total      int
}

func transactionsFilter(values url.Values) (transactions.ListFilter, error) {
	filter := transactions.ListFilter{Page: 1, Limit: 100, PaymentMethod: values.Get("payment_method"), Direction: values.Get("direction"), From: values.Get("from"), To: values.Get("to")}
	var err error
	if value := values.Get("page"); value != "" {
		filter.Page, err = strconv.Atoi(value)
		if err != nil || filter.Page < 1 || filter.Page > 1000000 {
			return filter, transactions.ErrInvalidInput
		}
	}
	if value := values.Get("limit"); value != "" {
		filter.Limit, err = strconv.Atoi(value)
		if err != nil || filter.Limit < 1 || filter.Limit > 100 {
			return filter, transactions.ErrInvalidInput
		}
	}
	if value := values.Get("client_id"); value != "" {
		filter.ClientID, err = clients.ParseClientID(value)
		if err != nil {
			return filter, transactions.ErrInvalidInput
		}
	}
	if value := values.Get("category_id"); value != "" {
		filter.CategoryID, err = transactions.ParseCategoryID(value)
		if err != nil {
			return filter, transactions.ErrInvalidInput
		}
	}
	if value := values.Get("entry_id"); value != "" {
		filter.EntryID, err = transactions.ParseEntryID(value)
		if err != nil {
			return filter, transactions.ErrInvalidInput
		}
	}
	if value := values.Get("refund_of_entry_id"); value != "" {
		filter.RefundOfEntryID, err = transactions.ParseEntryID(value)
		if err != nil {
			return filter, transactions.ErrInvalidInput
		}
	}
	return filter, nil
}

func transactionsPageURL(values url.Values, page int) string {
	query := cloneClientQuery(values)
	for _, key := range []string{"view", "details", "action", "amount_locale", "saved", "voided", "filter_field", "filter_op", "filter_value", "filter_value_to", "filter_picker_action", "filter_client_page"} {
		query.Del(key)
	}
	query.Set("page", strconv.Itoa(page))
	return "?" + query.Encode()
}

func transactionsDetailsURL(values url.Values, page int, entryID transactions.EntryID) string {
	return transactionsPageURL(values, page) + "&details=" + url.QueryEscape(entryID.String()) + "#transactions-entry-details"
}

func transactionsCurrencyEnabled(currencies []currency.Currency, value string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	for _, item := range currencies {
		if item.Code == value {
			return true
		}
	}
	return false
}

func transactionsCurrencyUsable(currencies []currency.Currency, value string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	for _, item := range currencies {
		if item.Code == value {
			return transactions.CurrencyAvailable(item)
		}
	}
	return false
}

func transactionsCurrencyLabel(data viewmodel.PageData, code string) string {
	key := "transactions.currency_" + strings.ToLower(code)
	label := data.T(key)
	if label == key || label == "" {
		return code
	}
	return label
}

func transactionsInput(d transactionsDraft) (transactions.Input, error) {
	var amount int64
	var err error
	currency := strings.ToUpper(strings.TrimSpace(d.currency))
	switch d.amountMode {
	case "", "minor":
		amount, err = strconv.ParseInt(strings.TrimSpace(d.amount), 10, 64)
	case "decimal":
		amount, err = transactions.ParseAmount(d.amount, currency, d.amountLocale)
	default:
		return transactions.Input{}, transactions.ErrInvalidInput
	}
	if err != nil || amount <= 0 {
		return transactions.Input{}, errTransactionsAmount
	}
	input := transactions.Input{Description: d.description, Direction: d.direction, AmountMinor: amount, Currency: currency, PaymentMethod: d.method, OccurredOn: d.occurredOn, Note: d.note}
	if id := strings.TrimSpace(d.clientID); id != "" {
		clientID, err := clients.ParseClientID(id)
		if err != nil {
			return transactions.Input{}, transactions.ErrInvalidInput
		}
		input.ClientID = &clientID
	}
	if id := strings.TrimSpace(d.categoryID); id != "" {
		categoryID, err := transactions.ParseCategoryID(id)
		if err != nil {
			return transactions.Input{}, transactions.ErrInvalidInput
		}
		input.CategoryID = &categoryID
	}
	for _, link := range []struct {
		value  string
		target *transactions.EntryID
	}{{d.replacesID, &input.ReplacesEntryID}, {d.refundOfID, &input.RefundOfEntryID}} {
		if strings.TrimSpace(link.value) != "" {
			parsed, err := transactions.ParseEntryID(strings.TrimSpace(link.value))
			if err != nil {
				return transactions.Input{}, transactions.ErrInvalidInput
			}
			*link.target = parsed
		}
	}
	return input, nil
}

func (h *Handler) transactionsList(w http.ResponseWriter, r *http.Request) {
	h.renderTransactions(w, r, http.StatusOK, transactionsDraft{}, "")
}

func (h *Handler) transactionsClientOptions(w http.ResponseWriter, r *http.Request) {
	if h.clients == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, _, _, ok := h.workspaceSession(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	if !identity.CanReadTransactions(principal) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		var err error
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 || page > clients.MaxListPage {
			h.renderError(w, http.StatusBadRequest)
			return
		}
	}
	if utf8.RuneCountInString(search) > 200 {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	result, err := h.clients.List(r.Context(), principal, workspace, clients.ListInput{Page: page, Limit: 25, Search: search})
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	type option struct {
		Value string `json:"value"`
		Label string `json:"label"`
	}
	options := make([]option, 0, len(result.Clients))
	for _, client := range result.Clients {
		options = append(options, option{Value: client.ID.String(), Label: client.DisplayName})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Options []option `json:"options"`
		Page    int      `json:"page"`
		Total   int      `json:"total"`
	}{options, page, result.Total})
}

func (h *Handler) transactionsCreate(w http.ResponseWriter, r *http.Request) {
	if h.transactions == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	draft := transactionsDraft{description: r.FormValue("description"), direction: r.FormValue("direction"), amount: r.FormValue("amount_minor"), currency: r.FormValue("currency"), method: r.FormValue("payment_method"), occurredOn: r.FormValue("occurred_on"), clientID: r.FormValue("client_id"), note: r.FormValue("note"), categoryID: r.FormValue("category_id"), replacesID: r.FormValue("replaces_entry_id"), refundOfID: r.FormValue("refund_of_entry_id")}
	draft.currencySearch = r.FormValue("currency_search")
	if _, ok := r.PostForm["amount"]; ok {
		draft.amount = r.FormValue("amount")
		draft.amountMode = r.FormValue("amount_mode")
		draft.amountLocale = r.FormValue("amount_locale")
		if draft.amountMode == "" {
			draft.amountMode = "decimal"
		}
	} else if _, ok := r.PostForm["amount_minor"]; ok {
		draft.amountMode = "minor"
	} else {
		draft.amountMode = "decimal"
	}
	if action := r.FormValue("correction_action"); action != "" {
		kind, key, ok := strings.Cut(action, ":")
		page, err := strconv.Atoi(r.FormValue("entry_page"))
		if !ok || err != nil || page < 1 || page > 1000000 {
			h.renderError(w, http.StatusBadRequest)
			return
		}
		methods, err := h.transactions.ListFilterMethods(r.Context(), principal, workspace)
		if err != nil {
			h.transactionsError(w, err)
			return
		}
		categories, err := h.transactions.Categories(r.Context(), principal, workspace)
		if err != nil {
			h.transactionsError(w, err)
			return
		}
		query, _, filterInput, conditions, valid := transactionsFilterQuery(r.URL.Query(), categories, methods)
		filter, err := transactionsFilter(query)
		if !valid || err != nil || filter.Page != page {
			h.renderError(w, http.StatusBadRequest)
			return
		}
		filter.Conditions, filter.Match = conditions, filterInput.Match
		filter.Search, filter.Sort, filter.Order = strings.TrimSpace(query.Get("q")), query.Get("sort"), query.Get("order")
		entries, err := h.transactions.List(r.Context(), principal, workspace, filter)
		if err != nil {
			h.transactionsError(w, err)
			return
		}
		for _, entry := range entries {
			if entry.ID.String() != key {
				continue
			}
			if kind == "refund" && entry.Direction == "in" && entry.VoidedAt == "" {
				draft.direction, draft.currency, draft.refundOfID, draft.replacesID = "out", entry.Currency, key, ""
				draft.clientID = ""
				if entry.ClientID != nil {
					draft.clientID = entry.ClientID.String()
				}
			} else if kind == "replace" && entry.VoidedAt != "" && identity.CanVoidTransactions(principal) {
				draft.direction, draft.currency, draft.replacesID, draft.refundOfID = entry.Direction, entry.Currency, key, entry.RefundOfEntryID.String()
				draft.clientID, draft.categoryID = "", ""
				if entry.ClientID != nil {
					draft.clientID = entry.ClientID.String()
				}
				if entry.CategoryID != nil {
					draft.categoryID = entry.CategoryID.String()
				}
			} else {
				h.renderError(w, http.StatusBadRequest)
				return
			}
			if draft.occurredOn == "" {
				draft.occurredOn, err = h.transactions.Today(r.Context(), principal, workspace)
				if err != nil {
					h.transactionsError(w, err)
					return
				}
			}
			if draft.method == "" {
				draft.method = entry.PaymentMethod
			}
			draft.correctionFocus = true
			h.renderTransactions(w, r, http.StatusOK, draft, "")
			return
		}
		h.renderError(w, http.StatusBadRequest)
		return
	}
	if r.FormValue("picker_action") != "" {
		draft.recordView = true
		draft.directionFocus = r.FormValue("picker_action") == "direction"
		h.renderTransactions(w, r, http.StatusOK, draft, "")
		return
	}
	input, err := transactionsInput(draft)
	draft.amountInvalid = errors.Is(err, errTransactionsAmount)
	if err == nil {
		_, err = h.transactions.Create(r.Context(), principal, workspace, input)
	}
	var refundLimit *transactions.RefundLimitError
	if errors.As(err, &refundLimit) {
		draft.refundRemaining = transactions.FormatAmount(refundLimit.Remaining, refundLimit.Currency, draft.amountLocale)
	}
	if errors.Is(err, transactions.ErrInvalidInput) {
		h.renderTransactions(w, r, http.StatusUnprocessableEntity, draft, "")
		return
	}
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	returnQuery := r.URL.Query()
	returnQuery.Del("entry_id")
	kept := returnQuery["filter"][:0]
	for _, value := range returnQuery["filter"] {
		if !strings.HasPrefix(value, "entry_id:") {
			kept = append(kept, value)
		}
	}
	if len(kept) == 0 {
		returnQuery.Del("filter")
	} else {
		returnQuery["filter"] = kept
	}
	h.redirect(w, r, "/workspaces/"+workspace.String()+"/transactions"+transactionsPageURL(returnQuery, listPageNumber(returnQuery))+"&saved=1#transactions-entries-heading")
}

func (h *Handler) transactionsSaveMethod(w http.ResponseWriter, r *http.Request) {
	if h.transactions == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	_, err := h.transactions.SaveMethod(r.Context(), principal, workspace, r.FormValue("id"), r.FormValue("name"), r.FormValue("enabled") == "true")
	if errors.Is(err, transactions.ErrInvalidInput) || errors.Is(err, transactions.ErrLastMethod) {
		key := "transactions.method_invalid"
		if errors.Is(err, transactions.ErrLastMethod) {
			key = "transactions.method_last"
		}
		h.renderTransactionsSettings(w, r, http.StatusUnprocessableEntity, transactionsDraft{}, key)
		return
	}
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	h.redirect(w, r, "/workspaces/"+workspace.String()+"/settings/transactions/methods?method_saved=1")
}

func (h *Handler) transactionsVoid(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	if r.FormValue("confirm_void") != "yes" {
		h.renderTransactions(w, r, http.StatusUnprocessableEntity, transactionsDraft{voidID: r.PathValue("entryId"), voidReason: r.FormValue("reason"), voidError: "transactions.confirm_required"}, "")
		return
	}
	entryID, err := transactions.ParseEntryID(r.PathValue("entryId"))
	if err == nil {
		_, err = h.transactions.Void(r.Context(), principal, workspace, entryID, r.FormValue("reason"))
	}
	if errors.Is(err, transactions.ErrActiveRefunds) {
		h.renderTransactions(w, r, http.StatusUnprocessableEntity, transactionsDraft{voidID: r.PathValue("entryId"), voidReason: r.FormValue("reason"), voidError: "transactions.active_refunds"}, "")
		return
	}
	if err != nil {
		if errors.Is(err, transactions.ErrInvalidInput) || entryID == 0 {
			h.renderError(w, http.StatusUnprocessableEntity)
		} else {
			h.transactionsError(w, err)
		}
		return
	}
	h.redirect(w, r, "/workspaces/"+workspace.String()+"/transactions"+transactionsPageURL(r.URL.Query(), listPageNumber(r.URL.Query()))+"&voided=1#transactions-entries-heading")
}

func (h *Handler) transactionsUpdateField(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	if !identity.CanVoidTransactions(principal) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	entryID, err := transactions.ParseEntryID(r.PathValue("entryId"))
	if err != nil {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	revision, err := strconv.ParseInt(r.FormValue("version"), 10, 64)
	if err != nil || revision <= 0 {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	field, value := r.FormValue("field"), r.FormValue("value")
	draft := transactionsDraft{editID: entryID.String(), editField: field, editValue: value, editVersion: r.FormValue("version")}
	if field == "client_id" {
		draft.clientID = value
	}
	if field == "currency" {
		draft.currencySearch = r.FormValue("value_search")
	}
	if r.FormValue("picker_action") != "" {
		h.renderTransactions(w, r, http.StatusOK, draft, "")
		return
	}
	if field == "amount_minor" {
		found, lookupErr := h.transactions.EntriesByID(r.Context(), principal, workspace, []transactions.EntryID{entryID})
		if lookupErr != nil {
			h.transactionsError(w, lookupErr)
			return
		}
		entry, exists := found[entryID]
		if !exists {
			h.renderError(w, http.StatusNotFound)
			return
		}
		if entry.Revision != revision {
			draft.editError = "transactions.edit_conflict"
			h.renderTransactions(w, r, http.StatusConflict, draft, "")
			return
		}
		var amount int64
		if _, known := transactions.CurrencyScale(entry.Currency); known {
			amount, err = transactions.ParseAmount(value, entry.Currency, r.FormValue("amount_locale"))
		} else {
			amount, err = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		}
		if err != nil {
			draft.editError = "transactions.edit_invalid"
			h.renderTransactions(w, r, http.StatusUnprocessableEntity, draft, "")
			return
		}
		value = strconv.FormatInt(amount, 10)
	}
	if err == nil {
		_, err = h.transactions.Update(r.Context(), principal, workspace, entryID, transactions.UpdateInput{ExpectedRevision: revision, Field: field, Value: value})
	}
	if errors.Is(err, transactions.ErrConflict) {
		draft.editError = "transactions.edit_conflict"
		h.renderTransactions(w, r, http.StatusConflict, draft, "")
		return
	}
	if errors.Is(err, transactions.ErrInvalidInput) {
		draft.editError = "transactions.edit_invalid"
		h.renderTransactions(w, r, http.StatusUnprocessableEntity, draft, "")
		return
	}
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	h.redirect(w, r, "/workspaces/"+workspace.String()+"/transactions"+transactionsDetailsURL(r.URL.Query(), listPageNumber(r.URL.Query()), entryID))
}

func (h *Handler) renderTransactions(w http.ResponseWriter, r *http.Request, status int, draft transactionsDraft, methodError string) {
	if h.transactions == nil || h.clients == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	if !identity.CanReadTransactions(principal) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	if h.openDefaultListView(w, r, listviews.ListTransactions, principal, workspace) {
		return
	}
	currencies, err := h.transactions.Currencies(r.Context(), principal, workspace)
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	draft.currencies = currencies
	if r.Method == http.MethodGet && draft.currency == "" {
		for _, item := range draft.currencies {
			if item.Default {
				draft.currency = item.Code
				break
			}
		}
	}
	methods, err := h.transactions.ListMethods(r.Context(), principal, workspace)
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	filterMethods, err := h.transactions.ListFilterMethods(r.Context(), principal, workspace)
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	categories, err := h.transactions.Categories(r.Context(), principal, workspace)
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	draft.editMethods, draft.editCategories = methods, categories
	query, filterFields, filterInput, conditions, valid := transactionsFilterQuery(r.URL.Query(), categories, filterMethods)
	dateError := false
	if !valid {
		for _, item := range filterInput.Filters {
			if item.Field == "occurred_on" && item.Op == "between" {
				from, to, found := strings.Cut(item.Value, ",")
				start, startErr := time.Parse("2006-01-02", from)
				end, endErr := time.Parse("2006-01-02", to)
				if found && startErr == nil && endErr == nil && start.After(end) {
					dateError = true
				}
			}
		}
		if !dateError {
			h.renderError(w, http.StatusBadRequest)
			return
		}
		status = http.StatusBadRequest
	}
	filter, err := transactionsFilter(query)
	if err != nil {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	filter.Conditions, filter.Match = conditions, filterInput.Match
	filter.Search, filter.Sort, filter.Order = strings.TrimSpace(query.Get("q")), query.Get("sort"), query.Get("order")
	page := filter.Page
	stableQuery := listFilterQuery(query, filterInput.Filters, filterInput.Match)
	for _, key := range []string{"filter_search", "filter_client_page", "filter_picker_action"} {
		stableQuery.Del(key)
	}
	var entries []transactions.Entry
	total := 0
	if !dateError {
		entries, total, err = h.transactions.ListPage(r.Context(), principal, workspace, filter)
		if err != nil {
			h.transactionsError(w, err)
			return
		}
	}
	detailID := query.Get("details")
	if draft.editID != "" {
		detailID = draft.editID
	}
	if detailID != "" {
		if id, parseErr := transactions.ParseEntryID(detailID); parseErr == nil {
			found := false
			for _, entry := range entries {
				if entry.ID == id {
					found = true
					break
				}
			}
			if !found {
				extra, lookupErr := h.transactions.EntriesByID(r.Context(), principal, workspace, []transactions.EntryID{id})
				if lookupErr != nil {
					h.transactionsError(w, lookupErr)
					return
				}
				if entry, ok := extra[id]; ok {
					entries = append(entries, entry)
				}
			}
		}
	}
	sources := make(map[transactions.EntryID]transactions.Entry, len(entries))
	for _, entry := range entries {
		sources[entry.ID] = entry
	}
	sourceIDs := make(map[transactions.EntryID]bool)
	for _, entry := range entries {
		sourceIDs[entry.ReplacesEntryID] = true
		sourceIDs[entry.RefundOfEntryID] = true
	}
	for _, value := range []string{draft.replacesID, draft.refundOfID} {
		if id, parseErr := transactions.ParseEntryID(value); parseErr == nil {
			sourceIDs[id] = true
		}
	}
	missingSources := make([]transactions.EntryID, 0, len(sourceIDs))
	for id := range sourceIDs {
		if _, found := sources[id]; id != 0 && !found {
			missingSources = append(missingSources, id)
		}
	}
	for start := 0; start < len(missingSources); start += 100 {
		end := min(start+100, len(missingSources))
		found, lookupErr := h.transactions.EntriesByID(r.Context(), principal, workspace, missingSources[start:end])
		if lookupErr != nil {
			h.transactionsError(w, lookupErr)
			return
		}
		for id, entry := range found {
			sources[id] = entry
		}
	}
	receiptIDs := make([]transactions.EntryID, 0, len(entries))
	for _, entry := range entries {
		if entry.Direction == "in" {
			receiptIDs = append(receiptIDs, entry.ID)
		}
	}
	refundCounts, err := h.transactions.RefundCounts(r.Context(), principal, workspace, receiptIDs[:min(100, len(receiptIDs))])
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	if len(receiptIDs) > 100 {
		counts, countErr := h.transactions.RefundCounts(r.Context(), principal, workspace, receiptIDs[100:])
		if countErr != nil {
			h.transactionsError(w, countErr)
			return
		}
		for id, count := range counts {
			refundCounts[id] = count
		}
	}
	filterPage := 1
	if value := r.URL.Query().Get("filter_client_page"); value != "" {
		filterPage, err = strconv.Atoi(value)
		if err != nil || filterPage < 1 || filterPage > clients.MaxListPage {
			h.renderError(w, http.StatusBadRequest)
			return
		}
	}
	filterClients, err := h.clients.List(r.Context(), principal, workspace, clients.ListInput{Page: filterPage, Limit: 25, Search: strings.TrimSpace(r.URL.Query().Get("filter_search"))})
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	var filterClient clients.Client
	for _, condition := range conditions {
		if condition.Field == "client_id" {
			id, parseErr := clients.ParseClientID(strings.Split(condition.Value, ",")[0])
			if parseErr == nil {
				filterClient, err = h.clients.Get(r.Context(), principal, workspace, id)
				if err != nil && !errors.Is(err, clients.ErrNotFound) {
					h.transactionsError(w, err)
					return
				}
			}
			break
		}
	}
	if ((status == http.StatusOK || dateError) && r.Method == http.MethodGet) || methodError != "" || draft.voidError != "" || (status == http.StatusUnprocessableEntity && strings.Contains(r.URL.Path, "/transactions/categories")) {
		draft.direction = "in"
		draft.occurredOn, err = h.transactions.Today(r.Context(), principal, workspace)
		if err != nil {
			h.transactionsError(w, err)
			return
		}
	}
	picker := transactionsPicker{search: strings.TrimSpace(r.FormValue("client_search")), page: 1, focus: r.Method == http.MethodPost && r.FormValue("picker_action") != "" && r.FormValue("picker_action") != "direction" && r.FormValue("picker_action") != "currency"}
	if action := r.FormValue("picker_action"); strings.HasPrefix(action, "page:") {
		picker.page, err = strconv.Atoi(strings.TrimPrefix(action, "page:"))
		if err != nil || picker.page < 1 || picker.page > clients.MaxListPage {
			h.renderError(w, http.StatusBadRequest)
			return
		}
	}
	clientPage, err := h.clients.List(r.Context(), principal, workspace, clients.ListInput{Page: picker.page, Limit: 25, Search: picker.search})
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	picker.choices, picker.total = clientPage.Clients, clientPage.Total
	if draft.clientID != "" {
		if id, parseErr := clients.ParseClientID(draft.clientID); parseErr == nil {
			picker.selected, err = h.clients.Get(r.Context(), principal, workspace, id)
			if err != nil && !errors.Is(err, clients.ErrNotFound) {
				h.transactionsError(w, err)
				return
			}
		}
	}
	draft.editClients = picker
	members, err := h.identity.ListMembers(r.Context(), principal)
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	memberNames := make(map[identity.UserID]string, len(members))
	for _, member := range members {
		memberNames[member.UserID] = member.Email
	}
	resolvedMembers := make(map[identity.UserID]bool)
	rows := make([]transactionsRow, 0, len(entries))
	clientNames := make(map[clients.ClientID]string)
	clientIDs := make(map[clients.ClientID]bool)
	for _, source := range sources {
		if source.ClientID != nil {
			clientIDs[*source.ClientID] = true
		}
	}
	lookupClients := make([]clients.ClientID, 0, len(clientIDs))
	for id := range clientIDs {
		lookupClients = append(lookupClients, id)
	}
	for start := 0; start < len(lookupClients); start += 100 {
		end := min(start+100, len(lookupClients))
		found, lookupErr := h.clients.DisplayNamesByID(r.Context(), principal, workspace, lookupClients[start:end])
		if lookupErr != nil {
			h.transactionsError(w, lookupErr)
			return
		}
		for id, name := range found {
			clientNames[id] = name
		}
	}
	for _, entry := range entries {
		row := transactionsRow{entry: entry, authorName: memberNames[entry.CreatedBy], methodName: entry.PaymentMethodName, voiderName: memberNames[entry.VoidedBy], refundCount: refundCounts[entry.ID]}
		if row.authorName != "" && !resolvedMembers[entry.CreatedBy] {
			profile, profileErr := h.identity.GetUserProfile(r.Context(), entry.CreatedBy)
			if profileErr != nil {
				h.transactionsError(w, profileErr)
				return
			}
			if profile.DisplayName != "" {
				row.authorName = profile.DisplayName
			}
			memberNames[entry.CreatedBy] = row.authorName
			resolvedMembers[entry.CreatedBy] = true
		}
		if entry.ClientID != nil {
			row.clientName = clientNames[*entry.ClientID]
		}
		rows = append(rows, row)
	}
	workspaces, err := h.identity.ListWorkspaces(r.Context(), session.UserID)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	path := "/workspaces/" + workspace.String() + "/transactions"
	columns, err := h.orderListColumns(r, principal, workspace, listprefs.ListTransactions, transactionsColumns())
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	visible := columnDefaults(columns)
	if h.listPreferences != nil {
		visible, err = h.listPreferences.Get(r.Context(), principal, workspace, listprefs.ListTransactions, visible)
		if err != nil {
			h.renderError(w, http.StatusInternalServerError)
			return
		}
	}
	for i := range columns {
		columns[i].Visible = visible[columns[i].Key]
		switch columns[i].Key {
		case "date", "direction", "amount", "method", "category", "client", "note", "status":
			columns[i].SortHref = clientSortHref(path, stableQuery, columns[i].Key, viewmodel.ClientQueryForm{Sort: filter.Sort, Order: filter.Order})
			if filter.Sort == columns[i].Key {
				columns[i].SortDirection = "ascending"
				if filter.Order == "desc" {
					columns[i].SortDirection = "descending"
				}
			}
		}
	}
	filterView := buildListFilterView(path, query, filterFields, filterInput)
	viewRequest := r.Clone(r.Context())
	viewURL := *r.URL
	viewQuery := cloneClientQuery(stableQuery)
	for _, key := range []string{"view", "details", "action", "saved", "voided", "method_saved", "category_saved"} {
		viewQuery.Del(key)
	}
	viewURL.RawQuery = viewQuery.Encode()
	viewRequest.URL = &viewURL
	view := h.listViewState(viewRequest, principal, workspace, listviews.ListTransactions, false)
	data := viewmodel.PageData{CSRF: csrf, Workspace: workspaceByID(workspaces, workspace), Workspaces: workspaces, NavPage: "transactions", NavSection: "workspace", SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle, FilesEnabled: h.filesEnabled, ClientsEnabled: h.clientsEnabled}
	data.CurrentPath, data.ListColumns, data.ListTotal = path, columns, total
	data.ListStart, data.ListPager = listStart(page, filter.Limit), newListPager(path, stableQuery, page, filter.Limit, total)
	data.ClientQuery.Limit = filter.Limit
	data.Filter, data.ListPreservedQuery = filterView, filterView.Preserved
	data.ListSearch, data.HasClientFilters = filter.Search, len(filterInput.Filters) > 0
	data.ListFragment = listResultsFragment(r)
	data.ClientQuerySuffix = clientQuerySuffix(stableQuery)
	data.ListViews, data.ListViewQuery, data.ListViewBase, data.ListViewRoute = view.Views, view.Query, view.Base, view.Route
	if h.listViews == nil || h.listPreferences == nil {
		data.ListViewRoute = ""
	}
	data.ListViewCanShare, data.ListViewSaveReason = view.CanShare, view.SaveReason
	h.localizePage(w, &data)
	data.Title = data.T("transactions.title")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	errorKey := ""
	if draft.editError != "" {
		errorKey = draft.editError
	} else if draft.voidError != "" {
		errorKey = draft.voidError
	} else if status == http.StatusUnprocessableEntity && methodError == "" {
		errorKey = "transactions.invalid"
		if strings.Contains(r.URL.Path, "/transactions/categories") {
			errorKey = "transactions.category_invalid"
			if draft.categoryDuplicate {
				errorKey = "transactions.category_duplicate"
			}
		} else if draft.direction == "in" && draft.categoryID != "" {
			errorKey = "transactions.category_direction"
		} else if draft.refundRemaining != "" {
			errorKey = "transactions.refund_limit"
		} else if !transactionsCurrencyEnabled(draft.currencies, draft.currency) {
			errorKey = "transactions.invalid_currency"
		} else if !transactionsCurrencyUsable(draft.currencies, draft.currency) {
			errorKey = "transactions.currency_precision_unavailable"
		} else if _, known := transactions.CurrencyScale(strings.ToUpper(strings.TrimSpace(draft.currency))); draft.amountInvalid && draft.amountMode == "decimal" && !known {
			errorKey = "transactions.amount_unknown"
		} else if draft.amountInvalid && draft.amountMode == "minor" {
			errorKey = "transactions.amount_invalid_minor"
		} else if draft.amountInvalid {
			errorKey = "transactions.amount_invalid"
		} else if description := strings.TrimSpace(draft.description); description == "" || !utf8.ValidString(description) || utf8.RuneCountInString(description) > 200 {
			errorKey = "transactions.description_invalid"
		}
	}
	errorMessage := data.T(errorKey)
	if errorKey == "transactions.refund_limit" {
		errorMessage = fmt.Sprintf(errorMessage, draft.refundRemaining)
	}
	if data.ListFragment {
		_ = transactionsResults(data, rows, sources, clientNames, draft, errorMessage, false, false, page, identity.CanWriteTransactions(principal), identity.CanVoidTransactions(principal), stableQuery, transactionsPicker{dateError: dateError}).Render(r.Context(), w)
		return
	}
	_ = transactionsPage(data, rows, sources, clientNames, methods, filterMethods, categories, identity.CanManageTransactions(principal), identity.CanWriteCatalog(principal), picker, draft, errorKey, errorMessage, methodError, r.FormValue("id"), r.FormValue("name"), r.URL.Query().Get("saved") == "1", r.URL.Query().Get("method_saved") == "1", r.URL.Query().Get("category_saved") == "1", r.URL.Query().Get("voided") == "1", page, identity.CanWriteTransactions(principal), identity.CanVoidTransactions(principal), h.listPreferences != nil, stableQuery, transactionsPicker{choices: filterClients.Clients, selected: filterClient, search: strings.TrimSpace(r.URL.Query().Get("filter_search")), page: filterPage, total: filterClients.Total, focus: r.URL.Query().Get("filter_picker_action") == "search" || r.URL.Query().Get("filter_client_page") != "", dateError: dateError}).Render(r.Context(), w)
}

func transactionsVoidTime(value string) string {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format("2006-01-02 15:04 UTC")
}

func transactionsVoidReason(draft transactionsDraft, entryID string) string {
	if draft.voidID == entryID {
		return draft.voidReason
	}
	return ""
}

func (h *Handler) transactionsError(w http.ResponseWriter, err error) {
	if errors.Is(err, transactions.ErrInvalidInput) {
		h.renderError(w, http.StatusBadRequest)
		return
	}
	if errors.Is(err, identity.ErrForbidden) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	h.renderError(w, http.StatusInternalServerError)
}

func transactionsMethodLabel(data viewmodel.PageData, id, name string) string {
	if name == "" {
		name = id
	}
	if name == id {
		switch id {
		case "check", "transfer", "cash", "card":
			return data.T("transactions.method_" + id)
		}
	}
	return name
}

func (h *Handler) transactionsCategoryChange(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	var categoryID transactions.CategoryID
	if value := r.PathValue("categoryId"); value != "" {
		var err error
		categoryID, err = transactions.ParseCategoryID(value)
		if err != nil {
			h.renderError(w, http.StatusBadRequest)
			return
		}
	}
	_, err := h.transactions.ChangeCategory(r.Context(), principal, workspace, categoryID, r.FormValue("name"), r.FormValue("action") == "archive")
	if errors.Is(err, transactions.ErrInvalidInput) {
		h.renderTransactionsSettings(w, r, http.StatusUnprocessableEntity, transactionsDraft{categoryName: r.FormValue("name"), categoryEditID: r.PathValue("categoryId"), categoryDuplicate: errors.Is(err, transactions.ErrDuplicateCategory)}, "")
		return
	}
	if err != nil {
		h.transactionsError(w, err)
		return
	}
	h.redirect(w, r, "/workspaces/"+workspace.String()+"/settings/transactions/categories?category_saved=1")
}

func categoryNameDraft(draft transactionsDraft, categoryID, fallback string, failed bool) string {
	if failed && draft.categoryEditID == categoryID {
		return draft.categoryName
	}
	return fallback
}

func categoryDraftActive(categories []transactions.Category, categoryID string) bool {
	for _, category := range categories {
		if !category.Archived && category.ID.String() == categoryID {
			return true
		}
	}
	return false
}
