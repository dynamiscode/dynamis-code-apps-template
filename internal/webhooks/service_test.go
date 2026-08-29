package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/jobs"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestItemEventsAreSignedEncryptedAndDeliveredOnce(t *testing.T) {
	ctx := context.Background()
	db, auth, owner := webhookTestDB(t)
	type capturedRequest struct {
		headers http.Header
		body    []byte
	}
	received := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		received <- capturedRequest{headers: request.Header.Clone(), body: body}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	secretKey := []byte("01234567890123456789012345678901")
	queue := jobs.NewQueue(db, config.SQLite, nil)
	service := NewService(db, config.SQLite, auth, secretKey, queue)
	if err := queue.Register(JobKind, service.HandleJob); err != nil {
		t.Fatal(err)
	}
	if err := queue.RegisterExhausted(JobKind, service.HandleExhaustedJob); err != nil {
		t.Fatal(err)
	}
	service.client = server.Client()
	created, err := service.Create(ctx, owner, owner.WorkspaceID, CreateInput{
		Name: "items", URL: server.URL + "/hook", Events: []string{"item.created"},
	}, identity.AuditContext{RequestID: "webhook-create"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Secret == "" {
		t.Fatal("webhook secret missing")
	}
	var encrypted string
	if err := db.QueryRow("SELECT secret_ciphertext FROM webhooks WHERE id = ?", created.ID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if encrypted == created.Secret || strings.Contains(encrypted, created.Secret) {
		t.Fatal("webhook secret stored in plaintext")
	}

	itemService := items.NewService(db, config.SQLite, auth, 100, service)
	if _, err := itemService.Create(ctx, owner, owner.WorkspaceID, "Sensitive title", "webhook-item-key", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	var pendingJobs int
	if err := db.QueryRow("SELECT COUNT(*) FROM background_jobs WHERE kind = ? AND status = 'pending'", JobKind).Scan(&pendingJobs); err != nil || pendingJobs != 1 {
		t.Fatalf("pending background jobs = %d, err = %v", pendingJobs, err)
	}
	if _, err := service.DeliverPending(ctx, 1); err != nil {
		t.Fatal(err)
	}
	receivedRequest := <-received
	body := receivedRequest.body
	eventID, timestamp := receivedRequest.headers.Get("Webhook-Id"), receivedRequest.headers.Get("Webhook-Timestamp")
	mac := hmac.New(sha256.New, []byte(created.Secret))
	_, _ = mac.Write([]byte(eventID + "." + timestamp + "." + string(body)))
	want := "v1," + base64.RawStdEncoding.EncodeToString(mac.Sum(nil))
	if receivedRequest.headers.Get("Webhook-Signature") != want || !strings.Contains(string(body), "Sensitive title") {
		t.Fatalf("signed request headers/body invalid: %v %s", receivedRequest.headers, body)
	}

	deliveries, err := service.ListDeliveries(ctx, owner, owner.WorkspaceID, created.ID)
	if err != nil || len(deliveries) != 1 || deliveries[0].Status != "delivered" || deliveries[0].AttemptCount != 1 {
		t.Fatalf("deliveries = %+v, err = %v", deliveries, err)
	}
	if _, err := itemService.Create(ctx, owner, owner.WorkspaceID, "Sensitive title", "webhook-item-key", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM webhook_deliveries WHERE webhook_id = ?", created.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("delivery count = %d, err = %v", count, err)
	}
}

func TestDeliveryRetriesAreBoundedAndRedacted(t *testing.T) {
	ctx := context.Background()
	db, auth, owner := webhookTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	queue := jobs.NewQueue(db, config.SQLite, nil)
	service := NewService(db, config.SQLite, auth, []byte("01234567890123456789012345678901"), queue)
	if err := queue.Register(JobKind, service.HandleJob); err != nil {
		t.Fatal(err)
	}
	if err := queue.RegisterExhausted(JobKind, service.HandleExhaustedJob); err != nil {
		t.Fatal(err)
	}
	service.client = server.Client()
	created, err := service.Create(ctx, owner, owner.WorkspaceID, CreateInput{
		Name: "failing", URL: server.URL, Events: []string{"item.created"},
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	itemService := items.NewService(db, config.SQLite, auth, 100, service)
	if _, err := itemService.Create(ctx, owner, owner.WorkspaceID, "retry", "retry-key", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if _, err := db.Exec("UPDATE webhook_deliveries SET next_attempt_at = ? WHERE webhook_id = ?", time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano), created.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("UPDATE background_jobs SET available_at = ? WHERE kind = ? AND deduplication_key = (SELECT id FROM webhook_deliveries WHERE webhook_id = ?)", time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano), JobKind, created.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := service.DeliverPending(ctx, 1); err != nil {
			t.Fatal(err)
		}
	}
	deliveries, err := service.ListDeliveries(ctx, owner, owner.WorkspaceID, created.ID)
	if err != nil || len(deliveries) != 1 || deliveries[0].Status != "failed" || deliveries[0].AttemptCount != maxAttempts || deliveries[0].LastError != "remote-status" {
		t.Fatalf("bounded delivery = %+v, err = %v", deliveries, err)
	}
}

func TestExhaustedWebhookJobSettlesDelivery(t *testing.T) {
	ctx := context.Background()
	db, auth, owner := webhookTestDB(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls++
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	queue := jobs.NewQueue(db, config.SQLite, nil)
	service := NewService(db, config.SQLite, auth, []byte("01234567890123456789012345678901"), queue)
	if err := queue.Register(JobKind, service.HandleJob); err != nil {
		t.Fatal(err)
	}
	if err := queue.RegisterExhausted(JobKind, service.HandleExhaustedJob); err != nil {
		t.Fatal(err)
	}
	service.client = server.Client()
	created, err := service.Create(ctx, owner, owner.WorkspaceID, CreateInput{
		Name: "exhausted", URL: server.URL, Events: []string{"item.created"},
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	itemService := items.NewService(db, config.SQLite, auth, 100, service)
	if _, err := itemService.Create(ctx, owner, owner.WorkspaceID, "retry", "exhausted-key", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		UPDATE background_jobs SET status = 'running', attempt_count = ?,
			lease_token = ?, leased_until = ?, started_at = ?
		WHERE kind = ? AND deduplication_key = (
			SELECT id FROM webhook_deliveries WHERE webhook_id = ?
		)
	`, maxAttempts, "stale-final-lease", stamp(time.Now().Add(-time.Minute)),
		stamp(time.Now().Add(-2*time.Minute)), JobKind, created.ID); err != nil {
		t.Fatal(err)
	}
	if processed, err := queue.Process(ctx, 1); err != nil || processed != 1 {
		t.Fatalf("Process() = %d, %v", processed, err)
	}
	deliveries, err := service.ListDeliveries(ctx, owner, owner.WorkspaceID, created.ID)
	if err != nil || len(deliveries) != 1 || deliveries[0].Status != "failed" ||
		deliveries[0].AttemptCount != maxAttempts || deliveries[0].LastError != "worker-exhausted" {
		t.Fatalf("exhausted delivery = %+v, err = %v", deliveries, err)
	}
	if calls != 0 {
		t.Fatalf("HTTP calls = %d, want 0", calls)
	}
}

func webhookTestDB(t *testing.T) (*sql.DB, *identity.Service, identity.Principal) {
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
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapped, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authorize(ctx, bootstrapped.UserID, bootstrapped.WorkspaceID, identity.WebhooksManage)
	if err != nil {
		t.Fatal(err)
	}
	return db, auth, owner
}
