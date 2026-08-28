package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	modulePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~/-]+$`)
	slugPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
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
	source := flags.String("source", "", "released template source URL")
	version := flags.String("version", "", "released semantic version; defaults to VERSION")
	commit := flags.String("commit", "", "released template commit SHA")
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
	}
	if err := copyTemplate(
		root, destination, replacements, templateSlug, slug,
	); err != nil {
		return err
	}
	if err := writeGeneratedReadme(
		filepath.Join(destination, "README.md"), templateName, *name, *module, slug,
	); err != nil {
		return err
	}
	command := exec.Command("go", "generate", "./api")
	command.Dir = destination
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("regenerate generated API contract: %s", strings.TrimSpace(string(output)))
	}
	lock := lockFile{GeneratedAt: now().UTC(), Profiles: []string{"Core", "Identity", "Agent"}}
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
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, info.Mode().Perm())
	})
}

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
		first == "node_modules" || first == "template.lock" {
		return true
	}
	return strings.HasSuffix(relative, ".db") || strings.HasSuffix(relative, ".dump") ||
		strings.HasSuffix(relative, ".backup") || filepath.Base(relative) == ".DS_Store"
}

func validSemver(value string) bool {
	return regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`).MatchString(value)
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

func writeGeneratedReadme(path, templateName, name, module, slug string) error {
	readme := strings.NewReplacer(
		"{{TEMPLATE_NAME}}", templateName,
		"{{NAME}}", name,
		"{{MODULE}}", module,
		"{{SLUG}}", slug,
		"{{CODE}}", "`",
	).Replace(generatedReadme)
	return os.WriteFile(path, []byte(readme), 0o644)
}

const generatedReadme = `# {{NAME}}

This repository is an application generated from **{{TEMPLATE_NAME}}**.
It is a resource-conscious Go modular-monolith starting point with
server-rendered HTML, REST, MCP, a REST-only remote CLI, SQLite by default,
and optional PostgreSQL deployment.

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

The application keeps business rules in shared application use cases. Web,
REST, and MCP adapters call those use cases; the remote CLI calls REST only.
Constructors use manual injection, SQLite is the one-instance default, and
PostgreSQL is required before multiple application instances. See
[architecture](docs/architecture.md) and the [documentation router](docs/README.md)
for source-of-truth boundaries.

## Interfaces

- Browser: server-rendered HTML, HTMX fragments, CSRF-protected forms, and
  accessible controls — [web and realtime](docs/web.md).
- REST: bearer-authenticated HTTP API and generated OpenAPI contract —
  [API guide](docs/api.md) and [OpenAPI](api/openapi.json).
- MCP: bounded authenticated tools over the server MCP endpoint — [MCP](docs/mcp.md).
- Remote CLI: {{CODE}}cmd/appctl{{CODE}} calls REST and never reaches the database — [CLI](docs/cli.md).
- Realtime: scoped, one-way SSE delivery; optional WebMCP only enhances the
  current browser tab and keeps ordinary HTML fallback — [web contract](docs/web.md).

## Operations and data

Read [deployment](docs/deployment.md) before exposing the service and
[operations](docs/operations.md) for health, telemetry, limits, backup,
restore, upgrades, and alerts. [Data lifecycle](docs/data-lifecycle.md)
defines persistence, export, retention, deletion, and recovery boundaries.

## Security

Read [SECURITY.md](SECURITY.md) for private vulnerability reporting and
operator-owned boundaries. [Authentication](docs/authentication.md) documents
bootstrap, sessions, workspace authorization, invitations, tokens, and OIDC.
Never publish credentials, authorization headers, session or invitation values,
token secrets, database URLs, backups, or signed URLs.

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
together. Before removing it, pass the browser, REST, CLI, MCP, SSE, and
WebMCP fallback checks described in [development](docs/development.md#replace-the-sample-feature).

## Template provenance

{{CODE}}template.lock{{CODE}} records the source, release version, commit, generation time,
and selected profiles. Read [template lifecycle](docs/template-lifecycle.md)
before updating this application; generate into a new directory and port
changes through reviewed commits instead of overwriting customizations.

The application module is {{CODE}}{{MODULE}}{{CODE}} and its image/telemetry slug is {{CODE}}{{SLUG}}{{CODE}}.
`
