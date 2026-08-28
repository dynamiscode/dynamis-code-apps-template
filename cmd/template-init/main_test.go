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
		"-source", "https://example.com/acme/template",
		"-commit", strings.Repeat("a", 40),
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
	for _, want := range []string{
		"generated from **Dynamis Code Apps Template**",
		"## Purpose",
		"## Configuration",
		"## Architecture",
		"## Interfaces",
		"## Operations and data",
		"## Security",
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
	if err := run([]string{
		"-template-dir", root, "-output", output, "-name", "Overwrite",
		"-module", "example.com/acme/overwrite", "-source", "https://example.com/template",
		"-commit", strings.Repeat("b", 40),
	}, time.Now); err == nil {
		t.Fatal("second generation overwrote existing output")
	}
}
