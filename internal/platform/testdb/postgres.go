package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func Postgres(t *testing.T) *sql.DB {
	t.Helper()
	baseURL := os.Getenv("POSTGRES_TEST_URL")
	if baseURL == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	admin, err := database.Open(ctx, config.Database{Driver: config.Postgres, URL: baseURL, MaxOpenConns: 2, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "test_" + hex.EncodeToString(suffix[:])
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	db, err := database.Open(ctx, config.Database{Driver: config.Postgres, URL: parsed.String(), MaxOpenConns: 4, MaxIdleConns: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.Postgres); err != nil {
		t.Fatal(err)
	}
	return db
}
