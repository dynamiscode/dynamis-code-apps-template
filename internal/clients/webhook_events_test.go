package clients

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/jobs"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/secretbox"
	"example.com/dynamis-code/apps-template/internal/webhooks"
)

// clientEventData is the wire shape of a client event's data.
type clientEventData struct {
	ID           string            `json:"id"`
	DisplayName  string            `json:"displayName"`
	Email        string            `json:"email"`
	Phone        string            `json:"phone"`
	Notes        string            `json:"notes"`
	ArchivedAt   *string           `json:"archivedAt"`
	Version      int64             `json:"version"`
	CustomValues map[string]string `json:"customValues"`
}

// outboxEvent is one event as the webhook outbox stored it.
type outboxEvent struct {
	Type string
	Data clientEventData
}

// newOutboxService returns a clients service wired to the real webhooks outbox,
// with one webhook subscribed to the four client events.
func newOutboxService(t *testing.T, f fixture, driver config.DatabaseDriver) *Service {
	t.Helper()
	queue := jobs.NewQueue(f.db, driver, nil)
	hooks := webhooks.NewService(f.db, driver, f.auth, secretbox.New(nil, 0), queue)
	if _, err := f.db.ExecContext(f.ctx, database.Rebind(driver, `
		INSERT INTO webhooks (workspace_id, name, url, secret_ciphertext, events, created_at)
		VALUES (?, 'clients', 'https://example.com/hook', 'unused', ?, ?)
	`), f.workspaceID,
		`["client.created","client.updated","client.archived","client.deleted"]`,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	return NewService(f.db, driver, f.auth, 50, hooks)
}

// outboxEvents reads every event the workspace's outbox holds, oldest first.
func outboxEvents(t *testing.T, f fixture, driver config.DatabaseDriver) []outboxEvent {
	t.Helper()
	rows, err := f.db.QueryContext(f.ctx, database.Rebind(driver, `
		SELECT e.event_type, d.payload FROM webhook_deliveries d
		JOIN webhook_events e ON e.id = d.event_id
		WHERE e.workspace_id = ? ORDER BY e.id
	`), f.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var events []outboxEvent
	for rows.Next() {
		var eventType, payload string
		if err := rows.Scan(&eventType, &payload); err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Type string          `json:"type"`
			Data clientEventData `json:"data"`
		}
		if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Type != eventType {
			t.Fatalf("envelope type %q does not match row type %q", envelope.Type, eventType)
		}
		events = append(events, outboxEvent{Type: eventType, Data: envelope.Data})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

// requireOutbox checks the outbox holds exactly the wanted event types, in order.
func requireOutbox(t *testing.T, f fixture, driver config.DatabaseDriver, want ...string) []outboxEvent {
	t.Helper()
	events := outboxEvents(t, f, driver)
	got := make([]string, 0, len(events))
	for _, event := range events {
		got = append(got, event.Type)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("outbox event types = %v, want %v", got, want)
	}
	return events
}

func TestClientWebhookEvents(t *testing.T) {
	runClientWebhookEvents(t, newFixture, config.SQLite)
}

func TestPostgresClientWebhookEvents(t *testing.T) {
	runClientWebhookEvents(t, newPostgresFixture, config.Postgres)
}

func runClientWebhookEvents(t *testing.T, newF func(*testing.T) fixture, driver config.DatabaseDriver) {
	t.Run("each change emits its event", func(t *testing.T) {
		checkClientEventSequence(t, newF(t), driver)
	})
	t.Run("failed changes emit nothing", func(t *testing.T) {
		checkFailedChangesEmitNothing(t, newF(t), driver)
	})
	t.Run("failed event write rolls back the change", func(t *testing.T) {
		checkEventWriteRollsBack(t, newF(t), driver)
	})
	t.Run("payload carries common fields and short-ID values", func(t *testing.T) {
		checkEventPayload(t, newF(t), driver)
	})
	t.Run("field definition changes emit nothing", func(t *testing.T) {
		checkFieldDefinitionChangesEmitNothing(t, newF(t), driver)
	})
	t.Run("value writes emit client updated", func(t *testing.T) {
		checkValueWritesEmitClientUpdated(t, newF(t), driver)
	})
}

func checkClientEventSequence(t *testing.T, f fixture, driver config.DatabaseDriver) {
	t.Helper()
	svc := newOutboxService(t, f, driver)
	audit := identity.AuditContext{}
	created, err := svc.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Ada"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	requireOutbox(t, f, driver, "client.created")

	if _, err := svc.Update(f.ctx, f.member, f.workspaceID, created.ID, created.Version,
		UpdateInput{DisplayName: ptr("Ada L")}, audit); err != nil {
		t.Fatal(err)
	}
	requireOutbox(t, f, driver, "client.created", "client.updated")

	archived, err := svc.Archive(f.ctx, f.member, f.workspaceID, created.ID, audit)
	if err != nil {
		t.Fatal(err)
	}
	requireOutbox(t, f, driver, "client.created", "client.updated", "client.archived")

	if _, err := svc.Archive(f.ctx, f.member, f.workspaceID, created.ID, audit); err != nil {
		t.Fatal(err)
	}
	requireOutbox(t, f, driver, "client.created", "client.updated", "client.archived")

	if _, err := svc.Unarchive(f.ctx, f.member, f.workspaceID, archived.ID, audit); err != nil {
		t.Fatal(err)
	}
	requireOutbox(t, f, driver, "client.created", "client.updated", "client.archived", "client.updated")

	if _, err := svc.Unarchive(f.ctx, f.member, f.workspaceID, archived.ID, audit); err != nil {
		t.Fatal(err)
	}
	requireOutbox(t, f, driver, "client.created", "client.updated", "client.archived", "client.updated")

	if err := svc.Delete(f.ctx, f.owner, f.workspaceID, created.ID, audit); err != nil {
		t.Fatal(err)
	}
	requireOutbox(t, f, driver,
		"client.created", "client.updated", "client.archived", "client.updated", "client.deleted")
}

func checkFailedChangesEmitNothing(t *testing.T, f fixture, driver config.DatabaseDriver) {
	t.Helper()
	svc := newOutboxService(t, f, driver)
	audit := identity.AuditContext{}
	created, err := svc.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Ada"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(f.ctx, f.viewer, f.workspaceID, Input{DisplayName: "Grace"}, audit); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer create error = %v, want forbidden", err)
	}
	if _, err := svc.Update(f.ctx, f.viewer, f.workspaceID, created.ID, created.Version,
		UpdateInput{DisplayName: ptr("Nope")}, audit); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer update error = %v, want forbidden", err)
	}
	if _, err := svc.Update(f.ctx, f.member, f.workspaceID, created.ID, created.Version+5,
		UpdateInput{DisplayName: ptr("Stale")}, audit); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("stale update error = %v, want precondition failed", err)
	}
	if _, err := svc.Archive(f.ctx, f.viewer, f.workspaceID, created.ID, audit); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer archive error = %v, want forbidden", err)
	}
	if err := svc.Delete(f.ctx, f.member, f.workspaceID, created.ID, audit); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member delete error = %v, want forbidden", err)
	}
	if _, err := svc.Archive(f.ctx, f.owner, f.workspaceID, ClientID(999999), audit); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archive missing client error = %v, want not found", err)
	}
	requireOutbox(t, f, driver, "client.created")
}

// failingPublisher stands in for an outbox write that cannot complete.
type failingPublisher struct{}

func (failingPublisher) PublishInTx(
	context.Context, *sql.Tx, identity.WorkspaceID, string, []byte, time.Time,
) error {
	return errors.New("outbox unavailable")
}

func checkEventWriteRollsBack(t *testing.T, f fixture, driver config.DatabaseDriver) {
	t.Helper()
	audit := identity.AuditContext{}
	created, err := newOutboxService(t, f, driver).Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Ada"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	broken := NewService(f.db, driver, f.auth, 50, failingPublisher{})
	if _, err := broken.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Grace"}, audit); err == nil {
		t.Fatal("create succeeded without its outbox row")
	}
	if _, err := broken.Archive(f.ctx, f.member, f.workspaceID, created.ID, audit); err == nil {
		t.Fatal("archive succeeded without its outbox row")
	}
	page, err := f.service.List(f.ctx, f.owner, f.workspaceID, ListInput{Limit: 10, Archived: ArchivedAll})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Clients) != 1 || page.Clients[0].ArchivedAt != nil {
		t.Fatalf("clients after failed event writes = %+v, want the one active client", page.Clients)
	}
}

func checkEventPayload(t *testing.T, f fixture, driver config.DatabaseDriver) {
	t.Helper()
	svc := newOutboxService(t, f, driver)
	audit := identity.AuditContext{}
	tier, err := svc.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Tier", Type: FieldTypeText}, audit)
	if err != nil {
		t.Fatal(err)
	}
	region, err := svc.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Region", Type: FieldTypeText}, audit)
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Create(f.ctx, f.member, f.workspaceID, Input{
		DisplayName: "Ada", Email: "ada@example.com", Phone: "555-0100", Notes: "Prefers email",
	}, audit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetValue(f.ctx, f.member, f.workspaceID, created.ID, created.Version, tier.ID, "gold", audit); err != nil {
		t.Fatal(err)
	}
	// Archiving a field hides it from forms and keeps its value, so the payload keeps it.
	if _, err := svc.ArchiveField(f.ctx, f.owner, f.workspaceID, tier.ID, audit); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Archive(f.ctx, f.member, f.workspaceID, created.ID, audit); err != nil {
		t.Fatal(err)
	}
	events := requireOutbox(t, f, driver, "client.created", "client.updated", "client.archived")

	first := events[0].Data
	if first.ID != created.ID.String() || first.DisplayName != "Ada" || first.Email != "ada@example.com" ||
		first.Phone != "555-0100" || first.Notes != "Prefers email" || first.ArchivedAt != nil ||
		first.Version != 1 || len(first.CustomValues) != 0 {
		t.Fatalf("created payload = %+v", first)
	}
	archived := events[2].Data
	if archived.ID != created.ID.String() || archived.DisplayName != "Ada" || archived.ArchivedAt == nil ||
		archived.Version != 3 {
		t.Fatalf("archived payload = %+v", archived)
	}
	if !slices.Equal(mapKeys(archived.CustomValues), []string{tier.ID.String()}) || archived.CustomValues[tier.ID.String()] != "gold" {
		t.Fatalf("custom values = %v, want only %s=gold (unset fields omitted)", archived.CustomValues, tier.ID)
	}
	if _, ok := archived.CustomValues[region.ID.String()]; ok {
		t.Fatal("unset field appeared in custom values")
	}
}

func checkFieldDefinitionChangesEmitNothing(t *testing.T, f fixture, driver config.DatabaseDriver) {
	t.Helper()
	svc := newOutboxService(t, f, driver)
	audit := identity.AuditContext{}
	if _, err := svc.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Ada"}, audit); err != nil {
		t.Fatal(err)
	}
	requireOutbox(t, f, driver, "client.created")

	field, err := svc.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Tier", Type: FieldTypeText}, audit)
	if err != nil {
		t.Fatal(err)
	}
	choice, err := svc.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Stage", Type: FieldTypeSingleChoice}, audit)
	if err != nil {
		t.Fatal(err)
	}
	option, err := svc.AddOption(f.ctx, f.owner, f.workspaceID, choice.ID, "Lead", audit)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteOption(f.ctx, f.owner, f.workspaceID, choice.ID, option.ID, audit); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ArchiveField(f.ctx, f.owner, f.workspaceID, field.ID, audit); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RestoreField(f.ctx, f.owner, f.workspaceID, field.ID, audit); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ArchiveField(f.ctx, f.owner, f.workspaceID, field.ID, audit); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteField(f.ctx, f.owner, f.workspaceID, field.ID, audit); err != nil {
		t.Fatal(err)
	}
	requireOutbox(t, f, driver, "client.created")
}

// checkValueWritesEmitClientUpdated checks that setting and clearing a value
// emits client.updated with the value in the payload, and that a value write
// whose outbox row fails leaves the value unchanged.
func checkValueWritesEmitClientUpdated(t *testing.T, f fixture, driver config.DatabaseDriver) {
	t.Helper()
	svc := newOutboxService(t, f, driver)
	audit := identity.AuditContext{}
	tier, err := svc.DefineField(f.ctx, f.owner, f.workspaceID, FieldInput{Label: "Tier", Type: FieldTypeText}, audit)
	if err != nil {
		t.Fatal(err)
	}
	created, err := svc.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Ada"}, audit)
	if err != nil {
		t.Fatal(err)
	}
	set, err := svc.SetValue(f.ctx, f.member, f.workspaceID, created.ID, created.Version, tier.ID, "gold", audit)
	if err != nil {
		t.Fatal(err)
	}
	events := requireOutbox(t, f, driver, "client.created", "client.updated")
	if got := events[1].Data; got.Version != set.Version || got.CustomValues[tier.ID.String()] != "gold" {
		t.Fatalf("set value payload = %+v, want version %d with tier=gold", got, set.Version)
	}

	if _, err := svc.ClearValue(f.ctx, f.member, f.workspaceID, created.ID, set.Version, tier.ID, audit); err != nil {
		t.Fatal(err)
	}
	events = requireOutbox(t, f, driver, "client.created", "client.updated", "client.updated")
	if got := events[2].Data; len(got.CustomValues) != 0 {
		t.Fatalf("clear value payload custom values = %v, want none (unset fields omitted)", got.CustomValues)
	}

	broken := NewService(f.db, driver, f.auth, 50, failingPublisher{})
	cleared, err := svc.Get(f.ctx, f.member, f.workspaceID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := broken.SetValue(f.ctx, f.member, f.workspaceID, created.ID, cleared.Version, tier.ID, "silver", audit); err == nil {
		t.Fatal("set value succeeded without its outbox row")
	}
	after, err := svc.Get(f.ctx, f.member, f.workspaceID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != cleared.Version {
		t.Fatalf("version after failed value write = %d, want %d", after.Version, cleared.Version)
	}
	if got, ok := valueOf(t, f, f.viewer, created.ID, tier.ID); ok {
		t.Fatalf("value after failed write = %q, want unset", got)
	}
	requireOutbox(t, f, driver, "client.created", "client.updated", "client.updated")
}

func mapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func ptr[T any](value T) *T {
	return &value
}
