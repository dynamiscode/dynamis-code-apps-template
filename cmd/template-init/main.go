package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	commitPattern     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	modulePattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~/-]+$`)
	slugPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	maintainerPattern = regexp.MustCompile(`^@[A-Za-z0-9][A-Za-z0-9-]*(/[A-Za-z0-9][A-Za-z0-9-]*)?$`)
	knownProfiles     = []string{"Core", "Identity", "Agent", "Files", "Clients", "Catalog"}
)

const (
	templateRepositoryURL = "https://github.com/dynamiscode/dynamis-code-apps-template"
	templateSecurityURL   = templateRepositoryURL + "/security/advisories/new"
)

type lockFile struct {
	Template struct {
		Source  string `json:"source"`
		Version string `json:"version"`
		Commit  string `json:"commit"`
	} `json:"template"`
	GeneratedAt time.Time `json:"generatedAt"`
	Profiles    []string  `json:"profiles"`
}

func main() {
	if err := run(os.Args[1:], time.Now); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, now func() time.Time) error {
	flags := flag.NewFlagSet("template-init", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	templateDir := flags.String("template-dir", ".", "released template checkout")
	output := flags.String("output", "", "new application directory")
	name := flags.String("name", "", "application display name")
	module := flags.String("module", "", "Go module path")
	repository := flags.String("repository", "", "application repository URL")
	securityURL := flags.String("security-url", "", "application private security-reporting URL")
	maintainer := flags.String("maintainer", "", "application repository maintainer (for CODEOWNERS)")
	source := flags.String("source", "", "released template source URL")
	version := flags.String("version", "", "released semantic version; defaults to VERSION")
	commit := flags.String("commit", "", "released template commit SHA")
	profiles := flags.String("profiles", "", "selected profiles, comma-separated")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if strings.TrimSpace(*name) == "" || len(*name) > 80 || strings.ContainsAny(*name, "\r\n\x00") {
		return errors.New("name must be 1 to 80 characters without control characters")
	}
	if !modulePattern.MatchString(*module) || !strings.Contains(*module, "/") ||
		strings.Contains(*module, "//") || strings.Contains(*module, "..") {
		return errors.New("module must be a valid fully qualified Go module path")
	}
	slug := filepath.Base(*module)
	if !slugPattern.MatchString(slug) {
		return errors.New("module basename must be a lowercase application slug")
	}
	if !validHTTPSURL(*repository) {
		return errors.New("repository must be an HTTPS application repository URL")
	}
	if !validHTTPSURL(*securityURL) {
		return errors.New("security-url must be an HTTPS private security-reporting URL")
	}
	if !validMaintainer(*maintainer) {
		return errors.New("maintainer must be a single @-prefixed repository owner")
	}
	selectedProfiles, err := parseProfiles(*profiles)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(*source, "https://") {
		return errors.New("source must be an HTTPS released-template URL")
	}
	if !commitPattern.MatchString(*commit) {
		return errors.New("commit must be a 40-character lowercase Git SHA")
	}
	root, err := filepath.Abs(*templateDir)
	if err != nil {
		return err
	}
	destination, err := filepath.Abs(*output)
	if err != nil || strings.TrimSpace(*output) == "" || destination == root {
		return errors.New("output must name a new application directory")
	}
	if _, err := os.Stat(destination); err == nil {
		return errors.New("output already exists; generation never overwrites files")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	selectedVersion := strings.TrimSpace(*version)
	if selectedVersion == "" {
		raw, err := os.ReadFile(filepath.Join(root, "VERSION"))
		if err != nil {
			return err
		}
		selectedVersion = strings.TrimSpace(string(raw))
	}
	if !validSemver(selectedVersion) {
		return errors.New("version must use semantic versioning without a v prefix")
	}
	templateModule, templateName, templateSlug, err := templateIdentity(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return err
	}
	replacements := []string{
		templateModule, *module,
		templateName, *name,
		"Dynamis Code", *name,
		"Dynamis-Code", strings.ReplaceAll(slug, "_", "-"),
		"dynamis-code-apps-template", slug,
		"Copyright (c) 2026 David Londono", "Copyright (c) 2026 " + *name + " contributors",
		"template maintainers", "application maintainers",
		"template maintainer", "application maintainer",
		templateRepositoryURL, strings.TrimRight(*repository, "/"),
		templateSecurityURL, strings.TrimRight(*securityURL, "/"),
		"@davidlondono", *maintainer,
	}
	if err := copyTemplate(
		root, destination, replacements, templateSlug, slug, hasProfile(selectedProfiles, "Agent"), hasProfile(selectedProfiles, "Files"),
		hasProfile(selectedProfiles, "Clients"), hasProfile(selectedProfiles, "Catalog"),
	); err != nil {
		return err
	}
	if err := writeGeneratedReadme(
		filepath.Join(destination, "README.md"), *name, *module, slug,
		strings.TrimRight(*repository, "/"), strings.TrimRight(*securityURL, "/"),
		hasProfile(selectedProfiles, "Agent"),
	); err != nil {
		return err
	}
	command := exec.Command("go", "generate", "./api")
	command.Dir = destination
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("regenerate generated API contract: %s", strings.TrimSpace(string(output)))
	}
	command = exec.Command("go", "mod", "tidy")
	command.Dir = destination
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("prune generated module dependencies: %s", strings.TrimSpace(string(output)))
	}
	lock := lockFile{GeneratedAt: now().UTC(), Profiles: selectedProfiles}
	lock.Template.Source = *source
	lock.Template.Version = selectedVersion
	lock.Template.Commit = *commit
	return writeLock(filepath.Join(destination, "template.lock"), lock)
}

func copyTemplate(
	root string,
	destination string,
	replacements []string,
	oldSlug string,
	newSlug string,
	agent bool,
	files bool,
	clients bool,
	catalog bool,
) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if absolute == destination || strings.HasPrefix(absolute, destination+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == "." {
			return err
		}
		if ignored(relative) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !agent && agentPath(relative) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !files && filesPath(relative) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !catalog && catalogPath(relative) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !clients && clientsPath(relative) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return copySymlink(root, path, target, relative)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.ContainsRune(string(raw), '\x00') {
			value := strings.NewReplacer(replacements...).Replace(string(raw))
			if !agent && filepath.ToSlash(relative) == "docs/capabilities.md" {
				value = strings.ReplaceAll(value, "; [MCP tests](../internal/mcpserver/server_test.go); [CLI tests](../internal/appctl/run_test.go)", "")
				value = strings.ReplaceAll(value, "[live surface smoke](../internal/bootstrap/agent_smoke_test.go)", "[composition test](../internal/bootstrap/app_test.go)")
			}
			if !agent && filepath.ToSlash(relative) == "NOTICE" {
				value = strings.ReplaceAll(value, "- github.com/modelcontextprotocol/go-sdk v1.8.0 — Apache-2.0 and MIT code\n", "")
			}
			value = strings.NewReplacer(
				`"`+oldSlug+`"`, `"`+newSlug+`"`,
				`"`+oldSlug+`-checks"`, `"`+newSlug+`-checks"`,
				oldSlug+":${", newSlug+":${",
				oldSlug+":$${", newSlug+":$${",
				oldSlug+":dev", newSlug+":dev",
				"OTEL_SERVICE_NAME="+oldSlug, "OTEL_SERVICE_NAME="+newSlug,
				"`"+oldSlug+"`", "`"+newSlug+"`",
			).Replace(value)
			raw = []byte(value)
		}
		if !agent && filepath.ToSlash(relative) == "internal/bootstrap/agent.go" {
			raw = []byte(strings.NewReplacer(replacements...).Replace(disabledAgent))
		}
		if !files {
			raw = disableFilesSource(filepath.ToSlash(relative), raw)
		}
		if !clients {
			raw = disableClientsSource(filepath.ToSlash(relative), raw)
		}
		if !catalog {
			raw = disableCatalogSource(filepath.ToSlash(relative), raw)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, info.Mode().Perm())
	})
}

// copySymlink recreates a relative symlink whose target stays inside the
// template tree, such as .claude/skills -> ../.agents/skills. Targets are copied
// as-is and not pruned by profile, so use it only for paths that are always kept.
// Any other symlink is rejected.
func copySymlink(root, path, target, relative string) error {
	link, err := os.Readlink(path)
	if err != nil {
		return err
	}
	if filepath.IsAbs(link) {
		return fmt.Errorf("template symlink must be relative: %s", relative)
	}
	resolved, err := filepath.Rel(root, filepath.Join(filepath.Dir(path), link))
	if err != nil || resolved == ".." || strings.HasPrefix(resolved, ".."+string(filepath.Separator)) {
		return fmt.Errorf("template symlink escapes the template: %s", relative)
	}
	return os.Symlink(link, target)
}

func catalogPath(relative string) bool {
	relative = filepath.ToSlash(relative)
	return relative == "api/catalog_contract_test.go" || relative == "api/catalog_authorization_contract_test.go" || relative == "internal/catalog" || strings.HasPrefix(relative, "internal/catalog/") ||
		relative == "internal/httpapi/catalog.go" || relative == "internal/httpapi/catalog_test.go" || relative == "internal/httpapi/catalog_authorization_test.go" || relative == "internal/httpapi/catalog_isolation_test.go" ||
		relative == "internal/web/catalog.go" || relative == "internal/web/catalog_test.go" || relative == "internal/web/catalog_authorization_test.go" || relative == "internal/web/catalog_isolation_test.go" ||
		relative == "internal/web/catalog.templ" || relative == "internal/web/catalog_templ.go" ||
		relative == "internal/platform/database/migrations/000040_catalog.sql" ||
		relative == "internal/platform/database/migrations/000041_catalog_prices.sql"
}

func disableCatalogSource(relative string, raw []byte) []byte {
	value := string(raw)
	switch relative {
	case "internal/bootstrap/app.go", "internal/httpapi/handler.go", "internal/web/handler.go", "internal/web/handler_test.go":
		if relative == "internal/httpapi/handler.go" {
			value = removeText(value, "func NewHandlerWithCatalog(", "func NewHandlerWithCatalogAndEngagement(")
			value = removeText(value, "func NewHandlerWithCatalogAndEngagement(", "func newHandler(")
			value = strings.ReplaceAll(value, ", catalog.NewService(db, identityService, nil)", "")
		}
		if relative == "internal/bootstrap/app.go" {
			value = strings.ReplaceAll(value, "httpapi.NewHandlerWithCatalogAndEngagement", "httpapi.NewHandlerWithEngagement")
			value = strings.ReplaceAll(value, "catalogService, engagementService", "engagementService")
		}
		value = removeLinesContaining(value, `/internal/catalog"`)
		value = removeLinesContaining(value, "catalogService")
		value = removeLinesContaining(value, "h.catalogList")
		value = removeLinesContaining(value, "h.catalogCreate")
		value = removeLinesContaining(value, "h.catalogCreatePrice")
		value = removeLinesContaining(value, "h.catalogArchive")
		value = removeLinesContaining(value, "h.catalogUnarchive")
		value = removeLinesContaining(value, "h.catalogDelete")
		value = removeLinesContaining(value, "h.catalogEdit")
		value = removeLinesContaining(value, `"updateCatalog":`)
		value = removeLinesContaining(value, "EnableCatalog(")
		value = removeLinesContaining(value, `"listCatalog":`)
		value = removeLinesContaining(value, `"archiveCatalog":`)
	case "docs/capabilities.md":
		value = removeLineStarting(value, "| Catalog |")
		value = removeText(value, "## Catalog authorization and isolation (#597)", "\n## ")
	case "docs/architecture.md":
		value = removeLineStarting(value, "Catalog is optional")
	case "docs/data-lifecycle.md":
		value = removeLineStarting(value, "| catalog / `catalog_entries`")
	case "docs/api.md", "docs/web.md":
		value = removeLineStarting(value, "Catalog profile:")
	case "api/openapi.json":
		var document map[string]any
		if json.Unmarshal(raw, &document) == nil {
			if paths, ok := document["paths"].(map[string]any); ok {
				delete(paths, "/api/v1/workspaces/{workspaceId}/catalog")
				delete(paths, "/api/v1/workspaces/{workspaceId}/catalog/{entryId}/prices")
				delete(paths, "/api/v1/workspaces/{workspaceId}/catalog/{entryId}/prices/{priceId}")
				delete(paths, "/api/v1/workspaces/{workspaceId}/catalog/{entryId}")
				delete(paths, "/api/v1/workspaces/{workspaceId}/catalog/{entryId}/archive")
				delete(paths, "/api/v1/workspaces/{workspaceId}/catalog/{entryId}/unarchive")
				if encoded, err := json.MarshalIndent(document, "", "  "); err == nil {
					value = string(encoded) + "\n"
				}
			}
		}
	}
	return []byte(value)
}

func filesPath(relative string) bool {
	relative = filepath.ToSlash(relative)
	return relative == "internal/files" || strings.HasPrefix(relative, "internal/files/") ||
		relative == "internal/httpapi/files.go" || relative == "internal/httpapi/files_test.go" || relative == "internal/web/files.go" || relative == "internal/web/files_test.go" ||
		relative == "internal/web/files.templ" || relative == "internal/web/files_templ.go" ||
		relative == "internal/web/viewmodel/files.go" || relative == "internal/web/files_actions.go" || relative == "internal/web/files_order.go" || relative == "docs/files-417-evidence.md" ||
		relative == "internal/web/files_no_javascript_test.go" || relative == "internal/browsertest/files_test.go" ||
		relative == "internal/platform/database/migrations/000014_files.sql" ||
		relative == "internal/platform/database/migrations/000020_file_short_ids.sql" ||
		relative == "internal/engagement/agreement_test.go" || relative == "internal/web/agreement_files_test.go" ||
		relative == "internal/platform/database/migrate_files_test.go" ||
		relative == "internal/platform/database/migrate_file_ids_test.go"
}

// clientsPath lists the Clients feature files. Generation without the Clients
// profile removes them; disableClientsSource removes the shared wiring that
// refers to them.
func clientsPath(relative string) bool {
	relative = filepath.ToSlash(relative)
	return relative == "internal/clients" || strings.HasPrefix(relative, "internal/clients/") ||
		relative == "internal/identity/engagement_membership_test.go" ||
		relative == "internal/engagement/engagement.go" || relative == "internal/engagement/list.go" || relative == "internal/engagement/list_test.go" ||
		relative == "internal/engagement/action_queue.go" || relative == "internal/engagement/action_queue_test.go" || relative == "internal/engagement/dashboard.go" ||
		relative == "internal/engagement/agreement.go" ||
		relative == "internal/engagement/agreement_test.go" ||
		relative == "internal/engagement/assignee.go" || relative == "internal/engagement/next_action.go" || relative == "internal/engagement/quote.go" ||
		relative == "internal/platform/database/migrations/000033_engagements.sql" ||
		relative == "internal/platform/database/migrations/000039_engagement_agreement.sql" ||
		relative == "internal/platform/database/migrations/000042_engagement_names.sql" ||
		relative == "internal/platform/database/migrations/000043_engagement_action_history.sql" ||
		relative == "internal/platform/database/migrations/000044_engagement_action_completion.sql" ||
		relative == "internal/platform/database/migrations/000045_engagement_action_version.sql" ||
		relative == "internal/platform/database/migrate_engagement_action_test.go" ||
		relative == "internal/platform/database/migrate_engagement_names_test.go" ||
		relative == "internal/platform/database/migrate_engagement_names_postgres_test.go" ||
		relative == "internal/platform/database/migrations/000034_engagement_next_action.sql" || relative == "internal/platform/database/migrations/000035_engagement_quote.sql" || relative == "internal/platform/database/migrations/000036_engagement_assignee.sql" ||
		relative == "internal/platform/database/migrations/000038_engagement_stage_history.sql" ||
		relative == "cmd/demo/clients.go" || relative == "cmd/demo/clients_test.go" ||
		relative == "cmd/appctl" || strings.HasPrefix(relative, "cmd/appctl/") ||
		relative == "internal/appctl" || strings.HasPrefix(relative, "internal/appctl/") ||
		relative == "internal/bootstrap/agent_smoke_test.go" ||
		relative == "internal/mcpserver/approval_test.go" || relative == "internal/mcpserver/schema_contract_test.go" ||
		relative == "internal/httpapi/clients.go" || relative == "internal/httpapi/clients_test.go" ||
		relative == "internal/httpapi/client_timeline.go" || relative == "internal/httpapi/client_timeline_test.go" ||
		relative == "internal/web/clients.go" || relative == "internal/web/clients_test.go" ||
		relative == "internal/web/engagement_test.go" || relative == "internal/web/engagement_dashboard_test.go" || relative == "internal/web/action_queue_test.go" ||
		relative == "internal/web/engagement_list.go" || relative == "internal/web/engagement_list_test.go" ||
		relative == "internal/web/engagement_board.go" || relative == "internal/web/engagement_board_test.go" ||
		relative == "internal/web/engagement_list.templ" || relative == "internal/web/engagement_list_templ.go" ||
		relative == "internal/web/engagement_board.templ" || relative == "internal/web/engagement_board_templ.go" ||
		relative == "internal/web/client_timeline.go" || relative == "internal/web/client_timeline_test.go" ||
		relative == "internal/web/clients_filters.go" ||
		relative == "internal/web/viewmodel/clients_filters.go" || relative == "internal/web/viewmodel/clients.go" ||
		relative == "internal/web/viewmodel/client_options.go" || relative == "internal/web/list_lazy_test.go" || relative == "internal/web/list_views_test.go" || relative == "internal/web/clients_actions.go" || relative == "internal/web/clients_order.go" ||
		relative == "internal/web/clients_no_javascript_test.go" ||
		relative == "internal/web/clients.templ" || relative == "internal/web/clients_templ.go" ||
		relative == "internal/web/clients_list.templ" || relative == "internal/web/clients_list_templ.go" ||
		relative == "internal/web/clients_bulk.templ" || relative == "internal/web/clients_bulk_templ.go" ||
		relative == "internal/web/client_details.templ" || relative == "internal/web/client_details_templ.go" ||
		relative == "internal/web/client_fields.templ" || relative == "internal/web/client_fields_templ.go" ||
		relative == "internal/web/templates/clients.html" || relative == "internal/web/templates/client.html" ||
		relative == "internal/platform/database/migrations/000022_clients.sql" ||
		relative == "internal/platform/database/migrations/000023_client_fields.sql" ||
		relative == "internal/platform/database/migrations/000024_client_events.sql" ||
		relative == "internal/platform/database/migrations/000031_client_timeline.sql" ||
		relative == "internal/web/clients_events.go" || relative == "internal/web/clients_events_test.go" ||
		relative == "internal/web/client_bulk.go" || relative == "internal/web/client_bulk_test.go" ||
		relative == "internal/platform/database/client_slot_indexes_test.go" ||
		relative == "internal/platform/database/client_slot_indexes_postgres_test.go" ||
		relative == "internal/httpapi/client_fields.go" || relative == "internal/httpapi/client_fields_test.go" ||
		relative == "internal/web/client_fields.go" || relative == "internal/web/client_fields_test.go" ||
		relative == "internal/httpapi/client_values.go" || relative == "internal/httpapi/client_values_test.go" ||
		relative == "internal/web/client_values_test.go" ||
		relative == "internal/platform/database/migrations/000025_client_field_options.sql" ||
		relative == "internal/httpapi/client_options.go" || relative == "internal/httpapi/client_options_test.go" ||
		relative == "internal/web/client_options.go" || relative == "internal/web/client_options_test.go" ||
		relative == "internal/web/templates/client-fields.html" ||
		relative == "internal/httpapi/client_field_delete_test.go" || relative == "internal/web/client_field_delete_test.go" ||
		relative == "internal/httpapi/client_lifecycle.go" || relative == "internal/httpapi/client_lifecycle_test.go" ||
		relative == "internal/web/client_lifecycle.go" || relative == "internal/web/client_lifecycle_test.go" ||
		relative == "internal/mcpserver/clients.go" || relative == "internal/mcpserver/clients_test.go" ||
		relative == "internal/mcpserver/testdata/clients-tools-v1.json" ||
		relative == "internal/bootstrap/client_authorization_matrix_test.go" ||
		relative == "internal/appctl/clients_test.go"
}

// disableClientsSource removes Clients wiring from shared files when the profile
// is not selected. Each rewrite drops whole lines that name Clients symbols, so
// the rest of the file compiles unchanged.
func disableClientsSource(relative string, raw []byte) []byte {
	value := string(raw)
	switch relative {
	case "internal/platform/database/migrate.go":
		value = removeLinesContaining(value, `/internal/platform/id"`)
		value = removeText(value, "\t\tif migration.version == 42 {", "\t\tif err := recordMigration(ctx, tx, driver, migration.version); err != nil {")
		value = removeText(value, "func backfillEngagementNames(", "// renderMigration")
	case "internal/identity/account.go":
		value = removeText(value, "\tif _, err := s.exec(ctx, tx, \"UPDATE engagements SET action_version", "\tresult, err := s.exec(ctx, tx, \"DELETE FROM users WHERE id = ?\", userID)")
	case "internal/identity/service.go":
		value = removeText(value, "func (s *Service) clearMemberEngagements(", "func normalizeEmail(")
		value = strings.Replace(value, "func normalizeEmail(", "func (s *Service) clearMemberEngagements(ctx context.Context, tx *sql.Tx, workspaceID WorkspaceID, userID UserID) error { return nil }\n\nfunc normalizeEmail(", 1)
	case "cmd/demo/main.go":
		value = strings.Replace(value, "workspaceID, userID, err := demoOwner(", "workspaceID, _, err := demoOwner(", 1)
		value = removeBlockIncludingEnd(value, "\tif err := seedClients(", "\t}\n")
	case "internal/bootstrap/app.go":
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLineStarting(value, "\tclientService := clients.NewService(")
		// The portability export takes the client source as its last argument.
		value = strings.ReplaceAll(value, "\t\tclientService,\n", "\t\tnil,\n")
		value = strings.ReplaceAll(value, " clientService,", "")
		value = removeLineStarting(value, "\twebHandler.EnableClients(")
	case "internal/httpapi/handler.go":
		value = removeLinesContaining(value, `/internal/clients"`)
		value = strings.ReplaceAll(value, ", clientServices ...*clients.Service", "")
		value = removeLinesContaining(value, "*clients.Service")
		value = removeLinesContaining(value, "firstClient(clientServices)")
		value = strings.ReplaceAll(value, ", clientServices...)", ")")
		value = removeLinesContaining(value, `"listClients": h.listClients`)
		value = removeLinesContaining(value, `"getClient": h.getClient`)
		value = removeLinesContaining(value, `"listClientTimeline": h.listClientTimeline`)
		value = removeLinesContaining(value, `"listClientFields": h.listClientFields`)
		value = removeLinesContaining(value, `"archiveClientField": h.archiveClientField`)
		value = removeLinesContaining(value, `"listClientValues": h.listClientValues`)
		value = removeLinesContaining(value, `"clearClientValue": h.clearClientValue`)
		value = removeLinesContaining(value, `"listClientFieldOptions": h.listClientFieldOptions`)
		value = removeLinesContaining(value, `"deleteClientFieldOption": h.deleteClientFieldOption`)
		value = removeLinesContaining(value, `"deleteClientField":`)
		value = removeLinesContaining(value, `"archiveClient":`)
		value = removeLinesContaining(value, `"deleteClient":`)
		value = removeLinesContaining(value, `"createEngagement":`)
		value = removeLinesContaining(value, `"setEngagementNextAction":`)
		value = removeLinesContaining(value, `"setEngagementQuote":`)
		value = removeLinesContaining(value, `"setEngagementAssignee":`)
		value = removeLinesContaining(value, `"setEngagementAgreement":`)
	case "internal/httpapi/engagement.go":
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, `"time"`)
		value = removeText(value, "func (h *handler) setEngagementAgreement(", "func (h *handler) setEngagementAssignee(")
		value = removeText(value, "func (h *handler) setEngagementAssignee(", "func (h *handler) getEngagementLabel(")
		value = removeText(value, "func (h *handler) setEngagementQuote(", "func (h *handler) createEngagement(")
		value = removeText(value, "func (h *handler) createEngagement(", "func (h *handler) listEngagementStages(")
		value = removeText(value, "\tcase errors.Is(err, engagement.ErrInvalidName):", "\tcase errors.Is(err, engagement.ErrInvalidInput):")
		value = removeText(value, "\tcase errors.Is(err, engagement.ErrNoOpenStage):", "\tdefault:")
	case "internal/httpapi/engagement_test.go":
		value = removeLinesContaining(value, `"context"`)
		value = removeLinesContaining(value, `/internal/identity"`)
		value = removeLinesContaining(value, `/internal/platform/config"`)
		if i := strings.Index(value, "\nfunc TestEngagementThroughREST("); i >= 0 {
			value = value[:i] + "\n"
		}
	case "internal/engagement/service_test.go":
		value = removeLinesContaining(value, `"encoding/json"`)
		value = removeLinesContaining(value, `"strings"`)
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, `/internal/platform/maintenance"`)
		if i := strings.Index(value, "\nfunc TestCreateEngagementRequiresOpenStageAndClient("); i >= 0 {
			value = value[:i] + "\n"
		}
	case "internal/bootstrap/agent.go":
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, "*clients.Service")
		value = strings.ReplaceAll(value, ", mcpserver.WithClients(clientService)", "")
	case "internal/mcpserver/server_test.go":
		value = removeLinesContaining(value, `/internal/clients"`)
		value = strings.ReplaceAll(value, "newTestStateWith(t, config.MCP{}, true)", "newTestState(t, config.MCP{})")
		value = strings.ReplaceAll(value, "newTestStateWith(t, config.MCP{AllowedOrigins: []string{\"https://client.example\"}}, true)", "newTestState(t, config.MCP{AllowedOrigins: []string{\"https://client.example\"}})")
		value = removeText(value, "\tserver := httptest.NewServer(state.handler)\n\tt.Cleanup(server.Close)\n\ttransport := &authTransport{token: readToken.Secret", "\n\tif err := state.auth.RevokeAPIToken")
		value = removeBlockIncludingEnd(value, "\tif withClients {\n\t\t// Field, option", "\t}\n")
		value = removeBlockIncludingEnd(value, "\tif withClients {\n\t\toptions = append(options, WithClients(", "\t}\n")
	case "internal/httpapi/webhooks_test.go":
		value = removeText(value, "\tclient := serveAuthorized(handler, http.MethodPost", "\n\tdeleted := serveAuthorized(handler, http.MethodDelete")
	case "internal/httpapi/handler_test.go":
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeText(value, "\tcollection := \"/api/v1/workspaces/\" + workspaceID + \"/clients\"", "\n\tif err := db.Close();")
		value = strings.Replace(value, "handler, db, workspaceID, token := testHandler(t)", "handler, db, _, _ := testHandler(t)", 1)
		value = strings.Replace(value, "\n\t\tclients.NewService(db, cfg.Database.Driver, auth, cfg.Data.ClientFieldsMaxPerWorkspace, webhookService))", ")", 1)
	case "internal/web/viewmodel/handler.go":
		value = removeLinesContaining(value, `/internal/engagement"`)
		value = removeLinesContaining(value, "engagement.Engagement")
		value = removeLinesContaining(value, "engagement.DashboardPage")
		value = removeLinesContaining(value, "engagement.ActionHistoryEntry")
		value = removeLinesContaining(value, "DashboardPrevious ")
		value = removeLinesContaining(value, "DashboardNext ")
		value = removeLinesContaining(value, "engagement.Stage")
		value = removeBlockIncludingEnd(value, "type TimelineDraft struct {", "}\n")
		value = removeLinesContaining(value, "TimelineDraft ")
		value = removeLinesContaining(value, "clients.TimelineEntry")
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, "clients.FilterInput")
		value = removeLinesContaining(value, "clients.Client")
		value = removeLinesContaining(value, "clients.Input")
		value = removeLinesContaining(value, "clients.Field")
		value = removeLinesContaining(value, "clients.Option")
		value = removeLinesContaining(value, "ClientQueryForm")
		value = removeLinesContaining(value, "ClientFieldRow")
		value = removeLinesContaining(value, "ClientValueRow")
	case "internal/web/handler.go":
		value = removeText(value, "\tif h.engagement != nil && principal.Permissions[identity.ResourcesRead] {", "\th.render(writer, http.StatusOK, \"workspace-home\", data)")
		value = removeLinesContaining(value, `"strconv"`)
		value = removeLinesContaining(value, "CanEditClients: principal.Permissions")
		value = strings.Replace(value, "principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceRead)", "_, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceRead)", 1)
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, "h.bulkClients")
		value = removeLinesContaining(value, "*clients.Service")
		value = removeLinesContaining(value, "clients.FilterInput")
		value = removeLinesContaining(value, "clients.Client")
		value = removeLinesContaining(value, "clients.Input")
		value = removeLinesContaining(value, "h.clients != nil")
		value = removeLinesContaining(value, "clients.Field")
		value = removeLinesContaining(value, "clients.Option")
		value = removeLinesContaining(value, "clientQueryForm")
		value = removeLinesContaining(value, "clientFieldRow")
		value = removeLinesContaining(value, "clientValueRow")
		value = removeLinesContaining(value, "clientDetailPage(data)")
		value = removeLinesContaining(value, "clientFieldsPage(data)")
		value = removeLinesContaining(value, "clientFilterField")
		value = removeLinesContaining(value, "clientFilterOperator")
		value = removeLinesContaining(value, "/workspaces/{workspaceId}/client-fields")
		value = removeLinesContaining(value, "/workspaces/{workspaceId}/clients")
		value = removeLinesContaining(value, "/workspaces/{workspaceId}/engagements\"")
		value = removeLinesContaining(value, "/workspaces/{workspaceId}/engagements/board")
		value = removeLinesContaining(value, "/workspaces/{workspaceId}/engagements/due")
		value = removeLinesContaining(value, "h.changeQueuedAction")
	case "internal/web/workspace.templ":
		value = removeLinesContaining(value, `import "strconv"`)
		value = removeLinesContaining(value, `/internal/engagement"`)
		value = removeText(value, "templ dashboardActionPreview(", "templ workspaceHome(")
		value = removeText(value, "\t\tif data.EngagementDashboard != nil {", "\t\t// The home reflects")
	case "internal/web/workspace_templ.go":
		value = removeLinesContaining(value, `import "strconv"`)
		value = removeLinesContaining(value, `/internal/engagement"`)
		value = removeText(value, "func dashboardActionPreview(", "func workspaceHome(")
		value = removeText(value, "\t\t\tif data.EngagementDashboard != nil {", "\t\t\ttempl_7745c5c3_Err = templruntime.WriteString(templ_7745c5c3_Buffer, 54,")
	case "internal/web/engagement.go":
		value = strings.ReplaceAll(value, "ClientsEnabled: h.clients != nil, ", "")
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, `"time"`)
		value = removeLinesContaining(value, `"net/url"`)
		value = removeLinesContaining(value, `"strconv"`)
		value = removeText(value, "func (h *Handler) setEngagementAgreement(", "func (h *Handler) setEngagementAssignee(")
		value = removeText(value, "func (h *Handler) setEngagementAssignee(", "func (h *Handler) EnableEngagement(")
		value = removeText(value, "func (h *Handler) moveEngagement(", "func (h *Handler) setEngagementLabel(")
		value = removeText(value, "func (h *Handler) createEngagement(", "func (h *Handler) engagementStages(")
	case "internal/web/engagement.templ":
		value = removeText(value, "templ engagementDetailPage(", "templ engagementStagesPage(")
	case "internal/web/engagement_templ.go":
		value = removeLinesContaining(value, `import "strconv"`)
		value = removeLinesContaining(value, `/internal/identity"`)
		value = removeText(value, "func engagementDetailPage(", "func engagementStagesPage(")
	case "internal/platform/maintenance/maintenance.go":
		// Retention prunes client_events, which Clients-off apps do not have.
		value = removeLinesContaining(value, "client_events")
		value = removeLinesContaining(value, "ClientRealtimeReplay")
	case "internal/platform/database/remove_items_test.go":
		value = removeLinesContaining(value, "INSERT INTO clients")
		value = strings.ReplaceAll(value, `"workspaces", "clients", "audit_events"`, `"workspaces", "audit_events"`)
	case "internal/platform/maintenance/maintenance_test.go":
		value = removeText(value, "\tvar clientEventOldID int64\n", "\tvar notificationOldID int64\n")
		value = strings.ReplaceAll(value, ` "client_events": clientEventOldID,`, "")
		value = strings.ReplaceAll(value, "result.ClientRealtimeReplay < 1 || ", "")
	case "docs/data-lifecycle.md":
		value = removeLineStarting(value, "| clients / `client_timeline_entries` |")
	case "docs/mcp.md":
		value = removeText(value, "\n## Clients tools (Clients profile)", "\n## Prompts\n")
	case "docs/capabilities.md":
		value = strings.ReplaceAll(value, "[web](../internal/web/engagement_test.go)", "web")
		value = removeText(value, " Writers can create engagements", "\nOwners and admins can set or reset")
		value = removeText(value, "\nNext actions have text", "\n\n## Client timeline REST")
		value = removeLineStarting(value, "| Clients |")
		value = removeLineStarting(value, "| Clients shared list view |")
		value = removeLineStarting(value, "| Client timeline service |")
		value = removeText(value, "\n## Client timeline REST (#575)", "\n## Route and navigation journey verification")
		value = strings.ReplaceAll(value, "; [CLI tests](../internal/appctl/run_test.go)", "")
		value = strings.ReplaceAll(value, "[live surface smoke](../internal/bootstrap/agent_smoke_test.go)", "[composition test](../internal/bootstrap/app_test.go)")
	case "docs/api.md":
		value = removeLineStarting(value, "- Engagements:")
		value = removeLineStarting(value, "- Next actions:")
		value = removeLineStarting(value, "- Quotes:")
		value = removeLineStarting(value, "- Assignee:")
		value = removeLineStarting(value, "- Agreement:")
		value = removeText(value, "\n- Clients (Clients profile):", "\n\n## Webhooks")
	case "docs/web.md":
		value = removeText(value, "\nOn a client detail page,", "\n`/account`")
		value = removeLineStarting(value, "| Workspace clients and custom fields;")
		value = removeText(value, "\n\n`/workspaces/{workspaceId}/clients` lists", "\n\n## Realtime contract")
		value = removeText(value, "\n\n`GET /workspaces/{workspaceId}/clients/events` is", "\n\n`GET /notifications/events`")
	case "api/openapi.json":
		value = stripClientsOpenAPI([]byte(value))
	}
	return []byte(value)
}

// removeLinesContaining drops every whole line that contains substring.
func removeLinesContaining(value, substring string) string {
	lines := strings.SplitAfter(value, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if !strings.Contains(line, substring) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "")
}

func stripClientsOpenAPI(raw []byte) string {
	var document map[string]any
	if json.Unmarshal(raw, &document) != nil {
		return string(raw)
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok {
		return string(raw)
	}
	for path := range paths {
		if strings.Contains(path, "/clients") || strings.Contains(path, "/client-fields") || strings.Contains(path, "/engagements") {
			delete(paths, path)
		}
	}
	if components, ok := document["components"].(map[string]any); ok {
		if schemas, ok := components["schemas"].(map[string]any); ok {
			if export, ok := schemas["WorkspaceExport"].(map[string]any); ok {
				if properties, ok := export["properties"].(map[string]any); ok {
					for _, name := range []string{"clients", "clientFields", "clientFieldOptions", "engagements", "engagementNextActions", "engagementActionHistory", "engagementStageHistory"} {
						delete(properties, name)
					}
				}
			}
		}
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(encoded) + "\n"
}

func disableFilesSource(relative string, raw []byte) []byte {
	value := string(raw)
	switch relative {
	case "internal/platform/database/migrations/000039_engagement_agreement.sql":
		value = removeLineStarting(value, "ALTER TABLE engagements ADD COLUMN agreement_file_id")
	case "internal/portability/engagement.go":
		value = strings.ReplaceAll(value, "agreement_recorded, agreement_file_id FROM engagements", "agreement_recorded, NULL FROM engagements")
	case "internal/engagement/engagement.go", "internal/engagement/next_action.go":
		value = strings.ReplaceAll(value, "COALESCE(e.agreement_file_id, 0)", "0")
	case "internal/engagement/agreement.go":
		value = removeLinesContaining(value, "/internal/files\"")
		value = strings.Replace(value, "\t\"errors\"\n", "", 1)
		value = removeText(value, "\tvar fileID any\n", "\tresult, err := tx.ExecContext")
		value = strings.Replace(value, "agreement_recorded = ?, agreement_file_id = ?", "agreement_recorded = ?", 1)
		value = strings.Replace(value, "agreement.Recorded, fileID, workspaceID", "agreement.Recorded, workspaceID", 1)
		value = strings.Replace(value, "\tresult, err := tx.ExecContext", "\tif agreement.FileID != \"\" { return ErrInvalidInput }\n\tresult, err := tx.ExecContext", 1)
	case "internal/bootstrap/app.go":
		value = removeImport(value, "appfiles")
		value = removeLineStarting(value, "\twebHandler.EnableFilesPreferences(")
		value = strings.Replace(value, "\tFiles       *appfiles.Service\n", "", 1)
		value = removeText(value, "\tstorageConfig := cfg.Storage", "\tif err := jobQueue.Register(webhooks.JobKind")
		value = strings.ReplaceAll(value, "httpapi.NewHandlerWithWebhooksAndFiles", "httpapi.NewHandlerWithWebhooks")
		value = strings.ReplaceAll(value, "web.NewHandlerWithServicesAndFilesAndSharing", "web.NewHandlerWithServices")
		value = strings.ReplaceAll(value, "web.NewHandlerWithServicesAndFiles", "web.NewHandlerWithServices")
		value = strings.ReplaceAll(value, "\t\twebhookService, fileService, cfg.PublicURL, mailer,", "\t\twebhookService, cfg.PublicURL, mailer,")
		value = strings.ReplaceAll(value, "\t\tfileService, cfg.Bootstrap.SetupToken, cfg.PublicURL, mailer,", "\t\tcfg.Bootstrap.SetupToken, cfg.PublicURL, mailer,")
		value = removeLineStarting(value, "\t\tFiles:")
	case "internal/httpapi/handler.go":
		value = removeImport(value, "appfiles")
		value = removeLineStarting(value, "\tfiles ")
		value = removeText(value, "func NewHandlerWithWebhooksAndFiles(", "func NewHandlerWithMail(")
		value = strings.ReplaceAll(value, "webhookService, nil, publicURL, mailer", "webhookService, publicURL, mailer")
		value = strings.ReplaceAll(value, "webhookService, fileService, publicURL, mailer", "webhookService, publicURL, mailer")
		for _, prefix := range []string{
			"\t\t\"listFiles\":", "\t\t\"getFile\":", "\t\t\"initiateFileUpload\":", "\t\t\"completeFileUpload\":",
		} {
			value = removeLineStarting(value, prefix)
		}
		value = strings.ReplaceAll(value, "\tfileService *appfiles.Service,\n", "")
		value = strings.Replace(value, "webhooks: webhookService, files: fileService", "webhooks: webhookService", 1)
	case "internal/httpapi/handler_test.go":
		value = removeImport(value, "appfiles")
		value = strings.Replace(value, "\tcfg.Storage.LocalPath = t.TempDir()\n", "", 1)
		value = removeText(value, "\tobjectStore, err := appfiles.NewStore(ctx, cfg.Storage)", "\thandler, err := NewHandlerWithEngagement(")
		value = strings.ReplaceAll(value, "NewHandlerWithWebhooksAndFiles", "NewHandlerWithWebhooks")
		value = strings.ReplaceAll(value, "webhookService, fileService, \"\", nil", "webhookService, \"\", nil")
	case "internal/web/clients.go":
		value = removeImport(value, "appfiles")
		value = removeLinesContaining(value, "var agreementFiles ")
		value = removeLinesContaining(value, "var agreementFilesAvailable ")
		value = removeText(value, "\t\tif principal.Permissions[identity.ResourcesWrite] {\n\t\t\tfilePage :=", "\t\tif errorKey == \"engagement.invalid_name\" {")
		value = removeLinesContaining(value, "AgreementFiles: agreementFiles.Files")
		value = removeLinesContaining(value, "AgreementFilesPager: newListPager")
		value = removeText(value, "func (h *Handler) agreementFileChoices(", "func (h *Handler) setClientValue(")
	case "internal/web/viewmodel/handler.go":
		value = removeImport(value, "appfiles")
		value = strings.Replace(value, "AgreementFiles             []appfiles.File", "AgreementFiles             []struct{ ID interface{ String() string }; OriginalName string }", 1)
		value = removeLinesContaining(value, "appfiles.File")
	case "internal/web/handler.go":
		value = removeImport(value, "appfiles")
		value = removeLinesContaining(value, "*appfiles.Service")
		value = removeLinesContaining(value, "appfiles.File")
		value = strings.Replace(value, "\tdata.FilesEnabled = h.files != nil\n", "", 1)
		value = removeText(value, "func NewHandlerWithServicesAndFiles(", "func NewHandlerWithServices(")
		value = strings.ReplaceAll(value, "cfg, nil, setupToken, publicURL, mailer", "cfg, setupToken, publicURL, mailer")
		value = strings.ReplaceAll(value, "cfg, fileService, setupToken, publicURL, mailer", "cfg, setupToken, publicURL, mailer")
		value = strings.Replace(value, "\tfileService *appfiles.Service,\n", "", 1)
		value = strings.Replace(value, "identity: identityService, files: fileService, exporter: exporterService", "identity: identityService, exporter: exporterService", 1)
		value = removeBlockIncludingEnd(value, "\tif h.files != nil {", "\t}\n")
		value = strings.Replace(value, "\t\tFilesEnabled: h.files != nil,\n", "", 1)
	case "internal/identity/account_notification_test.go":
		value = removeText(value, "func TestAccountDeletionRetainsOwnedFiles(", "func TestEmailVerificationAndPasswordResetAreSingleUse(")
		value = removeLinesContaining(value, `"database/sql"`)
	case "docs/capabilities.md":
		value = strings.ReplaceAll(value, "[service test](../internal/engagement/agreement_test.go)", "[REST test](../internal/httpapi/engagement_test.go)")
		value = strings.ReplaceAll(value, "stores the internal file key, and the", "stores the agreement flag, and the")
		value = removeLineStarting(value, "| Files |")
		value = removeLineStarting(value, "| Files shared list view |")
		value = removeLineStarting(value, "| Object storage |")
		if start := strings.Index(value, "\n## Files evidence\n"); start >= 0 {
			value = value[:start] + "\n"
		}
	case "docs/web.md":
		value = removeText(value, "- `/workspaces/{workspaceId}/files`", "- `/workspaces/{workspaceId}/settings/export`")
	case "api/openapi.json":
		value = stripFilesOpenAPI([]byte(value))
	}
	return []byte(value)
}

func removeText(value, start, end string) string {
	startAt := strings.Index(value, start)
	if startAt < 0 {
		return value
	}
	endAt := strings.Index(value[startAt:], end)
	if endAt < 0 {
		return value
	}
	return value[:startAt] + value[startAt+endAt:]
}

func removeImport(value, alias string) string {
	needle := "\t" + alias + " \""
	start := strings.Index(value, needle)
	if start < 0 {
		return value
	}
	end := strings.IndexByte(value[start:], '\n')
	if end < 0 {
		return value[:start]
	}
	return value[:start] + value[start+end+1:]
}

func removeLineStarting(value, prefix string) string {
	start := strings.Index(value, prefix)
	if start < 0 {
		return value
	}
	lineStart := strings.LastIndex(value[:start], "\n") + 1
	lineEnd := strings.IndexByte(value[start:], '\n')
	if lineEnd < 0 {
		return value[:lineStart]
	}
	return value[:lineStart] + value[start+lineEnd+1:]
}

func removeBlockIncludingEnd(value, start, end string) string {
	startAt := strings.Index(value, start)
	if startAt < 0 {
		return value
	}
	endAt := strings.Index(value[startAt:], end)
	if endAt < 0 {
		return value
	}
	return value[:startAt] + value[startAt+endAt+len(end):]
}

func stripFilesOpenAPI(raw []byte) string {
	var document map[string]any
	if json.Unmarshal(raw, &document) != nil {
		return string(raw)
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok {
		return string(raw)
	}
	for path := range paths {
		if strings.Contains(path, "/files") {
			delete(paths, path)
		}
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(encoded) + "\n"
}

func agentPath(relative string) bool {
	relative = filepath.ToSlash(relative)
	return relative == "cmd/appctl" || strings.HasPrefix(relative, "cmd/appctl/") ||
		relative == "internal/appctl" || strings.HasPrefix(relative, "internal/appctl/") ||
		relative == "internal/mcpserver" || strings.HasPrefix(relative, "internal/mcpserver/") ||
		relative == "internal/bootstrap/agent_smoke_test.go"
}

const disabledAgent = `package bootstrap

import (
	"net/http"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func registerAgent(
	_ *http.ServeMux,
	_ *identity.Service,
	_ *clients.Service,
	_ config.Config,
) {}
`

func templateIdentity(root string) (string, string, string, error) {
	moduleFile, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", "", "", err
	}
	moduleLine, _, _ := strings.Cut(string(moduleFile), "\n")
	module := strings.TrimSpace(strings.TrimPrefix(moduleLine, "module "))
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		return "", "", "", err
	}
	nameLine, _, _ := strings.Cut(string(readme), "\n")
	name := strings.TrimSpace(strings.TrimPrefix(nameLine, "# "))
	packageFile, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return "", "", "", err
	}
	var metadata struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(packageFile, &metadata) != nil {
		return "", "", "", errors.New("template package metadata is invalid")
	}
	identifier := strings.TrimSuffix(metadata.Name, "-checks")
	if !modulePattern.MatchString(module) || name == "" || !slugPattern.MatchString(identifier) {
		return "", "", "", errors.New("template identity is invalid")
	}
	return module, name, identifier, nil
}

func ignored(relative string) bool {
	first, _, _ := strings.Cut(filepath.ToSlash(relative), "/")
	if first == ".git" || first == ".env" || first == "data" || first == "dist" ||
		first == "node_modules" || first == "template.lock" || first == "PLAN.md" || relative == "cmd/template-init" ||
		strings.HasPrefix(filepath.ToSlash(relative), "cmd/template-init/") {
		return true
	}
	return strings.HasSuffix(relative, ".db") || strings.HasSuffix(relative, ".dump") ||
		strings.HasSuffix(relative, ".backup") || filepath.Base(relative) == ".DS_Store"
}

func validSemver(value string) bool {
	return regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`).MatchString(value)
}

func validHTTPSURL(value string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil &&
		!strings.ContainsAny(value, "\r\n\x00")
}

func validMaintainer(value string) bool {
	return len(value) <= 100 && maintainerPattern.MatchString(value)
}

func parseProfiles(value string) ([]string, error) {
	selected := make(map[string]bool)
	for _, profile := range strings.Split(value, ",") {
		profile = strings.TrimSpace(profile)
		if profile == "" {
			return nil, errors.New("profiles must contain one or more known profiles")
		}
		known := false
		for _, candidate := range knownProfiles {
			if profile == candidate {
				known = true
				break
			}
		}
		if !known || selected[profile] {
			return nil, fmt.Errorf("profile %q is unknown or duplicated", profile)
		}
		selected[profile] = true
	}
	if selected["Catalog"] && !selected["Identity"] {
		return nil, errors.New("Catalog profile requires Identity")
	}
	if selected["Clients"] && (!selected["Identity"] || !selected["Agent"]) {
		return nil, errors.New("Clients profile requires Identity and Agent")
	}
	if selected["Agent"] && !selected["Identity"] {
		return nil, errors.New("Agent profile requires Identity")
	}
	if selected["Files"] && !selected["Identity"] {
		return nil, errors.New("Files profile requires Identity")
	}
	if !selected["Core"] {
		return nil, errors.New("profile set must include Core")
	}
	if !selected["Identity"] {
		return nil, errors.New("profile set must include Identity; generated applications depend on it")
	}
	ordered := make([]string, 0, len(selected))
	for _, profile := range knownProfiles {
		if selected[profile] {
			ordered = append(ordered, profile)
		}
	}
	return ordered, nil
}

func hasProfile(profiles []string, want string) bool {
	for _, profile := range profiles {
		if profile == want {
			return true
		}
	}
	return false
}

func writeLock(path string, lock lockFile) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(lock)
}

func writeGeneratedReadme(path, name, module, slug, repository, securityURL string, agent bool) error {
	agentInterfaces := ""
	if agent {
		agentInterfaces = "- MCP: bounded authenticated tools over the server MCP endpoint — [MCP](docs/mcp.md).\n- Remote CLI: `cmd/appctl` calls REST and never reaches the database — [CLI](docs/cli.md).\n"
	}
	readme := strings.NewReplacer(
		"{{NAME}}", name,
		"{{MODULE}}", module,
		"{{SLUG}}", slug,
		"{{REPOSITORY}}", repository,
		"{{SECURITY_URL}}", securityURL,
		"{{AGENT_INTERFACES}}", agentInterfaces,
		"{{CODE}}", "`",
	).Replace(generatedReadme)
	return os.WriteFile(path, []byte(readme), 0o644)
}

const generatedReadme = `# {{NAME}}

This repository is an application generated from a verified template release.
Repository: [{{REPOSITORY}}]({{REPOSITORY}})
It is a resource-conscious Go modular-monolith starting point with
server-rendered HTML, REST, SQLite by default, and optional PostgreSQL
deployment.

## Purpose

The generator cannot infer your product purpose or domain rules. Replace this
paragraph with the product problem, users, and scope before shipping. The
included item feature is an executable reference slice, not a claim about your
product.

## Start locally

Requirements: Go 1.26 or newer. Go selects the recorded toolchain.

{{CODE}}{{CODE}}{{CODE}}sh
cp .env.example .env
set -a; . ./.env; set +a
go run ./cmd/server
{{CODE}}{{CODE}}{{CODE}}

For the first local owner, set {{CODE}}BOOTSTRAP_ADMIN_EMAIL{{CODE}}, {{CODE}}BOOTSTRAP_ADMIN_WORKSPACE{{CODE}},
and {{CODE}}BOOTSTRAP_ADMIN_PASSWORD{{CODE}} in the environment before starting the server. Keep
the password out of command arguments, files committed to Git, and logs. Open
{{CODE}}http://127.0.0.1:8080/login{{CODE}}; an empty local database also offers the loopback setup flow.

Docker users can run:

{{CODE}}{{CODE}}{{CODE}}sh
export BOOTSTRAP_ADMIN_EMAIL=owner@example.com
export BOOTSTRAP_ADMIN_WORKSPACE='My Workspace'
read -s BOOTSTRAP_ADMIN_PASSWORD; export BOOTSTRAP_ADMIN_PASSWORD
docker compose up --build -d
unset BOOTSTRAP_ADMIN_PASSWORD
{{CODE}}{{CODE}}{{CODE}}

Data persists in the {{CODE}}app-data{{CODE}} volume. See [deployment](docs/deployment.md) for
PostgreSQL, TLS termination, and production boundaries. Health endpoints are
{{CODE}}/health/live{{CODE}} and {{CODE}}/health/ready{{CODE}}; press {{CODE}}Ctrl-C{{CODE}} for graceful shutdown.

## Configuration

Runtime configuration is environment-owned and validated at startup. Read the
complete variable table, secret handling rules, SQLite defaults, PostgreSQL
requirements, OIDC, SMTP, HTTP, rate, SSE, and telemetry settings in
[configuration](docs/configuration.md). Do not commit {{CODE}}.env{{CODE}} or real
credentials.

## Architecture

The application keeps business rules in shared application use cases. Web and
REST adapters call those use cases. The optional Agent profile adds MCP and a
REST-only remote CLI.
Constructors use manual injection, SQLite is the one-instance default, and
PostgreSQL is required before multiple application instances. See
[architecture](docs/architecture.md) and the [documentation router](docs/README.md)
for source-of-truth boundaries.

## Interfaces

- Browser: server-rendered HTML, HTMX fragments, CSRF-protected forms, and
  accessible controls — [web and realtime](docs/web.md).
- REST: bearer-authenticated HTTP API and generated OpenAPI contract —
  [API guide](docs/api.md) and [OpenAPI](api/openapi.json).
{{AGENT_INTERFACES}}- Realtime: scoped, one-way SSE delivery; optional WebMCP only enhances the
  current browser tab and keeps ordinary HTML fallback — [web contract](docs/web.md).

## Operations and data

Read [deployment](docs/deployment.md) before exposing the service and
[operations](docs/operations.md) for health, telemetry, limits, backup,
restore, upgrades, and alerts. [Data lifecycle](docs/data-lifecycle.md)
defines persistence, export, retention, deletion, and recovery boundaries.

## License

This application is licensed under MIT. See [LICENSE](LICENSE) for the
complete terms and [NOTICE](NOTICE) for dependency attribution.

## Security

Read [SECURITY.md](SECURITY.md) for private vulnerability reporting. Report
security issues through [the repository's private channel]({{SECURITY_URL}}), and
operator-owned boundaries. [Authentication](docs/authentication.md) documents
bootstrap, sessions, workspace authorization, invitations, tokens, and OIDC.
Never publish credentials, authorization headers, session or invitation values,
token secrets, database URLs, backups, or signed URLs.

For setup, operation, documentation, and bug triage, use [SUPPORT.md](SUPPORT.md).

## Contributing and release

Use [CONTRIBUTING.md](CONTRIBUTING.md), [AGENTS.md](AGENTS.md), and the
[documentation router](docs/README.md) before changing code. Run {{CODE}}make verify{{CODE}} before
submission, plus the applicable PostgreSQL, accessibility, WebMCP, container,
restore, and security checks.

Release and artifact verification rules live in [release](docs/release.md).
Do not publish tags, credentials, or production changes without the required
authority.

## Add a product feature

Implement each product feature as a reviewed vertical change. Update shared use
cases, routes, authorization, contracts, migrations, tests, navigation, and
documentation together. See [development](docs/development.md#add-a-product-feature).

## Template provenance

{{CODE}}template.lock{{CODE}} records the template source, release version, commit, generation time,
and selected profiles. Read [template lifecycle](docs/template-lifecycle.md)
before updating this application; generate into a new directory and port
changes through reviewed commits instead of overwriting customizations.

The application module is {{CODE}}{{MODULE}}{{CODE}} and its image/telemetry slug is {{CODE}}{{SLUG}}{{CODE}}.
`
