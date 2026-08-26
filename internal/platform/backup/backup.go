package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

const manifestVersion = 1

var (
	ErrCorrupt = errors.New("backup checksum does not match")
	ErrStale   = errors.New("backup evidence is stale")
)

type Manifest struct {
	Version   int                   `json:"version"`
	Driver    config.DatabaseDriver `json:"driver"`
	CreatedAt time.Time             `json:"createdAt"`
	SHA256    string                `json:"sha256"`
}

func Create(
	ctx context.Context,
	db *sql.DB,
	cfg config.Database,
	output string,
	now time.Time,
) (Manifest, error) {
	if err := ensureNewOutput(output); err != nil {
		return Manifest{}, err
	}
	if err := ensureNewOutput(manifestPath(output)); err != nil {
		return Manifest{}, err
	}
	var err error
	switch cfg.Driver {
	case config.SQLite:
		_, err = db.ExecContext(ctx, "VACUUM INTO ?", output)
	case config.Postgres:
		err = runPostgres(ctx, cfg.URL, "pg_dump",
			"--format=custom", "--no-owner", "--no-privileges", "--file", output)
	default:
		err = fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
	if err != nil {
		_ = os.Remove(output)
		return Manifest{}, fmt.Errorf("create %s backup: %w", cfg.Driver, err)
	}
	if err := os.Chmod(output, 0o600); err != nil {
		_ = os.Remove(output)
		return Manifest{}, err
	}
	digest, err := checksum(output)
	if err != nil {
		_ = os.Remove(output)
		return Manifest{}, err
	}
	manifest := Manifest{
		Version: manifestVersion, Driver: cfg.Driver,
		CreatedAt: now.UTC(), SHA256: digest,
	}
	if err := writeManifest(output, manifest); err != nil {
		_ = os.Remove(output)
		return Manifest{}, err
	}
	return manifest, nil
}

func Verify(
	backupPath string,
	driver config.DatabaseDriver,
	now time.Time,
	maxAge time.Duration,
) (Manifest, error) {
	raw, err := os.ReadFile(manifestPath(backupPath))
	if err != nil {
		return Manifest{}, fmt.Errorf("read backup manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil ||
		manifest.Version != manifestVersion || manifest.Driver != driver ||
		manifest.CreatedAt.IsZero() || len(manifest.SHA256) != sha256.Size*2 {
		return Manifest{}, errors.New("backup manifest is invalid")
	}
	if manifest.CreatedAt.After(now.UTC().Add(5*time.Minute)) ||
		(maxAge > 0 && now.UTC().Sub(manifest.CreatedAt) > maxAge) {
		return Manifest{}, ErrStale
	}
	digest, err := checksum(backupPath)
	if err != nil {
		return Manifest{}, err
	}
	if !strings.EqualFold(digest, manifest.SHA256) {
		return Manifest{}, ErrCorrupt
	}
	return manifest, nil
}

func Restore(
	ctx context.Context,
	cfg config.Database,
	backupPath string,
	sqliteTarget string,
	now time.Time,
	maxAge time.Duration,
) error {
	if _, err := Verify(backupPath, cfg.Driver, now, maxAge); err != nil {
		return err
	}
	switch cfg.Driver {
	case config.SQLite:
		if err := ensureNewOutput(sqliteTarget); err != nil {
			return err
		}
		if err := copyFile(backupPath, sqliteTarget); err != nil {
			return err
		}
		db, err := database.Open(ctx, config.Database{
			Driver: config.SQLite, SQLitePath: sqliteTarget,
			MaxOpenConns: 1, MaxIdleConns: 1,
		})
		if err != nil {
			_ = os.Remove(sqliteTarget)
			return err
		}
		var result string
		if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil || result != "ok" {
			db.Close()
			_ = os.Remove(sqliteTarget)
			return errors.New("restored SQLite database failed integrity check")
		}
		err = verifyMigrations(ctx, db, config.SQLite)
		closeErr := db.Close()
		if err != nil {
			_ = os.Remove(sqliteTarget)
		}
		return errors.Join(err, closeErr)
	case config.Postgres:
		db, err := database.Open(ctx, cfg)
		if err != nil {
			return err
		}
		var tables int
		err = db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'",
		).Scan(&tables)
		db.Close()
		if err != nil {
			return err
		}
		if tables != 0 {
			return errors.New("PostgreSQL restore target must be empty")
		}
		if err := runPostgres(ctx, cfg.URL, "pg_restore",
			"--exit-on-error", "--no-owner", "--no-privileges", "--dbname", postgresDatabase(cfg.URL), backupPath); err != nil {
			return fmt.Errorf("restore PostgreSQL backup: %w", err)
		}
		db, err = database.Open(ctx, cfg)
		if err != nil {
			return err
		}
		defer db.Close()
		return verifyMigrations(ctx, db, config.Postgres)
	default:
		return fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
}

func ensureNewOutput(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("backup path must not be empty")
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("path already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.MkdirAll(filepath.Dir(path), 0o750)
}

func checksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeManifest(backupPath string, manifest Manifest) error {
	file, err := os.OpenFile(manifestPath(backupPath), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create backup manifest: %w", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(manifest)
}

func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(target)
		return err
	}
	return out.Close()
}

func runPostgres(ctx context.Context, rawURL, executable string, args ...string) error {
	cfg, err := pgx.ParseConfig(rawURL)
	if err != nil {
		return errors.New("DATABASE_URL is invalid")
	}
	command := exec.CommandContext(ctx, executable, args...)
	environment := make([]string, 0, len(os.Environ())+7)
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if key != "DATABASE_URL" && !strings.HasPrefix(key, "PG") {
			environment = append(environment, value)
		}
	}
	environment = append(environment,
		"PGHOST="+cfg.Host,
		"PGPORT="+strconv.Itoa(int(cfg.Port)),
		"PGUSER="+cfg.User,
		"PGPASSWORD="+cfg.Password,
		"PGDATABASE="+cfg.Database,
	)
	if mode := cfg.RuntimeParams["sslmode"]; mode != "" {
		environment = append(environment, "PGSSLMODE="+mode)
	}
	command.Env = environment
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s failed", executable)
	}
	return nil
}

func postgresDatabase(rawURL string) string {
	cfg, err := pgx.ParseConfig(rawURL)
	if err != nil {
		return ""
	}
	return cfg.Database
}

func verifyMigrations(ctx context.Context, db *sql.DB, driver config.DatabaseDriver) error {
	var count int
	query := database.Rebind(driver, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?")
	if err := db.QueryRowContext(ctx, query, 1).Scan(&count); err != nil || count != 1 {
		return errors.New("restored database has no valid migration history")
	}
	return nil
}

func manifestPath(backupPath string) string {
	return backupPath + ".manifest.json"
}
