package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/workspaceemail"
)

// workspaceEmailManager is the workspace SMTP service. At runtime the handler's
// mailer is that service, so the assertion below keeps constructor signatures unchanged.
type workspaceEmailManager interface {
	Get(context.Context, identity.Principal, string) (workspaceemail.Settings, error)
	Save(context.Context, identity.Principal, string, workspaceemail.SaveInput, identity.AuditContext) error
}

func (h *Handler) emailManager() (workspaceEmailManager, bool) {
	manager, ok := h.mailer.(workspaceEmailManager)
	return manager, ok
}

func (h *Handler) emailSettingsPage(writer http.ResponseWriter, request *http.Request) {
	manager, ok := h.emailManager()
	if !ok {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaceID := request.PathValue("workspaceId")
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	h.renderEmailSettings(writer, request, manager, workspaceID, principal, session, csrf, "", http.StatusOK, request.URL.Query().Get("saved") == "1")
}

func (h *Handler) emailSettingsSave(writer http.ResponseWriter, request *http.Request) {
	manager, ok := h.emailManager()
	if !ok {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaceID := request.PathValue("workspaceId")
	principal, session, csrf, ok := h.managementPrincipal(writer, request, workspaceID, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	// A non-numeric port becomes 0, which the service rejects as invalid input.
	port, _ := strconv.Atoi(strings.TrimSpace(request.FormValue("port")))
	input := workspaceemail.SaveInput{
		SenderAddress: request.FormValue("sender_address"), SenderName: request.FormValue("sender_name"),
		Host: request.FormValue("host"), Port: port, Username: request.FormValue("username"),
		Password: request.FormValue("password"),
	}
	err := manager.Save(request.Context(), principal, workspaceID, input, auditContext(request))
	if err == nil {
		h.redirect(writer, request, "/workspaces/"+workspaceID+"/settings/email?saved=1")
		return
	}
	var loginErr *workspaceemail.LoginError
	message := "The SMTP settings could not be saved."
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(writer, http.StatusForbidden)
		return
	case errors.As(err, &loginErr):
		message = "SMTP login test failed. Check the host, port, username, and password. Nothing was saved."
	case errors.Is(err, workspaceemail.ErrSecretKey):
		message = "SMTP password encryption is not configured. Set ENCRYPTION_KEY and restart the server."
	case errors.Is(err, workspaceemail.ErrInvalidInput):
		message = "The SMTP settings are invalid."
	}
	h.renderEmailSettings(writer, request, manager, workspaceID, principal, session, csrf, message, http.StatusUnprocessableEntity, false)
}

func (h *Handler) renderEmailSettings(
	writer http.ResponseWriter,
	request *http.Request,
	manager workspaceEmailManager,
	workspaceID string,
	principal identity.Principal,
	session identity.Session,
	csrf, message string,
	status int,
	saved bool,
) {
	settings, err := manager.Get(request.Context(), principal, workspaceID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, status, "email-settings.html", pageData{
		Title: "Email", NavPage: "email", NavSection: "settings", Error: message, CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		EmailSettings: settings, EmailSaved: saved,
	})
}
