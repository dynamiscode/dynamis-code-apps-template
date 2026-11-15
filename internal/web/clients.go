package web

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
)

type clientFilterOperator struct {
	Key      string
	LabelKey string
}

type clientFilterField struct {
	Key, Label, LabelKey string
}

// EnableClients mounts the Clients pages. Handlers built without it have no
// Clients routes, so the navigation link and the routes stay absent together.
func (h *Handler) EnableClients(service *clients.Service, preferences *listprefs.Service) {
	h.clients = service
	h.listPreferences = preferences
}

func (h *Handler) clientsPath(workspaceID identity.WorkspaceID) string {
	return "/workspaces/" + workspaceID.String() + "/clients"
}

func (h *Handler) clientList(writer http.ResponseWriter, request *http.Request) {
	h.renderClientList(writer, request, http.StatusOK, "", clients.Input{})
}

func (h *Handler) setClientColumn(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil || h.listPreferences == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.clientPreferencePrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	fields, err := h.clients.ListFields(request.Context(), principal, workspaceID, clients.ListFieldsInput{})
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	defaults := make(map[string]bool)
	for _, column := range clientColumns(fields) {
		defaults[column.Key] = column.DefaultVisible
	}
	visible, err := strconv.ParseBool(request.FormValue("visible"))
	if err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	if err := h.listPreferences.Set(request.Context(), principal, workspaceID, listprefs.ListClients,
		request.PathValue("column"), visible, defaults); err != nil {
		h.clientError(writer, request, err)
		return
	}
	h.redirect(writer, request, h.clientsPath(workspaceID)+clientQuerySuffix(request.URL.Query()))
}

func (h *Handler) resetClientColumns(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil || h.listPreferences == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.clientPreferencePrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	if err := h.listPreferences.Reset(request.Context(), principal, workspaceID, listprefs.ListClients); err != nil {
		h.clientError(writer, request, err)
		return
	}
	h.redirect(writer, request, h.clientsPath(workspaceID)+clientQuerySuffix(request.URL.Query()))
}

func (h *Handler) renderClientList(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	errorKey string,
	draft clients.Input,
) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	query := request.URL.Query()
	fields, err := h.clients.ListFields(request.Context(), principal, workspaceID, clients.ListFieldsInput{})
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	form, valid := clientQueryFromValues(query, fields)
	if !valid {
		status, errorKey = http.StatusBadRequest, "clients.query_invalid"
	}
	limit := h.cfg.DefaultPageSize
	if form.Limit > 0 {
		limit = form.Limit
	}
	page, err := h.clients.List(request.Context(), principal, workspaceID,
		form.listInput(limit, query.Get("cursor")))
	if errors.Is(err, clients.ErrInvalidQuery) {
		status, errorKey, page = http.StatusBadRequest, "clients.query_invalid", clients.Page{}
	} else if errors.Is(err, clients.ErrInvalidCursor) {
		status, errorKey, page = http.StatusBadRequest, "clients.cursor_invalid", clients.Page{}
	} else if err != nil {
		h.clientError(writer, request, err)
		return
	}
	columns := clientColumns(fields)
	defaults := make(map[string]bool, len(columns))
	for _, column := range columns {
		defaults[column.Key] = column.DefaultVisible
	}
	visible := defaults
	if h.listPreferences != nil {
		visible, err = h.listPreferences.Get(request.Context(), principal, workspaceID, listprefs.ListClients, defaults)
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
	}
	for i := range columns {
		columns[i].Visible = visible[columns[i].Key]
		columns[i].SortHref = clientSortHref(h.clientsPath(workspaceID), query, columns[i].Key, form)
		if form.Sort == columns[i].Key {
			columns[i].SortDirection = "ascending"
			if form.Order == "desc" {
				columns[i].SortDirection = "descending"
			}
		}
	}
	rows, err := h.clientListRows(request, principal, workspaceID, page.Clients, fields, visible)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	filterChips := clientFilterChips(form.Filters, fields, visible, query, h.clientsPath(workspaceID))
	filtersToPreserve := form.Filters
	if form.FilterField != "" {
		filtersToPreserve = slices.DeleteFunc(slices.Clone(form.Filters), func(filter clients.FilterInput) bool {
			return filter.Field == form.FilterField
		})
	}
	filterOperators := clientFilterOperators(form.FilterField, fields)
	hiddenSort := ""
	hiddenSortLabelKey := ""
	for _, column := range columns {
		if column.Key == form.Sort && !column.Visible {
			hiddenSort = column.Label
			hiddenSortLabelKey = column.LabelKey
		}
	}
	filterOptions, err := h.choiceOptionsFor(request, principal, workspaceID, fields, form.FilterField)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	nextPath := ""
	if page.NextCursor != "" {
		next := url.Values{}
		for key, values := range query {
			next[key] = values
		}
		next.Set("cursor", page.NextCursor)
		nextPath = request.URL.Path + "?" + next.Encode()
	}
	clearFilters := clientClearFiltersPath(h.clientsPath(workspaceID), query)
	querySuffix := clientQuerySuffix(query)
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	name := "clients.html"
	if request.Header.Get("HX-Request") == "true" {
		// A realtime refresh swaps only the list section, as the Items list does.
		name = "client-list"
	}
	h.render(writer, status, name, pageData{
		Title: "Clients", ErrorKey: errorKey, CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		NavPage: "clients", NavSection: "clients",
		CurrentPath: h.clientsPath(workspaceID),
		Clients:     page.Clients, NextCursor: page.NextCursor, NextPath: nextPath,
		ClientQuery: form, ClientQueryFields: fields, ClientFilterOptions: filterOptions,
		ClientFilterFields:    clientFilterFields(fields, form.Filters),
		ClientFilterValueType: clientFilterValueType(form.FilterField, fields),
		ClientListColumns:     columns, ClientListRows: rows, ClientFilterChips: filterChips,
		HasVisibleClientColumns: hasVisibleClientColumns(visible), HasClientFilters: len(form.Filters) > 0,
		ClientFiltersToPreserve: filtersToPreserve, ClientFilterOperators: filterOperators,
		ClientCanAddFilter: len(form.Filters) < 20, HiddenClientSort: hiddenSort,
		HiddenClientSortLabelKey: hiddenSortLabelKey,
		ClearClientFiltersPath:   clearFilters, ClientQuerySuffix: querySuffix,
		CanEditClients:  principal.Permissions[identity.ResourcesWrite],
		ClientDraft:     draft,
		FieldsPath:      h.clientFieldsPath(workspaceID),
		CanManageFields: principal.Permissions[identity.WorkspaceUpdate],
	})
}

func (h *Handler) createClient(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	draft := clients.Input{
		DisplayName: request.FormValue("display_name"), Email: request.FormValue("email"),
		Phone: request.FormValue("phone"), Notes: request.FormValue("notes"),
	}
	client, err := h.clients.Create(request.Context(), principal, workspaceID, draft, auditContext(request))
	if err != nil {
		if errors.Is(err, clients.ErrInvalidInput) {
			h.renderClientList(writer, request, http.StatusUnprocessableEntity, clientErrorKey(err), draft)
			return
		}
		h.clientError(writer, request, err)
		return
	}
	h.redirect(writer, request, h.clientsPath(workspaceID)+"/"+client.ID.String())
}

func (h *Handler) clientDetail(writer http.ResponseWriter, request *http.Request) {
	h.renderClientDetail(writer, request, http.StatusOK, "", nil)
}

// renderClientDetail shows the stored client. A refused edit passes its draft,
// so the form keeps what the user typed instead of the stored values.
func (h *Handler) renderClientDetail(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	errorKey string,
	draft *clients.Input,
) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(request.PathValue("clientId"))
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	client, err := h.clients.Get(request.Context(), principal, workspaceID, clientID)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	if draft != nil {
		client.DisplayName, client.Email, client.Phone, client.Notes = draft.DisplayName, draft.Email, draft.Phone, draft.Notes
	}
	values, err := h.clients.ListValues(request.Context(), principal, workspaceID, clientID)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	rows, err := h.clientValueRows(request, principal, workspaceID, values)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, status, "client.html", pageData{
		Title: client.DisplayName, ErrorKey: errorKey, CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		NavPage: "clients", NavSection: "clients",
		CurrentPath: h.clientsPath(workspaceID),
		Client:      client, ClientValues: rows, CanEditClients: principal.Permissions[identity.ResourcesWrite],
		CanDeleteClients: principal.Permissions[identity.WorkspaceUpdate],
	})
}

func (h *Handler) setClientValue(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(request.PathValue("clientId"))
	fieldID := clients.ParseFieldIDOrZero(request.PathValue("fieldId"))
	version, _ := strconv.ParseInt(request.FormValue("version"), 10, 64)
	_, err := h.clients.SetValue(request.Context(), principal, workspaceID, clientID, version, fieldID,
		request.FormValue("value"), auditContext(request))
	if err != nil {
		h.clientValueError(writer, request, err)
		return
	}
	h.redirect(writer, request, h.clientsPath(workspaceID)+"/"+clientID.String())
}

func (h *Handler) clearClientValue(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(request.PathValue("clientId"))
	fieldID := clients.ParseFieldIDOrZero(request.PathValue("fieldId"))
	version, _ := strconv.ParseInt(request.FormValue("version"), 10, 64)
	_, err := h.clients.ClearValue(request.Context(), principal, workspaceID, clientID, version, fieldID,
		auditContext(request))
	if err != nil {
		h.clientValueError(writer, request, err)
		return
	}
	h.redirect(writer, request, h.clientsPath(workspaceID)+"/"+clientID.String())
}

// clientValueError re-renders the detail page with the message for a refused
// value write. Other errors take the same path as the rest of the Clients pages.
func (h *Handler) clientValueError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, clients.ErrPreconditionFailed):
		h.renderClientDetail(writer, request, http.StatusPreconditionFailed, "clients.error_changed", nil)
	case errors.Is(err, clients.ErrFieldArchived):
		h.renderClientDetail(writer, request, http.StatusConflict, "clients.error_field_archived", nil)
	case errors.Is(err, clients.ErrInvalidOption):
		h.renderClientDetail(writer, request, http.StatusUnprocessableEntity, "clients.error_value_option", nil)
	case errors.Is(err, clients.ErrValueRequired):
		h.renderClientDetail(writer, request, http.StatusUnprocessableEntity, "clients.error_value_required", nil)
	case errors.Is(err, clients.ErrInvalidInput):
		h.renderClientDetail(writer, request, http.StatusUnprocessableEntity, "clients.error_value_invalid", nil)
	default:
		h.clientError(writer, request, err)
	}
}

func (h *Handler) changeClient(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(request.PathValue("clientId"))
	version, _ := strconv.ParseInt(request.FormValue("version"), 10, 64)
	displayName := request.FormValue("display_name")
	email := request.FormValue("email")
	phone := request.FormValue("phone")
	notes := request.FormValue("notes")
	_, err := h.clients.Update(request.Context(), principal, workspaceID, clientID, version,
		clients.UpdateInput{DisplayName: &displayName, Email: &email, Phone: &phone, Notes: &notes},
		auditContext(request))
	if err != nil {
		if errors.Is(err, clients.ErrPreconditionFailed) {
			h.renderClientDetail(writer, request, http.StatusPreconditionFailed, "clients.error_changed", nil)
			return
		}
		if errors.Is(err, clients.ErrInvalidInput) {
			draft := clients.Input{DisplayName: displayName, Email: email, Phone: phone, Notes: notes}
			h.renderClientDetail(writer, request, http.StatusUnprocessableEntity, clientErrorKey(err), &draft)
			return
		}
		h.clientError(writer, request, err)
		return
	}
	h.redirect(writer, request, h.clientsPath(workspaceID)+"/"+clientID.String())
}

// clientErrorKey names the catalog message for a rejected client value.
func clientErrorKey(err error) string {
	switch {
	case errors.Is(err, clients.ErrDisplayNameRequired):
		return "clients.error_name_required"
	case errors.Is(err, clients.ErrInvalidEmail):
		return "clients.error_email_invalid"
	default:
		return "clients.error_invalid"
	}
}

func (h *Handler) clientError(writer http.ResponseWriter, _ *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(writer, http.StatusForbidden)
	case errors.Is(err, listprefs.ErrInvalidInput):
		h.renderError(writer, http.StatusBadRequest)
	case errors.Is(err, clients.ErrNotFound), errors.Is(err, clients.ErrFieldNotFound), errors.Is(err, clients.ErrOptionNotFound):
		h.renderError(writer, http.StatusNotFound)
	default:
		h.renderError(writer, http.StatusInternalServerError)
	}
}

// clientQueryForm holds the list's GET form values. The web list maps them to
// one sort and at most one filter. A filter with no value is not applied, so
// choosing a field shows its options before any value is set. A between filter
// takes FilterValue and FilterValueTo.
type clientQueryForm struct {
	Sort          string
	Order         string
	Limit         int
	FilterField   string
	FilterOp      string
	FilterValue   string
	FilterValueTo string
	FilterValues  []string
	Archived      string
	Filters       []clients.FilterInput
}

func clientQueryFromValues(query url.Values, fields []clients.Field) (clientQueryForm, bool) {
	form := clientQueryForm{
		Sort: query.Get("sort"), Order: query.Get("order"),
		FilterField: query.Get("filter_field"), FilterOp: query.Get("filter_op"),
		FilterValue: query.Get("filter_value"), FilterValueTo: query.Get("filter_value_to"),
		FilterValues: query["filter_value"],
		Archived:     query.Get("archived"),
	}
	for _, key := range []string{"sort", "order", "archived", "limit", "cursor", "filter_field", "filter_op", "filter_value_to"} {
		if len(query[key]) > 1 {
			return form, false
		}
	}
	for key := range query {
		switch key {
		case "sort", "order", "archived", "limit", "cursor", "filter_field", "filter_op", "filter_value", "filter_value_to", "filter", "filter_error":
		default:
			return form, false
		}
	}
	if form.Order != "" && form.Order != "asc" && form.Order != "desc" || form.Order == "desc" && form.Sort == "" {
		return form, false
	}
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return form, false
		}
		form.Limit = limit
	}
	seen := map[string]bool{}
	for _, raw := range query["filter"] {
		parts := strings.SplitN(raw, ":", 3)
		if len(parts) != 3 || parts[0] == "" || utf8.RuneCountInString(raw) > 1024 || seen[parts[0]] {
			return form, false
		}
		seen[parts[0]] = true
		form.Filters = append(form.Filters, clients.FilterInput{Field: parts[0], Op: parts[1], Value: parts[2]})
	}
	if len(form.Filters) > 20 {
		return form, false
	}
	if form.FilterField != "" {
		if seen[form.FilterField] || len(form.Filters) == 20 {
			return form, false
		}
		op := form.FilterOp
		if op == "" {
			op = clients.OpEquals
		}
		value := form.FilterValue
		if len(form.FilterValues) > 1 {
			if !clientChoiceField(form.FilterField, fields) {
				return form, false
			}
			value = strings.Join(form.FilterValues, ",")
		}
		if op == clients.OpBetween {
			value += "," + form.FilterValueTo
		}
		if utf8.RuneCountInString(form.FilterField+":"+op+":"+value) > 1024 {
			return form, false
		}
		if op == clients.OpIsEmpty || value != "" {
			form.Filters = append(form.Filters, clients.FilterInput{Field: form.FilterField, Op: op, Value: value})
			form.FilterField, form.FilterOp, form.FilterValue, form.FilterValueTo = "", "", "", ""
		}
	}
	return form, true
}

func (form clientQueryForm) listInput(limit int, cursor string) clients.ListInput {
	input := clients.ListInput{Limit: limit, Cursor: cursor, Archived: form.Archived}
	if form.Sort != "" {
		input.Sort = clients.SortInput{Field: form.Sort, Descending: form.Order == "desc"}
	}
	input.Filters = form.Filters
	return input
}

func clientColumns(fields []clients.Field) []clientListColumn {
	columns := []clientListColumn{
		{Key: clients.FieldDisplayName, LabelKey: "clients.display_name", ValueType: clients.FieldTypeText, DefaultVisible: true},
		{Key: clients.FieldEmail, LabelKey: "common.email", ValueType: clients.FieldTypeText, DefaultVisible: true},
		{Key: clients.FieldPhone, LabelKey: "clients.phone", ValueType: clients.FieldTypeText},
		{Key: clients.FieldNotes, LabelKey: "clients.notes", ValueType: clients.FieldTypeText},
		{Key: clients.FieldCreatedAt, LabelKey: "clients.created_at", ValueType: "timestamp"},
		{Key: clients.FieldUpdatedAt, LabelKey: "clients.updated_at", ValueType: "timestamp"},
	}
	for _, field := range fields {
		columns = append(columns, clientListColumn{Key: field.ID.String(), Label: field.Label, ValueType: field.Type})
	}
	return columns
}

func (h *Handler) clientListRows(
	request *http.Request,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientsPage []clients.Client,
	fields []clients.Field,
	visible map[string]bool,
) ([]clientListRow, error) {
	clientIDs := make([]clients.ClientID, len(clientsPage))
	for i, client := range clientsPage {
		clientIDs[i] = client.ID
	}
	customValues := map[clients.ClientID]map[string]string{}
	if hasVisibleCustomColumn(fields, visible) {
		var err error
		customValues, err = h.clients.ListValuesForClients(request.Context(), actor, workspaceID, clientIDs)
		if err != nil {
			return nil, err
		}
	}
	choiceLabels := map[string]map[string]string{}
	for _, field := range fields {
		if !visible[field.ID.String()] || field.Type != clients.FieldTypeSingleChoice {
			continue
		}
		options, err := h.clients.ListOptions(request.Context(), actor, workspaceID, field.ID)
		if err != nil {
			return nil, err
		}
		choiceLabels[field.ID.String()] = make(map[string]string, len(options))
		for _, option := range options {
			choiceLabels[field.ID.String()][option.ID.String()] = option.Label
		}
	}
	rows := make([]clientListRow, 0, len(clientsPage))
	for i, client := range clientsPage {
		values := map[string]string{
			clients.FieldDisplayName: client.DisplayName,
			clients.FieldEmail:       blankDash(client.Email),
			clients.FieldPhone:       blankDash(client.Phone),
			clients.FieldNotes:       blankDash(client.Notes),
			clients.FieldCreatedAt:   client.CreatedAt.Format(time.RFC3339),
			clients.FieldUpdatedAt:   client.UpdatedAt.Format(time.RFC3339),
		}
		for fieldID, value := range customValues[client.ID] {
			if label := choiceLabels[fieldID][value]; label != "" {
				value = label
			}
			values[fieldID] = blankDash(value)
		}
		for _, field := range fields {
			if values[field.ID.String()] == "" {
				values[field.ID.String()] = "—"
			}
		}
		row := clientListRow{ID: client.ID.String(), Values: values, ActionPosition: i + 1}
		if visible[clients.FieldDisplayName] {
			row.ActionContext = client.DisplayName
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func hasVisibleCustomColumn(fields []clients.Field, visible map[string]bool) bool {
	for _, field := range fields {
		if visible[field.ID.String()] {
			return true
		}
	}
	return false
}

func hasVisibleClientColumns(visible map[string]bool) bool {
	for _, shown := range visible {
		if shown {
			return true
		}
	}
	return false
}

func blankDash(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func clientClearFiltersPath(path string, query url.Values) string {
	copy := cloneClientQuery(query)
	for _, key := range []string{"filter", "filter_field", "filter_op", "filter_value", "filter_value_to", "cursor"} {
		copy.Del(key)
	}
	if encoded := copy.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func clientSortHref(path string, query url.Values, field string, form clientQueryForm) string {
	copy := cloneClientQuery(query)
	copy.Del("cursor")
	copy.Del("sort")
	copy.Del("order")
	switch {
	case form.Sort != field:
		copy.Set("sort", field)
		copy.Set("order", "asc")
	case form.Order != "desc":
		copy.Set("sort", field)
		copy.Set("order", "desc")
	}
	if encoded := copy.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func clientFilterChips(filters []clients.FilterInput, fields []clients.Field, visible map[string]bool, query url.Values, path string) []clientFilterChip {
	labels := make(map[string]string, len(fields))
	for _, field := range fields {
		labels[field.ID.String()] = field.Label
	}
	chips := make([]clientFilterChip, 0, len(filters))
	for index, filter := range filters {
		label, labelKey := labels[filter.Field], ""
		if label == "" {
			labelKey = map[string]string{
				clients.FieldDisplayName: "clients.display_name", clients.FieldEmail: "common.email",
				clients.FieldPhone: "clients.phone", clients.FieldNotes: "clients.notes",
				clients.FieldCreatedAt: "clients.created_at", clients.FieldUpdatedAt: "clients.updated_at",
			}[filter.Field]
		}
		copy := cloneClientQuery(query)
		remaining := make([]string, 0, len(filters)-1)
		for i, other := range filters {
			if i == index {
				continue
			}
			remaining = append(remaining, other.Field+":"+other.Op+":"+other.Value)
		}
		copy.Del("filter")
		copy.Del("filter_field")
		copy.Del("filter_op")
		copy.Del("filter_value")
		copy.Del("filter_value_to")
		copy.Del("cursor")
		if len(remaining) > 0 {
			copy["filter"] = remaining
		}
		href := path
		if encoded := copy.Encode(); encoded != "" {
			href += "?" + encoded
		}
		chips = append(chips, clientFilterChip{FieldLabel: label, FieldLabelKey: labelKey,
			OperatorLabelKey: clientOperatorLabelKey(filter.Op), Value: filter.Value,
			RemoveHref: href, Hidden: !visible[filter.Field]})
	}
	return chips
}

func clientOperatorLabelKey(operator string) string {
	return map[string]string{
		clients.OpEquals: "clients.op_equals", clients.OpContains: "clients.op_contains",
		clients.OpBefore: "clients.op_before", clients.OpAfter: "clients.op_after",
		clients.OpBetween: "clients.op_between", clients.OpIsEmpty: "clients.op_is_empty",
	}[operator]
}

func clientFilterOperators(field string, fields []clients.Field) []clientFilterOperator {
	operators := []clientFilterOperator{{Key: clients.OpEquals, LabelKey: "clients.op_equals"}}
	typeName := "text"
	for _, custom := range fields {
		if custom.ID.String() == field {
			typeName = custom.Type
			break
		}
	}
	if field == clients.FieldCreatedAt || field == clients.FieldUpdatedAt || typeName == clients.FieldTypeNumber || typeName == clients.FieldTypeDate {
		operators = append(operators,
			clientFilterOperator{Key: clients.OpBefore, LabelKey: "clients.op_before"},
			clientFilterOperator{Key: clients.OpAfter, LabelKey: "clients.op_after"},
			clientFilterOperator{Key: clients.OpBetween, LabelKey: "clients.op_between"},
		)
	} else if typeName == "text" {
		operators = append(operators, clientFilterOperator{Key: clients.OpContains, LabelKey: "clients.op_contains"})
	}
	return append(operators, clientFilterOperator{Key: clients.OpIsEmpty, LabelKey: "clients.op_is_empty"})
}

func clientFilterFields(fields []clients.Field, filters []clients.FilterInput) []clientFilterField {
	used := make(map[string]bool, len(filters))
	for _, filter := range filters {
		used[filter.Field] = true
	}
	all := []clientFilterField{
		{Key: clients.FieldDisplayName, LabelKey: "clients.display_name"},
		{Key: clients.FieldEmail, LabelKey: "common.email"},
		{Key: clients.FieldPhone, LabelKey: "clients.phone"},
		{Key: clients.FieldNotes, LabelKey: "clients.notes"},
		{Key: clients.FieldCreatedAt, LabelKey: "clients.created_at"},
		{Key: clients.FieldUpdatedAt, LabelKey: "clients.updated_at"},
	}
	for _, field := range fields {
		all = append(all, clientFilterField{Key: field.ID.String(), Label: field.Label})
	}
	available := make([]clientFilterField, 0, len(all))
	for _, field := range all {
		if !used[field.Key] {
			available = append(available, field)
		}
	}
	return available
}

func clientChoiceField(field string, fields []clients.Field) bool {
	for _, custom := range fields {
		if custom.ID.String() == field {
			return custom.Type == clients.FieldTypeSingleChoice
		}
	}
	return false
}

func clientFilterValueType(field string, fields []clients.Field) string {
	for _, custom := range fields {
		if custom.ID.String() == field {
			switch custom.Type {
			case clients.FieldTypeNumber:
				return "number"
			case clients.FieldTypeDate:
				return "date"
			case clients.FieldTypeYesNo:
				return "boolean"
			}
		}
	}
	return "text"
}

// choiceOptionsFor returns the options of the filtered field when it is a
// single-choice field, so the form can offer them. Other fields return none.
func (h *Handler) choiceOptionsFor(
	request *http.Request,
	principal identity.Principal,
	workspaceID identity.WorkspaceID,
	fields []clients.Field,
	filterField string,
) ([]clients.Option, error) {
	for _, field := range fields {
		if field.ID.String() == filterField && field.Type == clients.FieldTypeSingleChoice {
			return h.clients.ListOptions(request.Context(), principal, workspaceID, field.ID)
		}
	}
	return nil, nil
}
