package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/jobs"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestWebhookResponsesExposeShortIDsOnly(t *testing.T) {
	ctx := context.Background()
	db, auth, owner := webhookTestDB(t)
	queue := jobs.NewQueue(db, config.SQLite, nil)
	service := NewService(db, config.SQLite, auth, []byte("01234567890123456789012345678901"), queue)

	created, err := service.Create(ctx, owner, owner.WorkspaceID, CreateInput{
		Name: "ids", URL: "https://example.com/hook", Events: []string{"item.created"},
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	var rowKey int64
	if err := db.QueryRow("SELECT id FROM webhooks").Scan(&rowKey); err != nil {
		t.Fatal(err)
	}
	if parsed, err := ParseWebhookID(created.ID.String()); err != nil || int64(parsed) != rowKey {
		t.Fatalf("short ID %q does not decode to row key %d: %v", created.ID, rowKey, err)
	}
	data, err := json.Marshal(created.Webhook)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"id":`+strconv.FormatInt(rowKey, 10)) || !strings.Contains(string(data), `"id":"`+created.ID.String()+`"`) {
		t.Fatalf("webhook JSON exposes the integer key or a different ID: %s", data)
	}
	listed, err := service.List(ctx, owner, owner.WorkspaceID)
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("listed webhooks = %+v, err = %v", listed, err)
	}
}

func TestMalformedAndIntegerWebhookIDsAreNotFound(t *testing.T) {
	ctx := context.Background()
	db, auth, owner := webhookTestDB(t)
	queue := jobs.NewQueue(db, config.SQLite, nil)
	service := NewService(db, config.SQLite, auth, []byte("01234567890123456789012345678901"), queue)
	created, err := service.Create(ctx, owner, owner.WorkspaceID, CreateInput{
		Name: "missing", URL: "https://example.com/hook", Events: []string{"item.created"},
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	// The integer key of the real webhook must not work as an input.
	integerKey := strconv.FormatInt(int64(ParseWebhookIDOrZero(created.ID.String())), 10)
	for _, input := range []string{"", "0", integerKey, strings.Repeat("a", 32)} {
		webhookID := ParseWebhookIDOrZero(input)
		if err := service.Delete(ctx, owner, owner.WorkspaceID, webhookID, identity.AuditContext{}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Delete(%q) error = %v; want ErrNotFound", input, err)
		}
		if _, err := service.ListDeliveries(ctx, owner, owner.WorkspaceID, webhookID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("ListDeliveries(%q) error = %v; want ErrNotFound", input, err)
		}
	}
}

func TestDeliveriesCarryTypedShortIDsAndSharedEventID(t *testing.T) {
	ctx := context.Background()
	db, auth, owner := webhookTestDB(t)
	queue := jobs.NewQueue(db, config.SQLite, nil)
	service := NewService(db, config.SQLite, auth, []byte("01234567890123456789012345678901"), queue)
	if err := queue.Register(JobKind, service.HandleJob); err != nil {
		t.Fatal(err)
	}
	var webhookIDs []WebhookID
	for _, name := range []string{"first", "second"} {
		created, err := service.Create(ctx, owner, owner.WorkspaceID, CreateInput{
			Name: name, URL: "https://example.com/" + name, Events: []string{"item.created"},
		}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		webhookIDs = append(webhookIDs, created.ID)
	}
	itemService := items.NewService(db, config.SQLite, auth, 100, service)
	if _, err := itemService.Create(ctx, owner, owner.WorkspaceID, "Shared event", "typed-ids-key", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}

	var eventIDs []string
	for _, webhookID := range webhookIDs {
		deliveries, err := service.ListDeliveries(ctx, owner, owner.WorkspaceID, webhookID)
		if err != nil || len(deliveries) != 1 {
			t.Fatalf("deliveries for %s = %+v, err = %v", webhookID, deliveries, err)
		}
		delivery := deliveries[0]
		if _, err := ParseDeliveryID(delivery.ID.String()); err != nil {
			t.Fatalf("delivery ID %q is not a delivery short ID: %v", delivery.ID, err)
		}
		if delivery.WebhookID != webhookID {
			t.Fatalf("delivery webhookId = %s; want %s", delivery.WebhookID, webhookID)
		}
		if _, err := ParseEventID(delivery.EventID.String()); err != nil {
			t.Fatalf("event ID %q is not an event short ID: %v", delivery.EventID, err)
		}
		eventIDs = append(eventIDs, delivery.EventID.String())
	}
	if eventIDs[0] != eventIDs[1] {
		t.Fatalf("one event produced two event IDs: %v", eventIDs)
	}
}
