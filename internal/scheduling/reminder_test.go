package scheduling

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/jobs"
	"example.com/dynamis-code/apps-template/internal/notifications"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestBookingRequestsReminderAndReplacesOrCancelsIt(t *testing.T) {
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
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	clientService := clients.NewService(db, config.SQLite, auth, 50)
	clientService.SetEmailChangeHook(s.RefreshClientRemindersTx)
	r, err := s.Create(ctx, actor, owner.WorkspaceID, "Room", nil)
	if err != nil {
		t.Fatal(err)
	}
	var hours []WorkingHours
	for day := 0; day < 7; day++ {
		hours = append(hours, WorkingHours{Weekday: day, Start: "00:00", End: "23:59"})
	}
	if err := s.SetWorkingHours(ctx, actor, owner.WorkspaceID, r.ID, hours); err != nil {
		t.Fatal(err)
	}
	duration := 60
	entry, err := catalog.NewService(db, auth, nil).Create(ctx, actor, owner.WorkspaceID, "Visit", &duration, []catalog.PriceOption{{Mode: "manual", Currency: "USD", Variant: "Standard"}})
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(db, config.SQLite, auth, 50).Create(ctx, actor, owner.WorkspaceID, clients.Input{DisplayName: "Client", Email: "client@example.com"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	es := engagement.NewService(db, config.SQLite, auth)
	if _, err := es.CreateStage(ctx, actor, owner.WorkspaceID, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	e, err := es.Create(ctx, actor, owner.WorkspaceID, engagement.Input{ClientID: client.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().Add(72 * time.Hour)
	start := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.UTC)
	b, err := s.BookWithEngagement(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start, &e.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertReminderRequest(t, db, b.ID, start.Add(-24*time.Hour))
	// Simulate a booking created before reminder jobs were introduced.
	if _, err := db.Exec(`DELETE FROM background_jobs WHERE kind = $1 AND deduplication_key = $2`, notifications.BookingReminderKind, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileReminders(ctx); err != nil {
		t.Fatal(err)
	}
	assertReminderRequest(t, db, b.ID, start.Add(-24*time.Hour))
	if err := s.ReconcileReminders(ctx); err != nil {
		t.Fatal(err)
	}
	assertReminderRequest(t, db, b.ID, start.Add(-24*time.Hour))
	payload, err := json.Marshal(notifications.BookingReminder{BookingID: int64(b.ID), Start: start})
	if err != nil {
		t.Fatal(err)
	}
	var jobID int64
	if err := db.QueryRow(`SELECT id FROM background_jobs WHERE kind = $1 AND deduplication_key = $2`, notifications.BookingReminderKind, b.ID).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	stale := jobs.Job{ID: jobID, WorkspaceID: owner.WorkspaceID, Payload: string(payload), LeaseToken: "old"}
	if _, err := db.Exec(`UPDATE background_jobs SET status = 'running', lease_token = 'old' WHERE id = $1`, jobID); err != nil {
		t.Fatal(err)
	}
	sender := &recordingReminderSender{}
	if err := s.MoveBooking(ctx, actor, owner.WorkspaceID, r.ID, b.ID, start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	assertReminderRequest(t, db, b.ID, start.Add(-23*time.Hour))
	if err := s.ReminderHandler(sender)(ctx, stale); err != nil || sender.calls != 0 {
		t.Fatalf("stale reminder: calls=%d err=%v", sender.calls, err)
	}
	if err := s.SetBookingStatus(ctx, actor, owner.WorkspaceID, r.ID, b.ID, BookingCancelled); err != nil {
		t.Fatal(err)
	}
	assertNoReminderRequest(t, db, b.ID)
	if err := s.ReminderHandler(sender)(ctx, stale); err != nil || sender.calls != 0 {
		t.Fatalf("cancelled reminder: calls=%d err=%v", sender.calls, err)
	}

	for i, status := range []BookingStatus{BookingDone, BookingNoShow} {
		other, err := s.BookWithEngagement(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start.Add(time.Duration(i+2)*time.Hour), &e.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SetBookingStatus(ctx, actor, owner.WorkspaceID, r.ID, other.ID, status); err != nil {
			t.Fatal(err)
		}
		assertNoReminderRequest(t, db, other.ID)
	}
	withoutClient, err := s.Book(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start.Add(5*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	assertNoReminderRequest(t, db, withoutClient.ID)
	noEmail, err := clientService.Create(ctx, actor, owner.WorkspaceID, clients.Input{DisplayName: "No email"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	noEmailEngagement, err := es.Create(ctx, actor, owner.WorkspaceID, engagement.Input{ClientID: noEmail.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetEngagement(ctx, actor, owner.WorkspaceID, withoutClient.ID, &noEmailEngagement.ID); err != nil {
		t.Fatal(err)
	}
	assertNoReminderRequest(t, db, withoutClient.ID)
	queue := jobs.NewQueue(db, config.SQLite, nil)
	if err := queue.Register(notifications.BookingReminderKind, s.ReminderHandler(sender)); err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"first@example.com", "second@example.com", "", "restored@example.com"} {
		noEmail, err = clientService.Update(ctx, actor, owner.WorkspaceID, noEmail.ID, noEmail.Version, clients.UpdateInput{Email: &email}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		if email == "" {
			assertNoReminderRequest(t, db, withoutClient.ID)
		} else {
			assertReminderRequest(t, db, withoutClient.ID, start.Add(-19*time.Hour))
		}
	}
	if _, err := db.Exec(`UPDATE background_jobs SET available_at = $1 WHERE kind = $2 AND deduplication_key = $3`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), notifications.BookingReminderKind, withoutClient.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := queue.Process(ctx, 1); err != nil || n != 1 || sender.recipient != "restored@example.com" {
		t.Fatalf("changed email delivery = %d, %v, recipient %q", n, err, sender.recipient)
	}
	// Simulate a previously queued request whose recipient disappeared before delivery.
	if _, err := db.Exec(`UPDATE clients SET email = '' WHERE id = $1`, noEmail.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE background_jobs SET status = 'pending', available_at = $1 WHERE kind = $2 AND deduplication_key = $3`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), notifications.BookingReminderKind, withoutClient.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := queue.Process(ctx, 1); err != nil || n != 1 || sender.calls != 1 {
		t.Fatalf("missing email delivery = %d, %v, calls %d", n, err, sender.calls)
	}
	address := "again@example.com"
	noEmail, err = clientService.Update(ctx, actor, owner.WorkspaceID, noEmail.ID, noEmail.Version, clients.UpdateInput{Email: &address}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	assertReminderRequest(t, db, withoutClient.ID, start.Add(-19*time.Hour))
	sender.calls = 0
	if err := s.SetEngagement(ctx, actor, owner.WorkspaceID, withoutClient.ID, &e.ID); err != nil {
		t.Fatal(err)
	}
	assertReminderRequest(t, db, withoutClient.ID, start.Add(-19*time.Hour))

	deliver, err := s.BookWithEngagement(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start.Add(6*time.Hour), &e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE background_jobs SET available_at = $1 WHERE kind = $2 AND deduplication_key = $3`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), notifications.BookingReminderKind, deliver.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := queue.Process(ctx, 1); err != nil || n != 1 {
		t.Fatalf("process = %d, %v", n, err)
	}
	if sender.calls != 1 || sender.recipient != "client@example.com" {
		t.Fatalf("delivery = %+v", sender)
	}
	if n, err := queue.Process(ctx, 1); err != nil || n != 0 || sender.calls != 1 {
		t.Fatalf("reprocess = %d, %v, calls %d", n, err, sender.calls)
	}
	if err := s.SetEngagement(ctx, actor, owner.WorkspaceID, deliver.ID, &e.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileReminders(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := queue.Process(ctx, 1); err != nil || n != 0 || sender.calls != 1 {
		t.Fatalf("repeat request = %d, %v, calls %d", n, err, sender.calls)
	}
	changedEmail := "changed-after-delivery@example.com"
	if _, err := clientService.Update(ctx, actor, owner.WorkspaceID, client.ID, client.Version, clients.UpdateInput{Email: &changedEmail}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM background_jobs WHERE kind = $1 AND deduplication_key = $2`, notifications.BookingReminderKind, deliver.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" {
		t.Fatalf("delivered reminder after email change = %q", status)
	}
}

type recordingReminderSender struct {
	calls     int
	recipient string
}

func (s *recordingReminderSender) Send(_ context.Context, recipient, _, _ string) error {
	s.calls++
	s.recipient = recipient
	return nil
}

func assertReminderRequest(t *testing.T, db *sql.DB, booking BookingID, due time.Time) {
	t.Helper()
	var value string
	if err := db.QueryRow(`SELECT available_at FROM background_jobs WHERE kind = $1 AND deduplication_key = $2 AND status = 'pending'`, notifications.BookingReminderKind, booking).Scan(&value); err != nil {
		t.Fatal(err)
	}
	got, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || !got.Equal(due) {
		t.Fatalf("reminder due = %v, %v; want %v", got, err, due)
	}
}

func assertNoReminderRequest(t *testing.T, db *sql.DB, booking BookingID) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM background_jobs WHERE kind = $1 AND deduplication_key = $2`, notifications.BookingReminderKind, booking).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("got %d reminder requests, want none", count)
	}
}
