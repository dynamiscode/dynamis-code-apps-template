package intake

import (
	"context"
	"errors"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/jobs"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/secretbox"
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
	svc.ConfigureEncryption(secretbox.New(map[int][]byte{1: []byte("01234567890123456789012345678901")}, 1))
	svc.ConfigureNotifications(jobs.NewQueue(db, config.SQLite, nil), "https://example.test")
	for _, role := range []string{"admin", "member", "viewer"} {
		var userID int64
		if err := db.QueryRow(`INSERT INTO users (email, created_at) VALUES (?, '2026-01-01T00:00:00Z') RETURNING id`, role+"@example.com").Scan(&userID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, ?, '2026-01-01T00:00:00Z')`, boot.WorkspaceID, userID, role); err != nil {
			t.Fatal(err)
		}
	}
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
	if err := db.QueryRow(`SELECT count(*) FROM notifications`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("notifications after refusal: %d, %v", count, err)
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
	if err := auth.SetNotificationPreference(ctx, boot.UserID, "public_intake", false, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Submit(ctx, token, input, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	notifications, err := auth.ListNotifications(ctx, boot.UserID, boot.WorkspaceID, true, 10)
	if err != nil || len(notifications) != 1 {
		t.Fatalf("owner notifications = %v, %v; want one", notifications, err)
	}
	var eligible, other int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notifications n JOIN workspace_members m ON m.user_id = n.user_id AND m.workspace_id = n.workspace_id WHERE m.role IN ('owner','admin')`).Scan(&eligible); err != nil || eligible != 2 {
		t.Fatalf("eligible notifications = %d, %v; want two", eligible, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM notifications n JOIN workspace_members m ON m.user_id = n.user_id AND m.workspace_id = n.workspace_id WHERE m.role IN ('member','viewer')`).Scan(&other); err != nil || other != 0 {
		t.Fatalf("other notifications = %d, %v; want zero", other, err)
	}
	var payload string
	if err := db.QueryRow(`SELECT payload FROM background_jobs WHERE kind = 'email.delivery'`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, "owner@example.com") || !strings.Contains(payload, "admin@example.com") || strings.Contains(payload, "visitor@example.com") || strings.Contains(payload, "Visitor") || !strings.Contains(payload, notifications[0].Body) {
		t.Fatalf("unexpected email payload %q", payload)
	}
	if err := db.QueryRow(`SELECT count(*) FROM background_jobs WHERE kind = 'email.delivery'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("email jobs = %d, %v; want one", count, err)
	}
	var stage string
	var consent string
	if err := db.QueryRow(`SELECT s.name, c.intake_consented_at FROM clients c JOIN engagements e ON e.client_id = c.id JOIN engagement_stages s ON s.id = e.stage_id`).Scan(&stage, &consent); err != nil || stage != "First" || consent == "" {
		t.Fatalf("submission: %q %q %v", stage, consent, err)
	}
	if err := svc.Submit(ctx, token, Submission{DisplayName: "Duplicate", Email: "VISITOR@example.com", Phone: "555-1234", Consent: true}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	var first, duplicate bool
	if err := db.QueryRow(`SELECT possible_duplicate FROM clients WHERE display_name = 'Visitor'`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT possible_duplicate FROM clients WHERE display_name = 'Duplicate'`).Scan(&duplicate); err != nil {
		t.Fatal(err)
	}
	if first || !duplicate {
		t.Fatalf("duplicate flags: original %v, new %v", first, duplicate)
	}
	if err := svc.Submit(ctx, token, Submission{DisplayName: "Phone match", Phone: "555-1234", Consent: true}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT possible_duplicate FROM clients WHERE display_name = 'Phone match'`).Scan(&duplicate); err != nil || !duplicate {
		t.Fatalf("phone duplicate flag: %v, %v", duplicate, err)
	}
}
