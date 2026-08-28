package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"example.com/dynamis-code/apps-template/internal/portability"
	"example.com/dynamis-code/apps-template/internal/webhooks"
)

func TestHTTPContracts(t *testing.T) {
	handler, db, workspaceID, token := testHandler(t)

	live := serve(handler, http.MethodGet, "/health/live", "", "")
	if live.Code != http.StatusOK || live.Header().Get("X-Request-ID") == "" ||
		live.Header().Get("X-Content-Type-Options") != "nosniff" ||
		live.Header().Get("Permissions-Policy") != "tools=(self)" ||
		live.Header().Get("Origin-Agent-Cluster") != "?1" {
		t.Fatalf("liveness response = %d, headers %v", live.Code, live.Header())
	}
	missing := serve(handler, http.MethodGet, "/missing", "", "")
	assertProblem(t, missing, http.StatusNotFound, "not-found")

	collection := "/api/v1/workspaces/" + workspaceID + "/items"
	created := serveAuthorized(handler, http.MethodPost, collection, `{"title":"First"}`, token, map[string]string{"Idempotency-Key": "key-12345678"})
	if created.Code != http.StatusCreated || created.Header().Get("ETag") != `"v1"` {
		t.Fatalf("create response = %d, %s", created.Code, created.Body.String())
	}
	var item items.Item
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	replay := serveAuthorized(handler, http.MethodPost, collection, `{"title":"First"}`, token, map[string]string{"Idempotency-Key": "key-12345678"})
	if replay.Code != http.StatusCreated || replay.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("replay response = %d, %s", replay.Code, replay.Body.String())
	}
	conflict := serveAuthorized(handler, http.MethodPost, collection, `{"title":"Changed"}`, token, map[string]string{"Idempotency-Key": "key-12345678"})
	assertProblem(t, conflict, http.StatusConflict, "idempotency-conflict")
	exported := serveAuthorized(handler, http.MethodGet,
		"/api/v1/workspaces/"+workspaceID+"/export", "", token, nil)
	if exported.Code != http.StatusOK || exported.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(exported.Header().Get("Content-Disposition"), "attachment") ||
		!strings.Contains(exported.Body.String(), item.ID) ||
		!strings.Contains(exported.Body.String(), portability.FormatVersion) ||
		strings.Contains(exported.Body.String(), token) {
		t.Fatalf("export response = %d, headers %v, body %s", exported.Code, exported.Header(), exported.Body.String())
	}
	imported := serveAuthorized(handler, http.MethodPost,
		"/api/v1/workspaces/"+workspaceID+"/import",
		"title,status\nImported,active\n", token,
		map[string]string{"Content-Type": "text/csv"})
	if imported.Code != http.StatusOK || imported.Body.String() != "{\"imported\":1}\n" {
		t.Fatalf("import response = %d, %s", imported.Code, imported.Body.String())
	}
	unsupported := serveAuthorized(handler, http.MethodGet, collection+"?filter=no", "", token, nil)
	assertProblem(t, unsupported, http.StatusBadRequest, "invalid-request")
	searched := serveAuthorized(handler, http.MethodGet, collection+"?search=first&limit=1&sort=created_at", "", token, nil)
	if searched.Code != http.StatusOK || !strings.Contains(searched.Body.String(), item.ID) {
		t.Fatalf("search response = %d, %s", searched.Code, searched.Body.String())
	}
	assertProblem(t, serveAuthorized(handler, http.MethodGet, collection+"?search=", "", token, nil), http.StatusBadRequest, "invalid-request")

	resource := collection + "/" + item.ID
	got := serveAuthorized(handler, http.MethodGet, resource, "", token, nil)
	if got.Code != http.StatusOK || got.Header().Get("ETag") != `"v1"` {
		t.Fatalf("get response = %d, %s", got.Code, got.Body.String())
	}
	withoutPrecondition := serveAuthorized(handler, http.MethodPatch, resource, `{"status":"complete"}`, token, nil)
	assertProblem(t, withoutPrecondition, http.StatusPreconditionRequired, "precondition-required")
	updated := serveAuthorized(handler, http.MethodPatch, resource, `{"status":"complete"}`, token, map[string]string{"If-Match": `"v1"`})
	if updated.Code != http.StatusOK || updated.Header().Get("ETag") != `"v2"` {
		t.Fatalf("update response = %d, %s", updated.Code, updated.Body.String())
	}
	stale := serveAuthorized(handler, http.MethodPatch, resource, `{"title":"Stale"}`, token, map[string]string{"If-Match": `"v1"`})
	assertProblem(t, stale, http.StatusPreconditionFailed, "precondition-failed")
	deleted := serveAuthorized(handler, http.MethodDelete, resource, "", token, map[string]string{"If-Match": `"v2"`})
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete response = %d, %s", deleted.Code, deleted.Body.String())
	}
	assertProblem(t, serveAuthorized(handler, http.MethodGet, resource, "", token, nil), http.StatusNotFound, "not-found")
	wrongWorkspace := serveAuthorized(handler, http.MethodGet, "/api/v1/workspaces/00000000000000000000000000000000/items", "", token, nil)
	assertProblem(t, wrongWorkspace, http.StatusForbidden, "forbidden")

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	ready := serve(handler, http.MethodGet, "/health/ready", "", "")
	if ready.Code != http.StatusServiceUnavailable || strings.Contains(ready.Body.String(), "sql") {
		t.Fatalf("readiness response = %d, %s", ready.Code, ready.Body.String())
	}
}

func TestHTTPBoundaries(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	large := serve(handler, http.MethodPost, "/api/v1/auth/login", `{"email":"`+strings.Repeat("a", 2048)+`"}`, "application/json")
	assertProblem(t, large, http.StatusRequestEntityTooLarge, "body-too-large")

	limited := newRateLimiter(2, 1)
	limited.window = time.Minute
	next := requestIDMiddleware(rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), limited))
	if serve(next, http.MethodPost, "/api/v1/auth/login", "", "").Code != http.StatusNoContent {
		t.Fatal("first authentication request rejected")
	}
	rateLimited := serve(next, http.MethodPost, "/api/v1/auth/login", "", "")
	assertProblem(t, rateLimited, http.StatusTooManyRequests, "rate-limited")
	if rateLimited.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After missing")
	}

	timed := requestIDMiddleware(timeoutMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}), time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil))))
	assertProblem(t, serve(timed, http.MethodGet, "/slow", "", ""), http.StatusGatewayTimeout, "request-timeout")
	panicked := requestIDMiddleware(timeoutMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("private failure")
	}), time.Second, slog.New(slog.NewTextHandler(io.Discard, nil))))
	result := serve(panicked, http.MethodGet, "/panic", "", "")
	assertProblem(t, result, http.StatusInternalServerError, "internal-error")
	if strings.Contains(result.Body.String(), "private failure") {
		t.Fatal("panic detail leaked")
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	concurrent := requestIDMiddleware(concurrencyMiddleware(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			close(entered)
			<-release
			writer.WriteHeader(http.StatusNoContent)
		},
	), 1))
	done := make(chan struct{})
	go func() {
		defer close(done)
		serve(concurrent, http.MethodGet, "/first", "", "")
	}()
	<-entered
	concurrencyLimited := serve(concurrent, http.MethodGet, "/second", "", "")
	assertProblem(t, concurrencyLimited, http.StatusTooManyRequests, "concurrency-limit")
	if concurrencyLimited.Header().Get("Retry-After") != "1" {
		t.Fatal("concurrency Retry-After missing")
	}
	close(release)
	<-done
}

func TestIdentityRESTContracts(t *testing.T) {
	handler, db, workspaceID, token := testHandler(t)
	base := "/api/v1/workspaces/" + workspaceID

	members := serveAuthorized(handler, http.MethodGet, base+"/members", "", token, nil)
	if members.Code != http.StatusOK || !strings.Contains(members.Body.String(), "owner@example.com") {
		t.Fatalf("members = %d, %s", members.Code, members.Body.String())
	}
	invitation := serveAuthorized(handler, http.MethodPost, base+"/invitations", `{"email":"invitee@example.com","role":"member","lifetimeSeconds":3600}`, token, nil)
	if invitation.Code != http.StatusCreated || !strings.Contains(invitation.Body.String(), "invitationUrl") || strings.Contains(invitation.Body.String(), "secret") {
		t.Fatalf("invitation = %d, %s", invitation.Code, invitation.Body.String())
	}
	var invite struct {
		Invitation struct {
			ID string `json:"id"`
		} `json:"invitation"`
		URL string `json:"invitationUrl"`
	}
	if err := json.Unmarshal(invitation.Body.Bytes(), &invite); err != nil || invite.Invitation.ID == "" || invite.URL == "" {
		t.Fatalf("invitation response = %+v, %v", invite, err)
	}
	invitations := serveAuthorized(handler, http.MethodGet, base+"/invitations", "", token, nil)
	if invitations.Code != http.StatusOK || !strings.Contains(invitations.Body.String(), "invitee@example.com") {
		t.Fatalf("invitations = %d, %s", invitations.Code, invitations.Body.String())
	}
	createdToken := serveAuthorized(handler, http.MethodPost, base+"/tokens", `{"name":"machine","scopes":["resources:read"]}`, token, nil)
	if createdToken.Code != http.StatusCreated || !strings.Contains(createdToken.Body.String(), "secret") {
		t.Fatalf("token create = %d, %s", createdToken.Code, createdToken.Body.String())
	}
	var tokenResponse struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(createdToken.Body.Bytes(), &tokenResponse); err != nil || tokenResponse.Secret == "" {
		t.Fatal("token secret missing")
	}
	tokens := serveAuthorized(handler, http.MethodGet, base+"/tokens", "", token, nil)
	if tokens.Code != http.StatusOK || strings.Contains(tokens.Body.String(), tokenResponse.Secret) {
		t.Fatalf("token list leaked secret: %d, %s", tokens.Code, tokens.Body.String())
	}
	login := serve(handler, http.MethodPost, "/api/v1/auth/login", `{"email":"owner@example.com","password":"long-enough-password"}`, "application/json")
	if login.Code != http.StatusOK {
		t.Fatalf("session login = %d, %s", login.Code, login.Body.String())
	}
	sessions := serveAuthorized(handler, http.MethodGet, "/api/v1/sessions", "", token, nil)
	if sessions.Code != http.StatusOK || strings.Contains(sessions.Body.String(), "csrf") || strings.Contains(sessions.Body.String(), tokenResponse.Secret) {
		t.Fatalf("sessions = %d, %s", sessions.Code, sessions.Body.String())
	}
	var sessionPage struct {
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(sessions.Body.Bytes(), &sessionPage); err != nil || len(sessionPage.Sessions) == 0 {
		t.Fatalf("session response = %+v, %v", sessionPage, err)
	}
	if revoked := serveAuthorized(handler, http.MethodDelete, "/api/v1/sessions/"+sessionPage.Sessions[0].ID, "", token, nil); revoked.Code != http.StatusNoContent {
		t.Fatalf("session revoke = %d, %s", revoked.Code, revoked.Body.String())
	}
	newUser, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, NULL, ?)", newUser, "member@example.com", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, ?, ?)", workspaceID, newUser, "member", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	changed := serveAuthorized(handler, http.MethodPatch, base+"/members/"+newUser, `{"role":"viewer"}`, token, nil)
	if changed.Code != http.StatusNoContent {
		t.Fatalf("member role = %d, %s", changed.Code, changed.Body.String())
	}
	removed := serveAuthorized(handler, http.MethodDelete, base+"/members/"+newUser, "", token, nil)
	if removed.Code != http.StatusNoContent {
		t.Fatalf("member remove = %d, %s", removed.Code, removed.Body.String())
	}
}

func testHandler(t *testing.T) (http.Handler, *sql.DB, string, string) {
	t.Helper()
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(key string) (string, bool) {
		if key == "HTTP_MAX_BODY_BYTES" {
			return "1024", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns = 1
	cfg.Database.MaxIdleConns = 1
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.CreateAPIToken(ctx, principal, "test", []identity.Permission{
		identity.WorkspaceRead, identity.WorkspaceUpdate, identity.WorkspaceExport,
		identity.OwnershipTransfer, identity.MembersRead, identity.MembersManage,
		identity.InvitationsManage, identity.WebhooksRead, identity.WebhooksManage,
		identity.ResourcesRead, identity.ResourcesWrite,
	}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	oidc, err := identity.NewOIDCRegistry(ctx, cfg.OIDC)
	if err != nil {
		t.Fatal(err)
	}
	webhookService := webhooks.NewService(db, cfg.Database.Driver, auth, []byte("01234567890123456789012345678901"), nil)
	itemService := items.NewService(db, cfg.Database.Driver, auth, cfg.Data.ItemsMaxPerWorkspace, webhookService)
	handler, err := NewHandlerWithWebhooks(db, auth, itemService,
		portability.NewService(db, cfg.Database.Driver, auth, cfg.Data.ExportMaxRecords, cfg.Data.ExportMaxBytes,
			cfg.Data.ImportMaxRecords, cfg.Data.ImportMaxBytes, itemService),
		oidc, cfg.HTTP, slog.New(slog.NewTextHandler(io.Discard, nil)), webhookService, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler, db, owner.WorkspaceID, token.Secret
}

func serve(handler http.Handler, method, target, body, contentType string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func serveAuthorized(handler http.Handler, method, target, body, token string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertProblem(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status || response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("problem response = %d %s, %s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	var value problem
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value.Status != status || value.Code != code || value.RequestID == "" || value.Type == "" || value.Instance == "" {
		t.Fatalf("problem = %+v", value)
	}
}
