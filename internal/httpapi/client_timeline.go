package httpapi

import (
	"net/http"
	"strconv"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func (h *handler) listClientTimeline(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(request.PathValue("clientId"))
	query := request.URL.Query()
	for key, values := range query {
		if (key != "limit" && key != "page") || len(values) != 1 || values[0] == "" {
			h.invalidRequest(writer, request, "The request parameters are invalid.")
			return
		}
	}
	page, valid := parsePage(query)
	limit := 50
	if raw := query.Get("limit"); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		valid = valid && err == nil && limit >= 1 && limit <= 100
	}
	if !valid {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	entries, err := h.clients.ListTimelineEntriesPage(request.Context(), principal, workspaceID, clientID, limit, page)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, entries)
}

func (h *handler) addClientTimelineEntry(writer http.ResponseWriter, request *http.Request) {
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
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	var input struct {
		Type       string  `json:"type"`
		Channel    string  `json:"channel"`
		Text       string  `json:"text"`
		CorrectsID *string `json:"correctsId"`
	}
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	var correctsID *clients.TimelineEntryID
	if input.CorrectsID != nil {
		parsed, err := clients.ParseTimelineEntryID(*input.CorrectsID)
		if err != nil {
			h.clientError(writer, request, clients.ErrTimelineCorrection)
			return
		}
		correctsID = &parsed
	}
	entry, err := h.clients.AddTimelineEntry(request.Context(), principal, workspaceID, clientID, clients.TimelineInput{Type: input.Type, Channel: input.Channel, Text: input.Text, CorrectsID: correctsID}, h.auditContext(request))
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, entry)
}
