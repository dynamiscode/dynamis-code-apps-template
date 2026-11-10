package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/webhooks"
)

func TestWebhooksBrowserLifecycleAndRedaction(t *testing.T) {
	handler, db, auth, service, workspaceID, owner, _ := webhookWebTest(t)
	ctx := context.Background()
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	base := "/workspaces/" + workspaceID + "/settings/webhooks"

	page := request(handler, http.MethodGet, base, nil, cookies, nil)
	if page.Code != http.StatusOK || page.Header().Get("Cache-Control") != "no-store" || strings.Contains(page.Body.String(), "data-webmcp-page") {
		t.Fatalf("webhooks page = %d, headers=%v, body=%s", page.Code, page.Header(), page.Body.String())
	}
	assertAccessiblePage(t, page.Body.String(), "HTTPS endpoint")
	if !strings.Contains(page.Body.String(), `aria-current="page" href="`+base+`"`) {
		t.Fatalf("webhooks navigation missing current state: %s", page.Body.String())
	}

	withoutCSRF := request(handler, http.MethodPost, base, url.Values{
		"action": {"create"}, "name": {"blocked"}, "url": {"https://hooks.example.test"}, "events": {"item.created"},
	}, cookies, nil)
	if withoutCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing webhook CSRF = %d", withoutCSRF.Code)
	}

	invalid := request(handler, http.MethodPost, base, url.Values{
		"action": {"create"}, "csrf": {session.CSRFSecret}, "name": {"invalid"}, "url": {"https://hooks.example.test"}, "events": {"unknown"},
	}, cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "webhook input is invalid") {
		t.Fatalf("invalid webhook = %d, %s", invalid.Code, invalid.Body.String())
	}

	created := request(handler, http.MethodPost, base, url.Values{
		"action": {"create"}, "csrf": {session.CSRFSecret}, "name": {"items"},
		"url": {"https://hooks.example.test/events"}, "events": {"item.created", "item.deleted"},
	}, cookies, nil)
	if created.Code != http.StatusOK || created.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create webhook = %d, headers=%v, body=%s", created.Code, created.Header(), created.Body.String())
	}
	secret := webhookSecret(t, created.Body.String())
	if !strings.Contains(created.Body.String(), "Copy this webhook secret now") {
		t.Fatalf("create response lacks one-time secret warning: %s", created.Body.String())
	}
	listed := request(handler, http.MethodGet, base, nil, cookies, nil)
	if strings.Contains(listed.Body.String(), secret) || !strings.Contains(listed.Body.String(), "item.deleted") {
		t.Fatalf("webhook list secret/events = %s", listed.Body.String())
	}
	registered, err := service.List(ctx, mustPrincipal(t, auth, owner.UserID, workspaceID, identity.WebhooksRead), workspaceID)
	if err != nil || len(registered) != 1 {
		t.Fatalf("registered webhooks = %+v, %v", registered, err)
	}
	webhookID := registered[0].ID

	rotated := request(handler, http.MethodPost, base+"/"+webhookID, url.Values{
		"action": {"rotate"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if rotated.Code != http.StatusOK || rotated.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("rotate webhook = %d, headers=%v, body=%s", rotated.Code, rotated.Header(), rotated.Body.String())
	}
	rotatedSecret := webhookSecret(t, rotated.Body.String())
	if rotatedSecret == secret || strings.Contains(request(handler, http.MethodGet, base, nil, cookies, nil).Body.String(), rotatedSecret) {
		t.Fatalf("rotated secret was not one-time: %q", rotatedSecret)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.Exec(`INSERT INTO webhook_deliveries (
		id, webhook_id, event_id, event_type, payload, attempt_count, status,
		next_attempt_at, last_status_code, last_error, created_at, delivered_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?, NULL)`,
		strings.Repeat("a", 32), webhookID, strings.Repeat("b", 32), "item.created",
		`{"privatePayload":"must not render"}`, 3, "failed", 500, "network-error", now)
	if err != nil {
		t.Fatal(err)
	}
	history := request(handler, http.MethodGet, base+"/"+webhookID+"/deliveries", nil, cookies, nil)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), "network-error") || !strings.Contains(history.Body.String(), "500") || strings.Contains(history.Body.String(), "must not render") {
		t.Fatalf("delivery history redaction = %d, %s", history.Code, history.Body.String())
	}

	otherWorkspace := strings.Repeat("f", 32)
	if response := request(handler, http.MethodGet, "/workspaces/"+otherWorkspace+"/settings/webhooks", nil, cookies, nil); response.Code != http.StatusForbidden {
		t.Fatalf("wrong workspace webhook access = %d", response.Code)
	}
	memberInvitation, err := auth.CreateInvitation(ctx, mustPrincipal(t, auth, owner.UserID, workspaceID, identity.InvitationsManage), "member@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	memberID, _, err := auth.CreateInvitedLocalUser(ctx, memberInvitation.Secret, "long-enough-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	memberSession, err := auth.CreateSession(ctx, memberID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if response := request(handler, http.MethodGet, base, nil, []*http.Cookie{{Name: "session", Value: memberSession.Secret}, {Name: "csrf", Value: memberSession.CSRFSecret}}, nil); response.Code != http.StatusForbidden {
		t.Fatalf("member webhook access = %d", response.Code)
	}

	deleted := request(handler, http.MethodPost, base+"/"+webhookID, url.Values{
		"action": {"delete"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if deleted.Code != http.StatusSeeOther || deleted.Header().Get("Location") != base {
		t.Fatalf("delete webhook = %d, location=%q", deleted.Code, deleted.Header().Get("Location"))
	}
	if response := request(handler, http.MethodGet, base+"/"+webhookID+"/deliveries", nil, cookies, nil); response.Code != http.StatusNotFound {
		t.Fatalf("deleted webhook history = %d", response.Code)
	}
}

func TestWebhookSecretSurvivesReadbackFailure(t *testing.T) {
	handler, db, auth, service, workspaceID, owner, rawHandler := webhookWebTest(t)
	ctx := context.Background()
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	base := "/workspaces/" + workspaceID + "/settings/webhooks"
	rawHandler.webhooks = failingWebhookManager{service: service, listErr: errors.New("webhook list unavailable"), secretConfigured: true}

	created := request(handler, http.MethodPost, base, url.Values{
		"action": {"create"}, "csrf": {session.CSRFSecret}, "name": {"readback"},
		"url": {"https://hooks.example.test"}, "events": {"item.created"},
	}, cookies, nil)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), "The webhook was saved") {
		t.Fatalf("create readback failure = %d, %s", created.Code, created.Body.String())
	}
	secret := webhookSecret(t, created.Body.String())
	if strings.Contains(created.Body.String(), "No webhooks yet.") || !strings.Contains(created.Body.String(), secret) {
		t.Fatalf("create readback fallback = %s", created.Body.String())
	}
	var webhookID string
	if err := db.QueryRow(`SELECT id FROM webhooks WHERE name = ?`, "readback").Scan(&webhookID); err != nil {
		t.Fatal(err)
	}

	rotated := request(handler, http.MethodPost, base+"/"+webhookID, url.Values{
		"action": {"rotate"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if rotated.Code != http.StatusOK || !strings.Contains(rotated.Body.String(), "The webhook was saved") {
		t.Fatalf("rotate readback failure = %d, %s", rotated.Code, rotated.Body.String())
	}
	rotatedSecret := webhookSecret(t, rotated.Body.String())
	if rotatedSecret == secret || !strings.Contains(rotated.Body.String(), rotatedSecret) {
		t.Fatalf("rotate readback fallback = %s", rotated.Body.String())
	}
}

func TestWebhookSecretKeyErrorIsActionable(t *testing.T) {
	handler, _, auth, service, workspaceID, owner, rawHandler := webhookWebTest(t)
	ctx := context.Background()
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	base := "/workspaces/" + workspaceID + "/settings/webhooks"
	rawHandler.webhooks = failingWebhookManager{service: service, createErr: webhooks.ErrSecretKey}

	page := request(handler, http.MethodGet, base, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `class="warning"`) || !strings.Contains(page.Body.String(), "Webhook configuration required") || !strings.Contains(page.Body.String(), "WEBHOOK_ENCRYPTION_KEY") || !strings.Contains(page.Body.String(), "disabled aria-describedby=\"webhook-secret-key-warning\"") {
		t.Fatalf("missing webhook secret key page = %d, %s", page.Code, page.Body.String())
	}

	response := request(handler, http.MethodPost, base, url.Values{
		"action": {"create"}, "csrf": {session.CSRFSecret}, "name": {"items"},
		"url": {"https://hooks.example.test"}, "events": {"item.created"},
	}, cookies, nil)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "WEBHOOK_ENCRYPTION_KEY") {
		t.Fatalf("webhook secret key error = %d, %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "<h1>Request failed</h1>") || !strings.Contains(response.Body.String(), "<h1>Example Webhooks</h1>") {
		t.Fatalf("webhook secret key error left the settings page: %s", response.Body.String())
	}
}

type failingWebhookManager struct {
	service          *webhooks.Service
	listErr          error
	createErr        error
	secretConfigured bool
}

func (f failingWebhookManager) SecretKeyConfigured() bool {
	return f.secretConfigured
}

func (f failingWebhookManager) List(ctx context.Context, actor identity.Principal, workspaceID string) ([]webhooks.Webhook, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.service.List(ctx, actor, workspaceID)
}

func (f failingWebhookManager) ListDeliveries(ctx context.Context, actor identity.Principal, workspaceID, webhookID string) ([]webhooks.Delivery, error) {
	return f.service.ListDeliveries(ctx, actor, workspaceID, webhookID)
}

func (f failingWebhookManager) Create(ctx context.Context, actor identity.Principal, workspaceID string, input webhooks.CreateInput, audit identity.AuditContext) (webhooks.NewWebhook, error) {
	if f.createErr != nil {
		return webhooks.NewWebhook{}, f.createErr
	}
	return f.service.Create(ctx, actor, workspaceID, input, audit)
}

func (f failingWebhookManager) RotateSecret(ctx context.Context, actor identity.Principal, workspaceID, webhookID string, audit identity.AuditContext) (string, error) {
	return f.service.RotateSecret(ctx, actor, workspaceID, webhookID, audit)
}

func (f failingWebhookManager) Delete(ctx context.Context, actor identity.Principal, workspaceID, webhookID string, audit identity.AuditContext) error {
	return f.service.Delete(ctx, actor, workspaceID, webhookID, audit)
}

func webhookWebTest(t *testing.T) (http.Handler, *sql.DB, *identity.Service, *webhooks.Service, string, identity.BootstrapResult, *Handler) {
	t.Helper()
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns = 1, 1
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
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
	service := webhooks.NewService(db, cfg.Database.Driver, auth, []byte("01234567890123456789012345678901"), nil)
	itemService := items.NewService(db, cfg.Database.Driver, auth, cfg.Data.ItemsMaxPerWorkspace)
	handler, err := NewHandlerWithServices(
		auth, itemService, nil, nil, nil, cfg.HTTP, "", "", nil, service,
	)
	if err != nil {
		t.Fatal(err)
	}
	return handler.Routes(), db, auth, service, owner.WorkspaceID, owner, handler
}

func webhookSecret(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, "whsec_")
	if start < 0 {
		t.Fatalf("webhook secret missing: %s", body)
	}
	end := strings.IndexByte(body[start:], '<')
	if end < 0 {
		t.Fatalf("webhook secret is not bounded: %s", body)
	}
	return body[start : start+end]
}

func mustPrincipal(t *testing.T, auth *identity.Service, userID, workspaceID string, permission identity.Permission) identity.Principal {
	t.Helper()
	principal, err := auth.Authorize(context.Background(), userID, workspaceID, permission)
	if err != nil {
		t.Fatal(err)
	}
	return principal
}
