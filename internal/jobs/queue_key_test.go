package jobs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// A deduplication key names an entity row, so only a positive integer key is valid.
func TestEnqueueRejectsNonPositiveDedupKey(t *testing.T) {
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
	defer tx.Rollback()
	for _, key := range []int64{0, -1} {
		if err := queue.EnqueueTx(ctx, tx, workspaceID, "keys", key, `{"value":1}`, time.Now()); err == nil {
			t.Fatalf("EnqueueTx(key %d) error = nil; want rejection", key)
		}
	}
}
