package web

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
	"example.com/dynamis-code/apps-template/internal/platform/telemetry"
	"example.com/dynamis-code/apps-template/internal/sharing"
)

//go:embed assets/* templates/*
var files embed.FS

type Handler struct {
	identity       *identity.Service
	items          *items.Service
	files          *appfiles.Service
	sharing        *sharing.Service
	exporter       exporter
	oidc           *identity.OIDCRegistry
	publicURL      string
	mailer         appmail.Sender
	cfg            config.HTTP
	setupTokenHash string
	template       *template.Template
	streams        *streamLimit
	catalog        *i18n.Catalog
}

type pageData struct {
	Locale                           string
	Catalog                          *i18n.Catalog
	Title                            string
	NavPage                          string
	NavSection                       string
	Error                            string
	CSRF                             string
	Workspace                        identity.WorkspaceSummary
	Workspaces                       []identity.WorkspaceSummary
	Items                            []items.Item
	Files                            []appfiles.File
	FilesEnabled                     bool
	FilesPresigned                   bool
	NextCursor                       string
	CreateKey                        string
	CurrentPath                      string
	Email                            string
	UserLocale                       string
	Saved                            bool
	WorkspaceName                    string
	WorkspaceLocale                  string
	SetupTokenRequired               bool
	Members                          []identity.MemberSummary
	Invitations                      []identity.Invitation
	Tokens                           []identity.APIToken
	Sessions                         []identity.Session
	Profile                          identity.UserProfile
	NotificationPreferences          []identity.NotificationPreference
	WorkspaceNotificationPreferences []identity.NotificationPreference
	Notifications                    []identity.Notification
	UnreadNotifications              int
	PasswordResetSecret              string
	PasswordResetCompleted           bool
	CurrentAuthMethod                string
	OIDCProviders                    []identity.OIDCProviderInfo
	Invitation                       identity.Invitation
	InvitationSecret                 string
	InvitationAuthenticated          bool
	InvitationURL                    string
	TokenSecret                      string
	DeliveryWarning                  string
	CanManage                        bool
	CanTransfer                      bool
	CanShare                         bool
	ShareLinks                       []sharing.Link
	PublicShareURL                   string
	PublicShareItemID                string
	PublicItem                       sharing.PublicItem
	ReturnTo                         string
}

func (p pageData) T(key string, values ...any) string {
	args := map[string]any{}
	if len(values) == 1 {
		if supplied, ok := values[0].(map[string]any); ok {
			args = supplied
		}
	}
	return p.Catalog.Translate(i18n.Locale(p.Locale), key, args)
}

func (p pageData) Date(value time.Time) string {
	return i18n.FormatDate(i18n.Locale(p.Locale), value)
}

func (p pageData) DateTime(value time.Time) string {
	return i18n.FormatDateTime(i18n.Locale(p.Locale), value)
}

func (p pageData) Role(value identity.Role) string {
	keys := map[identity.Role]string{
		identity.Owner: "members.owner", identity.Admin: "members.admin",
		identity.Member: "members.member", identity.Viewer: "members.viewer",
	}
	return p.T(keys[value])
}

func (p pageData) Scope(value identity.Permission) string {
	keys := map[identity.Permission]string{
		identity.WorkspaceRead: "tokens.workspace_read", identity.ResourcesRead: "tokens.resources_read",
		identity.WorkspaceUpdate: "tokens.workspace_update",
		identity.ResourcesWrite:  "tokens.resources_write", identity.WorkspaceExport: "tokens.workspace_export",
	}
	return p.T(keys[value])
}

func (p pageData) ScopeList(values []identity.Permission) string {
	labels := make([]string, 0, len(values))
	for _, value := range values {
		labels = append(labels, p.Scope(value))
	}
	return strings.Join(labels, ", ")
}

func (p pageData) AuthMethod(value string) string {
	if value == "oidc" {
		return p.T("security.oidc")
	}
	return p.T("security.local")
}

func NewHandler(
	identityService *identity.Service,
	itemService *items.Service,
	cfg config.HTTP,
	setupToken string,
) (*Handler, error) {
	return NewHandlerWithServices(
		identityService, itemService, nil, nil, nil, cfg, setupToken, "", nil,
	)
}

func NewHandlerWithServicesAndFiles(
	identityService *identity.Service,
	itemService *items.Service,
	exporterService exporter,
	oidcRegistry *identity.OIDCRegistry,
	cfg config.HTTP,
	fileService *appfiles.Service,
	setupToken string,
	publicURL string,
	mailer appmail.Sender,
) (*Handler, error) {
	return newHandler(identityService, itemService, nil, exporterService, oidcRegistry, cfg, fileService, setupToken, publicURL, mailer)
}

func NewHandlerWithServicesAndFilesAndSharing(
	identityService *identity.Service,
	itemService *items.Service,
	sharingService *sharing.Service,
	exporterService exporter,
	oidcRegistry *identity.OIDCRegistry,
	cfg config.HTTP,
	fileService *appfiles.Service,
	setupToken string,
	publicURL string,
	mailer appmail.Sender,
) (*Handler, error) {
	return newHandler(identityService, itemService, sharingService, exporterService, oidcRegistry, cfg, fileService, setupToken, publicURL, mailer)
}

func NewHandlerWithServices(
	identityService *identity.Service,
	itemService *items.Service,
	sharingService *sharing.Service,
	exporterService exporter,
	oidcRegistry *identity.OIDCRegistry,
	cfg config.HTTP,
	setupToken string,
	publicURL string,
	mailer appmail.Sender,
) (*Handler, error) {
	return newHandler(identityService, itemService, sharingService, exporterService, oidcRegistry, cfg, nil, setupToken, publicURL, mailer)
}

func newHandler(
	identityService *identity.Service,
	itemService *items.Service,
	sharingService *sharing.Service,
	exporterService exporter,
	oidcRegistry *identity.OIDCRegistry,
	cfg config.HTTP,
	fileService *appfiles.Service,
	setupToken string,
	publicURL string,
	mailer appmail.Sender,
) (*Handler, error) {
	catalog, err := i18n.New()
	if err != nil {
		return nil, err
	}
	templates, err := template.New("root").Funcs(template.FuncMap{
		"dict": func(values ...any) map[string]any {
			result := make(map[string]any, len(values)/2)
			for index := 0; index+1 < len(values); index += 2 {
				if key, ok := values[index].(string); ok {
					result[key] = values[index+1]
				}
			}
			return result
		},
	}).ParseFS(files, "templates/*.html")
	if err != nil {
		return nil, err
	}
	setupTokenHash := ""
	if setupToken != "" {
		setupTokenHash = identity.SecretHash(setupToken)
	}
	return &Handler{
		identity: identityService, items: itemService, files: fileService, sharing: sharingService, exporter: exporterService,
		oidc: oidcRegistry, publicURL: publicURL, mailer: mailer, cfg: cfg,
		setupTokenHash: setupTokenHash,
		template:       templates,
		streams:        newStreamLimit(cfg.SSEMaxConnections, cfg.SSEMaxPerUser),
		catalog:        catalog,
	}, nil
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	assets, _ := fs.Sub(files, "assets")
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(assets)))
	mux.HandleFunc("GET /language", h.language)
	mux.HandleFunc("GET /login", h.loginPage)
	mux.HandleFunc("POST /login", h.login)
	mux.HandleFunc("GET /setup", h.setupPage)
	mux.HandleFunc("POST /setup", h.setup)
	mux.HandleFunc("POST /logout", h.logout)
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
	mux.HandleFunc("GET /share/{token}", h.publicShare)
	mux.HandleFunc("POST /password-reset/{secret}", h.passwordResetComplete)
	mux.HandleFunc("GET /notifications", h.notificationsPage)
	mux.HandleFunc("POST /notifications/{notificationId}", h.notificationMutation)
	mux.HandleFunc("GET /notifications/events", h.notificationEvents)
	mux.HandleFunc("GET /auth/oidc/{providerId}", h.oidcLogin)
	mux.HandleFunc("GET /auth/oidc/{providerId}/callback", h.oidcCallback)
	mux.HandleFunc("GET /{$}", h.home)
	mux.HandleFunc("POST /workspaces", h.createWorkspace)
	mux.HandleFunc("GET /workspaces/{workspaceId}", h.workspaceHome)
	mux.HandleFunc("GET /workspaces/{workspaceId}/items", h.itemList)
	mux.HandleFunc("POST /workspaces/{workspaceId}/items", h.createItem)
	mux.HandleFunc("POST /workspaces/{workspaceId}/items/{itemId}", h.changeItem)
	mux.HandleFunc("POST /workspaces/{workspaceId}/items/{itemId}/share", h.shareMutation)
	mux.HandleFunc("GET /workspaces/{workspaceId}/items/events", h.itemEvents)
	if h.files != nil {
		mux.HandleFunc("GET /workspaces/{workspaceId}/files", h.filesPage)
		mux.HandleFunc("POST /workspaces/{workspaceId}/files", h.filesUpload)
		mux.HandleFunc("POST /workspaces/{workspaceId}/files/initiate", h.filesInitiateUpload)
		mux.HandleFunc("POST /workspaces/{workspaceId}/files/{fileId}/complete", h.filesCompleteUpload)
		mux.HandleFunc("GET /workspaces/{workspaceId}/files/{fileId}/content", h.fileDownload)
	}
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings", h.settingsPage)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/members", h.membersPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/members", h.memberMutation)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/invitations", h.invitationsPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/invitations", h.invitationMutation)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/tokens", h.tokensPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/tokens", h.tokenMutation)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/tokens/{tokenId}", h.tokenMutation)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/export", h.exportPage)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/export/download", h.exportWorkspace)
	mux.HandleFunc("GET /sessions", h.sessionsPage)
	mux.HandleFunc("POST /sessions/{sessionId}", h.sessionMutation)
	mux.HandleFunc("GET /security", h.securityPage)
	mux.HandleFunc("POST /security", h.securityMutation)
	mux.HandleFunc("GET /settings/language", h.languageSettingsPage)
	mux.HandleFunc("POST /settings/language", h.languageSettings)
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/general", h.generalSettingsPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/general", h.generalSettings)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/notifications", h.workspaceNotificationSettings)
	mux.HandleFunc("GET /invitations/{secret}", h.invitationPage)
	mux.HandleFunc("POST /invitations/{secret}", h.invitationPost)
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
	h.render(writer, http.StatusOK, "login.html", pageData{
		Title: "Sign in", CSRF: token, OIDCProviders: h.providers(),
		ReturnTo: safeReturnTo(request.URL.Query().Get("return_to")),
	})
}

func (h *Handler) setupPage(writer http.ResponseWriter, request *http.Request) {
	if !h.requireSetup(writer, request) {
		return
	}
	h.renderSetup(writer, http.StatusOK, pageData{Title: "Set up Dynamis Code"})
}

func (h *Handler) setup(writer http.ResponseWriter, request *http.Request) {
	if !h.requireSetup(writer, request) {
		return
	}
	if err := request.ParseForm(); err != nil || !h.validSetupCSRF(request) {
		h.renderSetup(writer, http.StatusForbidden, pageData{
			Title: "Set up Dynamis Code", Error: "The setup form expired. Reload and try again.",
		})
		return
	}
	if h.setupTokenHash != "" && !identity.EqualSecretHash(request.FormValue("setup_token"), h.setupTokenHash) {
		h.renderSetup(writer, http.StatusForbidden, pageData{
			Title: "Set up Dynamis Code", Error: "The setup token is invalid.",
		})
		return
	}
	data := pageData{
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
	setupRemoteConfigurationRequired
)

func (h *Handler) setupAvailability(request *http.Request) (setupAvailability, error) {
	bootstrapped, err := h.identity.IsBootstrapped(request.Context())
	if err != nil {
		return setupDisabled, err
	}
	if bootstrapped {
		return setupDisabled, nil
	}
	if h.setupTokenHash != "" || isLoopbackRequest(request) {
		return setupEnabled, nil
	}
	return setupRemoteConfigurationRequired, nil
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
	if availability == setupRemoteConfigurationRequired {
		h.renderSetupUnavailable(writer)
		return false
	}
	if availability != setupEnabled {
		h.renderError(writer, http.StatusNotFound)
		return false
	}
	return true
}

func (h *Handler) renderSetup(writer http.ResponseWriter, status int, data pageData) {
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

func (h *Handler) renderSetupUnavailable(writer http.ResponseWriter) {
	h.render(writer, http.StatusServiceUnavailable, "error.html", pageData{
		Title: "Remote setup required",
		Error: "Remote browser setup is disabled. Set BOOTSTRAP_SETUP_TOKEN or all BOOTSTRAP_ADMIN_* variables, then restart the server.",
	})
}

func isLoopbackRequest(request *http.Request) bool {
	return isLoopbackHost(requestHost(request.Host)) &&
		isLoopbackHost(requestHost(request.RemoteAddr))
}

func requestHost(value string) string {
	value = strings.TrimSpace(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	return strings.Trim(value, "[]")
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (h *Handler) login(writer http.ResponseWriter, request *http.Request) {
	if err := request.ParseForm(); err != nil || !h.validLoginCSRF(request) {
		h.render(writer, http.StatusForbidden, "login.html", pageData{
			Title: "Sign in", Error: "The sign-in form expired. Reload and try again.",
		})
		return
	}
	userID, err := h.identity.AuthenticateLocal(
		request.Context(), request.FormValue("email"), request.FormValue("password"),
	)
	if err != nil {
		h.render(writer, http.StatusUnauthorized, "login.html", pageData{
			Title: "Sign in", CSRF: request.FormValue("csrf"),
			Error:         "The email or password is invalid.",
			OIDCProviders: h.providers(), ReturnTo: safeReturnTo(request.FormValue("return_to")),
		})
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
	h.redirect(writer, request, safeReturnTo(request.FormValue("return_to")))
}

func (h *Handler) logout(writer http.ResponseWriter, request *http.Request) {
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
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusOK, "home.html", pageData{
		Title: "Workspaces", NavPage: "home", CSRF: csrf, Workspaces: workspaces,
	})
}

func (h *Handler) workspaceHome(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	_, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceRead)
	if !ok {
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusOK, "workspace.html", pageData{
		Title: "Workspace home", NavPage: "workspace", NavSection: "workspace",
		CSRF: csrf, Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
	})
}

func (h *Handler) itemList(writer http.ResponseWriter, request *http.Request) {
	h.renderItems(writer, request, http.StatusOK, "")
}

func (h *Handler) createItem(writer http.ResponseWriter, request *http.Request) {
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	_, err := h.items.Create(
		request.Context(), principal, request.PathValue("workspaceId"),
		request.FormValue("title"), request.FormValue("idempotency_key"),
		auditContext(request),
	)
	if err != nil {
		if errors.Is(err, items.ErrInvalidInput) {
			h.renderItems(writer, request, http.StatusUnprocessableEntity, "Enter a title between 1 and 200 characters.")
			return
		}
		h.itemError(writer, request, err)
		return
	}
	h.afterMutation(writer, request)
}

func (h *Handler) changeItem(writer http.ResponseWriter, request *http.Request) {
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	version, err := strconv.ParseInt(request.FormValue("version"), 10, 64)
	if err != nil || version < 1 {
		h.renderItems(writer, request, http.StatusBadRequest, "The item version is invalid. Reload and try again.")
		return
	}
	workspaceID := request.PathValue("workspaceId")
	itemID := request.PathValue("itemId")
	switch request.FormValue("action") {
	case "delete":
		err = h.items.Delete(
			request.Context(), principal, workspaceID, itemID, version, auditContext(request),
		)
	case "update":
		title := request.FormValue("title")
		status := items.Status(request.FormValue("status"))
		_, err = h.items.Update(
			request.Context(), principal, workspaceID, itemID, version,
			items.UpdateInput{Title: &title, Status: &status}, auditContext(request),
		)
	default:
		h.renderItems(writer, request, http.StatusBadRequest, "The requested item action is invalid.")
		return
	}
	if err != nil {
		h.itemError(writer, request, err)
		return
	}
	h.afterMutation(writer, request)
}

func (h *Handler) afterMutation(writer http.ResponseWriter, request *http.Request) {
	if request.Header.Get("HX-Request") == "true" {
		h.renderItems(writer, request, http.StatusOK, "")
		return
	}
	h.redirect(writer, request, "/workspaces/"+request.PathValue("workspaceId")+"/items")
}

func (h *Handler) renderItems(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	message string,
	shareURL ...string,
) {
	workspaceID := request.PathValue("workspaceId")
	principal, session, csrf, ok := h.workspaceSession(
		writer, request, workspaceID, identity.ResourcesRead,
	)
	if !ok {
		return
	}
	limit := h.cfg.DefaultPageSize
	cursor := request.URL.Query().Get("cursor")
	page, err := h.items.List(request.Context(), principal, workspaceID, items.ListInput{
		Sort: "-created_at", Limit: limit, Cursor: cursor,
	})
	if err != nil {
		h.itemError(writer, request, err)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	workspace := workspaceByID(workspaces, workspaceID)
	createKey, err := id.New()
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	data := pageData{
		Title: workspace.Name + " items", Error: message, CSRF: csrf,
		Workspace: workspace, Items: page.Items, NextCursor: page.NextCursor,
		Workspaces: workspaces, NavPage: "items", NavSection: "items", CreateKey: createKey,
		CurrentPath: "/workspaces/" + workspaceID + "/items",
		CanShare:    h.sharing != nil && principal.Permissions[identity.ResourcesWrite],
	}
	if h.sharing != nil && data.CanShare {
		data.ShareLinks, err = h.sharing.List(request.Context(), principal, workspaceID)
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
	}
	if len(shareURL) > 0 {
		data.PublicShareURL = shareURL[0]
		data.PublicShareItemID = request.PathValue("itemId")
	}
	name := "items.html"
	if request.Header.Get("HX-Request") == "true" {
		name = "item-list.html"
		if status >= http.StatusBadRequest {
			writer.Header().Set("HX-Trigger", "form-error")
			status = http.StatusOK
		}
	}
	h.render(writer, status, name, data)
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
		writer, request, request.PathValue("workspaceId"), identity.ResourcesWrite,
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

func (h *Handler) itemError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(writer, http.StatusForbidden)
	case errors.Is(err, items.ErrNotFound):
		h.renderError(writer, http.StatusNotFound)
	case errors.Is(err, items.ErrPreconditionFailed):
		h.renderItems(writer, request, http.StatusPreconditionFailed, "This item changed. Review the current value and try again.")
	case errors.Is(err, items.ErrInvalidInput):
		h.renderItems(writer, request, http.StatusUnprocessableEntity, "The item values are invalid.")
	case errors.Is(err, items.ErrLimit):
		h.renderItems(writer, request, http.StatusConflict, "The workspace item limit was reached. Delete an item before retrying.")
	default:
		h.renderError(writer, http.StatusInternalServerError)
	}
}

func (h *Handler) workspaceSession(
	writer http.ResponseWriter,
	request *http.Request,
	workspaceID string,
	permission identity.Permission,
) (identity.Principal, identity.Session, string, bool) {
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return identity.Principal{}, identity.Session{}, "", false
	}
	principal, err := h.identity.Authorize(
		request.Context(), session.UserID, workspaceID, permission,
	)
	if err != nil {
		h.renderError(writer, http.StatusForbidden)
		return identity.Principal{}, identity.Session{}, "", false
	}
	principal.AuthMethod = session.AuthMethod
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
	http.Redirect(writer, request, safeReturnTo(path), http.StatusSeeOther)
}

func (h *Handler) render(writer http.ResponseWriter, status int, name string, data pageData) {
	data.FilesEnabled = h.files != nil
	h.localizePage(writer, &data)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	if err := h.template.ExecuteTemplate(writer, name, data); err != nil {
		return
	}
}

func (h *Handler) publicShare(writer http.ResponseWriter, request *http.Request) {
	if h.sharing == nil {
		h.renderPublicError(writer)
		return
	}
	item, err := h.sharing.Resolve(request.Context(), request.PathValue("token"), auditContext(request))
	if err != nil {
		h.renderPublicError(writer)
		return
	}
	h.renderPublic(writer, http.StatusOK, pageData{Title: "Shared item", PublicItem: item})
}

func (h *Handler) renderPublic(writer http.ResponseWriter, status int, data pageData) {
	h.localizePage(writer, &data)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	writer.WriteHeader(status)
	_ = h.template.ExecuteTemplate(writer, "public-share.html", data)
}

func (h *Handler) renderPublicError(writer http.ResponseWriter) {
	h.renderPublic(writer, http.StatusNotFound, pageData{Title: "Shared item unavailable", Error: "This shared item is unavailable."})
}

func (h *Handler) renderError(writer http.ResponseWriter, status int) {
	h.render(writer, status, "error.html", pageData{
		Title: http.StatusText(status), Error: "The request could not be completed.",
	})
}

func workspaceByID(workspaces []identity.WorkspaceSummary, id string) identity.WorkspaceSummary {
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
	users   map[string]int
	maximum int
	perUser int
}

func newStreamLimit(maximum, perUser int) *streamLimit {
	return &streamLimit{users: make(map[string]int), maximum: maximum, perUser: perUser}
}

func (limit *streamLimit) acquire(userID string) bool {
	limit.mu.Lock()
	defer limit.mu.Unlock()
	if limit.total >= limit.maximum || limit.users[userID] >= limit.perUser {
		return false
	}
	limit.total++
	limit.users[userID]++
	return true
}

func (limit *streamLimit) release(userID string) {
	limit.mu.Lock()
	defer limit.mu.Unlock()
	limit.total--
	limit.users[userID]--
	if limit.users[userID] == 0 {
		delete(limit.users, userID)
	}
}

func (h *Handler) itemEvents(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, _, _, ok := h.workspaceSession(
		writer, request, workspaceID, identity.ResourcesRead,
	)
	if !ok {
		return
	}
	if !h.streams.acquire(principal.UserID) {
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
		h.streams.release(principal.UserID)
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
	after := request.Header.Get("Last-Event-ID")
	if !h.sendChanges(writer, request, principal, workspaceID, &after) {
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
			if !h.sendChanges(writer, request, principal, workspaceID, &after) {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *Handler) sendChanges(
	writer http.ResponseWriter,
	request *http.Request,
	principal identity.Principal,
	workspaceID string,
	after *string,
) bool {
	page, err := h.items.Changes(
		request.Context(), principal, workspaceID, *after, 100,
	)
	if err != nil {
		return false
	}
	if page.Resync {
		if page.Next != "" {
			_, _ = fmt.Fprintf(writer, "id: %s\n", page.Next)
		}
		_, _ = fmt.Fprint(writer, "event: resync\ndata: {\"reason\":\"cursor\"}\n\n")
		*after = page.Next
		return true
	}
	for _, change := range page.Changes {
		encoded, err := json.Marshal(change)
		if err != nil {
			return false
		}
		_, _ = fmt.Fprintf(
			writer, "id: %s\nevent: item.changed\ndata: %s\n\n",
			change.ID, encoded,
		)
		*after = change.ID
	}
	return true
}
