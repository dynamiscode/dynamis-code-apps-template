package clients

import (
	"database/sql"
	"fmt"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// wantAudit names one audit event by its action, target, and exact metadata.
// The metadata includes the client version or the cleared count, so each write
// matches exactly one row.
type wantAudit struct {
	action, eventType, targetType string
	targetKey                     int64
	actor                         identity.Principal
	metadata                      string
}

// requireAudit checks that exactly one audit event matches want, with the
// actor, auth method, outcome, and event type of the write.
func requireAudit(t *testing.T, f fixture, want wantAudit) {
	t.Helper()
	rows, err := f.db.QueryContext(f.ctx, database.Rebind(f.service.driver, `
		SELECT event_type, actor_user_id, auth_method, outcome, target_type
		FROM audit_events
		WHERE workspace_id = ? AND action = ? AND target_type = ? AND target_key = ? AND metadata = ?`),
		f.workspaceID, want.action, want.targetType, want.targetKey, want.metadata)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type row struct {
		eventType, authMethod, outcome, targetType string
		actorUserID                                sql.NullInt64
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.eventType, &r.actorUserID, &r.authMethod, &r.outcome, &r.targetType); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("%s on %s %d audit events with metadata %s = %d, want 1", want.action, want.targetType, want.targetKey, want.metadata, len(got))
	}
	r := got[0]
	if r.eventType != want.eventType {
		t.Fatalf("%s event_type = %q, want %q", want.action, r.eventType, want.eventType)
	}
	if !r.actorUserID.Valid || identity.UserID(r.actorUserID.Int64) != want.actor.UserID {
		t.Fatalf("%s actor_user_id = %v, want %d", want.action, r.actorUserID, want.actor.UserID)
	}
	if r.authMethod != want.actor.AuthMethod {
		t.Fatalf("%s auth_method = %q, want %q", want.action, r.authMethod, want.actor.AuthMethod)
	}
	if r.outcome != "success" {
		t.Fatalf("%s outcome = %q, want success", want.action, r.outcome)
	}
	if r.targetType != want.targetType {
		t.Fatalf("%s target_type = %q, want %q", want.action, r.targetType, want.targetType)
	}
}

// exerciseClientAudit runs each client and field-definition write once and
// checks its audit event. It then checks that no label or value reached the
// audit table, which holds only names, short IDs, and counts.
func exerciseClientAudit(t *testing.T, f fixture) {
	t.Helper()
	ctx, ws := f.ctx, f.workspaceID
	noAudit := identity.AuditContext{}

	client, err := f.service.Create(ctx, f.owner, ws, Input{DisplayName: "Audit Ada"}, noAudit)
	if err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client.create", "client.created", "client", int64(client.ID), f.owner, "{}"})

	edited := "Audit Ada Lovelace"
	client, err = f.service.Update(ctx, f.member, ws, client.ID, client.Version, UpdateInput{DisplayName: &edited}, noAudit)
	if err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client.update", "client.updated", "client", int64(client.ID), f.member,
		fmt.Sprintf(`{"version":%d}`, client.Version)})

	client, err = f.service.Archive(ctx, f.owner, ws, client.ID, noAudit)
	if err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client.archive", "client.archived", "client", int64(client.ID), f.owner, "{}"})

	client, err = f.service.Unarchive(ctx, f.owner, ws, client.ID, noAudit)
	if err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client.unarchive", "client.unarchived", "client", int64(client.ID), f.owner, "{}"})

	textField := defineTextField(t, f, "Audit Text Probe")
	requireAudit(t, f, wantAudit{"client_field.create", "client_field.created", "client_field", int64(textField.ID), f.owner, "{}"})

	choiceField := defineChoiceField(t, f, "Audit Choice Probe")
	requireAudit(t, f, wantAudit{"client_field.create", "client_field.created", "client_field", int64(choiceField.ID), f.owner, "{}"})

	option := addOptionOK(t, f, choiceField, "Audit Option Probe")
	requireAudit(t, f, wantAudit{"client_field_option.create", "client_field_option.created", "client_field_option", int64(option.ID), f.owner, "{}"})

	client, err = f.service.SetValue(ctx, f.member, ws, client.ID, client.Version, choiceField.ID, option.ID.String(), noAudit)
	if err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client.value.set", "client.updated", "client", int64(client.ID), f.member,
		fmt.Sprintf(`{"field":%q,"version":%d}`, choiceField.ID.String(), client.Version)})

	client, err = f.service.SetValue(ctx, f.member, ws, client.ID, client.Version, textField.ID, "Audit Value Probe", noAudit)
	if err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client.value.set", "client.updated", "client", int64(client.ID), f.member,
		fmt.Sprintf(`{"field":%q,"version":%d}`, textField.ID.String(), client.Version)})

	client, err = f.service.ClearValue(ctx, f.member, ws, client.ID, client.Version, textField.ID, noAudit)
	if err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client.value.clear", "client.updated", "client", int64(client.ID), f.member,
		fmt.Sprintf(`{"field":%q,"version":%d}`, textField.ID.String(), client.Version)})

	if err := f.service.DeleteOption(ctx, f.owner, ws, choiceField.ID, option.ID, noAudit); err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client_field_option.delete", "client_field_option.deleted", "client_field_option", int64(option.ID), f.owner,
		`{"clearedClients":1}`})

	numberField, err := f.service.DefineField(ctx, f.owner, ws, FieldInput{Label: "Audit Number Probe", Type: FieldTypeNumber}, noAudit)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.setFieldType(ctx, f.owner, ws, numberField.ID, FieldTypeText, noAudit); err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client_field.change_type", "client_field.type_changed", "client_field", int64(numberField.ID), f.owner, "{}"})

	if _, err := f.service.ArchiveField(ctx, f.owner, ws, textField.ID, noAudit); err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client_field.archive", "client_field.archived", "client_field", int64(textField.ID), f.owner, "{}"})

	if _, err := f.service.RestoreField(ctx, f.owner, ws, textField.ID, noAudit); err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client_field.restore", "client_field.restored", "client_field", int64(textField.ID), f.owner, "{}"})

	if _, err := f.service.ArchiveField(ctx, f.owner, ws, numberField.ID, noAudit); err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteField(ctx, f.owner, ws, numberField.ID, noAudit); err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client_field.delete", "client_field.deleted", "client_field", int64(numberField.ID), f.owner,
		`{"clearedClients":0}`})

	if err := f.service.Delete(ctx, f.owner, ws, client.ID, noAudit); err != nil {
		t.Fatal(err)
	}
	requireAudit(t, f, wantAudit{"client.delete", "client.deleted", "client", int64(client.ID), f.owner, "{}"})

	for _, probe := range []string{"Audit Ada", "Audit Text Probe", "Audit Choice Probe", "Audit Option Probe", "Audit Value Probe", "Audit Number Probe"} {
		var leaked int
		if err := f.db.QueryRowContext(ctx, database.Rebind(f.service.driver,
			`SELECT COUNT(*) FROM audit_events WHERE workspace_id = ? AND (metadata LIKE ? OR action LIKE ? OR event_type LIKE ?)`),
			ws, "%"+probe+"%", "%"+probe+"%", "%"+probe+"%",
		).Scan(&leaked); err != nil {
			t.Fatal(err)
		}
		if leaked != 0 {
			t.Fatalf("audit events carry %q in metadata, action, or event type", probe)
		}
	}
}

func TestClientAndFieldWritesAreAudited(t *testing.T) {
	exerciseClientAudit(t, newFixture(t))
}

// TestPostgresClientAndFieldWritesAreAudited runs the same audit checks on
// PostgreSQL. It is skipped unless POSTGRES_TEST_URL is set.
func TestPostgresClientAndFieldWritesAreAudited(t *testing.T) {
	exerciseClientAudit(t, newPostgresFixture(t))
}
