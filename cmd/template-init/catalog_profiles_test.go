package main

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
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

func TestClientsOmissionPrunesEngagementDetailImports(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	for _, tc := range []struct {
		path, kept string
	}{
		{"internal/web/engagement.templ", "templ engagementStagesPage("},
		{"internal/web/engagement_templ.go", "func engagementStagesPage("},
	} {
		t.Run(tc.path, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(root, tc.path))
			if err != nil {
				t.Fatal(err)
			}
			output := string(disableClientsSource(tc.path, source))
			if strings.Contains(output, "actionattrs") {
				t.Error("catalog-independent engagement stages retain detail-only actionattrs")
			}
			if !strings.Contains(output, tc.kept) {
				t.Error("engagement stages were removed")
			}
		})
	}
}

func TestClientsOmissionRetainsBookingWebTests(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "internal", "web", "scheduling_test.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	output := string(disableClientsSource("internal/web/scheduling_test.go", source))
	for _, removed := range []string{"engagementPage", "engagementDetailPath", "engagementPath", "eng.ID.String()"} {
		if strings.Contains(output, removed) {
			t.Errorf("Clients-off scheduling test retains %q", removed)
		}
	}
	for _, kept := range []string{"func TestSchedulingBookingWeb(", `"missing move form"`, `"DST gap move draft:`, `"outcome:`, `"terminal:`} {
		if !strings.Contains(output, kept) {
			t.Errorf("Clients-off scheduling test lost %q", kept)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), path, output, 0); err != nil {
		t.Fatalf("Clients-off scheduling test: %v", err)
	}
}

func TestCatalogOmissionPrunesEngagementBookings(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	for _, tc := range []struct {
		path string
		kept []string
	}{
		{"internal/web/clients.go", []string{"ownerError := false", "Engagements:      engagements", "EngagementStages: stages"}},
		{"internal/web/engagement.templ", []string{"engagement-quote-heading", "activity-heading"}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(root, tc.path))
			if err != nil {
				t.Fatal(err)
			}
			output := string(disableCatalogSource(tc.path, source))
			for _, removed := range []string{"engagementBookings", "EngagementBookings", "h.scheduling.Schedule"} {
				if strings.Contains(output, removed) {
					t.Errorf("catalog-free output contains %q", removed)
				}
			}
			for _, kept := range tc.kept {
				if !strings.Contains(output, kept) {
					t.Errorf("catalog-free output lost %q", kept)
				}
			}
			if strings.HasSuffix(tc.path, ".go") {
				if _, err := parser.ParseFile(token.NewFileSet(), tc.path, output, 0); err != nil {
					t.Fatalf("catalog-free Go source: %v", err)
				}
			}
		})
	}
}

func TestCopyTemplatePrunesActionExceptions(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		"internal/web/catalog.templ",
		"internal/web/engagement_list.templ",
		"internal/web/engagement.templ",
		"internal/web/components/molecules/list_column_heading.templ",
	}
	for _, path := range append(paths, "scripts/actioncheck/exceptions.json") {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(path)), 0o755); err != nil {
			t.Fatal(err)
		}
		content := []byte("template")
		if path == "scripts/actioncheck/exceptions.json" {
			content = []byte(`[{"Path":"internal/web/catalog.templ","Reason":"catalog"},{"Path":"internal/web/engagement_list.templ","Reason":"clients"},{"Path":"internal/web/engagement.templ","OpenTag":"<a class=\"link lg:hidden\" href=\"#action-history\">","Reason":"detail"},{"Path":"internal/web/engagement.templ","OpenTag":"<a class=\"other\">","Reason":"other"},{"Path":"internal/web/components/molecules/list_column_heading.templ","Reason":"shared"}]`)
		}
		if err := os.WriteFile(filepath.Join(root, path), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name             string
		clients, catalog bool
		want             []string
	}{
		{"neither", false, false, []string{"other", "shared"}},
		{"catalog", false, true, []string{"catalog", "other", "shared"}},
		{"clients", true, false, []string{"clients", "detail", "other", "shared"}},
		{"both", true, true, []string{"catalog", "clients", "detail", "other", "shared"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := t.TempDir()
			if err := copyTemplate(root, output, nil, "old", "new", true, true, tc.clients, tc.catalog); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(output, "scripts/actioncheck/exceptions.json"))
			if err != nil {
				t.Fatal(err)
			}
			var exceptions []struct {
				Path   string
				Reason string
			}
			if err := json.Unmarshal(data, &exceptions); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, exception := range exceptions {
				got = append(got, exception.Reason)
				if _, err := os.Stat(filepath.Join(output, exception.Path)); err != nil {
					t.Errorf("exception references missing %s: %v", exception.Path, err)
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("exception reasons = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGenerateCatalogProfile(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	exceptionData, err := os.ReadFile(filepath.Join(root, "scripts", "actioncheck", "exceptions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sourceExceptions []map[string]any
	if err := json.Unmarshal(exceptionData, &sourceExceptions); err != nil {
		t.Fatal(err)
	}
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
			data, err := os.ReadFile(filepath.Join(output, "scripts", "actioncheck", "exceptions.json"))
			if err != nil {
				t.Fatal(err)
			}
			var actual []map[string]any
			if err := json.Unmarshal(data, &actual); err != nil {
				t.Fatal(err)
			}
			var expected []map[string]any
			for _, exception := range sourceExceptions {
				path, ok := exception["Path"].(string)
				if !ok {
					t.Fatal("exception has no string Path")
				}
				if path == "internal/web/catalog.templ" && !selected ||
					(path == "internal/web/engagement_list.templ" || path == "internal/web/clients.templ" || path == "internal/web/engagement_new.templ") && !tc.clients ||
					path == "internal/web/engagement.templ" && exception["OpenTag"] == `<a class="link lg:hidden" href="#action-history">` && !tc.clients {
					continue
				}
				expected = append(expected, exception)
				if _, err := os.Stat(filepath.Join(output, path)); err != nil {
					t.Errorf("retained exception references missing %s: %v", path, err)
				}
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Errorf("actioncheck exceptions differ from retained source records")
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
			for _, path := range []string{"internal/scheduling/move.go", "internal/platform/database/migrations/000048_scheduling_bookings.sql", "internal/platform/database/migrations/000050_scheduling_booking_moves.sql", "internal/platform/database/migrations/000056_catalog_entry_revision.sql"} {
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
			for path, test := range map[string]string{
				"internal/scheduling/service_test.go": "func TestBookingRequiresAvailableWorkingHours(",
				"internal/httpapi/scheduling_test.go": "func TestSchedulingBookingsREST(",
				"internal/web/scheduling_test.go":     "func TestSchedulingBookingWeb(",
			} {
				raw, err = os.ReadFile(filepath.Join(output, path))
				if err != nil || strings.Contains(string(raw), test) != selected {
					t.Fatalf("%s booking test, selected=%v: %v", path, selected, err)
				}
			}
			if err := runCommand(output, "go", "test", "./..."); err != nil {
				t.Fatalf("generated application tests: %v", err)
			}
		})
	}
}
