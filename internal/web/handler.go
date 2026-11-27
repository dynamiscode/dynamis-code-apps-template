package web

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	appcatalog "example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/currency"
	"example.com/dynamis-code/apps-template/internal/engagement"
	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/intake"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/listviews"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
	"example.com/dynamis-code/apps-template/internal/platform/telemetry"
	"example.com/dynamis-code/apps-template/internal/scheduling"
	"example.com/dynamis-code/apps-template/internal/transactions"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"example.com/dynamis-code/apps-template/internal/webhooks"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

//go:embed assets/app.css assets/clients.css assets/components.css assets/tokens.css
//go:embed assets/alpine-components.js assets/htmx.min.js assets/js/alpine.min.js
//go:embed assets/notes-editor.js assets/notes-editor.css
//go:embed assets/favicon.ico assets/favicon.svg
var files embed.FS

type Handler struct {
	logger          *slog.Logger
	identity        *identity.Service
	catalogService  *appcatalog.Service
	currencyService *currency.Service
	transactions    *transactions.Service
	scheduling      *scheduling.Service
	clients         *clients.Service
	engagement      *engagement.Service
	intake          *intake.Service
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
func (h *Handler) SetCurrencyService(service *currency.Service) {
	h.currencyService = service
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
	mux.HandleFunc("GET /favicon.ico", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-cache")
		writer.Header().Set("Content-Type", "image/x-icon")
		content, err := files.ReadFile("assets/favicon.ico")
		if err != nil {
			http.Error(writer, "favicon unavailable", http.StatusInternalServerError)
			return
		}
		_, _ = writer.Write(content)
	})
	mux.Handle("GET /assets/", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-cache")
		assetsHandler.ServeHTTP(writer, request)
	}))
	mux.HandleFunc("GET /language", h.language)
	mux.HandleFunc("GET /intake/{token}", h.publicIntake)
	mux.HandleFunc("POST /intake/{token}", h.publicIntake)
	mux.HandleFunc("GET /login", h.serveAuthPage(h.loginView))
	mux.HandleFunc("POST /login", h.serveAuthPage(h.loginSubmission))
	mux.HandleFunc("GET /mfa", h.serveAuthPage(h.mfaPage))
	mux.HandleFunc("POST /mfa/totp", h.serveAuthPage(h.mfaTOTP))
	mux.HandleFunc("POST /mfa/recovery", h.serveAuthPage(h.mfaRecovery))
	mux.HandleFunc("POST /mfa/passkey", h.serveAuthPage(h.mfaPasskey))
	mux.HandleFunc("GET /setup", h.serveAuthPage(h.setupView))
	mux.HandleFunc("POST /setup", h.serveAuthPage(h.setupSubmission))
	mux.HandleFunc("POST /logout", h.serveAuthPage(h.logout))
	if h.mcpResource != "" {
		mux.HandleFunc("GET /oauth/authorize", h.mcpAuthorizePage)
		mux.HandleFunc("POST /oauth/authorize", h.mcpAuthorize)
	}
	mux.HandleFunc("GET /account", h.serveAuthPage(h.accountPage))
	mux.HandleFunc("POST /account/profile", h.serveAuthPage(h.accountProfileMutation))
	mux.HandleFunc("POST /account/theme", h.serveAuthPage(h.accountThemeMutation))
	mux.HandleFunc("POST /account/notifications", h.serveAuthPage(h.accountNotificationMutation))
	mux.HandleFunc("POST /account/password", h.serveAuthPage(h.accountPasswordMutation))
	mux.HandleFunc("POST /account/delete", h.serveAuthPage(h.accountDeleteMutation))
	mux.HandleFunc("POST /account/verify-email", h.serveAuthPage(h.emailVerificationMutation))
	mux.HandleFunc("GET /account/verify-email/{secret}", h.serveAuthPage(h.emailVerification))
	mux.HandleFunc("GET /password-reset", h.serveAuthPage(h.passwordResetPage))
	mux.HandleFunc("POST /password-reset", h.serveAuthPage(h.passwordResetRequest))
	mux.HandleFunc("GET /password-reset/{secret}", h.serveAuthPage(h.passwordResetTokenPage))
	mux.HandleFunc("POST /password-reset/{secret}", h.serveAuthPage(h.passwordResetComplete))
	mux.HandleFunc("GET /notifications", h.serveAuthPage(h.notificationsPage))
	mux.HandleFunc("POST /notifications/{notificationId}", h.serveAuthPage(h.notificationMutation))
	mux.HandleFunc("GET /notifications/events", h.notificationEvents)
	mux.HandleFunc("GET /auth/oidc/{providerId}", h.serveAuthPage(h.oidcLogin))
	mux.HandleFunc("GET /auth/oidc/{providerId}/callback", h.serveAuthPage(h.oidcCallback))
	mux.HandleFunc("GET /{$}", h.home)
	if h.workspaceCreationEnabled() {
		mux.HandleFunc("POST /workspaces", h.createWorkspace)
	}
	mux.HandleFunc("GET /workspaces/{workspaceId}", h.workspaceHome)
	if h.transactions != nil {
		mux.HandleFunc("GET /workspaces/{workspaceId}/settings/transactions", h.transactionsSettingsPage)
		mux.HandleFunc("GET /workspaces/{workspaceId}/settings/transactions/categories", h.transactionsSettingsPage)
		mux.HandleFunc("GET /workspaces/{workspaceId}/settings/transactions/methods", h.transactionsSettingsPage)
		mux.HandleFunc("GET /workspaces/{workspaceId}/transactions", h.transactionsList)
		mux.HandleFunc("GET /workspaces/{workspaceId}/transactions/client-options", h.transactionsClientOptions)
		mux.HandleFunc("GET /workspaces/{workspaceId}/transactions/totals", h.transactionsTotals)
		mux.HandleFunc("POST /workspaces/{workspaceId}/transactions", h.transactionsCreate)
		mux.HandleFunc("POST /workspaces/{workspaceId}/transactions/preferences/columns/{column}", h.setTransactionsColumn)
		mux.HandleFunc("POST /workspaces/{workspaceId}/transactions/preferences/columns/reset", h.resetTransactionsColumns)
		mux.HandleFunc("POST /workspaces/{workspaceId}/transactions/preferences/columns/order", h.setTransactionsColumnOrder)
		mux.HandleFunc("POST /workspaces/{workspaceId}/transactions/categories", h.transactionsCategoryChange)
		mux.HandleFunc("POST /workspaces/{workspaceId}/transactions/categories/{categoryId}", h.transactionsCategoryChange)
		mux.HandleFunc("POST /workspaces/{workspaceId}/transactions/methods", h.transactionsSaveMethod)
		mux.HandleFunc("POST /workspaces/{workspaceId}/transactions/entries/{entryId}/void", h.transactionsVoid)
		mux.HandleFunc("POST /workspaces/{workspaceId}/transactions/entries/{entryId}/field", h.transactionsUpdateField)
	}
	mux.HandleFunc("GET /workspaces/{workspaceId}/catalog", h.catalogList)
	mux.HandleFunc("GET /workspaces/{workspaceId}/scheduling", h.schedulingList)
	mux.HandleFunc("POST /workspaces/{workspaceId}/scheduling", h.schedulingCreate)
	mux.HandleFunc("POST /workspaces/{workspaceId}/scheduling/time-zone", h.schedulingZone)
	mux.HandleFunc("POST /workspaces/{workspaceId}/scheduling/{resourceId}/edit", h.schedulingEdit)
	mux.HandleFunc("POST /workspaces/{workspaceId}/scheduling/{resourceId}/working-hours", h.schedulingHours)
	mux.HandleFunc("POST /workspaces/{workspaceId}/scheduling/{resourceId}/bookings", h.schedulingBook)
	mux.HandleFunc("POST /workspaces/{workspaceId}/scheduling/bookings/{bookingId}/engagement", h.schedulingSetEngagement)
	mux.HandleFunc("POST /workspaces/{workspaceId}/scheduling/{resourceId}/bookings/{bookingId}/status", h.schedulingBookingStatus)
	mux.HandleFunc("POST /workspaces/{workspaceId}/scheduling/{resourceId}/bookings/{bookingId}/move", h.schedulingBook)
	mux.HandleFunc("POST /workspaces/{workspaceId}/catalog", h.catalogCreate)
	mux.HandleFunc("POST /workspaces/{workspaceId}/catalog/{entryId}/prices", h.catalogCreatePrice)
	mux.HandleFunc("POST /workspaces/{workspaceId}/catalog/{entryId}/edit", h.catalogEdit)
	mux.HandleFunc("POST /workspaces/{workspaceId}/catalog/{entryId}/field", h.catalogEdit)
	mux.HandleFunc("POST /workspaces/{workspaceId}/catalog/{entryId}/prices/{priceId}/edit", h.catalogEditPrice)
	mux.HandleFunc("POST /workspaces/{workspaceId}/catalog/{entryId}/prices/{priceId}/delete", h.catalogDeletePrice)
	mux.HandleFunc("POST /workspaces/{workspaceId}/catalog/{entryId}/archive", h.catalogArchive)
	mux.HandleFunc("POST /workspaces/{workspaceId}/catalog/{entryId}/unarchive", h.catalogUnarchive)
	mux.HandleFunc("POST /workspaces/{workspaceId}/catalog/{entryId}/delete", h.catalogDelete)
	mux.HandleFunc("GET /workspaces/{workspaceId}/engagement-stages", h.engagementStages)
	mux.HandleFunc("GET /workspaces/{workspaceId}/intake", h.intakePage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/intake", h.intakeMutation)
	mux.HandleFunc("POST /workspaces/{workspaceId}/engagement-stages/label", h.setEngagementLabel)
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
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/field", h.changeClientProperty)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/engagements", h.createEngagement)
	mux.HandleFunc("GET /workspaces/{workspaceId}/clients/{clientId}/engagements/{engagementId}", h.engagementDetail)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/engagements/{engagementId}/name", h.setEngagementName)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/engagements/{engagementId}/notes", h.setEngagementNotes)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/engagements/{engagementId}/stage", h.moveEngagement)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/engagements/{engagementId}/next-action", h.setEngagementNextAction)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/engagements/{engagementId}/quote", h.setEngagementQuote)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/engagements/{engagementId}/agreement", h.setEngagementAgreement)
	mux.HandleFunc("POST /workspaces/{workspaceId}/clients/{clientId}/engagements/{engagementId}/assignee", h.setEngagementAssignee)
	mux.HandleFunc("GET /workspaces/{workspaceId}/engagements", h.listEngagements)
	mux.HandleFunc("POST /workspaces/{workspaceId}/engagements/bulk-move", h.bulkMoveEngagements)
	mux.HandleFunc("GET /workspaces/{workspaceId}/engagements/new", h.newEngagement)
	mux.HandleFunc("POST /workspaces/{workspaceId}/engagements/new", h.newEngagement)
	mux.HandleFunc("POST /workspaces/{workspaceId}/engagements/notes-preview", h.previewEngagementNotes)
	mux.HandleFunc("GET /workspaces/{workspaceId}/engagements/board", h.engagementBoard)
	mux.HandleFunc("GET /workspaces/{workspaceId}/engagements/due", h.dueEngagements)
	mux.HandleFunc("POST /workspaces/{workspaceId}/engagements/{engagementId}/action/{operation}", h.changeQueuedAction)
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
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/currencies", h.currencySettingsPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/currencies", h.currencySettingsSave)
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
	mux.HandleFunc("GET /sessions", h.serveAuthPage(h.sessionsPage))
	mux.HandleFunc("POST /sessions/{sessionId}", h.serveAuthPage(h.sessionMutation))
	mux.HandleFunc("GET /security", h.serveAuthPage(h.securityPage))
	mux.HandleFunc("POST /security", h.serveAuthPage(h.securityMutation))
	mux.HandleFunc("POST /security/totp/start", h.serveAuthPage(h.securityTOTPStart))
	mux.HandleFunc("POST /security/totp/complete", h.serveAuthPage(h.securityTOTPComplete))
	mux.HandleFunc("POST /security/totp/remove", h.serveAuthPage(h.securityTOTPRemove))
	mux.HandleFunc("POST /security/passkey/{passkeyId}", h.serveAuthPage(h.securityPasskeyRemove))
	mux.HandleFunc("GET /settings/language", h.serveAuthPage(h.languageSettingsPage))
	mux.HandleFunc("POST /settings/language", h.serveAuthPage(h.languageSettings))
	mux.HandleFunc("GET /workspaces/{workspaceId}/settings/general", h.generalSettingsPage)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/general", h.generalSettings)
	mux.HandleFunc("POST /workspaces/{workspaceId}/settings/notifications", h.workspaceNotificationSettings)
	mux.HandleFunc("GET /invitations/{secret}", h.serveInvitation(h.invitationPage))
	mux.HandleFunc("POST /invitations/{secret}", h.serveInvitation(h.invitationPost))
	mux.HandleFunc("POST /invitations/{secret}/sign-in", h.serveInvitation(h.invitationSignIn))
	mux.HandleFunc("GET /invitations/continue", h.serveInvitation(h.invitationContinuePage))
	mux.HandleFunc("POST /invitations/continue", h.serveInvitation(h.invitationContinuePost))
	return h.localeMiddleware(mux)
}

func (h *Handler) setupSubmission(request *http.Request) authPageOutput {
	available, err := h.setupAvailable(request)
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	if !available {
		return authPageOutput{status: http.StatusNotFound}
	}
	if err := request.ParseForm(); err != nil || !h.validSetupCSRF(request) {
		return h.setupOutput(http.StatusForbidden, viewmodel.PageData{
			Title: "Set up Dynamis Code", Error: "The setup form expired. Reload and try again.",
		})
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
		return h.setupOutput(http.StatusForbidden, viewmodel.PageData{
			Title: "Set up Dynamis Code", Error: "The setup token is invalid.",
		})
	}
	data := viewmodel.PageData{
		Title: "Set up Dynamis Code", Email: request.FormValue("email"),
		WorkspaceName: request.FormValue("workspace"), WorkspaceLocale: request.FormValue("workspace_locale"),
	}
	if request.FormValue("password") != request.FormValue("password_confirmation") {
		data.Error = "The passwords do not match."
		return h.setupOutput(http.StatusUnprocessableEntity, data)
	}
	_, err = h.identity.BootstrapFirstOwner(request.Context(), identity.BootstrapInput{
		Email: data.Email, Password: request.FormValue("password"), WorkspaceName: data.WorkspaceName,
		WorkspaceLocale: data.WorkspaceLocale,
	}, auditContext(request))
	if errors.Is(err, identity.ErrAlreadyBootstrapped) {
		return authPageOutput{kind: authPageRedirect, target: "/login"}
	}
	if errors.Is(err, identity.ErrInvalidBootstrap) {
		data.Error = "Enter a valid email, workspace name, and password."
		return h.setupOutput(http.StatusUnprocessableEntity, data)
	}
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	return authPageOutput{kind: authPageSetupCompleted}
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

func (h *Handler) loginSubmission(request *http.Request) authPageOutput {
	if err := request.ParseForm(); err != nil || !h.validLoginCSRF(request) {
		return authPageOutput{kind: authPageLogin, status: http.StatusForbidden, data: viewmodel.PageData{
			Title: "Sign in", Error: "The sign-in form expired. Reload and try again.",
		}}
	}
	userID, err := h.identity.AuthenticateLocal(
		request.Context(), request.FormValue("email"), request.FormValue("password"),
	)
	if err != nil {
		return authPageOutput{kind: authPageLogin, status: http.StatusUnauthorized, data: viewmodel.PageData{
			Title: "Sign in", CSRF: request.FormValue("csrf"), Email: strings.TrimSpace(request.FormValue("email")),
			Error:         "The email or password is invalid.",
			OIDCProviders: h.providers(), ReturnTo: h.invitationLoginReturnTo(request, request.FormValue("return_to")),
		}}
	}
	mfaEnrolled, err := h.identity.MFAEnrolled(request.Context(), userID)
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	if mfaEnrolled {
		challenge, err := h.identity.BeginMFALogin(request.Context(), userID, auditContext(request))
		if err != nil {
			return authPageOutput{status: http.StatusInternalServerError}
		}
		return authPageOutput{kind: authPageMFALogin, challenge: challenge, target: h.invitationLoginReturnTo(request, request.FormValue("return_to"))}
	}
	session, err := h.identity.CreateSession(
		request.Context(), userID, "local", "", 0, auditContext(request),
	)
	if err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	return authPageOutput{kind: authPageSignedIn, session: session, target: h.invitationLoginReturnTo(request, request.FormValue("return_to"))}
}

func (h *Handler) logout(request *http.Request) authPageOutput {
	continueInvitation := h.invitationReturnActive(request)
	session, status := h.checkCSRFSession(request)
	if status != http.StatusOK {
		return authSessionFailure(status)
	}
	providerID, err := h.identity.RevokeSession(
		request.Context(), session.UserID, session.ID, auditContext(request),
	)
	if err != nil {
		return authPageOutput{status: http.StatusUnauthorized}
	}
	if continueInvitation {
		return authPageOutput{kind: authPageInvitationLogout}
	}
	if h.oidc != nil {
		if logoutURL, ok := h.oidc.LogoutURL(providerID); ok {
			return authPageOutput{kind: authPageProviderLogout, target: logoutURL}
		}
	}
	return authPageOutput{kind: authPageLocalLogout}
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
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceRead)
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
		CanEditClients: principal.Permissions[identity.ResourcesWrite],
	}
	if h.engagement != nil && principal.Permissions[identity.ResourcesRead] {
		page := 1
		if value := request.URL.Query().Get("page"); value != "" {
			page, err = strconv.Atoi(value)
			if err != nil || page < 1 || page > clients.MaxListPage {
				h.renderError(writer, http.StatusBadRequest)
				return
			}
		}
		dashboard, err := h.engagement.Dashboard(request.Context(), principal, workspaceID, page, 20)
		if err != nil {
			if errors.Is(err, identity.ErrForbidden) {
				h.renderError(writer, http.StatusForbidden)
			} else {
				h.renderError(writer, http.StatusInternalServerError)
			}
			return
		}
		data.EngagementDashboard = &dashboard
		path := "/workspaces/" + workspaceID.String() + "?page="
		if page > 1 {
			data.DashboardPrevious = path + strconv.Itoa(page-1)
		}
		if dashboard.HasMore && page < clients.MaxListPage {
			data.DashboardNext = path + strconv.Itoa(page+1)
		}
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
	if state, ok := writer.(*localeWriter); ok && h.engagement != nil && principal.Permissions[identity.ResourcesRead] && (!state.engagementLoaded || state.engagementWorkspace != workspaceID) {
		label, err := h.engagement.Label(request.Context(), principal, workspaceID)
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return identity.Principal{}, identity.Session{}, "", false
		}
		state.engagementWorkspace, state.engagementLabel, state.engagementLoaded = workspaceID, label, true
	}
	return principal, session, csrf, true
}

func (h *Handler) session(
	writer http.ResponseWriter,
	request *http.Request,
) (identity.Session, string, bool) {
	session, csrf, ok := h.checkSession(request)
	if !ok {
		h.redirect(writer, request, "/login")
	}
	return session, csrf, ok
}

func (h *Handler) checkSession(request *http.Request) (identity.Session, string, bool) {
	secret := cookieValue(request, "session")
	csrf := cookieValue(request, "csrf")
	if secret == "" || csrf == "" {
		return identity.Session{}, "", false
	}
	session, err := h.identity.AuthenticateSession(request.Context(), secret)
	if err != nil {
		return identity.Session{}, "", false
	}
	return session, csrf, true
}

func (h *Handler) validCSRFSession(writer http.ResponseWriter, request *http.Request) (identity.Session, bool) {
	session, status := h.checkCSRFSession(request)
	switch status {
	case http.StatusOK:
		return session, true
	case http.StatusSeeOther:
		h.redirect(writer, request, "/login")
	default:
		h.renderError(writer, status)
	}
	return identity.Session{}, false
}

func (h *Handler) checkCSRFSession(request *http.Request) (identity.Session, int) {
	if err := request.ParseForm(); err != nil {
		return identity.Session{}, http.StatusBadRequest
	}
	session, _, ok := h.checkSession(request)
	if !ok {
		return identity.Session{}, http.StatusSeeOther
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		return identity.Session{}, http.StatusForbidden
	}
	return session, http.StatusOK
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
		HttpOnly: httpOnly, Secure: true, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearCookie(writer http.ResponseWriter, name string) {
	http.SetCookie(writer, &http.Cookie{
		Name: name, Path: "/", MaxAge: -1, Expires: time.Unix(1, 0),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
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
	data.FilesEnabled = h.files != nil
	data.ClientsEnabled = h.clients != nil
	data.CatalogEnabled = h.catalogService != nil
	data.SchedulingEnabled = h.scheduling != nil
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
