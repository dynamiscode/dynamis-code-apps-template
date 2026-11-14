package web

import (
	"errors"
	"net/http"
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
	page, err := h.clients.List(request.Context(), principal, workspaceID, clients.ListInput{
		Limit: h.cfg.DefaultPageSize, Cursor: request.URL.Query().Get("cursor"),
	})
	if err != nil {
		h.clientError(writer, request, err)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, status, "clients.html", pageData{
		Title: "Clients", ErrorKey: errorKey, CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		NavPage: "clients", NavSection: "clients",
		CurrentPath: h.clientsPath(workspaceID),
		Clients:     page.Clients, NextCursor: page.NextCursor,
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
		Client:      client, CanEditClients: principal.Permissions[identity.ResourcesWrite],
	})
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
	case errors.Is(err, clients.ErrNotFound), errors.Is(err, clients.ErrFieldNotFound), errors.Is(err, clients.ErrInvalidCursor):
		h.renderError(writer, http.StatusNotFound)
	default:
		h.renderError(writer, http.StatusInternalServerError)
	}
}
