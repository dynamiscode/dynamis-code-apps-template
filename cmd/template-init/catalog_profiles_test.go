package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCatalogProfileRequiresIdentity(t *testing.T) {
	if _, err := parseProfiles("Core,Catalog"); err == nil {
		t.Fatal("Catalog without Identity must be refused")
	}
	if _, err := parseProfiles("Core,Identity,Catalog"); err != nil {
		t.Fatalf("Catalog with Identity: %v", err)
	}
}

func TestGenerateCatalogProfile(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	for _, selected := range []bool{false, true} {
		t.Run(map[bool]string{false: "omitted", true: "included"}[selected], func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "app")
			profiles := "Core,Identity"
			if selected {
				profiles += ",Catalog"
			}
			err := run([]string{"-template-dir", root, "-output", output, "-name", "App", "-module", "example.com/acme/app", "-repository", "https://github.com/acme/app", "-security-url", "https://github.com/acme/app/security", "-maintainer", "@acme", "-profiles", profiles, "-source", "https://example.com/template", "-commit", strings.Repeat("a", 40)}, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"internal/catalog/service.go", "internal/httpapi/catalog.go", "internal/web/catalog.go", "internal/web/catalog.templ"} {
				_, err := os.Stat(filepath.Join(output, path))
				if selected && err != nil || !selected && !os.IsNotExist(err) {
					t.Errorf("%s, selected=%v, err=%v", path, selected, err)
				}
			}
			for path, needle := range map[string]string{
				"api/openapi.json":            "/api/v1/workspaces/{workspaceId}/catalog",
				"internal/httpapi/handler.go": "h.listCatalog",
				"internal/web/handler.go":     "h.catalogList",
			} {
				raw, err := os.ReadFile(filepath.Join(output, path))
				if err != nil || strings.Contains(string(raw), needle) != selected {
					t.Errorf("%s catalog wiring present = %v, selected = %v, err = %v", path, strings.Contains(string(raw), needle), selected, err)
				}
			}
			if err := runCommand(output, "go", "test", "./..."); err != nil {
				t.Fatalf("generated application tests: %v", err)
			}
		})
	}
}
