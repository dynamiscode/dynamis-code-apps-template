package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version int64
	name    string
	sql     string
}

func Migrate(
	ctx context.Context,
	db *sql.DB,
	driver config.DatabaseDriver,
) error {
	return migrateFS(ctx, db, driver, migrationFiles)
}

func migrateFS(
	ctx context.Context,
	db *sql.DB,
	driver config.DatabaseDriver,
	source fs.FS,
) error {
	migrations, err := loadMigrations(source)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migrations: %w", err)
	}
	defer tx.Rollback()

	if driver == config.Postgres {
		if _, err := tx.ExecContext(
			ctx,
			"SELECT pg_advisory_xact_lock(1935767101)",
		); err != nil {
			return fmt.Errorf("lock migrations: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create migration history: %w", err)
	}

	applied, err := appliedVersions(ctx, tx)
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if applied[migration.version] {
			continue
		}
		if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.name, err)
		}
		if err := recordMigration(ctx, tx, driver, migration.version); err != nil {
			return fmt.Errorf("record migration %s: %w", migration.name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func loadMigrations(source fs.FS) ([]migration, error) {
	names, err := fs.Glob(source, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(names)

	migrations := make([]migration, 0, len(names))
	var previous int64
	for _, name := range names {
		prefix, _, ok := strings.Cut(path.Base(name), "_")
		if !ok {
			return nil, fmt.Errorf("migration %q must use VERSION_NAME.sql", name)
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil || version <= previous {
			return nil, fmt.Errorf("migration %q has invalid version", name)
		}
		contents, err := fs.ReadFile(source, name)
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", name, err)
		}
		migrations = append(migrations, migration{
			version: version,
			name:    name,
			sql:     string(contents),
		})
		previous = version
	}
	return migrations, nil
}

func appliedVersions(
	ctx context.Context,
	tx *sql.Tx,
) (map[int64]bool, error) {
	rows, err := tx.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read migration history: %w", err)
	}
	defer rows.Close()

	versions := make(map[int64]bool)
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan migration history: %w", err)
		}
		versions[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migration history: %w", err)
	}
	return versions, nil
}

func recordMigration(
	ctx context.Context,
	tx *sql.Tx,
	driver config.DatabaseDriver,
	version int64,
) error {
	query := "INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)"
	if driver == config.Postgres {
		query = "INSERT INTO schema_migrations (version, applied_at) VALUES ($1, $2)"
	}
	_, err := tx.ExecContext(
		ctx,
		query,
		version,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}
