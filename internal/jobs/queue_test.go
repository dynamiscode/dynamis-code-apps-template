package jobs

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestQueueRetriesWithLeaseAndRedactsHandlerErrors(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{
		Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "jobs.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO workspaces (id, name, created_at) VALUES (?, ?, ?)", "workspace-jobs", "Jobs", stamp(time.Now())); err != nil {
		t.Fatal(err)
	}

	queue := NewQueue(db, config.SQLite, nil)
	attempts := 0
	if err := queue.Register("test", func(_ context.Context, job Job) error {
		if job.WorkspaceID != "workspace-jobs" || job.Payload != `{"value":1}` {
			t.Fatalf("job = %+v", job)
		}
		attempts++
		if attempts == 1 {
			return errors.New("secret payload must not be persisted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	exhausted := false
	if err := queue.RegisterExhausted("test", func(_ context.Context, _ Job) error {
		exhausted = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueTx(ctx, tx, "workspace-jobs", "test", "dedup-1", `{"value":1}`, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if processed, err := queue.Process(ctx, 1); err != nil || processed != 1 {
		t.Fatalf("first Process() = %d, %v", processed, err)
	}
	var status, lastError string
	var attemptCount int
	if err := db.QueryRow("SELECT status, attempt_count, last_error FROM background_jobs WHERE deduplication_key = ?", "dedup-1").Scan(&status, &attemptCount, &lastError); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attemptCount != 1 || lastError != "handler-error" {
		t.Fatalf("first job state = %q, %d, %q", status, attemptCount, lastError)
	}
	if _, err := db.Exec("UPDATE background_jobs SET available_at = ? WHERE deduplication_key = ?", stamp(time.Now().Add(-time.Second)), "dedup-1"); err != nil {
		t.Fatal(err)
	}
	if processed, err := queue.Process(ctx, 1); err != nil || processed != 1 {
		t.Fatalf("second Process() = %d, %v", processed, err)
	}
	var startedAt, completedAt, finalError sql.NullString
	if err := db.QueryRow("SELECT status, attempt_count, started_at, completed_at, last_error FROM background_jobs WHERE deduplication_key = ?", "dedup-1").Scan(&status, &attemptCount, &startedAt, &completedAt, &finalError); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || attemptCount != 2 || !startedAt.Valid || !completedAt.Valid || finalError.Valid {
		t.Fatalf("final job state = %q, %d, %q, %q, %q", status, attemptCount, startedAt.String, completedAt.String, finalError.String)
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueTx(ctx, tx, "workspace-jobs", "test", "lease-1", `{"value":1}`, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE background_jobs SET status = 'running', attempt_count = 1, lease_token = ?, leased_until = ?, started_at = ? WHERE deduplication_key = ?", "stale-lease", stamp(time.Now().Add(-time.Minute)), stamp(time.Now().Add(-2*time.Minute)), "lease-1"); err != nil {
		t.Fatal(err)
	}
	if processed, err := queue.Process(ctx, 1); err != nil || processed != 1 {
		t.Fatalf("lease Process() = %d, %v", processed, err)
	}
	if err := db.QueryRow("SELECT status, attempt_count FROM background_jobs WHERE deduplication_key = ?", "lease-1").Scan(&status, &attemptCount); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || attemptCount != 2 {
		t.Fatalf("reclaimed job state = %q, %d", status, attemptCount)
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueTx(ctx, tx, "workspace-jobs", "test", "final-lease", `{"value":1}`, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE background_jobs SET status = 'running', attempt_count = ?, lease_token = ?, leased_until = ?, started_at = ? WHERE deduplication_key = ?", maxAttempts, "final-stale-lease", stamp(time.Now().Add(-time.Minute)), stamp(time.Now().Add(-2*time.Minute)), "final-lease"); err != nil {
		t.Fatal(err)
	}
	if processed, err := queue.Process(ctx, 1); err != nil || processed != 1 {
		t.Fatalf("final lease Process() = %d, %v", processed, err)
	}
	if err := db.QueryRow("SELECT status, attempt_count FROM background_jobs WHERE deduplication_key = ?", "final-lease").Scan(&status, &attemptCount); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || attemptCount != maxAttempts+1 || !exhausted {
		t.Fatalf("final reclaimed job state = %q, %d", status, attemptCount)
	}
}
