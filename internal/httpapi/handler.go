package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/api"
	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
	"example.com/dynamis-code/apps-template/internal/platform/telemetry"
	"example.com/dynamis-code/apps-template/internal/portability"
	"example.com/dynamis-code/apps-template/internal/webhooks"
)

type handler struct {
	db          *sql.DB
	identity    *identity.Service
	items       *items.Service
	oidc        *identity.OIDCRegistry
	cfg         config.HTTP
	logger      *slog.Logger
	portability *portability.Service
	publicURL   string
	mailer      appmail.Sender
	webhooks    *webhooks.Service
	files       *appfiles.Service
}

func NewHandler(
	db *sql.DB,
	identityService *identity.Service,
	itemService *items.Service,
	portabilityService *portability.Service,
	oidcRegistry *identity.OIDCRegistry,
	cfg config.HTTP,
	logger *slog.Logger,
) (http.Handler, error) {
	return NewHandlerWithWebhooks(
		db, identityService, itemService, portabilityService, oidcRegistry,
		cfg, logger, nil, "", nil,
	)
}

func NewHandlerWithWebhooksAndFiles(
	db *sql.DB,
	identityService *identity.Service,
	itemService *items.Service,
	portabilityService *portability.Service,
	oidcRegistry *identity.OIDCRegistry,
	cfg config.HTTP,
	logger *slog.Logger,
	webhookService *webhooks.Service,
	fileService *appfiles.Service,
	publicURL string,
	mailer appmail.Sender,
) (http.Handler, error) {
	return newHandler(db, identityService, itemService, portabilityService, oidcRegistry, cfg, logger, webhookService, fileService, publicURL, mailer)
}

func NewHandlerWithMail(
	db *sql.DB,
	identityService *identity.Service,
	itemService *items.Service,
	portabilityService *portability.Service,
	oidcRegistry *identity.OIDCRegistry,
	cfg config.HTTP,
	logger *slog.Logger,
	publicURL string,
	mailer appmail.Sender,
) (http.Handler, error) {
	return NewHandlerWithWebhooks(
		db, identityService, itemService, portabilityService, oidcRegistry,
		cfg, logger, nil, publicURL, mailer,
	)
}

func NewHandlerWithWebhooks(
	db *sql.DB,
	identityService *identity.Service,
	itemService *items.Service,
	portabilityService *portability.Service,
	oidcRegistry *identity.OIDCRegistry,
	cfg config.HTTP,
	logger *slog.Logger,
	webhookService *webhooks.Service,
	publicURL string,
	mailer appmail.Sender,
) (http.Handler, error) {
	return newHandler(db, identityService, itemService, portabilityService, oidcRegistry, cfg, logger, webhookService, nil, publicURL, mailer)
}

func newHandler(
	db *sql.DB,
	identityService *identity.Service,
	itemService *items.Service,
	portabilityService *portability.Service,
	oidcRegistry *identity.OIDCRegistry,
	cfg config.HTTP,
	logger *slog.Logger,
	webhookService *webhooks.Service,
	fileService *appfiles.Service,
	publicURL string,
	mailer appmail.Sender,
) (http.Handler, error) {
	if logger == nil {
		logger = slog.Default()
	}
	h := &handler{
		db: db, identity: identityService, items: itemService,
		oidc: oidcRegistry, cfg: cfg, logger: logger, portability: portabilityService,
		publicURL: publicURL, mailer: mailer, webhooks: webhookService, files: fileService,
	}
	handlers := map[string]http.HandlerFunc{
		"getLiveness": h.liveness, "getReadiness": h.readiness,
		"getOpenAPI": h.openAPI, "loginLocal": h.login,
		"logoutLocal": h.logout, "listItems": h.listItems,
		"createItem": h.createItem, "getItem": h.getItem,
		"updateItem": h.updateItem, "deleteItem": h.deleteItem,
		"exportWorkspace": h.exportWorkspace, "importWorkspace": h.importWorkspace,
		"listMembers": h.listMembers, "changeMemberRole": h.changeMemberRole,
		"removeMember": h.removeMember, "transferOwnership": h.transferOwnership,
		"listInvitations": h.listInvitations, "createInvitation": h.createInvitation,
		"resendInvitation": h.resendInvitation, "revokeInvitation": h.revokeInvitation,
		"listTokens": h.listTokens, "createToken": h.createToken,
		"updateToken": h.updateToken, "revokeToken": h.revokeToken,
		"listSessions": h.listSessions, "revokeSession": h.revokeSession,
		"listWebhooks": h.listWebhooks, "createWebhook": h.createWebhook,
		"deleteWebhook": h.deleteWebhook, "rotateWebhookSecret": h.rotateWebhookSecret,
		"listWebhookDeliveries": h.listWebhookDeliveries,
		"listFiles":             h.listFiles, "createFile": h.createFile,
		"getFile": h.getFile, "getFileContent": h.getFileContent,
		"initiateFileUpload": h.initiateFileUpload,
		"completeFileUpload": h.completeFileUpload, "uploadFileContent": h.uploadFileContent,
		"createSCIMToken": h.createSCIMToken, "revokeSCIMToken": h.revokeSCIMToken,
		"listSCIMUsers": h.listSCIMUsers, "createSCIMUser": h.createSCIMUser,
		"getSCIMUser": h.getSCIMUser, "patchSCIMUser": h.patchSCIMUser,
		"deleteSCIMUser": h.deleteSCIMUser, "listSCIMGroups": h.listSCIMGroups,
		"getSCIMGroup": h.getSCIMGroup, "patchSCIMGroup": h.patchSCIMGroup,
	}
	mux := http.NewServeMux()
	paths := make(map[string]map[string]http.HandlerFunc)
	for _, operation := range api.Operations {
		handle, ok := handlers[operation.ID]
		if !ok {
			return nil, fmt.Errorf("OpenAPI operation %q has no handler", operation.ID)
		}
		if paths[operation.Path] == nil {
			paths[operation.Path] = make(map[string]http.HandlerFunc)
		}
		paths[operation.Path][operation.Method] = handle
	}
	for path, methods := range paths {
		methods := methods
		mux.HandleFunc(path, func(writer http.ResponseWriter, request *http.Request) {
			handle, ok := methods[request.Method]
			if !ok {
				writer.Header().Set("Allow", allowedMethods(methods))
				methodProblem(writer, request)
				return
			}
			handle(writer, request)
		})
	}
	mux.HandleFunc("/", notFoundProblem)
	return Wrap(mux, cfg, logger), nil
}

func (h *handler) exportWorkspace(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The export parameters are invalid.")
		return
	}
	principal, ok := h.bearerPrincipal(writer, request, identity.WorkspaceExport)
	if !ok {
		return
	}
	encoded, err := h.portability.Export(
		request.Context(), principal, workspaceID, h.auditContext(request),
	)
	if errors.Is(err, identity.ErrForbidden) {
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
		return
	}
	if errors.Is(err, portability.ErrLimit) {
		writeProblem(
			writer, request, http.StatusConflict, "export-limit",
			"The workspace export limit was reached. Reduce retained data or raise the configured limit.",
		)
		return
	}
	if err != nil {
		h.internal(writer, request)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Disposition", `attachment; filename="workspace-export.json"`)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(encoded)
}

func (h *handler) importWorkspace(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The import parameters are invalid.")
		return
	}
	principal, ok := h.bearerPrincipal(writer, request, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || (mediaType != "application/json" && mediaType != "text/csv") {
		h.invalidRequest(writer, request, "The import content type is unsupported.")
		return
	}
	result, err := h.portability.Import(
		request.Context(), principal, workspaceID,
		portability.ImportInput{Format: mediaType, Reader: request.Body},
		h.auditContext(request),
	)
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, portability.ErrImportLimit), errors.Is(err, items.ErrLimit):
		writeProblem(writer, request, http.StatusConflict, "import-limit", "The workspace import limit was reached.")
	case errors.Is(err, portability.ErrInvalidImport), errors.Is(err, items.ErrInvalidInput):
		writeProblem(writer, request, http.StatusBadRequest, "invalid-import", "The import file is invalid.")
	case err != nil:
		h.internal(writer, request)
	default:
		writeJSON(writer, http.StatusOK, result)
	}
}

func Wrap(next http.Handler, cfg config.HTTP, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return middleware(
		next, cfg, newRateLimiter(cfg.RequestsPerMinute, cfg.AuthRequestsPerMin), logger,
	)
}

func NewServer(cfg config.HTTP, handler http.Handler) *http.Server {
	return &http.Server{
		Addr: cfg.Address, Handler: handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
	}
}

func allowedMethods(methods map[string]http.HandlerFunc) string {
	order := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	var allowed []string
	for _, method := range order {
		if methods[method] != nil {
			allowed = append(allowed, method)
		}
	}
	return strings.Join(allowed, ", ")
}

func (h *handler) liveness(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "alive"})
}

func (h *handler) readiness(writer http.ResponseWriter, request *http.Request) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(request.Context(), h.cfg.ReadinessTimeout)
	defer cancel()
	if err := h.db.PingContext(ctx); err != nil {
		telemetry.RecordDatabaseHealth(request.Context(), false, time.Since(started))
		writeJSON(writer, http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready", "failedChecks": []string{"database"},
		})
		return
	}
	telemetry.RecordDatabaseHealth(request.Context(), true, time.Since(started))
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *handler) openAPI(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "public, max-age=300")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(api.OpenAPI)
}

func (h *handler) login(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	userID, err := h.identity.AuthenticateLocal(
		request.Context(), input.Email, input.Password,
	)
	if err != nil {
		writeProblem(
			writer, request, http.StatusUnauthorized, "invalid-credentials",
			"The email or password is invalid.",
		)
		return
	}
	session, err := h.identity.CreateSession(
		request.Context(), userID, "local", "", 0, h.auditContext(request),
	)
	if err != nil {
		h.internal(writer, request)
		return
	}
	policy := identity.BrowserCookiePolicy(h.cfg.Secure)
	http.SetCookie(writer, &http.Cookie{
		Name: "session", Value: session.Secret, Path: "/",
		Expires: session.ExpiresAt, MaxAge: int(time.Until(session.ExpiresAt).Seconds()),
		HttpOnly: policy.HTTPOnly, Secure: policy.Secure, SameSite: policy.SameSite,
	})
	http.SetCookie(writer, &http.Cookie{
		Name: "csrf", Value: session.CSRFSecret, Path: "/",
		Expires: session.ExpiresAt, MaxAge: int(time.Until(session.ExpiresAt).Seconds()),
		HttpOnly: true, Secure: policy.Secure, SameSite: policy.SameSite,
	})
	writeJSON(writer, http.StatusOK, map[string]any{
		"csrfToken": session.CSRFSecret, "expiresAt": session.ExpiresAt,
	})
}

func (h *handler) logout(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	cookie, err := request.Cookie("session")
	if err != nil {
		h.unauthorized(writer, request)
		return
	}
	session, err := h.identity.AuthenticateSession(request.Context(), cookie.Value)
	if err != nil {
		h.unauthorized(writer, request)
		return
	}
	if !h.identity.VerifyCSRF(
		request.Context(), session.ID, request.Header.Get("X-CSRF-Token"),
	) {
		writeProblem(
			writer, request, http.StatusForbidden, "csrf-invalid",
			"The CSRF token is invalid.",
		)
		return
	}
	providerID, err := h.identity.RevokeSession(
		request.Context(), session.UserID, session.ID, h.auditContext(request),
	)
	if err != nil {
		h.unauthorized(writer, request)
		return
	}
	policy := identity.BrowserCookiePolicy(h.cfg.Secure)
	http.SetCookie(writer, &http.Cookie{
		Name: "session", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0),
		HttpOnly: policy.HTTPOnly, Secure: policy.Secure, SameSite: policy.SameSite,
	})
	http.SetCookie(writer, &http.Cookie{
		Name: "csrf", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0),
		HttpOnly: true, Secure: policy.Secure, SameSite: policy.SameSite,
	})
	if logoutURL, ok := h.oidc.LogoutURL(providerID); ok {
		writer.Header().Set("X-OIDC-Logout-URL", logoutURL)
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) listItems(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || !onlyQuery(
		request, "status", "search", "sort", "limit", "cursor",
	) {
		h.invalidRequest(writer, request, "The collection parameters are invalid.")
		return
	}
	query := request.URL.Query()
	search := query.Get("search")
	if _, specified := query["search"]; specified && strings.TrimSpace(search) == "" {
		h.invalidRequest(writer, request, "The search parameter is invalid.")
		return
	}
	principal, ok := h.bearerPrincipal(
		writer, request, identity.ResourcesRead,
	)
	if !ok {
		return
	}
	limit := h.cfg.DefaultPageSize
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > h.cfg.MaxPageSize {
			h.invalidRequest(writer, request, "The page limit is invalid.")
			return
		}
		limit = value
	}
	sort := query.Get("sort")
	if sort == "" {
		sort = "-created_at"
	}
	page, err := h.items.List(request.Context(), principal, workspaceID, items.ListInput{
		Status: items.Status(query.Get("status")), Search: search, Sort: sort,
		Limit: limit, Cursor: query.Get("cursor"),
	})
	if err != nil {
		h.itemError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (h *handler) createItem(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	idempotencyKey := request.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		writeProblem(
			writer, request, http.StatusBadRequest, "idempotency-key-required",
			"Idempotency-Key is required for item creation.",
		)
		return
	}
	principal, ok := h.bearerPrincipal(
		writer, request, identity.ResourcesWrite,
	)
	if !ok {
		return
	}
	var input struct {
		Title string `json:"title"`
	}
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	result, err := h.items.Create(
		request.Context(), principal, workspaceID, input.Title,
		idempotencyKey, h.auditContext(request),
	)
	if err != nil {
		h.itemError(writer, request, err)
		return
	}
	writer.Header().Set("ETag", etag(result.Item.Version))
	writer.Header().Set(
		"Location", "/api/v1/workspaces/"+workspaceID+"/items/"+result.Item.ID,
	)
	if result.Replay {
		writer.Header().Set("Idempotency-Replayed", "true")
	}
	writeJSON(writer, http.StatusCreated, result.Item)
}

func (h *handler) getItem(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	itemID := request.PathValue("itemId")
	if !validID(workspaceID) || !validID(itemID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.bearerPrincipal(
		writer, request, identity.ResourcesRead,
	)
	if !ok {
		return
	}
	item, err := h.items.Get(request.Context(), principal, workspaceID, itemID)
	if err != nil {
		h.itemError(writer, request, err)
		return
	}
	itemETag := etag(item.Version)
	writer.Header().Set("ETag", itemETag)
	writer.Header().Set("Cache-Control", "private, no-cache")
	if request.Header.Get("If-None-Match") == itemETag {
		writer.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(writer, http.StatusOK, item)
}

func (h *handler) updateItem(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	itemID := request.PathValue("itemId")
	if !validID(workspaceID) || !validID(itemID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	ifMatch := request.Header.Get("If-Match")
	if ifMatch == "" {
		writeProblem(
			writer, request, http.StatusPreconditionRequired,
			"precondition-required", "If-Match is required.",
		)
		return
	}
	version, ok := parseETag(ifMatch)
	if !ok {
		writeProblem(
			writer, request, http.StatusPreconditionFailed,
			"precondition-failed", "The entity tag is invalid or stale.",
		)
		return
	}
	principal, ok := h.bearerPrincipal(
		writer, request, identity.ResourcesWrite,
	)
	if !ok {
		return
	}
	var input struct {
		Title  *string       `json:"title"`
		Status *items.Status `json:"status"`
	}
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	item, err := h.items.Update(
		request.Context(), principal, workspaceID, itemID, version,
		items.UpdateInput{Title: input.Title, Status: input.Status},
		h.auditContext(request),
	)
	if err != nil {
		h.itemError(writer, request, err)
		return
	}
	writer.Header().Set("ETag", etag(item.Version))
	writeJSON(writer, http.StatusOK, item)
}

func (h *handler) deleteItem(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	itemID := request.PathValue("itemId")
	if !validID(workspaceID) || !validID(itemID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	version, ok := parseETag(request.Header.Get("If-Match"))
	if !ok {
		status := http.StatusPreconditionFailed
		code := "precondition-failed"
		detail := "The entity tag is invalid or stale."
		if request.Header.Get("If-Match") == "" {
			status = http.StatusPreconditionRequired
			code = "precondition-required"
			detail = "If-Match is required."
		}
		writeProblem(writer, request, status, code, detail)
		return
	}
	principal, ok := h.bearerPrincipal(writer, request, identity.ResourcesWrite)
	if !ok {
		return
	}
	if err := h.items.Delete(
		request.Context(), principal, workspaceID, itemID, version,
		h.auditContext(request),
	); err != nil {
		h.itemError(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) bearerPrincipal(
	writer http.ResponseWriter,
	request *http.Request,
	permission identity.Permission,
) (identity.Principal, bool) {
	parts := strings.Fields(request.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		h.unauthorized(writer, request)
		return identity.Principal{}, false
	}
	principal, err := h.identity.AuthenticateAPIToken(
		request.Context(), parts[1], permission, h.auditContext(request),
	)
	if errors.Is(err, identity.ErrForbidden) {
		writeProblem(
			writer, request, http.StatusForbidden, "insufficient-scope",
			"The credential lacks the required scope.",
		)
		return identity.Principal{}, false
	}
	if err != nil {
		h.unauthorized(writer, request)
		return identity.Principal{}, false
	}
	return principal, true
}

func (h *handler) itemError(
	writer http.ResponseWriter,
	request *http.Request,
	err error,
) {
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, items.ErrNotFound):
		notFoundProblem(writer, request)
	case errors.Is(err, items.ErrInvalidCursor):
		h.invalidRequest(writer, request, "The cursor is invalid.")
	case errors.Is(err, items.ErrInvalidInput):
		h.invalidRequest(writer, request, "The item input is invalid.")
	case errors.Is(err, items.ErrIdempotencyConflict):
		writeProblem(
			writer, request, http.StatusConflict, "idempotency-conflict",
			"The idempotency key was already used for another request.",
		)
	case errors.Is(err, items.ErrLimit):
		writeProblem(
			writer, request, http.StatusConflict, "resource-limit",
			"The workspace item limit was reached. Delete an item before retrying.",
		)
	case errors.Is(err, items.ErrPreconditionFailed):
		writeProblem(
			writer, request, http.StatusPreconditionFailed,
			"precondition-failed", "The entity tag is invalid or stale.",
		)
	default:
		h.internal(writer, request)
	}
}

func (h *handler) badJSON(
	writer http.ResponseWriter,
	request *http.Request,
	err error,
) {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		writeProblem(
			writer, request, http.StatusRequestEntityTooLarge, "body-too-large",
			"The request body exceeds the configured limit.",
		)
		return
	}
	h.invalidRequest(writer, request, "The JSON request body is invalid.")
}

func (h *handler) invalidRequest(
	writer http.ResponseWriter,
	request *http.Request,
	detail string,
) {
	writeProblem(writer, request, http.StatusBadRequest, "invalid-request", detail)
}

func (h *handler) unauthorized(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("WWW-Authenticate", "Bearer")
	writeProblem(
		writer, request, http.StatusUnauthorized, "unauthorized",
		"Authentication is required or invalid.",
	)
}

func (h *handler) internal(writer http.ResponseWriter, request *http.Request) {
	h.logger.Error("request failed", "request_id", requestIDFrom(request.Context()))
	internalProblem(writer, request)
}

func (h *handler) auditContext(request *http.Request) identity.AuditContext {
	return identity.AuditContext{
		RequestID:     requestIDFrom(request.Context()),
		SourceAddress: sourceHost(request.RemoteAddr),
	}
}

func decodeJSON(request *http.Request, destination any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func validID(value string) bool {
	return validRequestID(value)
}

func parseETag(value string) (int64, bool) {
	if len(value) < 4 || !strings.HasPrefix(value, `"v`) || !strings.HasSuffix(value, `"`) {
		return 0, false
	}
	version, err := strconv.ParseInt(value[2:len(value)-1], 10, 64)
	return version, err == nil && version > 0
}

func onlyQuery(request *http.Request, allowed ...string) bool {
	allow := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allow[key] = true
	}
	for key, values := range request.URL.Query() {
		if !allow[key] || len(values) != 1 || values[0] == "" {
			return false
		}
	}
	return true
}
