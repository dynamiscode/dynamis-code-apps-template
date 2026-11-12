package database

import (
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestRenderMigrationChoosesWorkspacePrimaryKeyPerDriver(t *testing.T) {
	const source = "CREATE TABLE workspaces (\n    id {{workspace_id_pk}},\n    name TEXT NOT NULL\n);"

	sqlite := renderMigration(source, config.SQLite)
	if !strings.Contains(sqlite, "id INTEGER PRIMARY KEY,") {
		t.Fatalf("SQLite render = %q", sqlite)
	}
	postgres := renderMigration(source, config.Postgres)
	if !strings.Contains(postgres, "id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,") {
		t.Fatalf("PostgreSQL render = %q", postgres)
	}
}

func TestRenderMigrationChoosesItemPrimaryKeyPerDriver(t *testing.T) {
	const source = "CREATE TABLE items (\n    id {{item_id_pk}},\n    title TEXT NOT NULL\n);"

	if sqlite := renderMigration(source, config.SQLite); !strings.Contains(sqlite, "id INTEGER PRIMARY KEY,") {
		t.Fatalf("SQLite render = %q", sqlite)
	}
	if postgres := renderMigration(source, config.Postgres); !strings.Contains(postgres, "id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,") {
		t.Fatalf("PostgreSQL render = %q", postgres)
	}
}

func TestEmbeddedMigrationsUseIntegerWorkspaceKeys(t *testing.T) {
	for _, driver := range []config.DatabaseDriver{config.SQLite, config.Postgres} {
		entries, err := loadMigrations(migrationFiles)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			rendered := renderMigration(entry.sql, driver)
			if strings.Contains(rendered, "{{") {
				t.Fatalf("%s leaves a placeholder for %v", entry.name, driver)
			}
			if strings.Contains(rendered, "workspace_id TEXT") {
				t.Fatalf("%s still declares a text workspace key for %v", entry.name, driver)
			}
		}
	}
}
