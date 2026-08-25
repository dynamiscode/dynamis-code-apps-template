package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"example.com/dynamis-code/apps-template/internal/platform/config"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, cfg config.Database) (*sql.DB, error) {
	db, err := open(cfg)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)

	if cfg.Driver == config.SQLite {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		if err := configureSQLite(ctx, db); err != nil {
			db.Close()
			return nil, err
		}
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to %s database: %w", cfg.Driver, err)
	}

	return db, nil
}

func Rebind(driver config.DatabaseDriver, query string) string {
	if driver != config.Postgres {
		return query
	}
	var result strings.Builder
	index := 1
	for _, character := range query {
		if character == '?' {
			fmt.Fprintf(&result, "$%d", index)
			index++
			continue
		}
		result.WriteRune(character)
	}
	return result.String()
}

func open(cfg config.Database) (*sql.DB, error) {
	switch cfg.Driver {
	case config.SQLite:
		if cfg.SQLitePath != ":memory:" {
			directory := filepath.Dir(cfg.SQLitePath)
			if err := os.MkdirAll(directory, 0o750); err != nil {
				return nil, fmt.Errorf("create SQLite directory: %w", err)
			}
		}
		db, err := sql.Open("sqlite", cfg.SQLitePath)
		if err != nil {
			return nil, fmt.Errorf("initialize SQLite driver: %w", err)
		}
		return db, nil
	case config.Postgres:
		parsed, err := pgx.ParseConfig(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("DATABASE_URL is invalid")
		}
		return stdlib.OpenDB(*parsed), nil
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
}

func configureSQLite(ctx context.Context, db *sql.DB) error {
	statements := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure SQLite: %w", err)
		}
	}
	return nil
}
