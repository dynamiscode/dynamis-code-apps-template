package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGenerateWithClientsAndFilesProfilesBuilds(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	output := filepath.Join(t.TempDir(), "clients-files-app")
	err := run([]string{
		"-template-dir", root, "-output", output, "-name", "Clients and Files Application",
		"-module", "example.com/acme/clients-files-app",
		"-repository", "https://github.com/acme/clients-files-app",
		"-security-url", "https://github.com/acme/clients-files-app/security/advisories/new",
		"-maintainer", "@acme/platform", "-profiles", "Core,Identity,Agent,Files,Clients",
		"-source", "https://example.com/acme/template", "-commit", strings.Repeat("a", 40),
	}, time.Now)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	for _, path := range []string{
		"internal/files", "internal/web/files.go",
		"internal/platform/database/migrations/000014_files.sql",
	} {
		if _, err := os.Stat(filepath.Join(output, path)); err != nil {
			t.Errorf("generated Clients+Files application is missing %s: %v", path, err)
		}
	}
	filesTempl := filepath.Join(output, "internal/web/files.templ")
	legacyFilesTemplate := filepath.Join(output, "internal/web/templates/files.html")
	if _, templErr := os.Stat(filesTempl); templErr != nil {
		if _, legacyErr := os.Stat(legacyFilesTemplate); legacyErr != nil {
			t.Fatalf("generated Clients+Files application has no Files view: templ error = %v; legacy template error = %v", templErr, legacyErr)
		}
	}
	assertClientProfileFeatures(t, output, true)
	if err := runCommand(output, "go", "test", "./..."); err != nil {
		t.Fatalf("generated Clients+Files application tests: %v", err)
	}
}

func assertClientProfileFeatures(t *testing.T, output string, withFiles bool) {
	t.Helper()
	clientsRenderer, err := os.ReadFile(filepath.Join(output, "internal/web/clients.go"))
	if err != nil {
		t.Fatalf("read generated Clients renderer: %v", err)
	}
	renderer := string(clientsRenderer)
	for _, required := range []string{
		"data.FilesEnabled = h.filesEnabled",
		"data.ClientsEnabled = h.clientsEnabled",
	} {
		if !strings.Contains(renderer, required) {
			t.Errorf("generated Clients renderer lacks stable feature flag %q", required)
		}
	}
	for _, forbidden := range []string{
		"h.template.Lookup(", "data.FilesEnabled = h.files != nil", "data.ClientsEnabled = h.clients != nil",
	} {
		if strings.Contains(renderer, forbidden) {
			t.Errorf("generated Clients renderer depends on optional feature signal %q", forbidden)
		}
	}
	bootstrap, err := os.ReadFile(filepath.Join(output, "internal/bootstrap/app.go"))
	if err != nil {
		t.Fatalf("read generated bootstrap registration: %v", err)
	}
	registration := string(bootstrap)
	if !strings.Contains(registration, "webHandler.EnableClients(") {
		t.Error("generated Clients profile lacks EnableClients registration")
	}
	hasFilesRegistration := strings.Contains(registration, "webHandler.EnableFilesPreferences(")
	if hasFilesRegistration != withFiles {
		t.Errorf("generated Files registration = %t, want %t", hasFilesRegistration, withFiles)
	}
}
