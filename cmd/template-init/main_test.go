package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestEngagementExportWithoutFilesOmitsFileColumn(t *testing.T) {
	input := "SELECT agreement_recorded, agreement_file_id FROM engagements"
	got := disableFilesSource("internal/portability/engagement.go", []byte(input))
	if string(got) != "SELECT agreement_recorded, NULL FROM engagements" {
		t.Fatalf("generated query = %q", got)
	}
}

func TestTemporaryPlanIsExcluded(t *testing.T) {
	if !ignored("PLAN.md") {
		t.Fatal("temporary plan must not be copied into generated applications")
	}
}

func TestGenerateApplicationAndLock(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	output := filepath.Join(t.TempDir(), "my-app")
	generatedAt := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	err := run([]string{
		"-template-dir", root,
		"-output", output,
		"-name", "My Application",
		"-module", "example.com/acme/my-app",
		"-repository", "https://github.com/acme/my-app",
		"-security-url", "https://github.com/acme/my-app/security/advisories/new",
		"-maintainer", "@acme/platform",
		"-source", "https://example.com/acme/template",
		"-commit", strings.Repeat("a", 40),
		"-profiles", "Agent,Core,Identity",
	}, func() time.Time { return generatedAt })
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	module, err := os.ReadFile(filepath.Join(output, "go.mod"))
	if err != nil || !strings.Contains(string(module), "module example.com/acme/my-app") ||
		strings.Contains(string(module), "example.com/dynamis-code/apps-template") ||
		strings.Contains(string(module), "aws-sdk-go-v2") || strings.Contains(string(module), "smithy-go") {
		t.Fatalf("generated go.mod = %q, error = %v", module, err)
	}
	readme, err := os.ReadFile(filepath.Join(output, "README.md"))
	if err != nil || !strings.Contains(string(readme), "# My Application") {
		t.Fatalf("generated README = %q, error = %v", readme, err)
	}
	for path, want := range map[string]string{
		"AGENTS.md":                                                   "docs/ux-review.md",
		"docs/README.md":                                              "ux-review.md",
		"docs/ux.md":                                                  "User experience standard",
		"docs/ux-review.md":                                           "Browser UX review",
		".agents/skills/verify/SKILL.md":                              "docs/ux-review.md",
		".github/pull_request_template.md":                            "Browser UX review",
		".agents/skills/ux-workflow/SKILL.md":                         "UX workflow",
		".agents/skills/ux-workflow/references/journeys.md":           "Journeys and structure",
		".agents/skills/ux-workflow/references/interactions.md":       "Interactions and states",
		".agents/skills/ux-workflow/references/evaluation.md":         "Evaluate a browser journey",
		".agents/skills/ux-workflow/references/patterns.md":           "Choose interaction patterns",
		".agents/skills/ux-workflow/references/data-workflows.md":     "Lists, search, filters, and bulk work",
		".agents/skills/ux-workflow/references/visual-design.md":      "Visual design review",
		".agents/skills/ux-workflow/references/overlays-and-forms.md": "Dialogs, menus, onboarding, and forms",
		".agents/skills/ux-workflow/references/review-example.md":     "Worked review: follow-up queue",
	} {
		content, err := os.ReadFile(filepath.Join(output, path))
		if err != nil || !strings.Contains(string(content), want) {
			t.Errorf("generated %s lacks UX guidance %q: %v", path, want, err)
		}
	}
	web, err := os.ReadFile(filepath.Join(output, "docs/web.md"))
	if err != nil || !strings.Contains(string(web), "capabilities.md#shared-navigation-migration-evidence-434-2026-11-16") {
		t.Fatalf("generated web contract lacks navigation evidence link: %v", err)
	}
	capabilities, err := os.ReadFile(filepath.Join(output, "docs/capabilities.md"))
	if err != nil || !strings.Contains(string(capabilities), "## Shared navigation migration evidence (#434, 2026-11-16)") {
		t.Fatalf("generated no-Files capabilities lacks navigation evidence: %v", err)
	}
	for _, path := range []string{"internal/mcpserver"} {
		if _, err := os.Stat(filepath.Join(output, path)); err != nil {
			t.Fatalf("generated Agent path %s missing: %v", path, err)
		}
	}
	license, err := os.ReadFile(filepath.Join(output, "LICENSE"))
	if err != nil || !strings.Contains(string(license), "MIT License") ||
		!strings.Contains(string(license), "Copyright (c) 2026 My Application contributors") ||
		strings.Contains(string(license), "David Londono") {
		t.Fatalf("generated LICENSE = %q, error = %v", license, err)
	}
	for _, want := range []string{
		"generated from a verified template release",
		"https://github.com/acme/my-app/security/advisories/new",
		"## Purpose",
		"## Configuration",
		"## Architecture",
		"## Interfaces",
		"## Operations and data",
		"## Security",
		"[SUPPORT.md](SUPPORT.md)",
		"## Contributing and release",
		"## Add a product feature",
		"## Template provenance",
		"`example.com/acme/my-app`",
		"`my-app`",
	} {
		if !strings.Contains(string(readme), want) {
			t.Errorf("generated README lacks %q", want)
		}
	}
	if strings.Count(string(readme), "```") != 4 {
		t.Errorf("generated README has malformed code fences: %d", strings.Count(string(readme), "```"))
	}
	if strings.Contains(string(readme), "go run ./cmd/template-init") {
		t.Error("generated README still contains template-only generation instructions")
	}
	for path, want := range map[string]string{
		"package.json":                       `"name": "my-app-checks"`,
		"compose.yaml":                       "image: my-app:${VERSION:-dev}",
		"docs/configuration.md":              "`my-app`",
		"internal/i18n/locales/en.json":      `"common.product_name": "My Application"`,
		"internal/web/assets/tokens.css":     "--brand-accent:",
		"internal/mcpserver/server.go":       `Name: "my-app"`,
		"internal/platform/config/config.go": `"OTEL_SERVICE_NAME", "my-app"`,
		".github/workflows/ci.yml":           "image-ref: my-app:dev",
	} {
		content, readErr := os.ReadFile(filepath.Join(output, path))
		if readErr != nil || !strings.Contains(string(content), want) {
			t.Errorf("generated %s lacks %q: %v", path, want, readErr)
		}
	}
	packageJSON, err := os.ReadFile(filepath.Join(output, "package.json"))
	if err != nil || !strings.Contains(string(packageJSON), `"license": "MIT"`) {
		t.Errorf("generated package.json lacks MIT metadata: %q, error = %v", packageJSON, err)
	}
	var lock lockFile
	raw, err := os.ReadFile(filepath.Join(output, "template.lock"))
	if err != nil || json.Unmarshal(raw, &lock) != nil {
		t.Fatalf("template.lock = %q, error = %v", raw, err)
	}
	if lock.Template.Version != "0.1.0" || lock.Template.Source != "https://example.com/acme/template" ||
		lock.Template.Commit != strings.Repeat("a", 40) || !lock.GeneratedAt.Equal(generatedAt) ||
		strings.Join(lock.Profiles, ",") != "Core,Identity,Agent" {
		t.Fatalf("template.lock = %+v", lock)
	}
	for _, path := range []string{".github/CODEOWNERS", ".github/ISSUE_TEMPLATE/config.yml", "README.md", "SUPPORT.md", "SECURITY.md", "NOTICE", "docs/governance.md", "docs/accessibility.md", "docs/decisions/0001-go-modular-monolith.md"} {
		content, err := os.ReadFile(filepath.Join(output, path))
		if err != nil {
			t.Fatalf("read generated %s: %v", path, err)
		}
		if strings.Contains(string(content), templateRepositoryURL) || strings.Contains(string(content), templateSecurityURL) ||
			strings.Contains(string(content), "@davidlondono") || strings.Contains(string(content), "Dynamis Code") ||
			strings.Contains(string(content), "template maintainer") || strings.Contains(string(content), "pending project license") {
			t.Errorf("generated %s retains template metadata", path)
		}
	}
	support, err := os.ReadFile(filepath.Join(output, "SUPPORT.md"))
	if err != nil || !strings.Contains(string(support), "https://github.com/acme/my-app/issues") ||
		!strings.Contains(string(support), "https://github.com/acme/my-app/security/advisories/new") {
		t.Errorf("generated SUPPORT.md = %q, error = %v", support, err)
	}
	codeowners, err := os.ReadFile(filepath.Join(output, ".github/CODEOWNERS"))
	if err != nil || !strings.Contains(string(codeowners), "@acme/platform") {
		t.Errorf("generated CODEOWNERS = %q, error = %v", codeowners, err)
	}
	if err := run([]string{
		"-template-dir", root, "-output", output, "-name", "Overwrite",
		"-module", "example.com/acme/overwrite", "-source", "https://example.com/template",
		"-repository", "https://github.com/acme/overwrite", "-security-url", "https://github.com/acme/overwrite/security/advisories/new",
		"-maintainer", "@acme/platform", "-profiles", "Core,Identity", "-commit", strings.Repeat("b", 40),
	}, time.Now); err == nil {
		t.Fatal("second generation overwrote existing output")
	}
	if err := run([]string{
		"-template-dir", root, "-output", filepath.Join(t.TempDir(), "missing-profile"),
		"-name", "Missing Profile", "-module", "example.com/acme/missing-profile",
		"-repository", "https://github.com/acme/missing-profile",
		"-security-url", "https://github.com/acme/missing-profile/security/advisories/new",
		"-maintainer", "@acme/platform", "-source", "https://example.com/template",
		"-commit", strings.Repeat("c", 40),
	}, time.Now); err == nil {
		t.Fatal("generation accepted missing profile selection")
	}
	if _, err := parseProfiles("Core,Agent"); err == nil {
		t.Fatal("profile selection accepted Agent without Identity")
	}
	if _, err := parseProfiles("Core,Files"); err == nil {
		t.Fatal("profile selection accepted Files without Identity")
	}
	if _, err := parseProfiles("Identity"); err == nil {
		t.Fatal("profile selection accepted Identity without Core")
	}
}

func TestNavigationEvidenceLinkWithoutFiles(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	web, err := os.ReadFile(filepath.Join(root, "docs/web.md"))
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := os.ReadFile(filepath.Join(root, "docs/capabilities.md"))
	if err != nil {
		t.Fatal(err)
	}
	generatedWeb := string(disableFilesSource("docs/web.md", web))
	generatedCapabilities := string(disableFilesSource("docs/capabilities.md", capabilities))
	if !strings.Contains(generatedWeb, "capabilities.md#shared-navigation-migration-evidence-434-2026-11-16") ||
		!strings.Contains(generatedCapabilities, "## Shared navigation migration evidence (#434, 2026-11-16)") {
		t.Fatal("generated no-Files profile has a broken navigation evidence link")
	}
	if strings.Contains(generatedCapabilities, "## Files evidence") {
		t.Fatal("generated no-Files profile retains Files evidence")
	}
}

func TestGenerateWithoutFilesPrunesFilesTemplAndBuilds(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	output := filepath.Join(t.TempDir(), "without-files-app")
	err := run([]string{
		"-template-dir", root, "-output", output, "-name", "Without Files Application",
		"-module", "example.com/acme/without-files-app",
		"-repository", "https://github.com/acme/without-files-app",
		"-security-url", "https://github.com/acme/without-files-app/security/advisories/new",
		"-maintainer", "@acme/platform", "-profiles", "Core,Identity,Agent",
		"-source", "https://example.com/acme/template", "-commit", strings.Repeat("a", 40),
	}, time.Now)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	for _, path := range []string{
		"internal/web/files.templ", "internal/web/files_templ.go", "internal/web/templates/files.html", "docs/files-417-evidence.md", "internal/browsertest/files_test.go",
	} {
		if _, err := os.Stat(filepath.Join(output, path)); !os.IsNotExist(err) {
			t.Errorf("generated application without Files still has %s: %v", path, err)
		}
	}
	if err := runCommand(output, "go", "build", "./..."); err != nil {
		t.Fatalf("generated application without Files does not build: %v", err)
	}
}

func TestGenerateWithoutAgentPrunesAgentSurfaceAndBuilds(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	output := filepath.Join(t.TempDir(), "core-app")
	err := run([]string{
		"-template-dir", root, "-output", output, "-name", "Core Application",
		"-module", "example.com/acme/core-app",
		"-repository", "https://github.com/acme/core-app",
		"-security-url", "https://github.com/acme/core-app/security/advisories/new",
		"-maintainer", "@acme/platform", "-profiles", "Core,Identity",
		"-source", "https://example.com/acme/template", "-commit", strings.Repeat("d", 40),
	}, time.Now)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	for _, path := range []string{"cmd/appctl", "internal/appctl", "internal/mcpserver", "internal/bootstrap/agent_smoke_test.go"} {
		if _, err := os.Stat(filepath.Join(output, path)); !os.IsNotExist(err) {
			t.Errorf("generated Agent path %s still exists: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(output, "internal/identity")); err != nil {
		t.Fatalf("generated Identity package missing: %v", err)
	}
	settingsPages, err := os.ReadFile(filepath.Join(output, "internal/web/settings_pages.go"))
	if err != nil || !strings.Contains(string(settingsPages), "h.filesEnabled") ||
		!strings.Contains(string(settingsPages), "h.clientsEnabled") ||
		strings.Contains(string(settingsPages), "h.template.Lookup") ||
		strings.Contains(string(settingsPages), "h.files ") || strings.Contains(string(settingsPages), "h.files.") ||
		strings.Contains(string(settingsPages), "h.clients ") || strings.Contains(string(settingsPages), "h.clients.") {
		t.Fatalf("generated settings renderer must use stable optional-feature flags without optional handler fields: error = %v", err)
	}
	readme, err := os.ReadFile(filepath.Join(output, "README.md"))
	if err != nil || strings.Contains(string(readme), "MCP:") || strings.Contains(string(readme), "cmd/appctl") {
		t.Fatalf("generated README retains Agent interface: %q, error = %v", readme, err)
	}
	capabilities, err := os.ReadFile(filepath.Join(output, "docs/capabilities.md"))
	if err != nil || strings.Contains(string(capabilities), "| Files |") || strings.Contains(string(capabilities), "| Files shared list view |") || strings.Contains(string(capabilities), "Object storage") || strings.Contains(string(capabilities), "Files evidence") {
		t.Fatalf("generated capabilities retains Files evidence: %q, error = %v", capabilities, err)
	}
	for _, path := range []string{"go.mod", "go.sum", "NOTICE"} {
		content, err := os.ReadFile(filepath.Join(output, path))
		if err != nil || strings.Contains(string(content), "modelcontextprotocol") {
			t.Fatalf("generated %s retains Agent dependency: %q, error = %v", path, content, err)
		}
	}
	if err := runCommand(output, "go", "test", "./..."); err != nil {
		t.Fatalf("generated application tests: %v", err)
	}
}

func runCommand(directory, name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Dir = directory
	command.Env = make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "POSTGRES_TEST_URL=") {
			continue
		}
		command.Env = append(command.Env, value)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func TestParseProfilesClientsRequiresIdentityAndAgent(t *testing.T) {
	for _, value := range []string{"Core,Identity,Clients", "Core,Agent,Clients"} {
		_, err := parseProfiles(value)
		if err == nil || !strings.Contains(err.Error(), "Clients profile requires Identity and Agent") {
			t.Fatalf("parseProfiles(%q) error = %v, want Clients requirement refusal", value, err)
		}
	}
	for value, want := range map[string]string{
		"Core,Identity,Agent,Clients":       "Core,Identity,Agent,Clients",
		"Clients,Agent,Identity,Core":       "Core,Identity,Agent,Clients",
		"Core,Identity,Agent,Files,Clients": "Core,Identity,Agent,Files,Clients",
	} {
		got, err := parseProfiles(value)
		if err != nil || strings.Join(got, ",") != want {
			t.Fatalf("parseProfiles(%q) = %v, %v; want %s", value, got, err, want)
		}
	}
}

func TestGenerateWithClientsProfileRecordsLockAndBuilds(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	output := filepath.Join(t.TempDir(), "clients-app")
	err := run([]string{
		"-template-dir", root, "-output", output, "-name", "Clients Application",
		"-module", "example.com/acme/clients-app",
		"-repository", "https://github.com/acme/clients-app",
		"-security-url", "https://github.com/acme/clients-app/security/advisories/new",
		"-maintainer", "@acme/platform", "-profiles", "Core,Identity,Agent,Clients",
		"-source", "https://example.com/acme/template", "-commit", strings.Repeat("e", 40),
	}, time.Now)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	var lock lockFile
	raw, err := os.ReadFile(filepath.Join(output, "template.lock"))
	if err != nil || json.Unmarshal(raw, &lock) != nil || strings.Join(lock.Profiles, ",") != "Core,Identity,Agent,Clients" {
		t.Fatalf("template.lock = %q, error = %v", raw, err)
	}
	for _, path := range []string{
		"internal/files", "internal/web/files.go", "internal/web/files.templ", "internal/web/files_templ.go",
		"internal/web/templates/files.html", "internal/platform/database/migrations/000014_files.sql", "internal/web/agreement_files_test.go",
	} {
		if _, err := os.Stat(filepath.Join(output, path)); !os.IsNotExist(err) {
			t.Errorf("generated Clients-only application still has Files path %s: %v", path, err)
		}
	}
	assertClientProfileFeatures(t, output, false)
	if err := runCommand(output, "go", "test", "./..."); err != nil {
		t.Fatalf("generated Clients application tests: %v", err)
	}
}

func TestGenerateWithoutClientsPrunesClientsSurfaceAndBuilds(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	output := filepath.Join(t.TempDir(), "agent-app")
	err := run([]string{
		"-template-dir", root, "-output", output, "-name", "Agent Application",
		"-module", "example.com/acme/agent-app",
		"-repository", "https://github.com/acme/agent-app",
		"-security-url", "https://github.com/acme/agent-app/security/advisories/new",
		"-maintainer", "@acme/platform", "-profiles", "Core,Identity,Agent",
		"-source", "https://example.com/acme/template", "-commit", strings.Repeat("f", 40),
	}, time.Now)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	for _, path := range []string{
		"internal/clients", "internal/httpapi/clients.go", "internal/httpapi/client_timeline.go", "internal/httpapi/client_timeline_test.go", "internal/web/clients.go",
		"internal/intake", "internal/httpapi/intake.go", "internal/httpapi/intake_test.go",
		"internal/web/intake.go", "internal/web/intake.templ", "internal/web/intake_templ.go", "internal/web/intake_test.go",
		"internal/web/public_intake.go", "internal/web/public_intake.templ", "internal/web/public_intake_templ.go",
		"internal/platform/database/migrations/000042_public_intake.sql",
		"internal/engagement/engagement.go", "internal/engagement/assignee.go", "internal/engagement/next_action.go", "internal/engagement/quote.go",
		"internal/engagement/list.go", "internal/engagement/list_test.go", "internal/engagement/dashboard.go",
		"internal/web/engagement_dashboard_test.go",
		"internal/web/engagement_list.go", "internal/web/engagement_list.templ", "internal/web/engagement_list_templ.go",
		"internal/web/engagement_board.go", "internal/web/engagement_board.templ", "internal/web/engagement_board_templ.go",
		"internal/web/client_timeline.go", "internal/web/client_timeline_test.go",
		"internal/web/clients.templ", "internal/web/clients_templ.go",
		"internal/web/client_details.templ", "internal/web/client_details_templ.go",
		"internal/web/client_fields.templ", "internal/web/client_fields_templ.go",
		"internal/web/templates/clients.html", "internal/web/templates/client.html",
		"internal/platform/database/migrations/000022_clients.sql",
		"internal/platform/database/migrations/000043_client_intake_consent.sql",
		"internal/platform/database/migrations/000044_client_possible_duplicate.sql",
		"internal/platform/database/migrations/000024_client_events.sql",
		"internal/platform/database/migrations/000031_client_timeline.sql",
		"internal/platform/database/migrations/000033_engagements.sql", "internal/platform/database/migrations/000034_engagement_next_action.sql", "internal/platform/database/migrations/000035_engagement_quote.sql", "internal/platform/database/migrations/000036_engagement_assignee.sql",
		"internal/platform/database/migrations/000051_engagement_names.sql", "internal/platform/database/migrations/000052_engagement_action_history.sql", "internal/platform/database/migrate_engagement_names_test.go",
		"internal/platform/database/migrate_engagement_names_postgres_test.go", "internal/platform/database/migrations/000053_engagement_action_completion.sql", "internal/platform/database/migrations/000054_engagement_action_version.sql", "internal/platform/database/migrate_engagement_action_test.go",
		"internal/web/clients_events.go", "internal/web/clients_no_javascript_test.go",
		"internal/mcpserver/clients.go", "internal/mcpserver/clients_test.go",
		"internal/appctl/clients_test.go",
	} {
		if _, err := os.Stat(filepath.Join(output, path)); !os.IsNotExist(err) {
			t.Errorf("generated application without Clients still has %s: %v", path, err)
		}
	}
	// No migration left in the generated app creates the client event table.
	migrations, err := filepath.Glob(filepath.Join(output, "internal/platform/database/migrations/*.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatalf("generated migrations = %v, error = %v", migrations, err)
	}
	for _, migration := range migrations {
		content, err := os.ReadFile(migration)
		if err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"client_events", "client_timeline_entries"} {
			if strings.Contains(string(content), table) {
				t.Errorf("generated application without Clients keeps %s in %s", table, filepath.Base(migration))
			}
		}
	}
	// No CLI commands remain without the Clients profile.
	if _, err := os.Stat(filepath.Join(output, "internal/appctl")); !os.IsNotExist(err) {
		t.Fatalf("generated application without Clients retains appctl: %v", err)
	}
	migrationPath := filepath.Join(output, "internal/platform/database/migrations/000042_application.sql")
	if err := os.WriteFile(migrationPath, []byte("CREATE TABLE application_feature (id INTEGER PRIMARY KEY);\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	regression := `package database

import (
 "context"
 "path/filepath"
 "testing"
 "example.com/acme/agent-app/internal/platform/config"
)

func TestApplicationMigration42(t *testing.T) {
 ctx := context.Background()
 db, err := Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "app.db")})
 if err != nil { t.Fatal(err) }
 defer db.Close()
 for i := 0; i < 2; i++ {
  if err := Migrate(ctx, db, config.SQLite); err != nil { t.Fatal(err) }
 }
 var count int
 if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM application_feature").Scan(&count); err != nil { t.Fatal(err) }
 if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = 42").Scan(&count); err != nil || count != 1 { t.Fatalf("migration record count = %d, error = %v", count, err) }
}
`
	if err := os.WriteFile(filepath.Join(output, "internal/platform/database/application_migration_test.go"), []byte(regression), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(output, "go", "test", "./internal/platform/database", "-run", "^TestApplicationMigration42$"); err != nil {
		t.Fatalf("generated application migration 42: %v", err)
	}
	for _, path := range []string{"internal/bootstrap/agent.go", "internal/mcpserver/server_test.go"} {
		content, err := os.ReadFile(filepath.Join(output, path))
		if err != nil || strings.Contains(string(content), "clients") {
			t.Fatalf("generated %s retains Clients wiring: error = %v", path, err)
		}
	}
	openAPI, err := os.ReadFile(filepath.Join(output, "api/openapi.json"))
	if err != nil || strings.Contains(string(openAPI), "/clients") || strings.Contains(string(openAPI), "/intake/") {
		t.Fatalf("generated OpenAPI retains Clients paths: error = %v", err)
	}
	for _, key := range []string{`"engagementNextActions"`, `"engagementActionHistory"`, `"engagementStageHistory"`} {
		if strings.Contains(string(openAPI), key) {
			t.Fatalf("generated OpenAPI retains Clients export key %s", key)
		}
	}
	for path, retained := range map[string]string{
		"docs/api.md": "/clients", "docs/web.md": "/clients", "docs/capabilities.md": "| Clients |",
		"docs/mcp.md": "clients_",
	} {
		content, err := os.ReadFile(filepath.Join(output, path))
		if err != nil || strings.Contains(string(content), retained) {
			t.Fatalf("generated %s retains Clients documentation %q: error = %v", path, retained, err)
		}
	}
	capabilities, err := os.ReadFile(filepath.Join(output, "docs/capabilities.md"))
	if err != nil || strings.Contains(string(capabilities), "Client timeline") {
		t.Fatalf("generated capabilities retain Client timeline documentation: error = %v", err)
	}
	if err := runCommand(output, "go", "build", "./..."); err != nil {
		t.Fatalf("generated application without Clients does not build: %v", err)
	}
	// go build skips test files; vet compiles the MCP tests that the generation rewrote.
	if err := runCommand(output, "go", "vet", "./internal/mcpserver/", "./internal/bootstrap/"); err != nil {
		t.Fatalf("generated MCP tests without Clients do not compile: %v", err)
	}
	// The export ships in every app, so a Clients-off app must not query client tables
	// or wire a client exporter.
	for _, dir := range []string{"internal/portability", "internal/bootstrap"} {
		files, err := filepath.Glob(filepath.Join(output, dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			for _, retained := range []string{"client_fields", "client_field_options", "FROM clients", "WithClients(clientService)"} {
				if strings.Contains(string(raw), retained) {
					t.Errorf("generated application without Clients retains %q in %s", retained, file)
				}
			}
		}
	}
	if err := runCommand(output, "go", "test", "./..."); err != nil {
		t.Fatalf("generated application without Clients does not pass its tests: %v", err)
	}
}
