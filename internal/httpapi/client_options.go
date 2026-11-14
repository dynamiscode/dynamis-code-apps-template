package httpapi

import (
	"net/http"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
)

// clientFieldOptionList is the list body. Options are in slot order.
type clientFieldOptionList struct {
	Options []clients.Option `json:"options"`
}

// clientFieldOptionFields is the add body. Unknown members are rejected by decodeJSON.
type clientFieldOptionFields struct {
	Label string `json:"label"`
}

// A nil client service means the Clients profile is not composed, so these
// routes get the same not-found response as any unknown path.
func (h *handler) listClientFieldOptions(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	fieldID := clients.ParseFieldIDOrZero(request.PathValue("fieldId"))
	if len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	options, err := h.clients.ListOptions(request.Context(), principal, workspaceID, fieldID)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, clientFieldOptionList{Options: options})
}

func (h *handler) createClientFieldOption(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	fieldID := clients.ParseFieldIDOrZero(request.PathValue("fieldId"))
	if len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	var input clientFieldOptionFields
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	option, err := h.clients.AddOption(request.Context(), principal, workspaceID, fieldID, input.Label, h.auditContext(request))
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writer.Header().Set(
		"Location", "/api/v1/workspaces/"+workspaceID.String()+"/client-fields/"+fieldID.String()+"/options/"+option.ID.String(),
	)
	writeJSON(writer, http.StatusCreated, option)
}

func (h *handler) deleteClientFieldOption(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	fieldID := clients.ParseFieldIDOrZero(request.PathValue("fieldId"))
	optionID := clients.ParseOptionIDOrZero(request.PathValue("optionId"))
	if len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	if err := h.clients.DeleteOption(request.Context(), principal, workspaceID, fieldID, optionID, h.auditContext(request)); err != nil {
		h.clientError(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
