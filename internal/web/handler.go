package web

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/listviews"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
	"example.com/dynamis-code/apps-template/internal/platform/telemetry"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"example.com/dynamis-code/apps-template/internal/webhooks"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

//go:embed assets/*
var files embed.FS

type Handler struct {
	logger          *slog.Logger
	identity        *identity.Service
	clients         *clients.Service
	engagement      *engagement.Service
	clientsEnabled  bool
	listPreferences *listprefs.Service
	listViews       *listviews.Service
	listColumns     map[string]listColumnsFunc
	files           *appfiles.Service
	filesEnabled    bool
	webhooks        webhookManager
	exporter        exporter
	oidc            *identity.OIDCRegistry
	publicURL       string
	mailer          appmail.Sender
	cfg             config.HTTP
	setupTokenHash  string
	streams         *streamLimit
	catalog         *i18n.Catalog
	mcpResource     string
	workspaceMode   config.WorkspaceMode
}

func (h *Handler) workspaceCreationEnabled() bool {
	return h.workspaceMode != config.WorkspaceModeSingle
}

func NewHandler(
	identityService *identity.Service,
	cfg config.HTTP,
	setupToken string,
) (*Handler, error) {
	return NewHandlerWithServices(
		identityService, nil, nil, cfg, setupToken, "", nil,
	)
}

func NewHandlerWithServicesAndFiles(
	identityService *identity.Service,
	exporterService exporter,
	oidcRegistry *identity.OIDCRegistry,
	cfg config.HTTP,
	fileService *appfiles.Service,
	setupToken string,
	publicURL string,
	mailer appmail.Sender,
) (*Handler, error) {
	return newHandler(identityService, exporterService, oidcRegistry, cfg, fileService, setupToken, publicURL, mailer, nil)
}

func NewHandlerWithServicesAndFilesAndSharing(
	identityService *identity.Service,
	exporterService exporter,
	oidcRegistry *identity.OIDCRegistry,
	cfg config.HTTP,
	fileService *appfiles.Service,
	setupToken string,
	publicURL string,
	mailer appmail.Sender,
	webhookServices ...*webhooks.Service,
) (*Handler, error) {
	return newHandler(identityService, exporterService, oidcRegistry, cfg, fileService, setupToken, publicURL, mailer, firstWebhook(webhookServices))
}

func NewHandlerWithServices(
	identityService *identity.Service,
	exporterService exporter,
	oidcRegistry *identity.OIDCRegistry,
	cfg config.HTTP,
	setupToken string,
	publicURL string,
	mailer appmail.Sender,
	webhookServices ...*webhooks.Service,
) (*Handler, error) {
	return newHandler(identityService, exporterService, oidcRegistry, cfg, nil, setupToken, publicURL, mailer, firstWebhook(webhookServices))
}

func newHandler(
	identityService *identity.Service,
	exporterService exporter,
	oidcRegistry *identity.OIDCRegistry,
	cfg config.HTTP,
	fileService *appfiles.Service,
	setupToken string,
	publicURL string,
	mailer appmail.Sender,
	webhookService webhookManager,
) (*Handler, error) {
	catalog, err := i18n.New()
	if err != nil {
		return nil, err
	}
	setupTokenHash := ""
	if strings.TrimSpace(setupToken) != "" {
		setupTokenHash = identity.SecretHash(setupToken)
	}
	return &Handler{
		logger:   slog.Default(),
		identity: identityService, files: fileService, exporter: exporterService,
		webhooks: webhookService,
		oidc:     oidcRegistry, publicURL: publicURL, mailer: mailer, cfg: cfg,
		setupTokenHash: setupTokenHash,
		streams:        newStreamLimit(cfg.SSEMaxConnections, cfg.SSEMaxPerUser),
		catalog:        catalog,
	}, nil
}

func firstWebhook(values []*webhooks.Service) webhookManager {
	if len(values) == 0 || values[0] == nil {
		return nil
	}
	return values[0]
}

// SetWorkspaceMode selects the browser root behavior. The zero value keeps the
// multi-workspace list, so handlers built without it are unchanged.
func (h *Handler) SetWorkspaceMode(mode config.WorkspaceMode) {
	h.workspaceMode = mode
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	assets, _ := fs.Sub(files, "assets")
	// Embedded assets carry zero modification time, so responses send no
	// Last-Modified or ETag; without an explicit policy browsers fall back to
	// heuristic caching and keep serving stale CSS between deploys.
	assetsHandler := http.StripPrefix("/assets/", http.FileServerFS(assets))
	mux.Handle("GET /assets/", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-cache")
		assetsHandler.ServeHTTP(writer, request)
	}))
	mux.HandleFunc("GET /language", h.language)
	mux.HandleFunc("GET /login", h.loginPage)
	mux.HandleFunc("POST /login", h.login)
	mux.HandleFunc("GET /mfa", h.mfaPage)
	mux.HandleFunc("POST /mfa/totp", h.mfaTOTP)
	mux.HandleFunc("POST /mfa/recovery", h.mfaRecovery)
	mux.HandleFunc("POST /mfa/passkey", h.mfaPasskey)
	mux.HandleFunc("GET /setup", h.setupPage)
	mux.HandleFunc("POST /setup", h.setup)
	mux.HandleFunc("POST /logout", h.logout)
	if h.mcpResource != "" {
		mux.HandleFunc("GET /oauth/authorize", h.mcpAuthorizePage)
		mux.HandleFunc("POST /oauth/authorize", h.mcpAuthorize)
	}
	mux.HandleFunc("GET /account", h.accountPage)
	mux.HandleFunc("POST /account/profile", h.accountProfileMutation)
	mux.HandleFunc("POST /account/notifications", h.accountNotificationMutation)
	mux.HandleFunc("POST /account/password", h.accountPasswordMutation)
	mux.HandleFunc("POST /account/delete", h.accountDeleteMutation)
	mux.HandleFunc("POST /account/verify-email", h.emailVerificationMutation)
	mux.HandleFunc("GET /account/verify-email/{secret}", h.emailVerification)
	mux.HandleFunc("GET /password-reset", h.passwordResetPage)
	mux.HandleFunc("POST /password-reset", h.passwordResetRequest)
	mux.HandleFunc("GET /password-reset/{secret}", h.passwordResetTokenPage)
	mux.HandleFunc("POST /password-reset/{secret}", h.passwordResetComplete)
	mux.HandleFunc("GET /notifications", h.notificationsPage)
	mux.HandleFunc("POST /notifications/{notificationId}", h.notificationMutation)
	mux.HandleFunc("GET /notifications/events", h.notificationEvents)
	mux.HandleFunc("GET /auth/oidc/{providerId}", h.oidcLogin)
	mux.HandleFunc("GET /auth/oidc/{providerId}/callback", h.oidcCallback)
	mux.HandleFunc("GET /{$}", h.home)
	if h.workspaceCreationEnabled() {
		mux.HandleFunc("POST /workspaces", h.createWorkspace)
	}
	mux.HandleFunc("GET /workspaces/{workspaceId}", h.workspaceHome)
	mux.HandleFunc("GET /workspaces/{workspaceId}/engagement-stages", h.engagementStages)
	mux.HandleFunc("POST /workspaces/{workspaceId}/engagement-stages", h.createEngagementStage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/engagement-stages/reorder", h.reorderEngagementStages)
	mux.HandleFunc("POST /workspaces/{workspaceId}/engagement-stages/{stageId}/archive", h.archiveEngagementStage)
	mux.HandleFunc("GET /workspaces/{workspaceId}/clients", h.clientList)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/preferences/columns/{column}", h.setClientColumn)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/preferences/columns/reset", h.resetClientColumns)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/preferences/columns/order", h.setClientColumnOrder)
	mux.HandleFunc("GET /workspaces/{workspaceId}/clients/events", h.clientEvents)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients", h.createClient)
	mux.HandleFunc("GET /workspaces/{workspaceId}/clients/new", h.clientNew)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/new", h.createClient)
	mux.HandleFunc("GET /workspaces/{workspaceId}/clients/{clientId}", h.clientDetail)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}", h.changeClient)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/engagements", h.createEngagement)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/engagements/{engagementId}/next-action", h.setEngagementNextAction)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/engagements/{engagementId}/quote", h.setEngagementQuote)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/engagements/{engagementId}/agreement", h.setEngagementAgreement)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/engagements/{engagementId}/assignee", h.setEngagementAssignee)
	mux.HandleFunc("GET /workspaces/{workspaceId}/engagements/due", h.dueEngagements)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/timeline", h.addClientTimelineEntry)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/bulk", h.bulkClients)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/archive", h.archiveClient)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/unarchive", h.unarchiveClient)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/delete", h.deleteClient)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/values/{fieldId}", h.setClientValue)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/values/{fieldId}/clear", h.clearClientValue)
	mux.HandleFunc("GET /workspaces/{workspaceId}/client-fields", h.clientFieldList)
	mux.HandleFunc("POST /workspaces/{workspaceId}/client-fields", h.createClientField)
	mux.HandleFunc("POST /workspaces/{workspaceId}/client-fields/{fieldId}/archive", h.archiveClientField)
	mux.HandleFunc("POST /workspaces/{workspaceId}/client-fields/{fieldId}/restore", h.restoreClientField)
	mux.HandleFunc("POST /workspaces/{workspaceId}/client-fields/{fieldId}/delete", h.deleteClientField)
	mux.HandleFunc("POST /workspaces/{workspaceId}/client-fields/{fieldId}/options", h.createClientFieldOption)
	mux.HandleFunc("POST /workspaces/{workspaceId}/client-fields/{fieldId}/options/{optionId}/delete", h.deleteClientFieldOption)
	if h.files != nil {
		mux.HandleFunc("GET /workspaces/{workspaceId}/files", h.filesPage)
		mux.HandleFunc("POST /workspaces/{workspaceId}/files", h.filesUpload)
		mux.HandleFunc("POST /workspaces/{workspaceId}/files/preferences/columns/{column}", h.setFileColumn)
		mux.HandleFunc("POST /workspaces/{workspaceId}/files/preferences/columns/reset", h.resetFileColumns)
		mux.HandleFunc("POST /workspaces/{workspaceId}/files/preferences/columns/order", h.setFileColumnOrder)
		mux.HandleFunc("GET /workspaces/{workspaceId}/files/{fileId}/content", h.fileDownload)
	}
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings", h.settingsPage)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/members", h.membersPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/members/preferences/columns/{column}", h.setMemberColumn)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/members/preferences/columns/reset", h.resetMemberColumns)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/members/preferences/columns/order", h.setMemberColumnOrder)
	mux.HandleFunc("POST /workspaces/{workspaceId}/lists/{list}/views", h.createListView)
	mux.HandleFunc("POST /workspaces/{workspaceId}/lists/{list}/views/{viewId}/update", h.updateListView)
	mux.HandleFunc("POST /workspaces/{workspaceId}/lists/{list}/views/{viewId}/apply", h.applyListView)
	mux.HandleFunc("POST /workspaces/{workspaceId}/lists/{list}/views/{viewId}/delete", h.deleteListView)
	mux.HandleFunc("POST /workspaces/{workspaceId}/lists/{list}/views/{viewId}/rename", h.renameListView)
	mux.HandleFunc("POST /workspaces/{workspaceId}/lists/{list}/views/{viewId}/default", h.setListViewDefault)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/members", h.memberMutation)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/invitations", h.invitationsPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/invitations", h.invitationMutation)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/tokens", h.tokensPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/tokens", h.tokenMutation)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/tokens/{tokenId}", h.tokenMutation)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/webhooks", h.webhooksPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/webhooks", h.webhookMutation)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/webhooks/{webhookId}/deliveries", h.webhookDeliveriesPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/webhooks/{webhookId}", h.webhookMutation)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/email", h.emailSettingsPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/email", h.emailSettingsSave)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/provisioning", h.provisioningPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/provisioning", h.provisioningMutation)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/export", h.exportPage)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/export/download", h.exportWorkspace)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/audit", h.auditHistoryPage)
	mux.HandleFunc("GET /sessions", h.sessionsPage)
	mux.HandleFunc("POST /sessions/{sessionId}", h.sessionMutation)
	mux.HandleFunc("GET /security", h.securityPage)
	mux.HandleFunc("POST /security", h.securityMutation)
	mux.HandleFunc("POST /security/totp/start", h.securityTOTPStart)
	mux.HandleFunc("POST /security/totp/complete", h.securityTOTPComplete)
	mux.HandleFunc("POST /security/totp/remove", h.securityTOTPRemove)
	mux.HandleFunc("POST /security/passkey/{passkeyId}", h.securityPasskeyRemove)
	mux.HandleFunc("GET /settings/language", h.languageSettingsPage)
	mux.HandleFunc("POST /settings/language", h.languageSettings)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/general", h.generalSettingsPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/general", h.generalSettings)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/notifications", h.workspaceNotificationSettings)
	mux.HandleFunc("GET /invitations/{secret}", h.invitationPage)
	mux.HandleFunc("POST /invitations/{secret}", h.invitationPost)
	mux.HandleFunc("POST /invitations/{secret}/sign-in", h.invitationSignIn)
	mux.HandleFunc("GET /invitations/continue", h.invitationContinuePage)
	mux.HandleFunc("POST /invitations/continue", h.invitationContinuePost)
	return h.localeMiddleware(mux)
}

func (h *Handler) loginPage(writer http.ResponseWriter, request *http.Request) {
	available, err := h.setupAvailable(request)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	if available {
		h.redirect(writer, request, "/setup")
		return
	}
	token, err := id.New()
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.setCookie(writer, "login_csrf", token, time.Now().Add(10*time.Minute), true)
	h.render(writer, http.StatusOK, "login.html", viewmodel.PageData{
		Title: "Sign in", CSRF: token, OIDCProviders: h.providers(),
		ReturnTo: h.invitationLoginReturnTo(request, request.URL.Query().Get("return_to")),
	})
}

func (h *Handler) setupPage(writer http.ResponseWriter, request *http.Request) {
	if !h.requireSetup(writer, request) {
		return
	}
	h.renderSetup(writer, http.StatusOK, viewmodel.PageData{Title: "Set up Dynamis Code"})
}

func (h *Handler) setup(writer http.ResponseWriter, request *http.Request) {
	if !h.requireSetup(writer, request) {
		return
	}
	if err := request.ParseForm(); err != nil || !h.validSetupCSRF(request) {
		h.renderSetup(writer, http.StatusForbidden, viewmodel.PageData{
			Title: "Set up Dynamis Code", Error: "The setup form expired. Reload and try again.",
		})
		return
	}
	token := request.FormValue("setup_token")
	switch request.FormValue("setup_token_format") {
	case "", "plain":
	case "base64":
		decoded, err := base64.StdEncoding.Strict().DecodeString(token)
		if err != nil || strings.ContainsAny(token, "\r\n") {
			token = ""
		} else {
			token = string(decoded)
		}
	default:
		token = ""
	}
	if h.setupTokenHash != "" && !identity.EqualSecretHash(token, h.setupTokenHash) {
		h.renderSetup(writer, http.StatusForbidden, viewmodel.PageData{
			Title: "Set up Dynamis Code", Error: "The setup token is invalid.",
		})
		return
	}
	data := viewmodel.PageData{
		Title: "Set up Dynamis Code", Email: request.FormValue("email"),
		WorkspaceName: request.FormValue("workspace"), WorkspaceLocale: request.FormValue("workspace_locale"),
	}
	if request.FormValue("password") != request.FormValue("password_confirmation") {
		data.Error = "The passwords do not match."
		h.renderSetup(writer, http.StatusUnprocessableEntity, data)
		return
	}
	_, err := h.identity.BootstrapFirstOwner(request.Context(), identity.BootstrapInput{
		Email: data.Email, Password: request.FormValue("password"), WorkspaceName: data.WorkspaceName,
		WorkspaceLocale: data.WorkspaceLocale,
	}, auditContext(request))
	if errors.Is(err, identity.ErrAlreadyBootstrapped) {
		h.redirect(writer, request, "/login")
		return
	}
	if errors.Is(err, identity.ErrInvalidBootstrap) {
		data.Error = "Enter a valid email, workspace name, and password."
		h.renderSetup(writer, http.StatusUnprocessableEntity, data)
		return
	}
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.clearCookie(writer, "setup_csrf")
	h.redirect(writer, request, "/login")
}

type setupAvailability uint8

const (
	setupDisabled setupAvailability = iota
	setupEnabled
)

func (h *Handler) setupAvailability(request *http.Request) (setupAvailability, error) {
	bootstrapped, err := h.identity.IsBootstrapped(request.Context())
	if err != nil {
		return setupDisabled, err
	}
	if bootstrapped {
		return setupDisabled, nil
	}
	return setupEnabled, nil
}

func (h *Handler) setupAvailable(request *http.Request) (bool, error) {
	availability, err := h.setupAvailability(request)
	return availability == setupEnabled, err
}

func (h *Handler) requireSetup(writer http.ResponseWriter, request *http.Request) bool {
	availability, err := h.setupAvailability(request)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return false
	}
	if availability != setupEnabled {
		h.renderError(writer, http.StatusNotFound)
		return false
	}
	return true
}

func (h *Handler) renderSetup(writer http.ResponseWriter, status int, data viewmodel.PageData) {
	token, err := id.New()
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.setCookie(writer, "setup_csrf", token, time.Now().Add(10*time.Minute), true)
	data.CSRF = token
	data.SetupTokenRequired = h.setupTokenHash != ""
	h.render(writer, status, "setup.html", data)
}

func (h *Handler) login(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil || !h.validLoginCSRF(request) {
		h.render(writer, http.StatusForbidden, "login.html", viewmodel.PageData{
			Title: "Sign in", Error: "The sign-in form expired. Reload and try again.",
		})
		return
	}
	userID, err := h.identity.AuthenticateLocal(
		request.Context(), request.FormValue("email"), request.FormValue("password"),
	)
	if err != nil {
		h.render(writer, http.StatusUnauthorized, "login.html", viewmodel.PageData{
			Title: "Sign in", CSRF: request.FormValue("csrf"), Email: strings.TrimSpace(request.FormValue("email")),
			Error:         "The email or password is invalid.",
			OIDCProviders: h.providers(), ReturnTo: h.invitationLoginReturnTo(request, request.FormValue("return_to")),
		})
		return
	}
	mfaEnrolled, err := h.identity.MFAEnrolled(request.Context(), userID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	if mfaEnrolled {
		challenge, err := h.identity.BeginMFALogin(request.Context(), userID, auditContext(request))
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		h.setMFAChallengeCookies(writer, challenge, h.invitationLoginReturnTo(request, request.FormValue("return_to")))
		h.redirect(writer, request, "/mfa")
		return
	}
	session, err := h.identity.CreateSession(
		request.Context(), userID, "local", "", 0, auditContext(request),
	)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.setCookie(writer, "session", session.Secret, session.ExpiresAt, true)
	h.setCookie(writer, "csrf", session.CSRFSecret, session.ExpiresAt, true)
	h.clearCookie(writer, "login_csrf")
	h.redirect(writer, request, h.invitationLoginReturnTo(request, request.FormValue("return_to")))
}

func (h *Handler) logout(writer http.ResponseWriter, request *http.Request) {
	continueInvitation := h.invitationReturnActive(request)
	session, ok := h.validCSRFSession(writer, request)
	if !ok {
		return
	}
	providerID, err := h.identity.RevokeSession(
		request.Context(), session.UserID, session.ID, auditContext(request),
	)
	if err != nil {
		h.renderError(writer, http.StatusUnauthorized)
		return
	}
	h.clearCookie(writer, "session")
	h.clearCookie(writer, "csrf")
	if continueInvitation {
		setInvitationSecurityHeaders(writer)
		http.Redirect(writer, request, invitationLoginLocation, http.StatusSeeOther)
		return
	}
	if h.oidc != nil {
		if logoutURL, ok := h.oidc.LogoutURL(providerID); ok {
			h.redirectURL(writer, request, logoutURL)
			return
		}
	}
	h.redirect(writer, request, "/login")
}

func (h *Handler) home(writer http.ResponseWriter, request *http.Request) {
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return
	}
	// Only an explicit single mode lands users in a workspace; the zero value
	// keeps the multi-workspace list.
	if h.workspaceMode == config.WorkspaceModeSingle {
		workspaceID, found, err := h.identity.DefaultWorkspaceID(request.Context(), session.UserID)
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		if found {
			http.Redirect(writer, request, "/workspaces/"+workspaceID.String(), http.StatusFound)
			return
		}
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusOK, "home.html", viewmodel.PageData{
		Title: "Workspaces", NavPage: "home", CSRF: csrf, Workspaces: workspaces,
		CanCreateWorkspace: h.workspaceCreationEnabled(),
	})
}

func (h *Handler) workspaceHome(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	_, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceRead)
	if !ok {
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	data := viewmodel.PageData{
		// The top bar names the workspace, so the page heading is the workspace rather than a second title.
		Title: workspaceByID(workspaces, workspaceID).Name, NavPage: "workspace", NavSection: "workspace",
		CSRF: csrf, Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
	}
	h.render(writer, http.StatusOK, "workspace-home", data)
}

func (h *Handler) mutationPrincipal(
	writer http.ResponseWriter,
	request *http.Request,
) (identity.Principal, bool) {
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return identity.Principal{}, false
	}
	principal, session, _, ok := h.workspaceSession(
		writer, request, identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId")), identity.ResourcesWrite,
	)
	if !ok {
		return identity.Principal{}, false
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return identity.Principal{}, false
	}
	return principal, true
}

func (h *Handler) workspaceSession(
	writer http.ResponseWriter,
	request *http.Request,
	workspaceID identity.WorkspaceID,
	permission identity.Permission,
) (identity.Principal, identity.Session, string, bool) {
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return identity.Principal{}, identity.Session{}, "", false
	}
	principal, err := h.identity.AuthenticateSessionForWorkspace(
		request.Context(), cookieValue(request, "session"), workspaceID, permission,
	)
	if err != nil {
		if errors.Is(err, identity.ErrMFARequired) {
			returnTo := "/"
			if request.Method == http.MethodGet {
				returnTo = request.URL.RequestURI()
			}
			h.beginMFALogin(writer, request, session, returnTo)
			return identity.Principal{}, identity.Session{}, "", false
		}
		h.renderError(writer, h.workspaceDenialStatus(request, session.UserID, workspaceID))
		return identity.Principal{}, identity.Session{}, "", false
	}
	principal.AuthMethod = session.AuthMethod
	principal.AuthLevel = session.AuthLevel
	return principal, session, csrf, true
}

func (h *Handler) session(
	writer http.ResponseWriter,
	request *http.Request,
) (identity.Session, string, bool) {
	secret := cookieValue(request, "session")
	csrf := cookieValue(request, "csrf")
	if secret == "" || csrf == "" {
		h.redirect(writer, request, "/login")
		return identity.Session{}, "", false
	}
	session, err := h.identity.AuthenticateSession(request.Context(), secret)
	if err != nil {
		h.redirect(writer, request, "/login")
		return identity.Session{}, "", false
	}
	return session, csrf, true
}

func (h *Handler) validCSRFSession(writer http.ResponseWriter, request *http.Request) (identity.Session, bool) {
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return identity.Session{}, false
	}
	session, _, ok := h.session(writer, request)
	if !ok {
		return identity.Session{}, false
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return identity.Session{}, false
	}
	return session, true
}

func (h *Handler) validLoginCSRF(request *http.Request) bool {
	want := cookieValue(request, "login_csrf")
	got := request.FormValue("csrf")
	return len(want) == 32 && len(got) == 32 &&
		subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

func (h *Handler) validSetupCSRF(request *http.Request) bool {
	want := cookieValue(request, "setup_csrf")
	got := request.FormValue("csrf")
	return len(want) == 32 && len(got) == 32 &&
		subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

func (h *Handler) setCookie(
	writer http.ResponseWriter,
	name string,
	value string,
	expires time.Time,
	httpOnly bool,
) {
	http.SetCookie(writer, &http.Cookie{
		Name: name, Value: value, Path: "/", Expires: expires,
		HttpOnly: httpOnly, Secure: h.cfg.Secure, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearCookie(writer http.ResponseWriter, name string) {
	http.SetCookie(writer, &http.Cookie{
		Name: name, Path: "/", MaxAge: -1, Expires: time.Unix(1, 0),
		HttpOnly: true, Secure: h.cfg.Secure, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) redirect(writer http.ResponseWriter, request *http.Request, path string) {
	if !isValidRedirect(path) {
		path = "/"
	}
	http.Redirect(writer, request, path, http.StatusSeeOther)
}

// defaultHomeHref returns the home of the user's default workspace in single
// mode. It is empty in multi mode, or when the user has no membership.
func (h *Handler) defaultHomeHref(ctx context.Context, userID identity.UserID) string {
	if h.workspaceMode != config.WorkspaceModeSingle {
		return ""
	}
	workspaceID, found, err := h.identity.DefaultWorkspaceID(ctx, userID)
	if err != nil {
		h.logger.Error("default workspace lookup failed", "error", err)
		return ""
	}
	if !found {
		return ""
	}
	return "/workspaces/" + workspaceID.String()
}

// listVisibleParam reads a column visibility POST. A missing visible field
// means the toggle was unchecked, so it reads as false.
func listVisibleParam(request *http.Request) string {
	if value := request.FormValue("visible"); value != "" {
		return value
	}
	return "false"
}

func (h *Handler) render(writer http.ResponseWriter, status int, name string, data viewmodel.PageData) {
	if lw, ok := writer.(*localeWriter); ok && len(data.Profile.Email) == 0 {
		if state, found := lw.context.Value(localeContextKey{}).(localeState); found && state.userEmail != "" {
			data.Profile = identity.UserProfile{DisplayName: state.userName, Email: state.userEmail}
		}
	}
	data.FilesEnabled = h.files != nil
	data.ClientsEnabled = h.clients != nil
	data.SingleWorkspace = h.workspaceMode == config.WorkspaceModeSingle
	h.localizePage(writer, &data)
	if data.ToastKey != "" {
		// htmx raises app:toast with this message; forms.js shows it in a live region.
		if trigger, err := json.Marshal(map[string]string{"app:toast": data.T(data.ToastKey)}); err == nil {
			writer.Header().Set("HX-Trigger", asciiJSON(trigger))
		}
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	switch name {
	case "workspace-home":
		_ = workspaceHome(data).Render(context.Background(), writer)
	case "webhooks.html":
		_ = webhooksPage(data).Render(context.Background(), writer)
	case "home.html":
		_ = workspaceSelection(data).Render(context.Background(), writer)
	case "audit-history":
		_ = auditHistory(data).Render(context.Background(), writer)
	case "audit-table":
		_ = auditTable(data).Render(context.Background(), writer)
	case "login.html":
		_ = authLogin(data).Render(context.Background(), writer)
	case "setup.html":
		_ = authSetup(data).Render(context.Background(), writer)
	case "password-reset.html":
		_ = authPasswordReset(data).Render(context.Background(), writer)
	case "mfa.html":
		_ = authMFA(data).Render(context.Background(), writer)
	case "verification.html":
		_ = authEmailVerification(data).Render(context.Background(), writer)
	case "sessions-page":
		_ = sessionsPage(data, data.CurrentSessionID, time.Now()).Render(context.Background(), writer)
	case "security-page":
		_ = securityPage(data).Render(context.Background(), writer)
	case "account-page":
		_ = accountPageView(data).Render(context.Background(), writer)
	case "language-settings-page", "language-page":
		_ = languageSettingsView(data).Render(context.Background(), writer)
	case "members.html":
		if data.ListFragment {
			_ = memberResults(data).Render(context.Background(), writer)
			break
		}
		_ = membersPage(data).Render(context.Background(), writer)
	case "client-detail-page":
		_ = clientDetailPage(data).Render(context.Background(), writer)
	case "client-fields-page":
		_ = clientFieldsPage(data).Render(context.Background(), writer)
	case "mcp-consent.html":
		_ = mcpConsentPage(data).Render(context.Background(), writer)
	case "error.html":
		_ = errorPage(data).Render(context.Background(), writer)
	default:
		// Every view name is a literal in the switch above; an unknown name is a programmer error.
		panic("web: unknown view " + name)
	}
}

// asciiJSON escapes non-ASCII runes as \uXXXX so a JSON value is safe in an
// HTTP header, which browsers decode as Latin-1.
func asciiJSON(raw []byte) string {
	var out strings.Builder
	for _, r := range string(raw) {
		if r < 0x80 {
			out.WriteRune(r)
			continue
		}
		for _, unit := range utf16.Encode([]rune{r}) {
			fmt.Fprintf(&out, "\\u%04x", unit)
		}
	}
	return out.String()
}

func (h *Handler) renderError(writer http.ResponseWriter, status int) {
	h.render(writer, status, "error.html", viewmodel.PageData{
		Title: http.StatusText(status), Error: "The request could not be completed.",
	})
}

func workspaceByID(workspaces []identity.WorkspaceSummary, id identity.WorkspaceID) identity.WorkspaceSummary {
	for _, workspace := range workspaces {
		if workspace.ID == id {
			return workspace
		}
	}
	return identity.WorkspaceSummary{ID: id, Name: "Workspace", Locale: "en"}
}

func cookieValue(request *http.Request, name string) string {
	cookie, err := request.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func auditContext(request *http.Request) identity.AuditContext {
	return identity.AuditContext{
		RequestID: request.Header.Get("X-Request-ID"), SourceAddress: request.RemoteAddr,
	}
}

type streamLimit struct {
	mu      sync.Mutex
	total   int
	users   map[identity.UserID]int
	maximum int
	perUser int
}

func newStreamLimit(maximum, perUser int) *streamLimit {
	return &streamLimit{users: make(map[identity.UserID]int), maximum: maximum, perUser: perUser}
}

func (limit *streamLimit) acquire(userID identity.UserID) bool {
	limit.mu.Lock()
	defer limit.mu.Unlock()
	if limit.total >= limit.maximum || limit.users[userID] >= limit.perUser {
		return false
	}
	limit.total++
	limit.users[userID]++
	return true
}

func (limit *streamLimit) release(userID identity.UserID) {
	limit.mu.Lock()
	defer limit.mu.Unlock()
	limit.total--
	limit.users[userID]--
	if limit.users[userID] == 0 {
		delete(limit.users, userID)
	}
}

// serveChangeStream holds one workspace realtime connection: the per-user limit,
// the SSE headers, the replay after Last-Event-ID, then polling with heartbeats
// and a lifetime. send writes the changes after a cursor and returns the next one.
func (h *Handler) serveChangeStream(
	writer http.ResponseWriter,
	request *http.Request,
	userID identity.UserID,
	send func(cursor string) (string, bool),
) {
	if !h.streams.acquire(userID) {
		telemetry.RecordStream(request.Context(), 0, true)
		writer.Header().Set("Content-Type", "application/problem+json")
		writer.Header().Set("Retry-After", "30")
		writer.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"type": "urn:dynamis-code:problem:stream-limit", "title": "Too Many Requests",
			"status": 429, "detail": "The realtime connection limit was reached.",
			"code": "stream-limit",
		})
		return
	}
	telemetry.RecordStream(request.Context(), 1, false)
	defer func() {
		h.streams.release(userID)
		telemetry.RecordStream(request.Context(), -1, false)
	}()
	flusher, ok := writer.(http.Flusher)
	if !ok {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)
	cursor := request.Header.Get("Last-Event-ID")
	if cursor, ok = send(cursor); !ok {
		return
	}
	flusher.Flush()
	poll := time.NewTicker(h.cfg.SSEPollInterval)
	heartbeat := time.NewTicker(h.cfg.SSEHeartbeat)
	lifetime := time.NewTimer(h.cfg.SSEMaxLifetime)
	defer poll.Stop()
	defer heartbeat.Stop()
	defer lifetime.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-lifetime.C:
			_, _ = fmt.Fprint(writer, "event: close\ndata: {\"reason\":\"lifetime\"}\n\n")
			flusher.Flush()
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(writer, ": heartbeat\n\n")
			flusher.Flush()
		case <-poll.C:
			if cursor, ok = send(cursor); !ok {
				return
			}
			flusher.Flush()
		}
	}
}

// workspaceDenialStatus answers a denied workspace request. A user who is not a
// member gets the same not-found as a missing workspace, so IDs cannot be
// probed. A member who lacks the permission gets forbidden.
func (h *Handler) workspaceDenialStatus(request *http.Request, userID identity.UserID, workspaceID identity.WorkspaceID) int {
	workspaces, err := h.identity.ListWorkspaces(request.Context(), userID)
	if err == nil && slices.ContainsFunc(workspaces, func(workspace identity.WorkspaceSummary) bool {
		return workspace.ID == workspaceID
	}) {
		return http.StatusForbidden
	}
	return http.StatusNotFound
}
