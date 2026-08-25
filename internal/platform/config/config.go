package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type DatabaseDriver string

const (
	SQLite   DatabaseDriver = "sqlite"
	Postgres DatabaseDriver = "postgres"
)

type Database struct {
	Driver       DatabaseDriver
	SQLitePath   string
	URL          string
	MaxOpenConns int
	MaxIdleConns int
}

type Config struct {
	Database Database
}

type LookupEnv func(string) (string, bool)

func Load() (Config, error) {
	return LoadFrom(os.LookupEnv)
}

func LoadFrom(lookup LookupEnv) (Config, error) {
	driver := DatabaseDriver(valueOrDefault(lookup, "DATABASE_DRIVER", string(SQLite)))
	if driver != SQLite && driver != Postgres {
		return Config{}, fmt.Errorf(
			"DATABASE_DRIVER must be %q or %q",
			SQLite,
			Postgres,
		)
	}

	maxOpen, err := positiveInt(lookup, "DATABASE_MAX_OPEN_CONNS", 4)
	if err != nil {
		return Config{}, err
	}
	maxIdle, err := positiveInt(lookup, "DATABASE_MAX_IDLE_CONNS", 2)
	if err != nil {
		return Config{}, err
	}
	if maxIdle > maxOpen {
		return Config{}, fmt.Errorf(
			"DATABASE_MAX_IDLE_CONNS must not exceed DATABASE_MAX_OPEN_CONNS",
		)
	}

	database := Database{
		Driver:       driver,
		MaxOpenConns: maxOpen,
		MaxIdleConns: maxIdle,
	}

	if driver == SQLite {
		database.SQLitePath = valueOrDefault(lookup, "SQLITE_PATH", "data/app.db")
		if strings.TrimSpace(database.SQLitePath) == "" {
			return Config{}, fmt.Errorf("SQLITE_PATH must not be empty")
		}
		return Config{Database: database}, nil
	}

	database.URL = valueOrDefault(lookup, "DATABASE_URL", "")
	if strings.TrimSpace(database.URL) == "" {
		return Config{}, fmt.Errorf(
			"DATABASE_URL is required when DATABASE_DRIVER is %q",
			Postgres,
		)
	}

	return Config{Database: database}, nil
}

func valueOrDefault(lookup LookupEnv, key, fallback string) string {
	value, ok := lookup(key)
	if !ok {
		return fallback
	}
	return value
}

func positiveInt(lookup LookupEnv, key string, fallback int) (int, error) {
	raw, ok := lookup(key)
	if !ok {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 64 {
		return 0, fmt.Errorf("%s must be an integer from 1 to 64", key)
	}
	return value, nil
}
