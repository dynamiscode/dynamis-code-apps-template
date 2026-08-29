package jobs

import (
	"context"
	"os"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func TestPostgresQueueClaimAndFinish(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.Postgres, URL: url, MaxOpenConns: 2, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.Postgres); err != nil {
		t.Fatal(err)
	}
	workspaceID, kind := "workspace-jobs-"+mustID(t), "test-postgres-"+mustID(t)
	if _, err := db.ExecContext(ctx, database.Rebind(config.Postgres, "INSERT INTO workspaces (id, name, created_at) VALUES (?, ?, ?)"), workspaceID, "Jobs", stamp(time.Now())); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DELETE FROM workspaces WHERE id = $1", workspaceID) })
	queue := NewQueue(db, config.Postgres, nil)
	if err := queue.Register(kind, func(_ context.Context, job Job) error {
		if job.WorkspaceID != workspaceID {
			t.Fatalf("workspace scope = %q", job.WorkspaceID)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueTx(ctx, tx, workspaceID, kind, "once", `{"ok":true}`, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if processed, err := queue.Process(ctx, 1); err != nil || processed != 1 {
		t.Fatalf("Process() = %d, %v", processed, err)
	}
	var status string
	if err := db.QueryRowContext(ctx, "SELECT status FROM background_jobs WHERE kind = $1 AND deduplication_key = $2", kind, "once").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" {
		t.Fatalf("status = %q", status)
	}
}

func mustID(t *testing.T) string {
	t.Helper()
	value, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	return value
}
