package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
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
		deliveries, err := service.ListDeliveries(ctx, owner, owner.WorkspaceID, created.ID)
		if err != nil || len(deliveries) != 1 {
			t.Fatalf("attempt %d ListDeliveries = %+v, err = %v", attempt, deliveries, err)
		}
		if attempt < maxAttempts {
			if deliveries[0].Status != DeliveryPending || deliveries[0].AttemptCount != attempt {
				t.Fatalf("attempt %d: status = %v, count = %d", attempt, deliveries[0].Status, deliveries[0].AttemptCount)
			}
		} else {
			if deliveries[0].Status != DeliveryFailed || deliveries[0].AttemptCount != maxAttempts {
				t.Fatalf("attempt %d: status = %v, count = %d", attempt, deliveries[0].Status, deliveries[0].AttemptCount)
			}
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

func TestEvaluateDeliveryOutcome(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

	t.Run("successful delivery on attempts 1 to 5", func(t *testing.T) {
		for attempt := 1; attempt <= 5; attempt++ {
			outcome := evaluateDeliveryOutcome(attempt, 5, 200, "", now)
			if outcome.Status != DeliveryDelivered {
				t.Fatalf("attempt %d status = %v, want %v", attempt, outcome.Status, DeliveryDelivered)
			}
			if outcome.NextAttemptAt != nil {
				t.Fatalf("attempt %d NextAttemptAt = %v, want nil", attempt, outcome.NextAttemptAt)
			}
			if outcome.LastStatusCode != 200 {
				t.Fatalf("attempt %d LastStatusCode = %d, want 200", attempt, outcome.LastStatusCode)
			}
			if outcome.LastError != "" {
				t.Fatalf("attempt %d LastError = %q, want empty", attempt, outcome.LastError)
			}
		}
	})

	t.Run("retryable failures on attempts 1 to 4 with exponential backoff", func(t *testing.T) {
		expectedDelays := map[int]time.Duration{
			1: time.Second,
			2: 2 * time.Second,
			3: 4 * time.Second,
			4: 8 * time.Second,
		}
		for attempt, delay := range expectedDelays {
			outcome := evaluateDeliveryOutcome(attempt, 5, 502, "remote-status", now)
			if outcome.Status != DeliveryPending {
				t.Fatalf("attempt %d status = %v, want %v", attempt, outcome.Status, DeliveryPending)
			}
			if outcome.NextAttemptAt == nil {
				t.Fatalf("attempt %d NextAttemptAt is nil, want %v", attempt, now.Add(delay))
			} else if !outcome.NextAttemptAt.Equal(now.Add(delay)) {
				t.Fatalf("attempt %d NextAttemptAt = %v, want %v", attempt, *outcome.NextAttemptAt, now.Add(delay))
			}
			if outcome.LastStatusCode != 502 {
				t.Fatalf("attempt %d LastStatusCode = %d, want 502", attempt, outcome.LastStatusCode)
			}
			if outcome.LastError != "remote-status" {
				t.Fatalf("attempt %d LastError = %q, want remote-status", attempt, outcome.LastError)
			}
		}
	})

	t.Run("exhausted on attempt 5", func(t *testing.T) {
		outcome := evaluateDeliveryOutcome(5, 5, 504, "remote-status", now)
		if outcome.Status != DeliveryFailed {
			t.Fatalf("attempt 5 status = %v, want %v", outcome.Status, DeliveryFailed)
		}
		if outcome.NextAttemptAt != nil {
			t.Fatalf("attempt 5 NextAttemptAt = %v, want nil", outcome.NextAttemptAt)
		}
		if outcome.LastStatusCode != 504 {
			t.Fatalf("attempt 5 LastStatusCode = %d, want 504", outcome.LastStatusCode)
		}
		if outcome.LastError != "remote-status" {
			t.Fatalf("attempt 5 LastError = %q, want remote-status", outcome.LastError)
		}
	})

	t.Run("worker exhausted outcome", func(t *testing.T) {
		outcome := evaluateDeliveryOutcome(5, 5, 0, "worker-exhausted", now)
		if outcome.Status != DeliveryFailed {
			t.Fatalf("status = %v, want %v", outcome.Status, DeliveryFailed)
		}
		if outcome.NextAttemptAt != nil {
			t.Fatalf("NextAttemptAt = %v, want nil", outcome.NextAttemptAt)
		}
		if outcome.LastError != "worker-exhausted" {
			t.Fatalf("LastError = %q, want worker-exhausted", outcome.LastError)
		}
	})

	t.Run("normalizes invalid attempt and maxAttempts bounds", func(t *testing.T) {
		outcome := evaluateDeliveryOutcome(0, 0, 500, "err", now)
		if outcome.Status != DeliveryFailed {
			t.Fatalf("outcome for attempt 0, max 0 = %+v", outcome)
		}
	})
}

func TestWebhookAndJobRetrySchedulesMatch(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		webhookOutcome := evaluateDeliveryOutcome(attempt, maxAttempts, 500, "remote-status", now)
		jobOutcome := jobs.EvaluateJobOutcome(attempt, maxAttempts, errors.New("err"), now)
		if (webhookOutcome.NextAttemptAt == nil) != (jobOutcome.AvailableAt == nil) {
			t.Fatalf("attempt %d: webhook next=%v, job next=%v", attempt, webhookOutcome.NextAttemptAt, jobOutcome.AvailableAt)
		}
		if webhookOutcome.NextAttemptAt != nil && !webhookOutcome.NextAttemptAt.Equal(*jobOutcome.AvailableAt) {
			t.Fatalf("attempt %d: webhook next = %v, job next = %v", attempt, *webhookOutcome.NextAttemptAt, *jobOutcome.AvailableAt)
		}
	}
}

func TestRetryScheduleBackoffIsConcrete(t *testing.T) {
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	for i, expected := range want {
		if got := jobs.RetryDelay(i + 1); got != expected {
			t.Fatalf("RetryDelay(%d) = %v, want %v", i+1, got, expected)
		}
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
