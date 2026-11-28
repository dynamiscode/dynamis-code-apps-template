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

	"example.com/dynamis-code/apps-template/internal/platform/sourceaudit"
)

var (
	commitPattern     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	modulePattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~/-]+$`)
	slugPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	maintainerPattern = regexp.MustCompile(`^@[A-Za-z0-9][A-Za-z0-9-]*(/[A-Za-z0-9][A-Za-z0-9-]*)?$`)
	knownProfiles     = []string{"Core", "Identity", "Agent", "Files", "Clients", "Catalog", "Transactions"}
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
	if err := sourceaudit.VerifyBuildEnvironment(root); err != nil {
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
		hasProfile(selectedProfiles, "Clients"), hasProfile(selectedProfiles, "Catalog"), hasProfile(selectedProfiles, "Transactions"),
		templateName, *name, "Dynamis Code", *name,
		"Copyright (c) 2026 David Londono", "Copyright (c) 2026 "+*name+" contributors",
		templateRepositoryURL, strings.TrimRight(*repository, "/"),
		templateSecurityURL, strings.TrimRight(*securityURL, "/"),
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
	if !hasProfile(selectedProfiles, "Catalog") || !hasProfile(selectedProfiles, "Clients") || !hasProfile(selectedProfiles, "Transactions") {
		command := exec.Command("go", "tool", "templ", "generate", "-path", "internal/web")
		command.Dir = destination
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("regenerate catalog-free templates: %s", strings.TrimSpace(string(output)))
		}
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
	if err := sourceaudit.VerifyBuildEnvironment(destination); err != nil {
		return err
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
	transactions bool,
	dynamicReplacements ...string,
) error {
	replace := strings.NewReplacer(replacements...)
	dynamic := strings.NewReplacer(dynamicReplacements...)
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
		if !transactions && transactionsPath(relative) {
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
		if (!clients || !catalog) && reminderPath(relative) {
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
			value, err := replaceBrandedSource(filepath.ToSlash(relative), string(raw), replace, dynamic)
			if err != nil {
				return fmt.Errorf("generate %s: %w", relative, err)
			}
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
		if !clients || !catalog {
			raw = disableReminderSource(filepath.ToSlash(relative), raw)
		}
		if !clients {
			raw = disableClientsSource(filepath.ToSlash(relative), raw)
		}
		if !catalog {
			raw = disableCatalogSource(filepath.ToSlash(relative), raw)
		}
		if !transactions {
			raw = disableTransactionsSource(filepath.ToSlash(relative), raw)
		}
		if filepath.ToSlash(relative) == "scripts/actioncheck/exceptions.json" && (!agent || !files || !clients || !catalog || !transactions) {
			var exceptions []json.RawMessage
			if err := json.Unmarshal(raw, &exceptions); err != nil {
				return fmt.Errorf("decode actioncheck exceptions: %w", err)
			}
			kept := exceptions[:0]
			for _, exception := range exceptions {
				var record struct {
					Path    string
					OpenTag string
					Line    int
				}
				if err := json.Unmarshal(exception, &record); err != nil {
					return fmt.Errorf("decode actioncheck exception: %w", err)
				}
				if !agent && agentPath(record.Path) || !files && filesPath(record.Path) ||
					!clients && clientsPath(record.Path) || !catalog && catalogPath(record.Path) || !transactions && transactionsPath(record.Path) ||
					(!clients || !catalog) && reminderPath(record.Path) ||
					!clients && record.Path == "internal/web/engagement.templ" && record.OpenTag == `<a class="link lg:hidden" href="#action-history">` {
					continue
				}
				if record.Path == "internal/web/engagement.templ" && (!clients || !files || !catalog || !transactions) {
					source, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(record.Path)))
					if err != nil {
						return fmt.Errorf("read actioncheck exception source: %w", err)
					}
					transformed := source
					if !clients {
						transformed = disableClientsSource(record.Path, transformed)
					}
					if !files {
						transformed = disableFilesSource(record.Path, transformed)
					}
					if !catalog {
						transformed = disableCatalogSource(record.Path, transformed)
					}
					if !transactions {
						transformed = disableTransactionsSource(record.Path, transformed)
					}
					content := string(transformed)
					if !strings.Contains(content, record.OpenTag) {
						continue
					}
					var fields map[string]json.RawMessage
					if err := json.Unmarshal(exception, &fields); err != nil {
						return fmt.Errorf("decode actioncheck exception: %w", err)
					}
					fields["Line"], err = json.Marshal(strings.Count(content[:strings.Index(content, record.OpenTag)], "\n") + 1)
					if err != nil {
						return fmt.Errorf("encode actioncheck exception line: %w", err)
					}
					exception, err = json.Marshal(fields)
					if err != nil {
						return fmt.Errorf("encode actioncheck exception: %w", err)
					}
				}
				if !transactions && record.Path == "internal/web/clients.templ" {
					source, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(record.Path)))
					if err != nil {
						return fmt.Errorf("read actioncheck exception source: %w", err)
					}
					original := string(source)
					if strings.Count(original, record.OpenTag) != 1 || strings.Count(original[:strings.Index(original, record.OpenTag)], "\n")+1 != record.Line {
						return fmt.Errorf("invalid actioncheck exception in %s at line %d", record.Path, record.Line)
					}
					transformed := string(disableTransactionsSource(record.Path, source))
					count := strings.Count(transformed, record.OpenTag)
					if count == 0 {
						continue
					}
					if count != 1 {
						return fmt.Errorf("ambiguous actioncheck exception in %s", record.Path)
					}
					var fields map[string]json.RawMessage
					if err := json.Unmarshal(exception, &fields); err != nil {
						return fmt.Errorf("decode actioncheck exception: %w", err)
					}
					fields["Line"], err = json.Marshal(strings.Count(transformed[:strings.Index(transformed, record.OpenTag)], "\n") + 1)
					if err != nil {
						return fmt.Errorf("encode actioncheck exception line: %w", err)
					}
					exception, err = json.Marshal(fields)
					if err != nil {
						return fmt.Errorf("encode actioncheck exception: %w", err)
					}
				}
				kept = append(kept, exception)
			}
			raw, err = json.MarshalIndent(kept, "", "  ")
			if err != nil {
				return fmt.Errorf("encode actioncheck exceptions: %w", err)
			}
			raw = append(raw, '\n')
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, info.Mode().Perm())
	})
}

func reminderPath(relative string) bool {
	relative = filepath.ToSlash(relative)
	return relative == "internal/notifications/reminder.go" || relative == "internal/scheduling/reminder.go" || relative == "internal/scheduling/reminder_test.go"
}

func disableReminderSource(relative string, raw []byte) []byte {
	value := string(raw)
	switch relative {
	case "internal/bootstrap/app.go":
		value = removeLinesContaining(value, `/internal/notifications"`)
		value = removeLinesContaining(value, "SetEmailChangeHook(")
		value = removeText(value, "\tif err := schedulingService.ReconcileReminders(", "\tcatalogService :=")
	case "internal/scheduling/booking.go", "internal/scheduling/move.go":
		value = strings.ReplaceAll(value, "\tif err := refreshReminder(ctx, tx, workspace, booking, false); err != nil {\n\t\treturn err\n\t}\n", "")
		value = strings.ReplaceAll(value, "\tif err := refreshReminder(ctx, tx, workspace, b.ID, false); err != nil {\n\t\treturn Booking{}, err\n\t}\n", "")
	case "docs/capabilities.md":
		value = removeText(value, "Upcoming booked appointments with an engagement client email", "Resource writers can record")
	}
	return []byte(value)
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

func transactionsPath(relative string) bool {
	relative = filepath.ToSlash(relative)
	return relative == "internal/transactions" || strings.HasPrefix(relative, "internal/transactions/") ||
		strings.HasPrefix(relative, "internal/httpapi/transactions") ||
		strings.HasPrefix(relative, "internal/web/transactions") ||
		strings.HasPrefix(relative, "internal/mcpserver/transactions") ||
		strings.HasPrefix(relative, "internal/browsertest/transactions") ||
		strings.HasPrefix(relative, "internal/web/transaction_create_fields") ||
		strings.HasPrefix(relative, "internal/appctl/transactions") ||
		relative == "internal/portability/transactions_test.go" ||
		relative == "internal/platform/database/transactions_migrate_test.go" ||
		relative == "internal/platform/database/migrations/000057_transactions.sql" ||
		relative == "internal/platform/database/migrations/000058_transactions_client_erasure.sql" ||
		relative == "internal/platform/database/migrations/000059_transactions_corrections.sql" ||
		relative == "internal/platform/database/migrations/000060_transactions_method_names.sql" ||
		relative == "internal/platform/database/migrations/000061_transactions_categories.sql" ||
		relative == "internal/platform/database/migrations/000062_transaction_description.sql" ||
		relative == "internal/platform/database/migrations/000064_transactions_revision.sql" ||
		relative == "internal/platform/database/migrations/000065_transactions_client_revision.sql"
}

func disableTransactionsSource(relative string, raw []byte) []byte {
	value := string(raw)
	switch relative {
	case "internal/bootstrap/app.go":
		value = removeLinesContaining(value, `/internal/transactions"`)
		value = removeLinesContaining(value, "transactionsService :=")
		value = removeLinesContaining(value, "webHandler.EnableTransactions(")
		value = strings.ReplaceAll(value, ", transactionsService,", ",")
	case "internal/bootstrap/agent.go":
		value = removeLinesContaining(value, `/internal/transactions"`)
		value = removeLinesContaining(value, "transactionsService *transactions.Service")
		value = strings.ReplaceAll(value, ", mcpserver.WithTransactions(transactionsService)", "")
	case "internal/httpapi/handler.go":
		value = removeLinesContaining(value, `/internal/transactions"`)
		value = removeLinesContaining(value, "*transactions.Service")
		value = removeLinesContaining(value, "transactions.NewService(")
		value = removeLinesContaining(value, `"listTransactionsEntries":`)
		value = removeLinesContaining(value, `"transactionsTotals":`)
		value = removeLinesContaining(value, `"listTransactionsCategories":`)
		value = removeLinesContaining(value, `"createTransactionsMethod":`)
	case "internal/listviews/service.go":
		value = removeLinesContaining(value, `ListTransactions = "transactions"`)
		value = strings.ReplaceAll(value, "ListClients, ListItems, ListFiles, ListMembers, ListTransactions", "ListClients, ListItems, ListFiles, ListMembers")
		value = strings.ReplaceAll(value, "func canReadList(principal identity.Principal, listKey string) bool {\n\treturn listKey != ListTransactions || identity.CanReadTransactions(principal)\n}", "func canReadList(_ identity.Principal, listKey string) bool {\n\treturn listKey != \"transactions\"\n}")
	case "internal/listviews/service_test.go":
		value = removeText(value, "\ttransactionsView, err :=", "\tif _, err := service.Create(ctx, actor, workspace, \"unknown\"")
		value = removeBlockIncludingEnd(value, "func TestTransactionsViewsRequireTransactionsReadPermission(", "\n}\n")
	case "internal/listprefs/service.go":
		value = removeLinesContaining(value, `ListTransactions = "transactions"`)
		value = strings.ReplaceAll(value, "ListClients, ListItems, ListFiles, ListMembers, ListTransactions", "ListClients, ListItems, ListFiles, ListMembers")
	case "internal/listprefs/service_test.go":
		value = removeText(value, "\tif err := service.Set(ctx, actor, boot.WorkspaceID, ListTransactions", "\tgot, err = service.Get(ctx, actor, boot.WorkspaceID, ListClients")
	case "internal/web/list_views.go":
		value = removeText(value, "\tcase listviews.ListTransactions:\n", "\t}\n")
		value = removeBlockIncludingEnd(value, "\tif listKey == listviews.ListTransactions", "\n\t}\n")
	case "internal/web/handler.go":
		value = removeLinesContaining(value, `/internal/transactions"`)
		value = removeLinesContaining(value, "*transactions.Service")
		value = removeBlockIncludingEnd(value, "\tif h.transactions != nil {", "\t}\n")
	case "internal/web/clients.go":
		value = removeLinesContaining(value, `/internal/transactions"`)
		value = removeBlockIncludingEnd(value, "func clientTransactionsPageURL(", "\n}\n")
		value = removeText(value, "\tvar transactionsEntries []transactions.Entry\n", "\teditDraft := clients.Input")
		value = removeLinesContaining(value, "ClientTransactions: transactionsEntries")
	case "internal/web/clients.templ":
		value = removeLinesContaining(value, `/internal/transactions"`)
		value = removeText(value, "\t\tif data.CanReadClientTransactions {\n", "\t\tif data.EngagementEnabled {\n")
	case "internal/web/localization.go":
		value = removeLinesContaining(value, "data.TransactionsEnabled")
	case "internal/web/viewmodel/handler.go":
		value = removeLinesContaining(value, `/internal/transactions"`)
		value = removeLinesContaining(value, "ClientTransactions")
		value = removeLinesContaining(value, "TransactionsEnabled")
	case "internal/identity/policy.go":
		value = removeText(value, "// CanReadTransactions", "func permissionsForRole")
	case "internal/identity/policy_test.go":
		if start := strings.Index(value, "\nfunc TestTransactionsPolicy("); start >= 0 {
			value = value[:start] + "\n"
		}
	case "internal/web/navigation_test.go":
		value = removeLinesContaining(value, `{"transactions member"`)
		value = removeLinesContaining(value, `{"transactions viewer"`)
		value = removeLinesContaining(value, `{"transactions omitted"`)
	case "internal/web/components/organisms/navigation.templ":
		value = removeBlockIncludingEnd(value, "\t\t\t\t\tif data.TransactionsEnabled", "\t\t\t\t\t}\n")
		value = removeBlockIncludingEnd(value, "\t\t\t\tif data.TransactionsEnabled", "\t\t\t\t}\n")
	case "internal/web/components/organisms/navigation_templ.go":
		value = removeBlockIncludingEnd(value, "\n\t\t\t\tif data.TransactionsEnabled", "\n\t\t\t\t}\n")
		value = removeBlockIncludingEnd(value, "\n\t\t\tif data.TransactionsEnabled", "\n\t\t\t}\n")
	case "internal/mcpserver/server.go":
		value = removeLinesContaining(value, `"transactions_create_v1":`)
		value = removeLinesContaining(value, `"transactions_category_`)
		value = removeLinesContaining(value, `"transactions_method_create_v1":`)
		value = removeLinesContaining(value, `"transactions_method_update_v1":`)
		value = removeLinesContaining(value, `"transactions_void_v1":`)
	case "internal/appctl/run.go":
		value = strings.ReplaceAll(value, "<clients|fields|transactions>", "<clients|fields>")
		value = removeBlockIncludingEnd(value, "\tif args[0] == \"transactions\" {", "\t}\n")
	case "api/openapi.json":
		var document map[string]any
		if json.Unmarshal(raw, &document) == nil {
			if paths, ok := document["paths"].(map[string]any); ok {
				delete(paths, "/api/v1/workspaces/{workspaceId}/transactions/entries")
				delete(paths, "/api/v1/workspaces/{workspaceId}/transactions/entries/{entryId}/void")
				delete(paths, "/api/v1/workspaces/{workspaceId}/transactions/methods")
				delete(paths, "/api/v1/workspaces/{workspaceId}/transactions/categories")
				delete(paths, "/api/v1/workspaces/{workspaceId}/transactions/totals")
				delete(paths, "/api/v1/workspaces/{workspaceId}/transactions/categories/{categoryId}")
				if encoded, err := json.MarshalIndent(document, "", "  "); err == nil {
					value = string(encoded) + "\n"
				}
			}
		}
	case "docs/capabilities.md":
		value = removeText(value, "## Transactions filters and client history (#633)", "## Scheduling resources (#579)")
		value = removeLineStarting(value, "| Transactions |")
	case "docs/architecture.md":
		value = removeText(value, "Transactions is optional and requires Identity and Clients.", "\n\nCompanySite")
	}
	return []byte(value)
}

func catalogPath(relative string) bool {
	relative = filepath.ToSlash(relative)
	return relative == "api/catalog_contract_test.go" || relative == "api/catalog_authorization_contract_test.go" || relative == "internal/catalog" || strings.HasPrefix(relative, "internal/catalog/") ||
		relative == "internal/httpapi/catalog.go" || relative == "internal/httpapi/catalog_test.go" || relative == "internal/httpapi/catalog_authorization_test.go" || relative == "internal/httpapi/catalog_isolation_test.go" ||
		relative == "internal/web/catalog.go" || relative == "internal/web/catalog_test.go" || relative == "internal/web/catalog_authorization_test.go" || relative == "internal/web/catalog_isolation_test.go" ||
		relative == "internal/platform/database/migrate_workspace_currencies_postgres_test.go" ||
		relative == "internal/platform/database/migrate_catalog_currencies_test.go" ||
		relative == "internal/web/catalog.templ" || relative == "internal/web/catalog_templ.go" ||
		relative == "internal/scheduling/booking.go" || relative == "internal/scheduling/move.go" || relative == "internal/scheduling/hours_bounds_test.go" || relative == "internal/platform/database/migrations/000048_scheduling_bookings.sql" ||
		relative == "internal/platform/database/migrations/000049_scheduling_engagement.sql" || relative == "internal/platform/database/migrations/000050_scheduling_booking_moves.sql" ||
		relative == "internal/platform/database/migrations/000040_catalog.sql" ||
		relative == "internal/platform/database/migrations/000041_catalog_prices.sql" ||
		relative == "internal/platform/database/migrations/000056_catalog_entry_revision.sql" ||
		relative == "internal/platform/database/migrations/000045_catalog_currencies.sql"
}

func disableCatalogSource(relative string, raw []byte) []byte {
	value := string(raw)
	switch relative {
	case "internal/bootstrap/app.go", "internal/httpapi/handler.go", "internal/web/handler.go", "internal/web/handler_test.go":
		if relative == "internal/httpapi/handler.go" {
			value = removeText(value, "func NewHandlerWithCatalog(", "func NewHandlerWithCatalogAndEngagement(")
			value = strings.ReplaceAll(value, ", catalog.NewService(db, identityService, scheduling.CatalogEntryInUse)", "")
			value = removeLinesContaining(value, `"listSchedulingBookings":`)
			value = strings.ReplaceAll(value, "NewHandlerWithCatalogAndEngagement", "NewHandlerWithEngagementAndIntake")
			value = strings.ReplaceAll(value, ", catalog.NewService(db, identityService, nil)", "")
			value = strings.ReplaceAll(value, "catalogService *catalog.Service, ", "")
			value = strings.ReplaceAll(value, "catalogService, ", "")
		}
		if relative == "internal/bootstrap/app.go" {
			value = strings.ReplaceAll(value, "httpapi.NewHandlerWithCatalogAndEngagement", "httpapi.NewHandlerWithEngagementAndIntake")
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
		value = removeLinesContaining(value, "h.schedulingBook)")
		value = removeLinesContaining(value, "h.schedulingBookingStatus)")
		value = removeLinesContaining(value, "h.schedulingSetEngagement)")
		value = removeLinesContaining(value, `"setSchedulingEngagement":`)
		value = removeLinesContaining(value, `"setSchedulingBookingStatus":`)
		value = removeLinesContaining(value, `"moveSchedulingBooking":`)
	case "internal/platform/database/migrate_workspace_currencies_test.go":
		value = removeText(value, "func TestWorkspaceCurrenciesMigrationPreservesCatalog(", "func TestWorkspaceCurrenciesMigrationWithoutCatalog(")
	case "internal/httpapi/scheduling.go":
		value = removeText(value, "func (h *handler) moveSchedulingBooking(", "func (h *handler) listSchedulingResources(")
		value = removeText(value, "\tcase errors.Is(err, scheduling.ErrNoDuration):", "\tcase err != nil:")
		value = removeLinesContaining(value, `"time"`)
		value = removeLinesContaining(value, `/internal/catalog"`)
		value = removeLinesContaining(value, `/internal/engagement"`)
		value = removeLinesContaining(value, `"encoding/json"`)
	case "internal/httpapi/scheduling_test.go":
		value = removeText(value, "func TestSchedulingBookingsREST(", "func TestSchedulingRESTReadOnlyRoles(")
		value = removeText(value, "\tclient, err := clients.NewService", "\tbase := \"/api/v1/workspaces/\" + workspace")
		value = removeText(value, "\tbase := \"/api/v1/workspaces/\" + workspace", "\tfor _, role := range")
		value = removeText(value, "\t\tif role == identity.Viewer {\n\t\t\tbefore := serveAuthorized", "\t\tafter := serveAuthorized(h, http.MethodGet, path,")
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, `/internal/engagement"`)
	case "internal/scheduling/service_test.go":
		value = removeText(value, "func TestBookingTransitions(", "func TestResourceAndWorkspaceTimeZone(")
		value = removeText(value, "\tduration := 60\n\tentry, err := catalog.NewService", "\tfor _, role := range")
		value = removeText(value, "\t\tif role == identity.Member || role == identity.Viewer {", "\t\tif role == identity.Member {")
		value = removeLinesContaining(value, `/internal/catalog"`)
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, `/internal/engagement"`)
		value = removeLinesContaining(value, `"path/filepath"`)
	case "internal/scheduling/postgres_test.go":
		value = removeText(value, "\tif err := s.SetTimeZone(ctx, actor, owner.WorkspaceID, \"UTC\");", "\n}")
	case "internal/web/scheduling_test.go":
		value = removeText(value, "func TestSchedulingBookingWeb(", "func TestSchedulingResourcesWeb(")
		value = removeText(value, "\tduration := 60\n\tentry, err := catalog.NewService", "\tinv, err := auth.CreateInvitation")
		value = removeText(value, "\tif !strings.Contains(page.Body.String(), \"Move history\")", "\tif page.Code != http.StatusOK")
		value = removeLinesContaining(value, `/internal/catalog"`)
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, `/internal/engagement"`)
		value = removeLinesContaining(value, `/internal/listprefs"`)
		value = removeLinesContaining(value, `/internal/platform/config"`)
	case "internal/web/scheduling.go":
		value = removeText(value, "func (h *Handler) schedulingBookingStatus(", "type schedulingDraft struct")
		value = removeText(value, "\tbookings := make(map[scheduling.ResourceID]", "\tworkspaces, err :=")
		value = removeText(value, "func (h *Handler) schedulingBook(", "func (h *Handler) schedulingCreate(")
		value = strings.ReplaceAll(value, "members, bookings, history, entries, engagements, identity.CanManageScheduling(principal), principal.Permissions[identity.ResourcesWrite],", "members, identity.CanManageScheduling(principal),")
		value = strings.ReplaceAll(value, "members, bookings, history, entries, identity.CanManageScheduling(principal), principal.Permissions[identity.ResourcesWrite],", "members, identity.CanManageScheduling(principal),")
		value = removeLinesContaining(value, `/internal/catalog"`)
		value = removeLinesContaining(value, `/internal/engagement"`)
		value = removeLinesContaining(value, `"time"`)
	case "internal/web/scheduling.templ":
		value = removeLinesContaining(value, `/internal/catalog"`)
		value = removeText(value, "templ schedulingEngagement(", "templ schedulingPage(")
		value = strings.ReplaceAll(value, "bookings map[scheduling.ResourceID][]scheduling.Booking, history map[scheduling.BookingID][]scheduling.BookingMove, entries []catalog.Entry, engagements []scheduling.EngagementOption, canWrite bool, canBook bool,", "canWrite bool,")
		value = strings.ReplaceAll(value, "bookings map[scheduling.ResourceID][]scheduling.Booking, history map[scheduling.BookingID][]scheduling.BookingMove, entries []catalog.Entry, canWrite bool, canBook bool,", "canWrite bool,")
		value = removeText(value, "    if draft.action == \"booking\" || draft.action == \"move\" {", "    } else if draft.action == \"zone\" {")
		value = strings.Replace(value, "    } else if draft.action == \"zone\" {", "    if draft.action == \"zone\" {", 1)
		value = removeText(value, "      <h2 class=\"font-semibold\">{ data.T(\"scheduling.bookings\") }</h2>", "      <p>{ data.T(\"scheduling.hours\") }</p>")
	case "internal/web/viewmodel/handler.go":
		value = removeLinesContaining(value, `/internal/scheduling"`)
		value = removeLinesContaining(value, "EngagementBookings")
	case "internal/web/clients.go":
		value = removeLinesContaining(value, `/internal/scheduling"`)
		value = removeLinesContaining(value, "engagementBookings :=")
		value = removeText(value, "\tif h.scheduling != nil && request.PathValue(\"engagementId\") != \"\" && len(engagements) > 0 {", "\townerError := false")
		value = strings.ReplaceAll(value, "EngagementBookings: engagementBookings, ", "")
	case "internal/web/engagement.templ":
		value = removeText(value, "\n\t\t\t\tif len(data.EngagementBookings[item.ID]) > 0 {", "\n\t\t\t</section>")
	case "docs/capabilities.md":
		value = removeLinesContaining(value, "000049_scheduling_engagement.sql")
		value = removeLinesContaining(value, "000050_scheduling_booking_moves.sql")
		value = removeLineStarting(value, "| Catalog |")
		value = removeText(value, "## Catalog authorization and isolation (#597)", "\n## ")
		value = removeText(value, "## Scheduling bookings (#588)", "## Engagement stages")
	case "docs/architecture.md":
		value = removeLineStarting(value, "Catalog is optional")
	case "docs/data-lifecycle.md":
		value = removeLineStarting(value, "| catalog / `catalog_entries`")
		value = removeLineStarting(value, "| catalog / `catalog_entry_revision_seeds`")
		value = removeLineStarting(value, "| scheduling / `scheduling_bookings`")
		value = removeLineStarting(value, "| catalog / `catalog_currencies`")
	case "docs/api.md", "docs/web.md":
		value = removeLineStarting(value, "Catalog profile:")
	case "docs/operations.md":
		value = removeLineStarting(value, "Catalog currency migration 000045")
		value = removeLineStarting(value, "Catalog revision migration 000056")
	case "internal/web/localization.go":
		value = removeLinesContaining(value, "h.catalogService")
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
				delete(paths, "/api/v1/workspaces/{workspaceId}/scheduling/resources/{resourceId}/bookings")
				delete(paths, "/api/v1/workspaces/{workspaceId}/scheduling/bookings/{bookingId}/engagement")
				delete(paths, "/api/v1/workspaces/{workspaceId}/scheduling/resources/{resourceId}/bookings/{bookingId}/move")
				delete(paths, "/api/v1/workspaces/{workspaceId}/scheduling/resources/{resourceId}/bookings/{bookingId}/history")
				delete(paths, "/api/v1/workspaces/{workspaceId}/scheduling/resources/{resourceId}/bookings/{bookingId}/status")
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
		relative == "internal/web/files_stream_test.go" ||
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
		relative == "internal/platform/database/migrations/000049_scheduling_engagement.sql" ||
		relative == "internal/intake" || strings.HasPrefix(relative, "internal/intake/") ||
		relative == "internal/httpapi/intake.go" || relative == "internal/httpapi/intake_test.go" ||
		relative == "internal/web/intake.go" || relative == "internal/web/intake_test.go" || relative == "internal/web/intake.templ" || relative == "internal/web/intake_templ.go" ||
		relative == "internal/web/public_intake.go" || relative == "internal/web/public_intake.templ" || relative == "internal/web/public_intake_templ.go" ||
		relative == "internal/platform/database/migrations/000042_public_intake.sql" || relative == "internal/platform/database/migrations/000055_public_intake_encrypted_token.sql" ||
		relative == "internal/identity/engagement_membership_test.go" ||
		relative == "internal/engagement/engagement.go" || relative == "internal/engagement/list.go" || relative == "internal/engagement/list_test.go" ||
		relative == "internal/engagement/action_queue.go" || relative == "internal/engagement/action_queue_test.go" || relative == "internal/engagement/dashboard.go" ||
		relative == "internal/engagement/agreement.go" ||
		relative == "internal/engagement/agreement_test.go" ||
		relative == "internal/engagement/assignee.go" || relative == "internal/engagement/next_action.go" || relative == "internal/engagement/quote.go" ||
		relative == "internal/engagement/stage_move_test.go" ||
		relative == "internal/platform/database/migrations/000033_engagements.sql" ||
		relative == "internal/platform/database/migrations/000039_engagement_agreement.sql" ||
		relative == "internal/platform/database/migrations/000051_engagement_names.sql" ||
		relative == "internal/platform/database/migrations/000052_engagement_action_history.sql" ||
		relative == "internal/platform/database/migrations/000053_engagement_action_completion.sql" ||
		relative == "internal/platform/database/migrations/000054_engagement_action_version.sql" ||
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
		relative == "internal/web/clients.go" || relative == "internal/web/clients_test.go" || relative == "internal/web/clients_scope_test.go" || relative == "internal/web/clients_intake_menu_test.go" ||
		relative == "internal/web/clients_stream_test.go" ||
		relative == "internal/web/engagement_test.go" || relative == "internal/web/engagement_dashboard_test.go" || relative == "internal/web/action_queue_test.go" ||
		relative == "internal/web/engagement_notes.go" || relative == "internal/web/engagement_notes_test.go" ||
		relative == "internal/web/client_delete.go" || relative == "internal/web/client_delete.templ" || relative == "internal/web/client_delete_templ.go" || relative == "internal/web/client_inline_field_test.go" ||
		relative == "internal/web/engagement_new.go" || relative == "internal/web/engagement_new.templ" || relative == "internal/web/engagement_new_templ.go" || relative == "internal/web/engagement_new_search_test.go" || relative == "internal/web/engagement_new_return_test.go" ||
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
		relative == "internal/platform/database/migrations/000043_client_intake_consent.sql" ||
		relative == "internal/platform/database/migrations/000044_client_possible_duplicate.sql" ||
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
		value = removeText(value, "\t\tif migration.version == 51 {", "\t\tif migration.version == 63 {")
		value = removeText(value, "func backfillEngagementNames(", "// renderMigration")
	case "internal/identity/account.go":
		value = removeText(value, "\tif _, err := s.exec(ctx, tx, \"UPDATE engagements SET action_version", "\tresult, err := s.exec(ctx, tx, \"DELETE FROM users WHERE id = ?\", userID)")
	case "internal/scheduling/booking.go":
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, `/internal/engagement"`)
		value = removeLinesContaining(value, "EngagementID *engagement.ID")
		value = removeText(value, "type EngagementOption struct", "func (s *Service) SetBookingStatus(")
		value = strings.ReplaceAll(value, ", b.engagement_id FROM scheduling_bookings", " FROM scheduling_bookings")
		value = strings.ReplaceAll(value, ", &b.EngagementID", "")
		value = removeText(value, "func (s *Service) Book(ctx", "func (s *Service) BookWithEngagement(")
		value = strings.ReplaceAll(value, "BookWithEngagement(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, resource ResourceID, entry catalog.EntryID, start time.Time, link *engagement.ID)", "Book(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, resource ResourceID, entry catalog.EntryID, start time.Time)")
		value = strings.ReplaceAll(value, "resource, entry, start, link)", "resource, entry, start)")
		value = strings.ReplaceAll(value, "start time.Time, link *engagement.ID)", "start time.Time)")
		value = strings.ReplaceAll(value, ", EngagementID: link", "")
		value = removeText(value, "\tif err := validateEngagement(", "\t// Lock the resource")
		value = strings.ReplaceAll(value, ", status, engagement_id) VALUES ($1, $2, $3, $4, $5, $6, $7)", ", status) VALUES ($1, $2, $3, $4, $5, $6)")
		value = strings.ReplaceAll(value, ", b.Status, link).Scan", ", b.Status).Scan")
		if start := strings.Index(value, "func validateEngagement("); start >= 0 {
			value = value[:start]
		}
	case "internal/scheduling/service_test.go":
		value = removeText(value, "\tclient, err := clients.NewService", "\tif _, err := s.Book(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start.Add(30*time.Minute))")
		value = strings.Replace(value, "\tif _, err := s.Book(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start.Add(30*time.Minute))", "\tif _, err := s.Book(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start.Add(time.Hour)); err != nil {\n\t\tt.Fatal(err)\n\t}\n\tif _, err := s.Book(ctx, actor, owner.WorkspaceID, r.ID, entry.ID, start.Add(30*time.Minute))", 1)
		value = removeText(value, "\tif err := clients.NewService", "\tfor _, outcome := range")
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, `/internal/engagement"`)
		value = removeText(value, "\t\t\t} else if err := s.SetEngagement(", "\t\t\t} else if role == identity.Viewer {")
	case "internal/httpapi/scheduling_test.go":
		value = removeText(value, "\tctx := context.Background()\n\tauth, err := identity.NewService", "\tbase := \"/api/v1/workspaces/\" + workspace")
		value = removeText(value, "\tclient, err := clients.NewService", "\tbase := \"/api/v1/workspaces/\" + workspace")
		value = removeText(value, "\t\t\tif response := serveAuthorized(h, http.MethodPut, base+\"/scheduling/bookings/\"+booking.ID+\"/engagement\"", "\t\t\tif response := serveAuthorized(h, http.MethodPut, bookingsPath+\"/\"+booking.ID+\"/status\"")
		value = strings.Replace(value, "h, db, workspace, token := testHandler(t)", "h, _, workspace, token := testHandler(t)", 1)
		value = removeText(value, "\tresponse = serveAuthorized(h, http.MethodPut, base+\"/scheduling/bookings/\"+booking.ID+\"/engagement\"", "\tresponse = serveAuthorized(h, http.MethodPost, path+\"/bookings\", body")
		value = removeText(value, "\t\t\tif response := serveAuthorized(h, http.MethodPut, \"/api/v1/workspaces/\"+workspace+\"/scheduling/bookings/invalid/engagement\"", "\t\t\tif response := serveAuthorized(h, http.MethodPut, path+\"/\"+resource.ID+\"/bookings/invalid/status\"")
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, `/internal/engagement"`)
	case "internal/web/scheduling_test.go":
		value = removeText(value, "\tes := engagement.NewService", "\tactor, err := auth.Authorize")
		value = removeText(value, "\tclient, err := clients.NewService", "\tr, err := s.Create")
		value = strings.Replace(value, `, "engagement_id": {eng.ID.String()}`, "", 1)
		value = strings.Replace(value, ` || !strings.Contains(page.Body.String(), eng.ID.String())`, "", 1)
		value = strings.Replace(value, " `name=\"engagement_id\" data-entry-picker`,", "", 1)
		value = removeText(value, "\tclientPage := request", "\tbookings, _, err := s.Schedule")
		value = removeText(value, "\tengagementPath := path +", "\tinvalidMove := request")
		value = removeText(value, "\tengagementPage = request", "\tinvalidMove = request")
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, `/internal/engagement"`)
		value = removeLinesContaining(value, `/internal/listprefs"`)
		value = removeLinesContaining(value, `/internal/platform/config"`)
	case "internal/httpapi/scheduling.go":
		value = removeLinesContaining(value, `/internal/engagement"`)
		value = removeLinesContaining(value, `"encoding/json"`)
		value = removeLinesContaining(value, "EngagementID *string")
		value = removeLinesContaining(value, "link := schedulingEngagementID")
		value = strings.ReplaceAll(value, "BookWithEngagement(r.Context(), principal, workspace, resource, catalog.ParseEntryID(body.EntryID), body.Start, link)", "Book(r.Context(), principal, workspace, resource, catalog.ParseEntryID(body.EntryID), body.Start)")
		value = removeText(value, "func schedulingEngagementID(", "func (h *handler) listSchedulingResources(")
	case "internal/web/scheduling.go":
		value = removeLinesContaining(value, `/internal/engagement"`)
		value = removeText(value, "\tvar engagements []scheduling.EngagementOption", "\tif h.catalogService != nil")
		value = strings.ReplaceAll(value, "entries, engagements, identity.CanManageScheduling", "entries, identity.CanManageScheduling")
		value = strings.ReplaceAll(value, "BookWithEngagement(r.Context(), principal, workspace, scheduling.ParseResourceID(r.PathValue(\"resourceId\")), catalog.ParseEntryID(r.FormValue(\"entry_id\")), start, webEngagementID(r.FormValue(\"engagement_id\")))", "Book(r.Context(), principal, workspace, scheduling.ParseResourceID(r.PathValue(\"resourceId\")), catalog.ParseEntryID(r.FormValue(\"entry_id\")), start)")
		value = removeText(value, "func webEngagementID(", "func (h *Handler) schedulingCreate(")
	case "internal/web/scheduling.templ":
		value = removeText(value, "templ schedulingEngagement(", "templ schedulingPage(")
		value = strings.ReplaceAll(value, "engagements []scheduling.EngagementOption, ", "")
		value = removeText(value, "          for _, option := range engagements {", "          if canBook && booking.Status")
		value = removeText(value, "         if len(engagements) > 0 {", "         @molecules.EntryField(molecules.EntryFieldProps{ID: \"booking-new-start-")
	case "internal/identity/service.go":
		value = removeText(value, "func (s *Service) clearMemberAssignments(", "func normalizeEmail(")
		value = strings.Replace(value, "func normalizeEmail(", "func (s *Service) clearMemberAssignments(ctx context.Context, tx *sql.Tx, workspaceID WorkspaceID, userID UserID) error { _, err := s.exec(ctx, tx, \"UPDATE scheduling_resources SET user_id = NULL WHERE workspace_id = ? AND user_id = ?\", workspaceID, userID); return err }\n\nfunc normalizeEmail(", 1)
	case "cmd/demo/main.go":
		value = strings.Replace(value, "workspaceID, userID, err := demoOwner(", "workspaceID, _, err := demoOwner(", 1)
		value = removeBlockIncludingEnd(value, "\tif err := seedClients(", "\t}\n")
	case "internal/bootstrap/app.go":
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, `/internal/intake"`)
		value = removeLineStarting(value, "\tintakeService := intake.NewService(")
		value = removeLineStarting(value, "\tintakeService.ConfigureNotifications(")
		value = removeLineStarting(value, "\tintakeService.ConfigureEncryption(")
		value = strings.ReplaceAll(value, ", intakeService,", ",")
		value = removeLineStarting(value, "\twebHandler.EnableIntake(")
		value = removeLineStarting(value, "\tclientService := clients.NewService(")
		// The portability export takes the client source as its last argument.
		value = strings.ReplaceAll(value, "\t\tclientService,\n", "\t\tnil,\n")
		value = strings.ReplaceAll(value, " clientService,", "")
		value = removeLineStarting(value, "\twebHandler.EnableClients(")
	case "internal/httpapi/handler.go":
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, `"setSchedulingEngagement":`)
		value = removeLinesContaining(value, `/internal/intake"`)
		value = removeLinesContaining(value, "intake         *intake.Service")
		value = strings.ReplaceAll(value, "intakeService *intake.Service, clientServices", "clientServices")
		value = removeLinesContaining(value, "\tintakeService *intake.Service,")
		value = strings.ReplaceAll(value, "engagementService, intakeService, clientServices", "engagementService, clientServices")
		value = strings.ReplaceAll(value, "engagementService, nil, clientServices", "engagementService, clientServices")
		value = removeLinesContaining(value, "intake:         intakeService")
		value = removeLinesContaining(value, `"submitPublicIntake":`)
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
		value = removeLinesContaining(value, "h.getClient")
		value = removeLinesContaining(value, "h.updateClient")
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
	case "internal/engagement/stage_move.go":
		value = strings.Replace(value, "(SELECT COUNT(*) FROM engagements e WHERE e.workspace_id = s.workspace_id AND e.stage_id = s.id)", "0", 1)
		value = strings.Replace(value, "SELECT COUNT(*) FROM engagements WHERE workspace_id = ? AND stage_id = ?", "SELECT 0 WHERE ? IS NOT NULL AND ? IS NOT NULL", 1)
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
		value = removeLinesContaining(value, `/internal/scheduling"`)
		value = removeLinesContaining(value, "EngagementBookings")
		value = removeLinesContaining(value, "engagement.Engagement")
		value = removeLinesContaining(value, "engagement.DashboardPage")
		value = removeLinesContaining(value, "engagement.ActionHistoryEntry")
		value = removeLinesContaining(value, "engagement.ActivityEntry")
		value = removeLinesContaining(value, "DashboardPrevious ")
		value = removeLinesContaining(value, "DashboardNext ")
		value = removeLinesContaining(value, "engagement.Stage")
		value = removeBlockIncludingEnd(value, "type TimelineDraft struct {", "}\n")
		value = removeLinesContaining(value, "TimelineDraft ")
		value = removeLinesContaining(value, "clients.TimelineEntry")
		value = removeLinesContaining(value, "EngagementQuoteDraft")
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, "clients.FilterInput")
		value = removeLinesContaining(value, "clients.Client")
		value = removeLinesContaining(value, "clients.Input")
		value = removeLinesContaining(value, "clients.Field")
		value = removeLinesContaining(value, "clients.Option")
		value = removeLinesContaining(value, "ClientQueryForm")
		value = removeLinesContaining(value, "ClientFieldRow")
		value = removeLinesContaining(value, "ClientValueRow")
	case "internal/web/navigation_test.go":
		value = removeText(value, "func TestAvatarUsesSessionProfileAcrossRenderPaths(", "func TestCatalogWorkspaceNavigation(")
		value = removeLinesContaining(value, `"net/http"`)
	case "internal/web/handler.go":
		value = removeText(value, "\tif h.engagement != nil && principal.Permissions[identity.ResourcesRead] {", "\th.render(writer, http.StatusOK, \"workspace-home\", data)")
		value = removeLinesContaining(value, `"strconv"`)
		value = removeLinesContaining(value, "CanEditClients: principal.Permissions")
		value = strings.Replace(value, "principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceRead)", "_, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WorkspaceRead)", 1)
		value = removeLinesContaining(value, `/internal/clients"`)
		value = removeLinesContaining(value, "h.schedulingSetEngagement)")
		value = removeLinesContaining(value, `/internal/intake"`)
		value = removeLinesContaining(value, "intake          *intake.Service")
		value = removeLinesContaining(value, "h.publicIntake")
		value = removeLinesContaining(value, "h.intakePage")
		value = removeLinesContaining(value, "h.intakeMutation")
		value = removeLinesContaining(value, "h.bulkClients")
		value = removeLinesContaining(value, "*clients.Service")
		value = removeLinesContaining(value, "clients.FilterInput")
		value = removeLinesContaining(value, "clients.Client")
		value = removeLinesContaining(value, "clients.Input")
		value = removeLinesContaining(value, "h.getClient")
		value = removeLinesContaining(value, "h.updateClient")
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
		value = removeLinesContaining(value, "/workspaces/{workspaceId}/engagements/new")
		value = removeLinesContaining(value, "/workspaces/{workspaceId}/engagements/notes-preview")
		value = removeLinesContaining(value, "/workspaces/{workspaceId}/engagements/board")
		value = removeLinesContaining(value, "/workspaces/{workspaceId}/engagements/due")
		value = removeLinesContaining(value, "/workspaces/{workspaceId}/engagements/bulk-move")
		value = removeLinesContaining(value, "h.changeQueuedAction")
	case "internal/web/clients_scope.go":
		value = removeLinesContaining(value, `"strconv"`)
		value = removeText(value, "func clientTimelineURL(", "func clientReturnHref(")
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
		value = removeText(value, "func (h *Handler) setEngagementAgreement(", "func (h *Handler) setEngagementAssignee(")
		value = removeText(value, "func (h *Handler) setEngagementAssignee(", "func (h *Handler) EnableEngagement(")
		value = removeText(value, "func (h *Handler) moveEngagement(", "func (h *Handler) setEngagementLabel(")
		value = removeText(value, "func (h *Handler) createEngagement(", "func (h *Handler) engagementStages(")
	case "internal/web/engagement.templ":
		value = removeLinesContaining(value, `import "strings"`)
		value = removeLinesContaining(value, `/internal/identity"`)
		value = removeLinesContaining(value, `/internal/web/components/actionattrs"`)
		value = removeText(value, "templ engagementDetailPage(", "templ engagementStagesPage(")
	case "internal/web/engagement_templ.go":
		value = removeLinesContaining(value, `import "strings"`)
		value = removeLinesContaining(value, `/internal/identity"`)
		value = removeLinesContaining(value, `/internal/web/components/actionattrs"`)
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
		value = removeLineStarting(value, "| clients / `possible_duplicate` |")
		value = removeLineStarting(value, "| intake / `public_intake_links` |")
	case "docs/mcp.md":
		value = removeText(value, "\n## Clients tools (Clients profile)", "\n## Prompts\n")
	case "docs/capabilities.md":
		value = removeLinesContaining(value, "000049_scheduling_engagement.sql")
		value = strings.ReplaceAll(value, "[web](../internal/web/engagement_test.go)", "web")
		value = removeText(value, " Writers can create engagements", "\nOwners and admins can set or reset")
		value = removeText(value, "\nNext actions have text", "\n\n## Client timeline REST")
		value = removeLineStarting(value, "| Clients |")
		value = removeLineStarting(value, "| Public intake |")
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
		value = removeText(value, "\nOwners and admins manage the public intake", "\nmatches another client in the workspace by email or phone.")
		value = removeLineStarting(value, "matches another client in the workspace by email or phone.")
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
		if strings.Contains(path, "/clients") || strings.Contains(path, "/client-fields") || strings.Contains(path, "/engagements") || strings.Contains(path, "/intake/") {
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
	delete(paths, "/api/v1/workspaces/{workspaceId}/scheduling/bookings/{bookingId}/engagement")
	bookingRequest := nestedObject(paths, "/api/v1/workspaces/{workspaceId}/scheduling/resources/{resourceId}/bookings", "post", "requestBody", "content", "application/json", "schema", "properties")
	delete(bookingRequest, "engagementId")
	booking := nestedObject(document, "components", "schemas", "SchedulingBooking")
	delete(nestedObject(booking, "properties"), "engagementId")
	if required, ok := booking["required"].([]any); ok {
		for i, field := range required {
			if field == "engagementId" {
				booking["required"] = append(required[:i], required[i+1:]...)
				break
			}
		}
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(encoded) + "\n"
}

func nestedObject(value map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		value, _ = value[key].(map[string]any)
	}
	return value
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
		value = strings.ReplaceAll(value, "fileService *appfiles.Service, ", "")
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
		value = removeText(value, "\t\tif principal.Permissions[identity.ResourcesWrite] {\n\t\t\tfilePage :=", "\n\t}\n\th.renderClientPage(")
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
	if selected["Transactions"] && (!selected["Identity"] || !selected["Clients"]) {
		return nil, errors.New("Transactions profile requires Identity and Clients")
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
