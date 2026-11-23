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
	if err := queue.Register(EmailJobKind, EmailHandler(sender)); err != nil {
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
	handler := EmailHandler(sender)
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
