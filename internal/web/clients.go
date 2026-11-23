package web

import (
	"context"
	"errors"
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/listviews"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	atoms "example.com/dynamis-code/apps-template/internal/web/components/atoms"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"github.com/a-h/templ"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type clientFilterOperator struct {
	Key      string
	LabelKey string
}

// EnableClients mounts the Clients pages. Handlers built without it have no
// Clients routes, so the navigation link and the routes stay absent together.
func (h *Handler) EnableClients(service *clients.Service, preferences *listprefs.Service) {
	h.clients = service
	h.clientsEnabled = service != nil
	h.listPreferences = preferences
	if service != nil {
		h.registerListColumns(listviews.ListClients, func(ctx context.Context, principal identity.Principal, workspaceID identity.WorkspaceID) ([]viewmodel.ListColumn, error) {
			fields, err := service.ListFields(ctx, principal, workspaceID, clients.ListFieldsInput{})
			return clientColumns(fields), err
		})
	}
}

func (h *Handler) clientsPath(workspaceID identity.WorkspaceID) string {
	return "/workspaces/" + workspaceID.String() + "/clients"
}

func clientOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
	visible, err := strconv.ParseBool(listVisibleParam(request))
	if err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	if err := h.listPreferences.Set(request.Context(), principal, workspaceID, listprefs.ListClients,
		request.PathValue("column"), visible, defaults); err != nil {
		h.clientError(writer, request, err)
		return
	}
	h.afterListChange(writer, request, h.clientsPath(workspaceID)+clientQuerySuffix(request.URL.Query()), h.rerenderClients)
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
	h.afterListChange(writer, request, h.clientsPath(workspaceID)+clientQuerySuffix(request.URL.Query()), h.rerenderClients)
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
	choices, err := h.clientFilterChoices(request, principal, workspaceID, fields)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	form, valid := clientQueryFromValues(query, fields, choices)
	if !valid {
		status, errorKey = http.StatusBadRequest, "clients.query_invalid"
	}
	limit := h.cfg.DefaultPageSize
	if form.Limit > 0 {
		limit = form.Limit
	}
	pageNumber := listPageNumber(query)
	page, err := h.clients.List(request.Context(), principal, workspaceID,
		form.ListInput(limit, pageNumber))
	if errors.Is(err, clients.ErrInvalidQuery) || errors.Is(err, clients.ErrInvalidInput) {
		status, errorKey, page = http.StatusBadRequest, "clients.query_invalid", clients.Page{}
	} else if err != nil {
		h.clientError(writer, request, err)
		return
	}
	columns := clientColumns(fields)
	var orderErr error
	columns, orderErr = h.orderListColumns(request, principal, workspaceID, listprefs.ListClients, columns)
	if orderErr != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
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
	filterView := buildListFilterView(h.clientsPath(workspaceID), query, clientListFilterFields(fields, choices), form.Filter)
	hiddenSort := ""
	hiddenSortLabelKey := ""
	for _, column := range columns {
		if column.Key == form.Sort && !column.Visible {
			hiddenSort = column.Label
			hiddenSortLabelKey = column.LabelKey
		}
	}
	pager := newListPager(request.URL.Path, query, pageNumber, limit, page.Total)
	clearFilters := clientClearFiltersPath(h.clientsPath(workspaceID), query)
	querySuffix := clientQuerySuffix(query)
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	component := clientsPage
	switch {
	case listResultsFragment(request):
		component = clientResults
	case request.Header.Get("HX-Request") == "true":
		// A realtime refresh swaps only the list section, as the Items list does.
		component = clientList
	}
	if h.openDefaultListView(writer, request, listviews.ListClients, principal, workspaceID) {
		return
	}
	archivedCount, err := h.clients.CountArchived(request.Context(), principal, workspaceID)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	archived := query.Get("archived") != "" && query.Get("archived") != clients.ArchivedActive
	view := h.listViewState(request, principal, workspaceID, listviews.ListClients, archived)
	h.renderClientPage(writer, request, status, component, viewmodel.PageData{
		Title: "Clients", TopbarTitleKey: "clients.title", ErrorKey: errorKey, CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		NavPage: "clients", NavSection: "clients",
		CurrentPath: h.clientsPath(workspaceID),
		Clients:     page.Clients, ListPager: pager,
		HiddenClientSortLabelKey: hiddenSortLabelKey,
		ClientQuery:              form, ClientQueryFields: fields,
		ListColumns: columns, ListRows: rows, ListTotal: page.Total, ListStart: listStart(pageNumber, limit), Filter: filterView,
		ClientPreservedQuery: clientPreservedQuery(query), ListPreservedQuery: filterView.Preserved,
		ListLive: true, ListSearch: form.Search, ListFragment: listResultsFragment(request), ListViews: view.Views, ListViewQuery: view.Query, ListViewBase: view.Base, ListViewRoute: view.Route, ListViewCanShare: view.CanShare, ListViewSaveReason: view.SaveReason, ArchivedClientCount: archivedCount,
		HasVisibleClientColumns: hasVisibleClientColumns(visible), HasClientFilters: len(form.Filters) > 0,
		HiddenClientSort:       hiddenSort,
		ClearClientFiltersPath: clearFilters, ClientQuerySuffix: querySuffix,
		CanEditClients:   principal.Permissions[identity.ResourcesWrite],
		ListSelectable:   principal.Permissions[identity.ResourcesWrite],
		CanDeleteClients: principal.Permissions[identity.WorkspaceUpdate],
		ClientDraft:      draft,
		FieldsPath:       h.clientFieldsPath(workspaceID),
		CanManageFields:  principal.Permissions[identity.WorkspaceUpdate],
	})
}

func (h *Handler) renderClientPage(writer http.ResponseWriter, request *http.Request, status int, component func(viewmodel.PageData) templ.Component, data viewmodel.PageData) {
	data.FilesEnabled = h.filesEnabled
	data.ClientsEnabled = h.clientsEnabled
	data.SingleWorkspace = h.workspaceMode == config.WorkspaceModeSingle
	h.localizePage(writer, &data)
	data.Title = data.T("clients.title")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = component(data).Render(request.Context(), writer)
}

func clientDraftHasInput(draft clients.Input) bool {
	return draft.DisplayName != "" || draft.Email != "" || draft.Phone != "" || draft.Notes != ""
}

func clientValueDisplay(data viewmodel.PageData, value viewmodel.ClientValueRow) string {
	if value.Value == nil {
		return data.T("clients.value_none")
	}
	switch value.Type {
	case "yes_no":
		if *value.Value == "true" {
			return data.T("clients.yes")
		}
		return data.T("clients.no")
	case "single_choice":
		return value.ValueLabel
	default:
		return *value.Value
	}
}

func clientValueDisplayValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func clientValueYesNoOptions(data viewmodel.PageData, value viewmodel.ClientValueRow) []atoms.SelectOption {
	current := clientValueDisplayValue(value.Value)
	return []atoms.SelectOption{
		{Label: data.T("clients.value_choose"), Disabled: true, Selected: value.Value == nil},
		{Label: data.T("clients.yes"), Value: "true", Selected: current == "true"},
		{Label: data.T("clients.no"), Value: "false", Selected: current == "false"},
	}
}

func clientValueChoiceOptions(data viewmodel.PageData, value viewmodel.ClientValueRow) []atoms.SelectOption {
	options := make([]atoms.SelectOption, 0, len(value.Choices)+1)
	options = append(options, atoms.SelectOption{Label: data.T("clients.value_choose"), Disabled: true, Selected: value.Value == nil})
	for _, choice := range value.Choices {
		options = append(options, atoms.SelectOption{Label: choice.Label, Value: choice.ID, Selected: choice.Selected})
	}
	return options
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
			// A refused create always returns the create form with the draft, even
			// when the post came from the list: the list carries no form of its own.
			h.renderClientNew(writer, request, http.StatusUnprocessableEntity, clientErrorKey(err), &draft)
			return
		}
		h.clientError(writer, request, err)
		return
	}
	h.redirect(writer, request, h.clientsPath(workspaceID)+"/"+client.ID.String())
}

func (h *Handler) clientNew(writer http.ResponseWriter, request *http.Request) {
	h.renderClientNew(writer, request, http.StatusOK, "", nil)
}

// renderClientNew is the client create page: one full page for the form, so the
// list keeps search, views, and filters instead of an inline form or a modal.
func (h *Handler) renderClientNew(
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
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	data := viewmodel.PageData{
		Title: "clients.new_client", ErrorKey: errorKey, CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		NavPage: "clients", NavSection: "clients",
		CurrentPath:     h.clientsPath(workspaceID),
		FieldsPath:      h.clientFieldsPath(workspaceID),
		CanManageFields: principal.Permissions[identity.WorkspaceUpdate],
		CanEditClients:  true,
	}
	if draft != nil {
		data.ClientDraft = *draft
	}
	h.renderClientPage(writer, request, status, clientNewPage, data)
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
	h.renderClientDetailWithTimeline(writer, request, status, errorKey, draft, viewmodel.TimelineDraft{})
}

func (h *Handler) renderClientDetailWithTimeline(writer http.ResponseWriter, request *http.Request, status int, errorKey string, draft *clients.Input, timelineDraft viewmodel.TimelineDraft) {
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
	page := 1
	if raw := request.URL.Query().Get("timeline_page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 || page > 1000000 {
			h.renderError(writer, http.StatusBadRequest)
			return
		}
	}
	entries, err := h.clients.ListTimelineEntriesPage(request.Context(), principal, workspaceID, clientID, 50, page)
	if err != nil {
		h.clientError(writer, request, err)
		return
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
	var engagements []engagement.Engagement
	var stages []engagement.Stage
	var members []identity.MemberSummary
	var engagementNotes string
	if strings.HasPrefix(errorKey, "engagement.") {
		engagementNotes = request.FormValue("notes")
	}
	if h.engagement != nil {
		engagements, err = h.engagement.ListForClient(request.Context(), principal, workspaceID, clientID)
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		stages, err = h.engagement.ListStages(request.Context(), principal, workspaceID)
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		members, err = h.identity.ListMembers(request.Context(), principal)
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
	}
	h.renderClientPage(writer, request, status, clientDetailPage, viewmodel.PageData{
		Title: client.DisplayName, ErrorKey: errorKey, CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		NavPage: "clients", NavSection: "clients",
		CurrentPath:     h.clientsPath(workspaceID),
		FieldsPath:      h.clientFieldsPath(workspaceID),
		CanManageFields: principal.Permissions[identity.WorkspaceUpdate],
		Client:          client, ClientValues: rows, ClientTimeline: entries, TimelinePage: page, TimelineDraft: timelineDraft, CanEditClients: principal.Permissions[identity.ResourcesWrite],
		CanDeleteClients: principal.Permissions[identity.WorkspaceUpdate],
		Engagements:      engagements, EngagementStages: stages, EngagementMembers: members, EngagementEnabled: h.engagement != nil,
		EngagementNotes: engagementNotes,
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
	case errors.Is(err, clients.ErrHasEngagements):
		http.Error(writer, "A client with engagements cannot be deleted.", http.StatusConflict)
	default:
		h.renderError(writer, http.StatusInternalServerError)
	}
}

func clientColumns(fields []clients.Field) []viewmodel.ListColumn {
	columns := []viewmodel.ListColumn{
		{Key: clients.FieldDisplayName, LabelKey: "clients.display_name", DefaultVisible: true},
		{Key: "id", LabelKey: "ui.id"},
		{Key: clients.FieldEmail, LabelKey: "common.email", DefaultVisible: true},
		{Key: clients.FieldPhone, LabelKey: "clients.phone"},
		{Key: clients.FieldNotes, LabelKey: "clients.notes"},
		{Key: clients.FieldCreatedAt, LabelKey: "clients.created_at"},
		{Key: clients.FieldUpdatedAt, LabelKey: "clients.updated_at"},
	}
	for _, field := range fields {
		columns = append(columns, viewmodel.ListColumn{Key: field.ID.String(), Label: field.Label})
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
) ([]viewmodel.ListRow, error) {
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
	rows := make([]viewmodel.ListRow, 0, len(clientsPage))
	for i, client := range clientsPage {
		cells := map[string]viewmodel.ListCell{
			clients.FieldDisplayName: viewmodel.TextCell(client.DisplayName),
			clients.FieldEmail:       viewmodel.TextCell(blankDash(client.Email)),
			clients.FieldPhone:       viewmodel.TextCell(blankDash(client.Phone)),
			clients.FieldNotes:       viewmodel.TextCell(blankDash(client.Notes)),
			clients.FieldCreatedAt:   viewmodel.TimeCell(client.CreatedAt),
			clients.FieldUpdatedAt:   viewmodel.TimeCell(client.UpdatedAt),
		}
		for _, field := range fields {
			if !visible[field.ID.String()] {
				continue
			}
			value := customValues[client.ID][field.ID.String()]
			if label := choiceLabels[field.ID.String()][value]; label != "" {
				value = label
			}
			cells[field.ID.String()] = clientFieldCell(field.Type, value)
		}
		row := viewmodel.ListRow{ID: client.ID.String(), Cells: cells, ActionPosition: i + 1, Archived: client.ArchivedAt != nil}
		if visible[clients.FieldDisplayName] {
			row.ActionContext = client.DisplayName
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// clientFieldCell types a custom field's stored value. A yes/no field shows yes or no; an empty value shows a dash.
func clientFieldCell(fieldType, value string) viewmodel.ListCell {
	if fieldType == clients.FieldTypeYesNo {
		switch value {
		case "true":
			return viewmodel.YesNoCell(true)
		case "false":
			return viewmodel.YesNoCell(false)
		}
	}
	return viewmodel.TextCell(blankDash(value))
}

func hasVisibleCustomColumn(fields []clients.Field, visible map[string]bool) bool {
	for _, field := range fields {
		if visible[field.ID.String()] {
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
	for _, key := range []string{"q", "filter", "filter_field", "filter_op", "filter_value", "filter_value_to", "page"} {
		copy.Del(key)
	}
	if encoded := copy.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func clientSortHref(path string, query url.Values, field string, form viewmodel.ClientQueryForm) string {
	copy := cloneClientQuery(query)
	copy.Del("page")
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

func clientListOpenLabel(data viewmodel.PageData, row viewmodel.ListRow) string {
	label := data.T("clients.open")
	if row.ActionContext != "" {
		return label + " " + row.ActionContext
	}
	return label + " " + data.T("clients.row_number") + " " + strconv.Itoa(row.ActionPosition)
}

// clientPageLimit is the effective page size: the default of 50 when none is set.
func clientPageLimit(limit int) int {
	if limit == 0 {
		return 50
	}
	return limit
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
