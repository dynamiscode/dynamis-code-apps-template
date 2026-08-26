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
