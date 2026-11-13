package webhooks

import (
	"context"
	"fmt"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/jobs"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// TestDeliveryOutcomeRulesHoldForEveryInput checks the rules in
// .claude/skills/verify-webhook-delivery for every input in the bounded range
// below, not only the sample values in TestEvaluateDeliveryOutcome.
func TestDeliveryOutcomeRulesHoldForEveryInput(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	reasons := []string{"", "remote-status", "network-error", "worker-exhausted", "secret-unavailable", "request-invalid"}
	codes := []int{999}
	for code := -1; code <= 600; code++ {
		codes = append(codes, code)
	}
	for maxAttempts := 0; maxAttempts <= 5; maxAttempts++ {
		for attempt := -1; attempt <= 7; attempt++ {
			for _, code := range codes {
				for _, reason := range reasons {
					outcome := evaluateDeliveryOutcome(attempt, maxAttempts, code, reason, now)
					effAttempt, effMax := attempt, maxAttempts
					if effAttempt < 1 {
						effAttempt = 1
					}
					if effMax < 1 {
						effMax = 1
					}
					input := func() string {
						return fmt.Sprintf("attempt=%d max=%d code=%d reason=%q", attempt, maxAttempts, code, reason)
					}
					if outcome.LastStatusCode != code {
						t.Fatalf("%s: LastStatusCode = %d", input(), outcome.LastStatusCode)
					}
					delivered := reason == "" && code >= 200 && code <= 299
					switch {
					case delivered:
						if outcome.Status != DeliveryDelivered || outcome.NextAttemptAt != nil || outcome.LastError != "" {
							t.Fatalf("%s: want delivered with no next attempt and no error, got %+v", input(), outcome)
						}
					case effAttempt >= effMax:
						if outcome.Status != DeliveryFailed || outcome.NextAttemptAt != nil || outcome.LastError != reason {
							t.Fatalf("%s: want failed at max with error %q, got %+v", input(), reason, outcome)
						}
					default:
						want := now.Add(jobs.RetryDelay(effAttempt))
						if outcome.Status != DeliveryPending || outcome.NextAttemptAt == nil ||
							!outcome.NextAttemptAt.Equal(want) || outcome.LastError != reason {
							t.Fatalf("%s: want pending retry at %v with error %q, got %+v", input(), want, reason, outcome)
						}
					}
				}
			}
		}
	}
}

// TestDeliveredRowIsNotOverwrittenByLaterFailure checks that a delivery already
// marked delivered stays delivered when a failure write arrives afterwards.
// Every delivery UPDATE is guarded by status = 'pending'; removing that guard
// makes this test fail.
func TestDeliveredRowIsNotOverwrittenByLaterFailure(t *testing.T) {
	ctx := context.Background()
	db, auth, owner := webhookTestDB(t)
	queue := jobs.NewQueue(db, config.SQLite, nil)
	service := NewService(db, config.SQLite, auth, testKeyring(), queue)
	created, err := service.Create(ctx, owner, owner.WorkspaceID, CreateInput{
		Name: "guard", URL: "https://example.com/hook", Events: []string{"item.created"},
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	itemService := items.NewService(db, config.SQLite, auth, 100, service)
	if _, err := itemService.Create(ctx, owner, owner.WorkspaceID, "guard", "guard-key", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	var deliveryID int64
	if err := db.QueryRow(`SELECT id FROM webhook_deliveries WHERE webhook_id = ?`, created.ID).Scan(&deliveryID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE webhook_deliveries SET status = 'delivered' WHERE id = ?`, deliveryID); err != nil {
		t.Fatal(err)
	}
	// deliveryFailure returns the job failure for the queue on purpose; the
	// row write is what this test checks, so the return value is ignored.
	_ = service.deliveryFailure(ctx, deliveryID, 1, 500, "remote-status", time.Now().UTC())
	var status string
	if err := db.QueryRow(`SELECT status FROM webhook_deliveries WHERE id = ?`, deliveryID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(DeliveryDelivered) {
		t.Fatalf("delivered row changed to %q by a later failure", status)
	}
}
