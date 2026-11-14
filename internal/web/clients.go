package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
)

// EnableClients mounts the Clients pages. Handlers built without it have no
// Clients routes, so the navigation link and the routes stay absent together.
func (h *Handler) EnableClients(service *clients.Service) {
	h.clients = service
}

func (h *Handler) clientsPath(workspaceID identity.WorkspaceID) string {
	return "/workspaces/" + workspaceID.String() + "/clients"
}

func (h *Handler) clientList(writer http.ResponseWriter, request *http.Request) {
	h.renderClientList(writer, request, http.StatusOK, "", clients.Input{})
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
	form := clientQueryFromValues(query)
	page, err := h.clients.List(request.Context(), principal, workspaceID,
		form.listInput(h.cfg.DefaultPageSize, query.Get("cursor")))
	if errors.Is(err, clients.ErrInvalidQuery) {
		status, errorKey, page = http.StatusBadRequest, "clients.query_invalid", clients.Page{}
	} else if err != nil {
		h.clientError(writer, request, err)
		return
	}
	fields, err := h.clients.ListFields(request.Context(), principal, workspaceID, clients.ListFieldsInput{})
	if err != nil {
		h.clientError(writer, request, err)
		return
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
	h.renderClientDetail(writer, request, http.StatusOK, "")
}

func (h *Handler) renderClientDetail(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	errorKey string,
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
		h.renderClientDetail(writer, request, http.StatusPreconditionFailed, "clients.error_changed")
	case errors.Is(err, clients.ErrFieldArchived):
		h.renderClientDetail(writer, request, http.StatusConflict, "clients.error_field_archived")
	case errors.Is(err, clients.ErrInvalidOption):
		h.renderClientDetail(writer, request, http.StatusUnprocessableEntity, "clients.error_value_option")
	case errors.Is(err, clients.ErrValueRequired):
		h.renderClientDetail(writer, request, http.StatusUnprocessableEntity, "clients.error_value_required")
	case errors.Is(err, clients.ErrInvalidInput):
		h.renderClientDetail(writer, request, http.StatusUnprocessableEntity, "clients.error_value_invalid")
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
			h.renderClientDetail(writer, request, http.StatusPreconditionFailed, "clients.error_changed")
			return
		}
		if errors.Is(err, clients.ErrInvalidInput) {
			h.renderClientDetail(writer, request, http.StatusUnprocessableEntity, clientErrorKey(err))
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
	case errors.Is(err, clients.ErrNotFound), errors.Is(err, clients.ErrFieldNotFound), errors.Is(err, clients.ErrOptionNotFound), errors.Is(err, clients.ErrInvalidCursor):
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
	FilterField   string
	FilterOp      string
	FilterValue   string
	FilterValueTo string
}

func clientQueryFromValues(query url.Values) clientQueryForm {
	return clientQueryForm{
		Sort: query.Get("sort"), Order: query.Get("order"),
		FilterField: query.Get("filter_field"), FilterOp: query.Get("filter_op"),
		FilterValue: query.Get("filter_value"), FilterValueTo: query.Get("filter_value_to"),
	}
}

func (form clientQueryForm) listInput(limit int, cursor string) clients.ListInput {
	input := clients.ListInput{Limit: limit, Cursor: cursor}
	if form.Sort != "" {
		input.Sort = clients.SortInput{Field: form.Sort, Descending: form.Order == "desc"}
	}
	if form.FilterField != "" && form.FilterValue != "" {
		value := form.FilterValue
		if form.FilterOp == clients.OpBetween {
			value += "," + form.FilterValueTo
		}
		input.Filters = []clients.FilterInput{{Field: form.FilterField, Op: form.FilterOp, Value: value}}
	}
	return input
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
