package portability

import (
	"context"
	"os"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestPostgresWorkspaceExport(t *testing.T) {
	databaseURL := os.Getenv("POSTGRES_TEST_URL")
	if databaseURL == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	db, err := database.Open(context.Background(), config.Database{
		Driver: config.Postgres, URL: databaseURL, MaxOpenConns: 4, MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(context.Background(), db, config.Postgres); err != nil {
		t.Fatal(err)
	}
	runExportContract(t, db, config.Postgres)
}
