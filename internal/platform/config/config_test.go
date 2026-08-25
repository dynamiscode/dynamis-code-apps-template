package config

import (
	"strings"
	"testing"
)

func TestLoadFromDefaultsToSQLite(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFrom(env(nil))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if cfg.Database.Driver != SQLite {
		t.Fatalf("Driver = %q, want %q", cfg.Database.Driver, SQLite)
	}
	if cfg.Database.SQLitePath != "data/app.db" {
		t.Fatalf("SQLitePath = %q, want data/app.db", cfg.Database.SQLitePath)
	}
	if cfg.Database.MaxOpenConns != 4 || cfg.Database.MaxIdleConns != 2 {
		t.Fatalf(
			"pool = %d/%d, want 4/2",
			cfg.Database.MaxOpenConns,
			cfg.Database.MaxIdleConns,
		)
	}
}

func TestLoadFromRequiresPostgresURLWithoutLeakingValue(t *testing.T) {
	t.Parallel()

	secret := "postgres://user:secret@example.invalid/app"
	_, err := LoadFrom(env(map[string]string{
		"DATABASE_DRIVER": "postgres",
		"DATABASE_URL":    secret,
		"SQLITE_PATH":     "ignored.db",
	}))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	_, err = LoadFrom(env(map[string]string{
		"DATABASE_DRIVER": "postgres",
		"DATABASE_URL":    "",
	}))
	if err == nil {
		t.Fatal("LoadFrom() error = nil, want validation error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked DATABASE_URL: %v", err)
	}
}

func TestLoadFromRejectsInvalidPool(t *testing.T) {
	t.Parallel()

	_, err := LoadFrom(env(map[string]string{
		"DATABASE_MAX_OPEN_CONNS": "1",
		"DATABASE_MAX_IDLE_CONNS": "2",
	}))
	if err == nil {
		t.Fatal("LoadFrom() error = nil, want validation error")
	}
}

func env(values map[string]string) LookupEnv {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
