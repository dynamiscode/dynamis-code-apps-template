package clients

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/portability"
)

// exportSlotKey matches a bare fNN slot key, which must never appear in an export.
var exportSlotKey = regexp.MustCompile(`"f[0-9]{2}"`)

// exportFixture holds the short IDs of the seeded client rows.
type exportFixture struct {
	active, archived string // clients
	text, choice     string // fields; text is archived after its value is set
	number           string // field defined and never set
	gold             string // option of the choice field
}

func TestSQLiteClientExportContents(t *testing.T) {
	db := openExportDB(t, config.SQLite)
	runClientExportContract(t, db, config.SQLite)
}

func TestPostgresClientExportContents(t *testing.T) {
	databaseURL := os.Getenv("POSTGRES_TEST_URL")
	if databaseURL == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	db, err := database.Open(context.Background(), config.Database{
		Driver: config.Postgres, URL: databaseURL, MaxOpenConns: 4, MaxIdleConns: 2,
	})
	exportCheck(t, err)
	t.Cleanup(func() { db.Close() })
	exportCheck(t, database.Migrate(context.Background(), db, config.Postgres))
	runClientExportContract(t, db, config.Postgres)
}

func TestSQLiteClientExportRecordLimitCountsClientRecords(t *testing.T) {
	ctx := context.Background()
	db := openExportDB(t, config.SQLite)
	auth, actor, workspaceID := newExportWorkspace(t, db, config.SQLite)
	seedExportFixture(t, db, config.SQLite, workspaceID, actor.UserID)
	audit := identity.AuditContext{RequestID: "limit-export", SourceAddress: "127.0.0.1"}
	clientService := NewService(db, config.SQLite, auth, 50)

	// Each export writes an audit row, so read the boundary from the tables right before each attempt.
	count := exportRecordCount(t, db, workspaceID)
	under := portability.NewService(db, config.SQLite, auth, count-1, 1<<20, clientService)
	if _, err := under.Export(ctx, actor, workspaceID, audit); !errors.Is(err, portability.ErrLimit) {
		t.Fatalf("export one record over limit = %v; want ErrLimit", err)
	}
	count = exportRecordCount(t, db, workspaceID)
	exact := portability.NewService(db, config.SQLite, auth, count, 1<<20, clientService)
	if _, err := exact.Export(ctx, actor, workspaceID, audit); err != nil {
		t.Fatalf("export at exact record limit = %v", err)
	}
}

// runClientExportContract seeds clients and checks the export contents.
func runClientExportContract(t *testing.T, db *sql.DB, driver config.DatabaseDriver) {
	t.Helper()
	ctx := context.Background()
	auth, actor, workspaceID := newExportWorkspace(t, db, driver)
	fixture := seedExportFixture(t, db, driver, workspaceID, actor.UserID)
	audit := identity.AuditContext{RequestID: "client-export", SourceAddress: "127.0.0.1"}
	service := portability.NewService(db, driver, auth, 100, 1<<20, NewService(db, driver, auth, 50))

	encoded, err := service.Export(ctx, actor, workspaceID, audit)
	exportCheck(t, err)
	var result portability.Export
	exportCheck(t, json.Unmarshal(encoded, &result))
	assertExportContents(t, encoded, result, fixture)

}

func seedExportFixture(
	t *testing.T,
	db *sql.DB,
	driver config.DatabaseDriver,
	workspaceID identity.WorkspaceID,
	userID identity.UserID,
) exportFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	archivedAt := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	insert := func(query string, args ...any) int64 {
		t.Helper()
		var key int64
		exportCheck(t, db.QueryRowContext(ctx, database.Rebind(driver, query), args...).Scan(&key))
		return key
	}
	// Slots are internal: the text field is slot 2, the choice field slot 1.
	textKey := insert(`INSERT INTO client_fields
		(workspace_id, created_by_user_id, label, field_type, slot, archived_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		workspaceID, userID, "Referral", "text", 2, archivedAt, now, now)
	choiceKey := insert(`INSERT INTO client_fields
		(workspace_id, created_by_user_id, label, field_type, slot, archived_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		workspaceID, userID, "Tier", "single_choice", 1, nil, now, now)
	numberKey := insert(`INSERT INTO client_fields
		(workspace_id, created_by_user_id, label, field_type, slot, archived_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		workspaceID, userID, "Seats", "number", 3, nil, now, now)
	goldKey := insert(`INSERT INTO client_field_options
		(workspace_id, field_id, slot, label, created_at) VALUES (?, ?, ?, ?, ?) RETURNING id`,
		workspaceID, choiceKey, 1, "Gold", now)

	fixture := exportFixture{
		text:   FieldID(textKey).String(),
		choice: FieldID(choiceKey).String(),
		number: FieldID(numberKey).String(),
		gold:   OptionID(goldKey).String(),
	}
	// A choice value stores the option's short ID under the field's slot key.
	activeKey := insert(`INSERT INTO clients
		(workspace_id, created_by_user_id, display_name, email, phone, notes, archived_at,
		 custom_values, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, '', '', NULL, ?, 3, ?, ?) RETURNING id`,
		workspaceID, userID, "Active client", "active@example.com",
		exportValuesJSON(t, map[string]string{"f01": fixture.gold, "f02": "Partner"}), now, now)
	archivedKey := insert(`INSERT INTO clients
		(workspace_id, created_by_user_id, display_name, email, phone, notes, archived_at,
		 custom_values, version, created_at, updated_at)
		VALUES (?, ?, ?, '', '', '', ?, ?, 2, ?, ?) RETURNING id`,
		workspaceID, userID, "Archived client", archivedAt,
		exportValuesJSON(t, map[string]string{"f02": "Former"}), now, now)
	fixture.active = ClientID(activeKey).String()
	fixture.archived = ClientID(archivedKey).String()
	return fixture
}

// newExportWorkspace seeds an owner and a workspace and returns an owner
// principal. The email is unique per run, so a reused Postgres database works.
func newExportWorkspace(
	t *testing.T,
	db *sql.DB,
	driver config.DatabaseDriver,
) (*identity.Service, identity.Principal, identity.WorkspaceID) {
	t.Helper()
	ctx := context.Background()
	auth, err := identity.NewService(db, driver)
	exportCheck(t, err)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	email := fmt.Sprintf("client-export-%d@example.com", time.Now().UnixNano())
	var userID identity.UserID
	var workspaceID identity.WorkspaceID
	exportCheck(t, db.QueryRowContext(ctx,
		database.Rebind(driver, "INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id"),
		email, now).Scan(&userID))
	exportCheck(t, db.QueryRowContext(ctx,
		database.Rebind(driver, "INSERT INTO workspaces (name, locale, created_at) VALUES (?, ?, ?) RETURNING id"),
		"Clients export", "en", now).Scan(&workspaceID))
	_, err = db.ExecContext(ctx,
		database.Rebind(driver, "INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, ?, ?)"),
		workspaceID, userID, identity.Owner, now)
	exportCheck(t, err)
	actor, err := auth.Authorize(ctx, userID, workspaceID, identity.WorkspaceExport)
	exportCheck(t, err)
	actor.AuthMethod = "test"
	return auth, actor, workspaceID
}

func openExportDB(t *testing.T, driver config.DatabaseDriver) *sql.DB {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{
		Driver: driver, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1,
	})
	exportCheck(t, err)
	t.Cleanup(func() { db.Close() })
	exportCheck(t, database.Migrate(ctx, db, driver))
	return db
}

// exportRecordCount counts the rows the export's record limit covers, read
// straight from the tables. It is the independent side of the boundary check.
func exportRecordCount(t *testing.T, db *sql.DB, workspaceID identity.WorkspaceID) int {
	t.Helper()
	total := 1
	for _, table := range []string{
		"workspace_members", "clients", "client_fields", "client_field_options", "audit_events",
	} {
		var count int
		exportCheck(t, db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE workspace_id = ?", workspaceID).Scan(&count))
		total += count
	}
	return total
}

func assertExportContents(t *testing.T, encoded []byte, result portability.Export, fixture exportFixture) {
	t.Helper()
	var keys map[string]json.RawMessage
	exportCheck(t, json.Unmarshal(encoded, &keys))
	for _, key := range []string{"clients", "clientFields", "clientFieldOptions"} {
		if _, ok := keys[key]; !ok {
			t.Fatalf("export lacks top-level key %q", key)
		}
	}
	if len(result.Clients) != 2 || len(result.ClientFields) != 3 || len(result.ClientFieldOptions) != 1 {
		t.Fatalf("client export counts: clients=%d fields=%d options=%d",
			len(result.Clients), len(result.ClientFields), len(result.ClientFieldOptions))
	}
	byClient := map[string]portability.ClientRecord{}
	for _, record := range result.Clients {
		byClient[record.ID] = record
	}
	active, ok := byClient[fixture.active]
	if !ok || active.DisplayName != "Active client" || active.Email != "active@example.com" || active.ArchivedAt != nil {
		t.Fatalf("active client export = %+v", active)
	}
	// Values are keyed by field short ID; the unset number field has no entry.
	if len(active.CustomValues) != 2 ||
		active.CustomValues[fixture.choice] != fixture.gold ||
		active.CustomValues[fixture.text] != "Partner" {
		t.Fatalf("active client values = %v", active.CustomValues)
	}
	archived, ok := byClient[fixture.archived]
	if !ok || archived.ArchivedAt == nil {
		t.Fatalf("archived client missing or not marked archived: %+v", archived)
	}
	if archived.CustomValues[fixture.text] != "Former" {
		t.Fatalf("archived client values = %v; want the archived field's value", archived.CustomValues)
	}
	byField := map[string]portability.ClientField{}
	for _, field := range result.ClientFields {
		byField[field.ID] = field
	}
	if text, ok := byField[fixture.text]; !ok || text.ArchivedAt == nil || text.Type != "text" {
		t.Fatalf("archived text field export = %+v", text)
	}
	if choice, ok := byField[fixture.choice]; !ok || choice.ArchivedAt != nil || choice.Type != "single_choice" {
		t.Fatalf("choice field export = %+v", choice)
	}
	// A field without values is still a definition: it is exported, but no client holds its value.
	if number, ok := byField[fixture.number]; !ok || number.ArchivedAt != nil || number.Type != "number" {
		t.Fatalf("number field export = %+v", number)
	}
	option := result.ClientFieldOptions[0]
	if option.ID != fixture.gold || option.FieldID != fixture.choice || option.Label != "Gold" {
		t.Fatalf("option export = %+v; want option %q of field %q", option, fixture.gold, fixture.choice)
	}
	if exportSlotKey.Match(encoded) {
		t.Fatal("export leaks an internal fNN slot key")
	}
}

func exportValuesJSON(t *testing.T, values map[string]string) string {
	t.Helper()
	encoded, err := json.Marshal(values)
	exportCheck(t, err)
	return string(encoded)
}

func exportCheck(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
