package bootstrap

import (
	"context"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestNewBuildsSQLiteApplication(t *testing.T) {
	t.Parallel()

	app, err := New(context.Background(), config.Config{
		Database: config.Database{
			Driver:       config.SQLite,
			SQLitePath:   ":memory:",
			MaxOpenConns: 1,
			MaxIdleConns: 1,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { app.Close() })

	if err := app.DB.PingContext(context.Background()); err != nil {
		t.Fatalf("PingContext() error = %v", err)
	}
}
