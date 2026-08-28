package main

import (
	"encoding/json"
	"os"
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
		"-profiles", "Agent,Core",
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
		strings.Join(lock.Profiles, ",") != "Core,Agent" {
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
		"-maintainer", "@acme/platform", "-profiles", "Core", "-commit", strings.Repeat("b", 40),
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
}
