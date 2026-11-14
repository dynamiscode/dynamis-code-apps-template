package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
)

// firstClient returns the Clients service passed to a constructor, or nil when
// the profile is not composed.
func firstClient(services []*clients.Service) *clients.Service {
	if len(services) == 0 {
		return nil
	}
	return services[0]
}

// A nil client service means the Clients profile is not composed, so its routes
// do not exist and get the same not-found response as any unknown path.
func (h *handler) listClients(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	if !onlyQuery(request, "limit", "cursor") {
		h.invalidRequest(writer, request, "The collection parameters are invalid.")
		return
	}
	query := request.URL.Query()
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	limit := h.cfg.DefaultPageSize
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > h.cfg.MaxPageSize {
			h.invalidRequest(writer, request, "The page limit is invalid.")
			return
		}
		limit = value
	}
	page, err := h.clients.List(request.Context(), principal, workspaceID, clients.ListInput{
		Limit: limit, Cursor: query.Get("cursor"),
	})
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (h *handler) createClient(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	if len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	var input clientFields
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	client, err := h.clients.Create(request.Context(), principal, workspaceID, clients.Input{
		DisplayName: input.DisplayName, Email: input.Email, Phone: input.Phone, Notes: input.Notes,
	}, h.auditContext(request))
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writer.Header().Set("ETag", etag(client.Version))
	writer.Header().Set(
		"Location", "/api/v1/workspaces/"+workspaceID.String()+"/clients/"+client.ID.String(),
	)
	writeJSON(writer, http.StatusCreated, client)
}

func (h *handler) getClient(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(request.PathValue("clientId"))
	if len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	client, err := h.clients.Get(request.Context(), principal, workspaceID, clientID)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writer.Header().Set("ETag", etag(client.Version))
	writeJSON(writer, http.StatusOK, client)
}

func (h *handler) updateClient(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(request.PathValue("clientId"))
	if len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	ifMatch := request.Header.Get("If-Match")
	if ifMatch == "" {
		writeProblem(
			writer, request, http.StatusPreconditionRequired,
			"precondition-required", "If-Match is required.",
		)
		return
	}
	version, ok := parseETag(ifMatch)
	if !ok {
		writeProblem(
			writer, request, http.StatusPreconditionFailed,
			"precondition-failed", "The entity tag is invalid or stale.",
		)
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	var input struct {
		DisplayName *string `json:"displayName"`
		Email       *string `json:"email"`
		Phone       *string `json:"phone"`
		Notes       *string `json:"notes"`
	}
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	client, err := h.clients.Update(
		request.Context(), principal, workspaceID, clientID, version,
		clients.UpdateInput{
			DisplayName: input.DisplayName, Email: input.Email, Phone: input.Phone, Notes: input.Notes,
		},
		h.auditContext(request),
	)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writer.Header().Set("ETag", etag(client.Version))
	writeJSON(writer, http.StatusOK, client)
}

// clientFields is the create body. Unknown members are rejected by decodeJSON.
type clientFields struct {
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Notes       string `json:"notes"`
}

func (h *handler) clientError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, clients.ErrNotFound):
		notFoundProblem(writer, request)
	case errors.Is(err, clients.ErrFieldNotFound):
		notFoundProblem(writer, request)
	case errors.Is(err, clients.ErrLimit):
		writeProblem(
			writer, request, http.StatusConflict, "resource-limit",
			"The workspace custom field limit was reached. Archived fields count toward it.",
		)
	case errors.Is(err, clients.ErrInvalidCursor):
		h.invalidRequest(writer, request, "The cursor is invalid.")
	case errors.Is(err, clients.ErrDisplayNameRequired):
		h.invalidRequest(writer, request, "The display name is required.")
	case errors.Is(err, clients.ErrInvalidEmail):
		h.invalidRequest(writer, request, "The email address is invalid.")
	case errors.Is(err, clients.ErrValueRequired):
		h.invalidRequest(writer, request, "The value is required. Use DELETE to clear a value.")
	case errors.Is(err, clients.ErrFieldArchived):
		h.invalidRequest(writer, request, "The custom field is archived.")
	case errors.Is(err, clients.ErrOptionNotFound):
		notFoundProblem(writer, request)
	case errors.Is(err, clients.ErrOptionLimit):
		writeProblem(
			writer, request, http.StatusConflict, "resource-limit",
			"A choice field can have at most 100 options.",
		)
	case errors.Is(err, clients.ErrOptionLabelRequired):
		h.invalidRequest(writer, request, "The option label is required.")
	case errors.Is(err, clients.ErrNotChoiceField):
		h.invalidRequest(writer, request, "The custom field is not a single-choice field.")
	case errors.Is(err, clients.ErrInvalidOption):
		h.invalidRequest(writer, request, "The value must be an option of this field.")
	case errors.Is(err, clients.ErrInvalidInput):
		h.invalidRequest(writer, request, "The client input is invalid.")
	case errors.Is(err, clients.ErrPreconditionFailed):
		writeProblem(
			writer, request, http.StatusPreconditionFailed,
			"precondition-failed", "The entity tag is invalid or stale.",
		)
	default:
		h.internal(writer, request)
	}
}
