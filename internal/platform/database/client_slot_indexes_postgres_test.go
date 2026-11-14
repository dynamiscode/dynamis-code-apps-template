package database

import (
	"context"
	"fmt"
	"os"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestPostgresClientSlotIndexesExistForEverySlot(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	db, err := Open(ctx, config.Database{
		Driver: config.Postgres, URL: url, MaxOpenConns: 4, MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(ctx, db, config.Postgres); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'clients' AND indexname LIKE 'clients_custom_slot_%_idx'",
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != clientSlotCount {
		t.Fatalf("slot indexes = %d, want %d", count, clientSlotCount)
	}
	for slot := 1; slot <= clientSlotCount; slot++ {
		name := fmt.Sprintf("clients_custom_slot_%02d_idx", slot)
		var found int
		if err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'clients' AND indexname = $1", name,
		).Scan(&found); err != nil {
			t.Fatal(err)
		}
		if found != 1 {
			t.Fatalf("index %s missing", name)
		}
	}
}
