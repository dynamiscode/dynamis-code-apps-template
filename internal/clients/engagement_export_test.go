package clients

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"example.com/dynamis-code/apps-template/internal/portability"
)

func TestSQLiteEngagementExport(t *testing.T) {
	runEngagementExport(t, openExportDB(t, config.SQLite), config.SQLite)
}
func TestPostgresEngagementExport(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	db, err := database.Open(context.Background(), config.Database{Driver: config.Postgres, URL: url, MaxOpenConns: 4, MaxIdleConns: 2})
	exportCheck(t, err)
	t.Cleanup(func() { db.Close() })
	exportCheck(t, database.Migrate(context.Background(), db, config.Postgres))
	runEngagementExport(t, db, config.Postgres)
}
func runEngagementExport(t *testing.T, db *sql.DB, driver config.DatabaseDriver) {
	t.Helper()
	ctx := context.Background()
	auth, actor, workspace := newExportWorkspace(t, db, driver)
	fixture := seedExportFixture(t, db, driver, workspace, actor.UserID)
	var client int64
	clientID, err := id.DecodeKey(id.TypeClient, fixture.archived)
	exportCheck(t, err)
	client = clientID
	insert := func(query string, args ...any) int64 {
		t.Helper()
		var key int64
		exportCheck(t, db.QueryRowContext(ctx, database.Rebind(driver, query), args...).Scan(&key))
		return key
	}
	stage := insert(`INSERT INTO engagement_stages (workspace_id, name, kind, position, archived_at) VALUES (?, 'Archived', 'won', 1, ?) RETURNING id`, workspace, "2026-08-22T09:00:00Z")
	engagement := insert(`INSERT INTO engagements (workspace_id, client_id, stage_id, name, notes, quote_status, quote_amount, quote_date_sent, assignee_id, agreement_recorded) VALUES (?, ?, ?, ?, 'notes', 'sent', '12.50', '2026-08-20', ?, TRUE) RETURNING id`, workspace, client, stage, nil, actor.UserID)
	_, otherActor, other := newExportWorkspace(t, db, driver)
	otherFixture := seedExportFixture(t, db, driver, other, otherActor.UserID)
	otherClient, err := id.DecodeKey(id.TypeClient, otherFixture.active)
	exportCheck(t, err)
	otherStage := insert(`INSERT INTO engagement_stages (workspace_id, name, kind, position) VALUES (?, 'Secret stage', 'open', 1) RETURNING id`, other)
	insert(`INSERT INTO engagements (workspace_id, client_id, stage_id, notes) VALUES (?, ?, ?, 'Secret engagement') RETURNING id`, other, otherClient, otherStage)
	service := portability.NewService(db, driver, auth, 100, 1<<20, NewService(db, driver, auth, 50))
	encoded, err := service.Export(ctx, actor, workspace, identity.AuditContext{})
	exportCheck(t, err)
	var result portability.Export
	exportCheck(t, json.Unmarshal(encoded, &result))
	if len(result.EngagementStages) != 1 || result.EngagementStages[0].ID != id.EncodeKey(id.TypeEngagementStage, stage) || result.EngagementStages[0].ArchivedAt == nil || result.EngagementStages[0].Position != 1 {
		t.Fatalf("stages = %+v", result.EngagementStages)
	}
	if len(result.Engagements) != 1 {
		t.Fatalf("engagements = %+v", result.Engagements)
	}
	got := result.Engagements[0]
	if got.ID != id.EncodeKey(id.TypeEngagement, engagement) || got.ClientID != fixture.archived || got.StageID != result.EngagementStages[0].ID || got.Name != nil || got.Notes != "notes" || got.QuoteAmount != "12.50" || got.QuoteStatus != "sent" || got.QuoteDateSent != "2026-08-20" || got.AssigneeID == nil || *got.AssigneeID != actor.UserID || !got.AgreementRecorded || got.AgreementFileID != nil {
		t.Fatalf("engagement = %+v", got)
	}
	// Count all exported rows immediately before each attempt; each attempt adds an audit row.
	count := exportRecordCount(t, db, workspace) + 2
	limited := portability.NewService(db, driver, auth, count-1, 1<<20, NewService(db, driver, auth, 50))
	if _, err := limited.Export(ctx, actor, workspace, identity.AuditContext{}); !errors.Is(err, portability.ErrLimit) {
		t.Fatalf("limit = %v", err)
	}
}
