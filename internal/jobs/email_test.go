package jobs

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

type testEmailSender struct {
	calls                    int
	fail                     bool
	recipient, subject, body string
}

func (s *testEmailSender) Send(_ context.Context, recipient, subject, body string) error {
	s.calls++
	s.recipient, s.subject, s.body = recipient, subject, body
	if s.fail {
		return errors.New("private provider detail")
	}
	return nil
}

func TestEmailJobDeliveryAndRetries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		failures int
		want     string
	}{{"success", 0, "succeeded"}, {"retry", 1, "succeeded"}, {"exhaustion", MaxAttempts, "failed"}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "jobs.db"), MaxOpenConns: 1, MaxIdleConns: 1})
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := database.Migrate(ctx, db, config.SQLite); err != nil {
				t.Fatal(err)
			}
			workspace := insertJobsWorkspace(t, db)
			queue := NewQueue(db, config.SQLite, nil)
			sender := &testEmailSender{}
			if err := queue.Register(EmailJobKind, EmailHandler(queue, sender)); err != nil {
				t.Fatal(err)
			}
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := EnqueueEmailTx(ctx, queue, tx, workspace, 1, Email{Recipient: "staff@example.com", Subject: "Alert", Body: "New request"}, time.Now()); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if sender.calls != 0 {
				t.Fatal("email sent before processing")
			}
			for attempt := 1; attempt <= tc.failures+1 && attempt <= MaxAttempts; attempt++ {
				sender.fail = attempt <= tc.failures
				if _, err := db.Exec("UPDATE background_jobs SET available_at = ?", stamp(time.Now().Add(-time.Second))); err != nil {
					t.Fatal(err)
				}
				if count, err := queue.Process(ctx, 1); err != nil || count != 1 {
					t.Fatalf("Process = %d, %v", count, err)
				}
				var status string
				var attempts int
				if err := db.QueryRow("SELECT status, attempt_count FROM background_jobs").Scan(&status, &attempts); err != nil {
					t.Fatal(err)
				}
				want := "pending"
				if attempt == tc.failures+1 || attempt == MaxAttempts {
					want = tc.want
				}
				if status != want || attempts != attempt {
					t.Fatalf("state = %s, %d; want %s, %d", status, attempts, want, attempt)
				}
			}
			if sender.recipient != "staff@example.com" || sender.subject != "Alert" || sender.body != "New request" {
				t.Fatal("message changed during delivery")
			}
		})
	}
}

type emailTestSender struct {
	err   error
	calls int
}

func TestQueuedEmailRetriesAndExhausts(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "email.db"), MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	workspaceID := insertJobsWorkspace(t, db)
	queue := NewQueue(db, config.SQLite, nil)
	sender := &emailTestSender{err: errors.New("private error")}
	if err := queue.Register(EmailJobKind, EmailHandler(queue, sender)); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	message := Email{Recipient: "staff@example.com", Subject: "New request", Body: "Open record"}
	if err := EnqueueEmailTx(ctx, queue, tx, workspaceID, 1, message, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 0 {
		t.Fatal("email sent inline")
	}
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		if _, err := db.Exec("UPDATE background_jobs SET available_at = ?", stamp(time.Now().Add(-time.Second))); err != nil {
			t.Fatal(err)
		}
		if n, err := queue.Process(ctx, 1); err != nil || n != 1 {
			t.Fatalf("attempt %d: processed %d, error %v", attempt, n, err)
		}
		var status, category string
		if err := db.QueryRow("SELECT status, last_error FROM background_jobs").Scan(&status, &category); err != nil {
			t.Fatal(err)
		}
		want := "pending"
		if attempt == MaxAttempts {
			want = "failed"
		}
		if status != want || category != "email-delivery" {
			t.Fatalf("attempt %d: status %q, category %q", attempt, status, category)
		}
	}
	if sender.calls != MaxAttempts {
		t.Fatalf("calls = %d", sender.calls)
	}
}

func (s *emailTestSender) Send(_ context.Context, recipient, subject, body string) error {
	s.calls++
	if recipient != "staff@example.com" || subject != "New request" || body != "Open record" {
		return errors.New("wrong message")
	}
	return s.err
}

func TestEmailJobOutcomes(t *testing.T) {
	sender := &emailTestSender{}
	handler := EmailHandler(nil, sender)
	job := Job{Payload: `{"recipient":"staff@example.com","subject":"New request","body":"Open record"}`}
	if err := handler(context.Background(), job); err != nil || sender.calls != 1 {
		t.Fatalf("delivery = %v, calls = %d; want successful send", err, sender.calls)
	}
	sender.err = errors.New("private provider error")
	err := handler(context.Background(), job)
	now := time.Now()
	if got := EvaluateJobOutcome(1, MaxAttempts, err, now); got.Status != JobPending || got.FailureCategory != "email-delivery" {
		t.Fatalf("retry = %+v", got)
	}
	if got := EvaluateJobOutcome(MaxAttempts, MaxAttempts, err, now); got.Status != JobFailed || !got.Exhausted {
		t.Fatalf("exhaustion = %+v", got)
	}
}

func TestEmailJobSendsEachRecipientEvenWhenOneFails(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "batch.db"), MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	workspace := insertJobsWorkspace(t, db)
	queue := NewQueue(db, config.SQLite, nil)
	sender := &batchSender{}
	if err := queue.Register(EmailJobKind, EmailHandler(queue, sender)); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnqueueEmailTx(ctx, queue, tx, workspace, 1, Email{Recipients: []string{"first@example.com", "second@example.com"}, Subject: "New request", Body: "Open record"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if n, err := queue.Process(ctx, 1); err != nil || n != 1 {
		t.Fatalf("first attempt: %d %v", n, err)
	}
	if _, err := db.Exec("UPDATE background_jobs SET available_at = ?", stamp(time.Now().Add(-time.Second))); err != nil {
		t.Fatal(err)
	}
	if n, err := queue.Process(ctx, 1); err != nil || n != 1 {
		t.Fatalf("retry: %d %v", n, err)
	}
	if len(sender.recipients) != 3 || sender.recipients[0] != "first@example.com" || sender.recipients[1] != "second@example.com" || sender.recipients[2] != "first@example.com" {
		t.Fatalf("recipients = %v", sender.recipients)
	}
}

type batchSender struct{ recipients []string }

func (s *batchSender) Send(_ context.Context, recipient, _, _ string) error {
	s.recipients = append(s.recipients, recipient)
	if recipient == "first@example.com" {
		return errors.New("delivery failed")
	}
	return nil
}
