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

func TestClientsOmissionKeepsSchedulingMembershipCleanup(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "internal", "identity", "service.go")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	output := string(disableClientsSource("internal/identity/service.go", raw))
	if strings.Contains(output, "UPDATE engagements SET assignee_id") || strings.Contains(output, "UPDATE engagement_next_actions SET owner_id") || !strings.Contains(output, "UPDATE scheduling_resources SET user_id = NULL") {
		t.Fatal("Clients omission must remove engagement cleanup and retain scheduling cleanup")
	}
}

func TestGenerateCatalogProfile(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	for _, tc := range []struct {
		name     string
		profiles string
		selected bool
		clients  bool
	}{{"omitted", "Core,Identity", false, false}, {"included", "Core,Identity,Catalog", true, false}, {"clients", "Core,Identity,Agent,Clients", false, true}, {"both", "Core,Identity,Agent,Clients,Catalog", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			selected := tc.selected
			output := filepath.Join(t.TempDir(), "app")
			err := run([]string{"-template-dir", root, "-output", output, "-name", "App", "-module", "example.com/acme/app", "-repository", "https://github.com/acme/app", "-security-url", "https://github.com/acme/app/security", "-maintainer", "@acme", "-profiles", tc.profiles, "-source", "https://example.com/template", "-commit", strings.Repeat("a", 40)}, time.Now)
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
			raw, err := os.ReadFile(filepath.Join(output, "api/openapi.json"))
			if err != nil || strings.Contains(string(raw), "/scheduling/resources/{resourceId}/bookings") != selected || !strings.Contains(string(raw), "/scheduling/resources/{resourceId}/working-hours") {
				t.Fatalf("booking profile or resource scheduling mismatch: selected=%v err=%v", selected, err)
			}
			if strings.Contains(string(raw), "/scheduling/bookings/{bookingId}/engagement") != (selected && tc.clients) {
				t.Fatalf("engagement route mismatch: catalog=%v clients=%v", selected, tc.clients)
			}
			for _, path := range []string{"internal/scheduling/move.go", "internal/platform/database/migrations/000048_scheduling_bookings.sql", "internal/platform/database/migrations/000050_scheduling_booking_moves.sql"} {
				_, err = os.Stat(filepath.Join(output, path))
				if selected && err != nil || !selected && !os.IsNotExist(err) {
					t.Fatalf("%s, selected=%v err=%v", path, selected, err)
				}
			}
			if _, err = os.Stat(filepath.Join(output, "internal/platform/database/migrations/000049_scheduling_engagement.sql")); selected && tc.clients && err != nil || !(selected && tc.clients) && !os.IsNotExist(err) {
				t.Fatalf("engagement migration, catalog=%v clients=%v: %v", selected, tc.clients, err)
			}
			if _, err = os.Stat(filepath.Join(output, "internal/scheduling/reminder.go")); selected && tc.clients && err != nil || !(selected && tc.clients) && !os.IsNotExist(err) {
				t.Fatalf("reminder source, catalog=%v clients=%v: %v", selected, tc.clients, err)
			}
			raw, err = os.ReadFile(filepath.Join(output, "internal/web/scheduling.templ"))
			if err != nil || strings.Contains(string(raw), `data.T("scheduling.bookings")`) != selected || !strings.Contains(string(raw), `data.T("scheduling.hours")`) {
				t.Fatalf("booking controls or working hours mismatch: selected=%v err=%v", selected, err)
			}
			if err := runCommand(output, "go", "test", "./..."); err != nil {
				t.Fatalf("generated application tests: %v", err)
			}
		})
	}
}
