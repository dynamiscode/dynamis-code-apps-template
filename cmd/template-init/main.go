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
	knownProfiles     = []string{"Core", "Identity", "Agent", "Files"}
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
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("template symlink is unsupported: %s", relative)
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
				value = strings.ReplaceAll(value, "- github.com/modelcontextprotocol/go-sdk v1.7.0 — Apache-2.0 and MIT code\n", "")
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
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, info.Mode().Perm())
	})
}

func filesPath(relative string) bool {
	relative = filepath.ToSlash(relative)
	return relative == "internal/files" || strings.HasPrefix(relative, "internal/files/") ||
		relative == "internal/httpapi/files.go" || relative == "internal/httpapi/files_test.go" || relative == "internal/web/files.go" || relative == "internal/web/files_test.go" ||
		relative == "internal/web/templates/files.html" ||
		relative == "internal/platform/database/migrations/000014_files.sql" ||
		relative == "internal/platform/database/migrate_files_test.go"
}

func disableFilesSource(relative string, raw []byte) []byte {
	value := string(raw)
	switch relative {
	case "internal/bootstrap/app.go":
		value = removeImport(value, "appfiles")
		value = strings.Replace(value, "\tFiles       *appfiles.Service\n", "", 1)
		value = removeText(value, "\tstorageConfig := cfg.Storage", "\titemService := items.NewService(")
		value = strings.ReplaceAll(value, "httpapi.NewHandlerWithWebhooksAndFiles", "httpapi.NewHandlerWithWebhooks")
		value = strings.ReplaceAll(value, "web.NewHandlerWithServicesAndFilesAndSharing", "web.NewHandlerWithServices")
		value = strings.ReplaceAll(value, "web.NewHandlerWithServicesAndFiles", "web.NewHandlerWithServices")
		value = strings.ReplaceAll(value, "\t\twebhookService, fileService, cfg.PublicURL, mailer,", "\t\twebhookService, cfg.PublicURL, mailer,")
		value = strings.ReplaceAll(value, "\t\tfileService, cfg.Bootstrap.SetupToken, cfg.PublicURL, mailer,", "\t\tcfg.Bootstrap.SetupToken, cfg.PublicURL, mailer,")
		value = removeLineStarting(value, "\t\tFiles:")
	case "internal/httpapi/handler.go":
		value = removeImport(value, "appfiles")
		value = strings.Replace(value, "\tfiles       *appfiles.Service\n", "", 1)
		value = removeText(value, "func NewHandlerWithWebhooksAndFiles(", "func NewHandlerWithMail(")
		value = strings.ReplaceAll(value, "webhookService, nil, publicURL, mailer", "webhookService, publicURL, mailer")
		value = strings.ReplaceAll(value, "webhookService, fileService, publicURL, mailer", "webhookService, publicURL, mailer")
		for _, prefix := range []string{
			"\t\t\"listFiles\":", "\t\t\"getFile\":", "\t\t\"initiateFileUpload\":", "\t\t\"completeFileUpload\":",
		} {
			value = removeLineStarting(value, prefix)
		}
		value = strings.Replace(value, "\tfileService *appfiles.Service,\n", "", 1)
		value = strings.Replace(value, "webhooks: webhookService, files: fileService", "webhooks: webhookService", 1)
	case "internal/httpapi/handler_test.go":
		value = removeImport(value, "appfiles")
		value = strings.Replace(value, "\tcfg.Storage.LocalPath = t.TempDir()\n", "", 1)
		value = removeText(value, "\tobjectStore, err := appfiles.NewStore(ctx, cfg.Storage)", "\thandler, err := NewHandlerWithWebhooksAndFiles(")
		value = strings.ReplaceAll(value, "NewHandlerWithWebhooksAndFiles", "NewHandlerWithWebhooks")
		value = strings.ReplaceAll(value, "webhookService, fileService, \"\", nil", "webhookService, \"\", nil")
	case "internal/web/handler.go":
		value = removeImport(value, "appfiles")
		value = strings.Replace(value, "\tfiles          *appfiles.Service\n", "", 1)
		value = strings.Replace(value, "\tFiles                            []appfiles.File\n", "", 1)
		value = strings.Replace(value, "\tFilesPresigned                   bool\n", "", 1)
		value = strings.Replace(value, "\tdata.FilesEnabled = h.files != nil\n", "", 1)
		value = removeText(value, "func NewHandlerWithServicesAndFiles(", "func NewHandlerWithServices(")
		value = strings.ReplaceAll(value, "cfg, nil, setupToken, publicURL, mailer", "cfg, setupToken, publicURL, mailer")
		value = strings.ReplaceAll(value, "cfg, fileService, setupToken, publicURL, mailer", "cfg, setupToken, publicURL, mailer")
		value = strings.Replace(value, "\tfileService *appfiles.Service,\n", "", 1)
		value = strings.Replace(value, "identity: identityService, items: itemService, files: fileService, sharing: sharingService, exporter: exporterService", "identity: identityService, items: itemService, sharing: sharingService, exporter: exporterService", 1)
		value = strings.Replace(value, "identity: identityService, items: itemService, files: fileService, exporter: exporterService", "identity: identityService, items: itemService, exporter: exporterService", 1)
		value = removeBlockIncludingEnd(value, "\tif h.files != nil {", "\t}\n")
		value = strings.Replace(value, "\t\tFilesEnabled: h.files != nil,\n", "", 1)
	case "docs/capabilities.md":
		value = removeLineStarting(value, "| Files |")
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

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func registerAgent(
	_ *http.ServeMux,
	_ *identity.Service,
	_ *items.Service,
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
		first == "node_modules" || first == "template.lock" || relative == "cmd/template-init" ||
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

## Replace the sample feature

The item feature is the executable reference for a complete vertical slice.
Replace or remove it as one reviewed change: update shared use cases, routes,
authorization, OpenAPI, migrations, tests, navigation, and documentation
together. Before removing it, pass the applicable browser, REST, CLI, MCP, SSE,
and WebMCP fallback checks described in [development](docs/development.md#replace-the-sample-feature).

## Template provenance

{{CODE}}template.lock{{CODE}} records the template source, release version, commit, generation time,
and selected profiles. Read [template lifecycle](docs/template-lifecycle.md)
before updating this application; generate into a new directory and port
changes through reviewed commits instead of overwriting customizations.

The application module is {{CODE}}{{MODULE}}{{CODE}} and its image/telemetry slug is {{CODE}}{{SLUG}}{{CODE}}.
`
