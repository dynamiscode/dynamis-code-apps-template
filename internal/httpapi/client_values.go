package httpapi

import (
	"net/http"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
)

// clientValueList is the values body. Values are in field creation order.
type clientValueList struct {
	Values []clients.FieldValue `json:"values"`
}

// A nil client service means the Clients profile is not composed, so these
// routes get the same not-found response as any unknown path.
func (h *handler) listClientValues(writer http.ResponseWriter, request *http.Request) {
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
	values, err := h.clients.ListValues(request.Context(), principal, workspaceID, clientID)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, clientValueList{Values: values})
}

func (h *handler) setClientValue(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(request.PathValue("clientId"))
	fieldID := clients.ParseFieldIDOrZero(request.PathValue("fieldId"))
	if len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	version, ok := h.clientVersion(writer, request)
	if !ok {
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	var input struct {
		Value string `json:"value"`
	}
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	client, err := h.clients.SetValue(
		request.Context(), principal, workspaceID, clientID, version, fieldID, input.Value, h.auditContext(request),
	)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writer.Header().Set("ETag", etag(client.Version))
	writeJSON(writer, http.StatusOK, client)
}

func (h *handler) clearClientValue(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		notFoundProblem(writer, request)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(request.PathValue("clientId"))
	fieldID := clients.ParseFieldIDOrZero(request.PathValue("fieldId"))
	if len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	version, ok := h.clientVersion(writer, request)
	if !ok {
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	client, err := h.clients.ClearValue(
		request.Context(), principal, workspaceID, clientID, version, fieldID, h.auditContext(request),
	)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	writer.Header().Set("ETag", etag(client.Version))
	writeJSON(writer, http.StatusOK, client)
}

// clientVersion reads the client version from If-Match. A missing header is 428;
// a malformed or stale one is 412, as the client update route does.
func (h *handler) clientVersion(writer http.ResponseWriter, request *http.Request) (int64, bool) {
	ifMatch := request.Header.Get("If-Match")
	if ifMatch == "" {
		writeProblem(
			writer, request, http.StatusPreconditionRequired,
			"precondition-required", "If-Match is required.",
		)
		return 0, false
	}
	version, ok := parseETag(ifMatch)
	if !ok {
		writeProblem(
			writer, request, http.StatusPreconditionFailed,
			"precondition-failed", "The entity tag is invalid or stale.",
		)
		return 0, false
	}
	return version, true
}
