package jobs

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// TestStaleLeaseFinishCannotOverwriteReclaimedJob models the stale-lease
// interleaving in verification/jobs/JobQueue.tla: worker A claims a job, its
// lease expires, worker B reclaims it, and then A finishes. A's finish must
// match zero rows and leave B's running claim untouched.
func TestStaleLeaseFinishCannotOverwriteReclaimedJob(t *testing.T) {
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

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueTx(ctx, tx, workspaceID, "test", 1, `{"value":1}`, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	jobA, ok, err := queue.claim(ctx)
	if err != nil || !ok {
		t.Fatalf("worker A claim: ok=%v err=%v", ok, err)
	}
	// Lease expiry: worker A's leased_until is now in the past.
	if _, err := db.ExecContext(ctx, `UPDATE background_jobs SET leased_until = ? WHERE id = ?`,
		stamp(time.Now().Add(-time.Minute)), jobA.ID); err != nil {
		t.Fatal(err)
	}
	jobB, ok, err := queue.claim(ctx)
	if err != nil || !ok {
		t.Fatalf("worker B reclaim: ok=%v err=%v", ok, err)
	}
	if jobB.ID != jobA.ID || jobB.LeaseToken == jobA.LeaseToken {
		t.Fatalf("reclaim did not issue a new lease: A=%+v B=%+v", jobA, jobB)
	}

	if err := queue.finish(ctx, jobA, nil); err != nil {
		t.Fatalf("stale finish returned error: %v", err)
	}

	var status string
	var attempts int
	var leaseToken sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT status, attempt_count, lease_token FROM background_jobs WHERE id = ?`, jobA.ID,
	).Scan(&status, &attempts, &leaseToken); err != nil {
		t.Fatal(err)
	}
	if status != string(JobRunning) || !leaseToken.Valid || leaseToken.String != jobB.LeaseToken || attempts != 2 {
		t.Fatalf("stale finish changed reclaimed job: status=%s attempts=%d lease=%q valid=%v want running, 2, %s",
			status, attempts, leaseToken.String, leaseToken.Valid, jobB.LeaseToken)
	}
}
