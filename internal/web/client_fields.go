package web

import (
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func (h *Handler) clientFieldsPath(workspaceID identity.WorkspaceID) string {
	return "/workspaces/" + workspaceID.String() + "/client-fields"
}

func (h *Handler) clientFieldList(writer http.ResponseWriter, request *http.Request) {
	h.renderClientFieldList(writer, request, http.StatusOK, "", clients.FieldInput{})
}

// renderClientFieldList shows every field, archived ones included, so owners and
// admins can restore them. Forms appear only to principals with WorkspaceUpdate.
func (h *Handler) renderClientFieldList(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	errorKey string,
	draft clients.FieldInput,
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
	fields, err := h.clients.ListFields(request.Context(), principal, workspaceID, clients.ListFieldsInput{
		IncludeArchived: true,
	})
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	rows, err := h.clientFieldRows(request, principal, workspaceID, fields)
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, status, "client-fields.html", pageData{
		Title: "Custom fields", ErrorKey: errorKey, CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		NavPage: "clients", NavSection: "clients",
		CurrentPath:     h.clientFieldsPath(workspaceID),
		ClientFields:    rows,
		CanManageFields: principal.Permissions[identity.WorkspaceUpdate],
		FieldDraft:      draft,
		FieldsPath:      h.clientFieldsPath(workspaceID),
	})
}

func (h *Handler) createClientField(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	draft := clients.FieldInput{Label: request.FormValue("label"), Type: request.FormValue("type")}
	_, err := h.clients.DefineField(request.Context(), principal, workspaceID, draft, auditContext(request))
	if err != nil {
		if errors.Is(err, clients.ErrLimit) {
			h.renderClientFieldList(writer, request, http.StatusConflict, "clients.error_field_limit", draft)
			return
		}
		if errors.Is(err, clients.ErrInvalidInput) {
			h.renderClientFieldList(writer, request, http.StatusUnprocessableEntity, "clients.error_field_label", draft)
			return
		}
		h.clientError(writer, request, err)
		return
	}
	h.redirect(writer, request, h.clientFieldsPath(workspaceID))
}

func (h *Handler) archiveClientField(writer http.ResponseWriter, request *http.Request) {
	h.changeClientField(writer, request, true)
}

func (h *Handler) restoreClientField(writer http.ResponseWriter, request *http.Request) {
	h.changeClientField(writer, request, false)
}

func (h *Handler) changeClientField(writer http.ResponseWriter, request *http.Request, archive bool) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	fieldID := clients.ParseFieldIDOrZero(request.PathValue("fieldId"))
	var err error
	if archive {
		_, err = h.clients.ArchiveField(request.Context(), principal, workspaceID, fieldID, auditContext(request))
	} else {
		_, err = h.clients.RestoreField(request.Context(), principal, workspaceID, fieldID, auditContext(request))
	}
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	h.redirect(writer, request, h.clientFieldsPath(workspaceID))
}

// deleteClientField permanently deletes an archived field. An active field is
// refused on the page, so the owner archives it first.
func (h *Handler) deleteClientField(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	fieldID := clients.ParseFieldIDOrZero(request.PathValue("fieldId"))
	err := h.clients.DeleteField(request.Context(), principal, workspaceID, fieldID, auditContext(request))
	if errors.Is(err, clients.ErrFieldNotArchived) {
		h.renderClientFieldList(writer, request, http.StatusConflict, "clients.error_field_not_archived", clients.FieldInput{})
		return
	}
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	h.redirect(writer, request, h.clientFieldsPath(workspaceID))
}
