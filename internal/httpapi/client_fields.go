package httpapi

import (
	"net/http"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
)

// clientFieldList is the list body. Fields are in creation order.
type clientFieldList struct {
	Fields []clients.Field `json:"fields"`
}

// clientFieldFields is the define body. Unknown members are rejected by decodeJSON.
type clientFieldFields struct {
	Label string `json:"label"`
	Type  string `json:"type"`
}

// A nil client service means the Clients profile is not composed, so these
// routes get the same not-found response as any unknown path.
func (h *handler) listClientFields(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	if !onlyQuery(request, "includeArchived") {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	includeArchived := false
	switch request.URL.Query().Get("includeArchived") {
	case "", "false":
	case "true":
		includeArchived = true
	default:
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	fields, err := h.clients.ListFields(request.Context(), principal, workspaceID, clients.ListFieldsInput{
		IncludeArchived: includeArchived,
	})
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, clientFieldList{Fields: fields})
}

func (h *handler) createClientField(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	if len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	var input clientFieldFields
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	field, err := h.clients.DefineField(request.Context(), principal, workspaceID, clients.FieldInput{
		Label: input.Label, Type: input.Type,
	}, h.auditContext(request))
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writer.Header().Set(
		"Location", "/api/v1/workspaces/"+workspaceID.String()+"/client-fields/"+field.ID.String(),
	)
	writeJSON(writer, http.StatusCreated, field)
}

func (h *handler) archiveClientField(writer http.ResponseWriter, request *http.Request) {
	h.changeClientField(writer, request, true)
}

func (h *handler) restoreClientField(writer http.ResponseWriter, request *http.Request) {
	h.changeClientField(writer, request, false)
}

func (h *handler) changeClientField(writer http.ResponseWriter, request *http.Request, archive bool) {
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
	var (
		field clients.Field
		err   error
	)
	if archive {
		field, err = h.clients.ArchiveField(request.Context(), principal, workspaceID, fieldID, h.auditContext(request))
	} else {
		field, err = h.clients.RestoreField(request.Context(), principal, workspaceID, fieldID, h.auditContext(request))
	}
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, field)
}
