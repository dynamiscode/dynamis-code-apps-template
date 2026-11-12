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
	Get(context.Context, identity.Principal, identity.WorkspaceID) (workspaceemail.Settings, error)
	Save(context.Context, identity.Principal, identity.WorkspaceID, workspaceemail.SaveInput, identity.AuditContext) error
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
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	h.renderEmailSettings(writer, request, manager, workspaceID, principal, session, csrf, "", "", http.StatusOK, request.URL.Query().Get("saved") == "1")
}

func (h *Handler) emailSettingsSave(writer http.ResponseWriter, request *http.Request) {
	manager, ok := h.emailManager()
	if !ok {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.managementPrincipal(writer, request, workspaceID, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	// A non-numeric port becomes 0, which the service rejects as invalid input.
	port, _ := strconv.Atoi(strings.TrimSpace(request.FormValue("port")))
	provider := strings.TrimSpace(request.FormValue("provider"))
	// Resend reads its key from api_key; Postmark reads its Server Token from server_token
	// and its Account Token from account_token; SMTP reads its password from password.
	secret := request.FormValue("password")
	accountToken := ""
	switch provider {
	case workspaceemail.ProviderResend:
		secret = request.FormValue("api_key")
	case workspaceemail.ProviderPostmark:
		secret = request.FormValue("server_token")
		accountToken = request.FormValue("account_token")
	}
	input := workspaceemail.SaveInput{
		Provider: provider, SenderAddress: request.FormValue("sender_address"), SenderName: request.FormValue("sender_name"),
		Host: request.FormValue("host"), Port: port, Username: request.FormValue("username"),
		Password: secret, AccountToken: accountToken,
	}
	err := manager.Save(request.Context(), principal, workspaceID, input, auditContext(request))
	if err == nil {
		h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/settings/email?saved=1")
		return
	}
	var loginErr *workspaceemail.LoginError
	message, loginReason := "The email settings could not be saved.", ""
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(writer, http.StatusForbidden)
		return
	case errors.As(err, &loginErr) && provider == workspaceemail.ProviderResend:
		message = "Resend check failed. Check the API key, then save again. Nothing was saved."
		loginReason = loginErr.Reason
	case errors.As(err, &loginErr) && provider == workspaceemail.ProviderPostmark:
		message = "Postmark check failed. Check the server and account tokens, then save again. Nothing was saved."
		loginReason = loginErr.Reason
	case errors.As(err, &loginErr):
		message = "SMTP login test failed. Check the host, port, username, and password. Nothing was saved."
		loginReason = loginErr.Reason
	case errors.Is(err, workspaceemail.ErrAccountTokenRequired):
		message = "Enter the Postmark account token on the first save. Nothing was saved."
	case errors.Is(err, workspaceemail.ErrPasswordRequired):
		message = "Enter the password or API key again when the provider, server, port, or username changes."
	case errors.Is(err, workspaceemail.ErrSecretKey):
		message = "Secret encryption is not configured. Set ENCRYPTION_KEY and restart the server."
	case errors.Is(err, workspaceemail.ErrInvalidInput):
		message = "The email settings are invalid."
	}
	h.renderEmailSettings(writer, request, manager, workspaceID, principal, session, csrf, message, loginReason, http.StatusUnprocessableEntity, false)
}

func (h *Handler) renderEmailSettings(
	writer http.ResponseWriter,
	request *http.Request,
	manager workspaceEmailManager,
	workspaceID identity.WorkspaceID,
	principal identity.Principal,
	session identity.Session,
	csrf, message, loginReason string,
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
		EmailSettings: settings, EmailSaved: saved, EmailLoginReason: loginReason,
	})
}
