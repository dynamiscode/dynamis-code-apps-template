package intake

import (
	"context"
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestSubmissionCreatesClientAndEngagementTogether(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	boot, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authorize(ctx, boot.UserID, boot.WorkspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, config.SQLite, auth)
	token, err := svc.Create(ctx, owner, boot.WorkspaceID, "Consent", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	input := Submission{DisplayName: "Visitor", Email: "visitor@example.com", Consent: true}
	if _, err := svc.PublicForm(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("form without stage: %v", err)
	}
	if err := svc.Submit(ctx, token, input, identity.AuditContext{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no stage: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM clients`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("clients after refusal: %d, %v", count, err)
	}
	for _, q := range []string{`INSERT INTO engagement_stages (workspace_id,name,kind,position) VALUES (?, 'Won', 'won', 1)`, `INSERT INTO engagement_stages (workspace_id,name,kind,position) VALUES (?, 'First', 'open', 2)`, `INSERT INTO engagement_stages (workspace_id,name,kind,position) VALUES (?, 'Second', 'open', 3)`} {
		if _, err := db.Exec(q, boot.WorkspaceID); err != nil {
			t.Fatal(err)
		}
	}
	if consent, err := svc.PublicForm(ctx, token); err != nil || consent != "Consent" {
		t.Fatalf("form: %q %v", consent, err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_engagement BEFORE INSERT ON engagements BEGIN SELECT RAISE(FAIL, 'forced failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := svc.Submit(ctx, token, input, identity.AuditContext{}); err == nil {
		t.Fatal("expected engagement failure")
	}
	if err := db.QueryRow(`SELECT count(*) FROM clients`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("clients after failure: %d, %v", count, err)
	}
	if _, err := db.Exec(`DROP TRIGGER fail_engagement`); err != nil {
		t.Fatal(err)
	}
	if err := svc.Submit(ctx, token, input, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	var stage string
	var consent string
	if err := db.QueryRow(`SELECT s.name, c.intake_consented_at FROM clients c JOIN engagements e ON e.client_id = c.id JOIN engagement_stages s ON s.id = e.stage_id`).Scan(&stage, &consent); err != nil || stage != "First" || consent == "" {
		t.Fatalf("submission: %q %q %v", stage, consent, err)
	}
}
