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
		strings.Contains(string(module), "example.com/dynamis-code/apps-template") {
		t.Fatalf("generated go.mod = %q, error = %v", module, err)
	}
	readme, err := os.ReadFile(filepath.Join(output, "README.md"))
	if err != nil || !strings.Contains(string(readme), "# My Application") {
		t.Fatalf("generated README = %q, error = %v", readme, err)
	}
	for _, path := range []string{"cmd/appctl", "internal/appctl", "internal/mcpserver"} {
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
		"## Replace the sample feature",
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
	readme, err := os.ReadFile(filepath.Join(output, "README.md"))
	if err != nil || strings.Contains(string(readme), "MCP:") || strings.Contains(string(readme), "cmd/appctl") {
		t.Fatalf("generated README retains Agent interface: %q, error = %v", readme, err)
	}
	capabilities, err := os.ReadFile(filepath.Join(output, "docs/capabilities.md"))
	if err != nil || strings.Contains(string(capabilities), "| Files |") || strings.Contains(string(capabilities), "Object storage") || strings.Contains(string(capabilities), "Phase 09") {
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
