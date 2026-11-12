package jobs

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
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
	workspaceID := insertJobsWorkspace(t, db)

	queue := NewQueue(db, config.SQLite, nil)
	attempts := 0
	if err := queue.Register("test", func(_ context.Context, job Job) error {
		if job.WorkspaceID != workspaceID || job.Payload != `{"value":1}` {
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
	if err := queue.EnqueueTx(ctx, tx, workspaceID, "test", "dedup-1", `{"value":1}`, time.Now()); err != nil {
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
	if err := queue.EnqueueTx(ctx, tx, workspaceID, "test", "lease-1", `{"value":1}`, time.Now()); err != nil {
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
	if err := queue.EnqueueTx(ctx, tx, workspaceID, "test", "final-lease", `{"value":1}`, time.Now()); err != nil {
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

func TestQueueExhaustionTriggeredOnMaxAttempts(t *testing.T) {
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
	workspaceID := insertJobsWorkspace(t, db)

	queue := NewQueue(db, config.SQLite, nil)
	if err := queue.Register("fail-job", func(_ context.Context, _ Job) error {
		return errors.New("permanent failure")
	}); err != nil {
		t.Fatal(err)
	}
	exhaustedCount := 0
	var exhaustedJob Job
	if err := queue.RegisterExhausted("fail-job", func(_ context.Context, j Job) error {
		exhaustedCount++
		exhaustedJob = j
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueTx(ctx, tx, workspaceID, "fail-job", "exhaust-key", `{"test":true}`, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			if _, err := db.Exec("UPDATE background_jobs SET available_at = ? WHERE deduplication_key = ?", stamp(time.Now().Add(-time.Second)), "exhaust-key"); err != nil {
				t.Fatal(err)
			}
		}
		processed, err := queue.Process(ctx, 1)
		if err != nil || processed != 1 {
			t.Fatalf("attempt %d Process() = %d, %v", attempt, processed, err)
		}
		if attempt < maxAttempts {
			if exhaustedCount != 0 {
				t.Fatalf("attempt %d: exhaustedCount = %d, want 0", attempt, exhaustedCount)
			}
		}
	}

	if exhaustedCount != 1 {
		t.Fatalf("after %d failed attempts, exhaustedCount = %d, want 1", maxAttempts, exhaustedCount)
	}
	if exhaustedJob.AttemptCount != maxAttempts {
		t.Fatalf("exhaustedJob.AttemptCount = %d, want %d", exhaustedJob.AttemptCount, maxAttempts)
	}
}

func TestEvaluateJobOutcome(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

	t.Run("succeeded on attempt 1 to 5", func(t *testing.T) {
		for attempt := 1; attempt <= 5; attempt++ {
			outcome := EvaluateJobOutcome(attempt, 5, nil, now)
			if outcome.Status != JobSucceeded {
				t.Fatalf("attempt %d status = %v, want %v", attempt, outcome.Status, JobSucceeded)
			}
			if outcome.Exhausted {
				t.Fatalf("attempt %d exhausted = true, want false", attempt)
			}
			if outcome.AvailableAt != nil {
				t.Fatalf("attempt %d AvailableAt = %v, want nil", attempt, outcome.AvailableAt)
			}
			if outcome.FailureCategory != "" {
				t.Fatalf("attempt %d FailureCategory = %q, want empty", attempt, outcome.FailureCategory)
			}
		}
	})

	t.Run("retryable failures on attempts 1 to 4 with exponential backoff", func(t *testing.T) {
		expectedDelays := map[int]time.Duration{
			1: time.Second,
			2: 2 * time.Second,
			3: 4 * time.Second,
			4: 8 * time.Second,
		}
		for attempt, delay := range expectedDelays {
			err := Failure{Category: "transient-err"}
			outcome := EvaluateJobOutcome(attempt, 5, err, now)
			if outcome.Status != JobPending {
				t.Fatalf("attempt %d status = %v, want %v", attempt, outcome.Status, JobPending)
			}
			if outcome.Exhausted {
				t.Fatalf("attempt %d exhausted = true, want false", attempt)
			}
			if outcome.AvailableAt == nil {
				t.Fatalf("attempt %d AvailableAt is nil, want %v", attempt, now.Add(delay))
			} else if !outcome.AvailableAt.Equal(now.Add(delay)) {
				t.Fatalf("attempt %d AvailableAt = %v, want %v", attempt, *outcome.AvailableAt, now.Add(delay))
			}
			if outcome.FailureCategory != "transient-err" {
				t.Fatalf("attempt %d FailureCategory = %q, want %q", attempt, outcome.FailureCategory, "transient-err")
			}
		}
	})

	t.Run("exhausted on attempt 5", func(t *testing.T) {
		err := Failure{Category: "final-failure"}
		outcome := EvaluateJobOutcome(5, 5, err, now)
		if outcome.Status != JobFailed {
			t.Fatalf("attempt 5 status = %v, want %v", outcome.Status, JobFailed)
		}
		if !outcome.Exhausted {
			t.Fatalf("attempt 5 exhausted = false, want true")
		}
		if outcome.AvailableAt != nil {
			t.Fatalf("attempt 5 AvailableAt = %v, want nil", outcome.AvailableAt)
		}
		if outcome.FailureCategory != "final-failure" {
			t.Fatalf("attempt 5 FailureCategory = %q, want %q", outcome.FailureCategory, "final-failure")
		}
	})

	t.Run("exhausted when attempt exceeds maxAttempts", func(t *testing.T) {
		// e.g. reclaimed after lease expired on attempt 5 (attempt = 6)
		outcome := EvaluateJobOutcome(6, 5, nil, now)
		if outcome.Status != JobFailed {
			t.Fatalf("attempt 6 status = %v, want %v", outcome.Status, JobFailed)
		}
		if !outcome.Exhausted {
			t.Fatalf("attempt 6 exhausted = false, want true")
		}
		if outcome.AvailableAt != nil {
			t.Fatalf("attempt 6 AvailableAt = %v, want nil", outcome.AvailableAt)
		}
		if outcome.FailureCategory != "attempts-exhausted" {
			t.Fatalf("attempt 6 FailureCategory = %q, want %q", outcome.FailureCategory, "attempts-exhausted")
		}

		// With explicit handler error
		errOutcome := EvaluateJobOutcome(6, 5, Failure{Category: "stale-lease"}, now)
		if errOutcome.Status != JobFailed || !errOutcome.Exhausted || errOutcome.FailureCategory != "stale-lease" {
			t.Fatalf("attempt 6 with error outcome = %+v", errOutcome)
		}
	})

	t.Run("redacts arbitrary error to handler-error", func(t *testing.T) {
		outcome := EvaluateJobOutcome(1, 5, errors.New("sensitive db error"), now)
		if outcome.FailureCategory != "handler-error" {
			t.Fatalf("FailureCategory = %q, want %q", outcome.FailureCategory, "handler-error")
		}
	})

	t.Run("normalizes invalid attempt and maxAttempts bounds", func(t *testing.T) {
		outcome := EvaluateJobOutcome(0, 0, errors.New("err"), now)
		// normalized attempt=1, maxAttempts=1 -> attempt >= maxAttempts -> JobFailed, Exhausted=true
		if outcome.Status != JobFailed || !outcome.Exhausted {
			t.Fatalf("outcome for attempt 0, max 0 = %+v", outcome)
		}
	})
}

func insertJobsWorkspace(t *testing.T, db *sql.DB) identity.WorkspaceID {
	t.Helper()
	var workspaceID identity.WorkspaceID
	if err := db.QueryRow(
		"INSERT INTO workspaces (name, created_at) VALUES (?, ?) RETURNING id",
		"Jobs", stamp(time.Now()),
	).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	return workspaceID
}
