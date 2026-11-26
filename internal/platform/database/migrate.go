package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/id"
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
		if migration.version == 15 {
			mfaApplied, err := mfaMigrationAlreadyApplied(ctx, tx, driver)
			if err != nil {
				return fmt.Errorf("check MFA migration compatibility: %w", err)
			}
			if mfaApplied {
				if err := recordMigration(ctx, tx, driver, migration.version); err != nil {
					return fmt.Errorf("record migration %s: %w", migration.name, err)
				}
				continue
			}
		}
		if _, err := tx.ExecContext(ctx, renderMigration(migration.sql, driver)); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.name, err)
		}
		if migration.version == 51 {
			if err := backfillEngagementNames(ctx, tx, driver); err != nil {
				return fmt.Errorf("backfill engagement names: %w", err)
			}
		}
		if migration.version == 63 {
			if err := backfillWorkspaceCurrencies(ctx, tx, applied); err != nil {
				return fmt.Errorf("backfill workspace currencies: %w", err)
			}
		}
		if err := recordMigration(ctx, tx, driver, migration.version); err != nil {
			return fmt.Errorf("record migration %s: %w", migration.name, err)
		}
		applied[migration.version] = true
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func backfillWorkspaceCurrencies(ctx context.Context, tx *sql.Tx, applied map[int64]bool) error {
	if applied[45] {
		if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_currencies (workspace_id, code, decimals, is_default)
			SELECT workspace_id, code, decimals, is_default FROM catalog_currencies`); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO workspace_currencies (workspace_id, code, decimals, is_default)
		SELECT id, 'USD', 2, TRUE FROM workspaces w
		WHERE NOT EXISTS (SELECT 1 FROM workspace_currencies c WHERE c.workspace_id = w.id AND c.is_default = TRUE)
		ON CONFLICT (workspace_id, code) DO UPDATE SET is_default = TRUE`)
	return err
}

func backfillEngagementNames(ctx context.Context, tx *sql.Tx, driver config.DatabaseDriver) error {
	rows, err := tx.QueryContext(ctx, `SELECT e.id, c.display_name FROM engagements e JOIN clients c ON c.workspace_id = e.workspace_id AND c.id = e.client_id`)
	if err != nil {
		return err
	}
	type entry struct {
		key  int64
		name string
	}
	var entries []entry
	for rows.Next() {
		var value entry
		if err := rows.Scan(&value.key, &value.name); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, value := range entries {
		name := value.name + " — " + id.EncodeKey(id.TypeEngagement, value.key)
		if _, err := tx.ExecContext(ctx, Rebind(driver, `UPDATE engagements SET name = ? WHERE id = ?`), name, value.key); err != nil {
			return err
		}
	}
	return nil
}

// renderMigration fills the dialect-specific primary-key fragments in the shared
// migrations (workspace and file keys share one integer shape). SQLite needs INTEGER PRIMARY KEY for a rowid alias; PostgreSQL
// needs an identity column, which SQLite cannot parse.
func renderMigration(sql string, driver config.DatabaseDriver) string {
	pk := "INTEGER PRIMARY KEY"
	// Files never reuse a key: SQLite needs AUTOINCREMENT, or a deleted file's
	// short ID would resolve to the next file (PostgreSQL identities never reuse).
	filePK := pk
	if driver == config.Postgres {
		pk = "BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY"
		filePK = pk
	} else {
		filePK = "INTEGER PRIMARY KEY AUTOINCREMENT"
	}
	sql = strings.ReplaceAll(sql, "{{integer_pk}}", pk)
	sql = strings.ReplaceAll(sql, "{{file_id_pk}}", filePK)
	override, reset := "", ""
	if driver == config.Postgres {
		override = "OVERRIDING SYSTEM VALUE"
		reset = `SELECT setval(pg_get_serial_sequence('engagement_action_history_new', 'id'), GREATEST(COALESCE((SELECT MAX(id) FROM engagement_action_history), 0), (SELECT last_value FROM engagement_action_history_id_seq)), true);`
	}
	sql = strings.ReplaceAll(sql, "{{history_identity_override}}", override)
	sql = strings.ReplaceAll(sql, "{{history_identity_reset}}", reset)
	transactionsReset := ""
	if driver == config.Postgres {
		transactionsReset = `SELECT setval(pg_get_serial_sequence('transactions_entries_next', 'id'), GREATEST(COALESCE((SELECT MAX(id) FROM transactions_entries), 0), (SELECT last_value FROM transactions_entries_id_seq)), true);`
	}
	sql = strings.ReplaceAll(sql, "{{transactions_identity_reset}}", transactionsReset)
	clientRevisionTrigger := `CREATE TRIGGER transactions_client_revision AFTER UPDATE OF client_id ON transactions_entries
		WHEN OLD.client_id IS NOT NULL AND NEW.client_id IS NULL AND NEW.revision = OLD.revision
		BEGIN UPDATE transactions_entries SET revision = revision + 1 WHERE id = NEW.id; END;`
	if driver == config.Postgres {
		clientRevisionTrigger = `CREATE FUNCTION transactions_client_revision_fn() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF OLD.client_id IS NOT NULL AND NEW.client_id IS NULL AND NEW.revision = OLD.revision THEN
				UPDATE transactions_entries SET revision = revision + 1 WHERE id = NEW.id;
			END IF;
			RETURN NEW;
		END;
		$$;
		CREATE TRIGGER transactions_client_revision AFTER UPDATE OF client_id ON transactions_entries
		FOR EACH ROW EXECUTE FUNCTION transactions_client_revision_fn();`
	}
	sql = strings.ReplaceAll(sql, "{{transactions_client_revision_trigger}}", clientRevisionTrigger)
	return renderJSONTextExpr(sql, driver)
}

// jsonTextExprPattern matches {{json_text_expr:COLUMN:KEY}}, the only JSON
// placeholder. It names a text column and a top-level key (ADR 0012 slot keys).
var jsonTextExprPattern = regexp.MustCompile(`\{\{json_text_expr:([a-z_]+):([a-z0-9_]+)\}\}`)

// renderJSONTextExpr turns a top-level JSON text value into its dialect's
// expression, which migrations use to index one custom-field slot.
func renderJSONTextExpr(sql string, driver config.DatabaseDriver) string {
	return jsonTextExprPattern.ReplaceAllStringFunc(sql, func(match string) string {
		parts := jsonTextExprPattern.FindStringSubmatch(match)
		return JSONTextExpr(driver, parts[1], parts[2])
	})
}

// JSONTextExpr returns the dialect expression for a top-level text key of a JSON
// text column. Migrations index slots with it, and a query that must use those
// indexes repeats it exactly. Callers pass constant column and key names.
func JSONTextExpr(driver config.DatabaseDriver, column, key string) string {
	if driver == config.Postgres {
		return fmt.Sprintf("%s::jsonb ->> '%s'", column, key)
	}
	return fmt.Sprintf("json_extract(%s, '$.%s')", column, key)
}

func mfaMigrationAlreadyApplied(
	ctx context.Context,
	tx *sql.Tx,
	driver config.DatabaseDriver,
) (bool, error) {
	query := `
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'mfa_challenges'
	`
	if driver == config.Postgres {
		query = `
			SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_name = 'mfa_challenges'
		`
	}
	var count int
	if err := tx.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func loadMigrations(source fs.FS) ([]migration, error) {
	names, err := fs.Glob(source, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	slices.Sort(names)

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
