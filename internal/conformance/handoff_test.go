package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode"
)

var markdownLink = regexp.MustCompile(`\[[^]]*\]\(([^)[:space:]]+)\)`)

func TestPermanentContextHandoff(t *testing.T) {
	root := repositoryRoot(t)
	required := []string{
		"AGENTS.md", "README.md", "LICENSE", "SECURITY.md", "SUPPORT.md", ".editorconfig", ".gitattributes", "CONTRIBUTING.md",
		"CODE_OF_CONDUCT.md", "NOTICE", ".github/CODEOWNERS",
		".github/ISSUE_TEMPLATE/config.yml", ".github/ISSUE_TEMPLATE/bug_report.yml",
		".github/ISSUE_TEMPLATE/feature_request.yml", ".github/ISSUE_TEMPLATE/documentation.yml",
		".github/pull_request_template.md",
		"CHANGELOG.md", "VERSION", "Dockerfile", "compose.yaml",
		"docs/governance.md",
		"docs/README.md", "docs/architecture.md", "docs/configuration.md",
		"docs/deployment.md", "docs/operations.md", "docs/authentication.md",
		"docs/api.md", "docs/mcp.md", "docs/cli.md", "docs/web.md",
		"docs/accessibility.md", "docs/development.md",
		"docs/capabilities.md", "docs/template-lifecycle.md", "docs/release.md",
	}
	for _, relative := range required {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Errorf("required handoff file %s: %v", relative, err)
		}
	}
	for _, relative := range []string{"PLAN.md", "STANDARDS.md", "docs/implementation"} {
		if _, err := os.Stat(filepath.Join(root, relative)); !os.IsNotExist(err) {
			t.Errorf("temporary construction source still exists: %s", relative)
		}
	}

	checkMarkdownLinks(t, root)
	checkSkills(t, root)
	checkPinnedDelivery(t, root)
	checkWebMCPHandoff(t, root)
	checkLicense(t, root)
	checkRepositoryHygiene(t, root)

	capabilities := readFile(t, filepath.Join(root, "docs/capabilities.md"))
	if strings.Contains(capabilities, "| pending |") {
		t.Error("capability ledger contains a pending group")
	}
	version := strings.TrimSpace(readFile(t, filepath.Join(root, "VERSION")))
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(version) {
		t.Errorf("VERSION %q is not stable semantic versioning", version)
	}
}

func checkLicense(t *testing.T, root string) {
	t.Helper()
	license := readFile(t, filepath.Join(root, "LICENSE"))
	for _, required := range []string{
		"MIT License", "Copyright (c) 2026 David Londono",
		"THE SOFTWARE IS PROVIDED \"AS IS\"",
	} {
		if !strings.Contains(license, required) {
			t.Errorf("LICENSE lacks %q", required)
		}
	}
	packageJSON := readFile(t, filepath.Join(root, "package.json"))
	if !strings.Contains(packageJSON, `"license": "MIT"`) {
		t.Error("package.json lacks SPDX MIT metadata")
	}
}

func checkRepositoryHygiene(t *testing.T, root string) {
	t.Helper()
	support := readFile(t, filepath.Join(root, "SUPPORT.md"))
	for _, required := range []string{
		"issue tracker", "SECURITY.md", "credentials", "private reporting channel",
	} {
		if !strings.Contains(support, required) {
			t.Errorf("SUPPORT.md lacks %q", required)
		}
	}
	if !strings.Contains(readFile(t, filepath.Join(root, ".editorconfig")), "root = true") {
		t.Error(".editorconfig lacks root marker")
	}
	if !strings.Contains(readFile(t, filepath.Join(root, ".gitattributes")), "text=auto eol=lf") {
		t.Error(".gitattributes lacks normalized text policy")
	}
}

func checkWebMCPHandoff(t *testing.T, root string) {
	t.Helper()
	web := readFile(t, filepath.Join(root, "docs/web.md"))
	for _, required := range []string{
		"## WebMCP progressive enhancement", "document.modelContext", "ordinary HTML",
		"workspace-create-v1", "workspace-export-v1", "Permissions Policy", "requestSubmit",
	} {
		if !strings.Contains(web, required) {
			t.Errorf("WebMCP contract lacks %q", required)
		}
	}
	router := readFile(t, filepath.Join(root, "docs/README.md"))
	if !strings.Contains(router, "WebMCP") {
		t.Error("documentation router lacks WebMCP route")
	}
	capabilities := readFile(t, filepath.Join(root, "docs/capabilities.md"))
	if !strings.Contains(capabilities, "Optional WebMCP browser enhancement") || !strings.Contains(capabilities, "conforming") {
		t.Error("capability ledger lacks conforming WebMCP evidence")
	}
	makefile := readFile(t, filepath.Join(root, "Makefile"))
	if !strings.Contains(makefile, "webmcp-smoke") {
		t.Error("Makefile lacks webmcp-smoke target")
	}
	workflow := readFile(t, filepath.Join(root, ".github/workflows/ci.yml"))
	if !strings.Contains(workflow, "make webmcp-smoke") {
		t.Error("CI lacks WebMCP smoke target")
	}
}

func checkMarkdownLinks(t *testing.T, root string) {
	t.Helper()
	paths := []string{
		filepath.Join(root, "AGENTS.md"), filepath.Join(root, "README.md"),
		filepath.Join(root, "SECURITY.md"), filepath.Join(root, "SUPPORT.md"),
		filepath.Join(root, "CONTRIBUTING.md"),
		filepath.Join(root, "CHANGELOG.md"),
	}
	for _, directory := range []string{filepath.Join(root, "docs"), filepath.Join(root, ".agents/skills")} {
		err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".md") {
				paths = append(paths, path)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range paths {
		content := readFile(t, path)
		for _, match := range markdownLink.FindAllStringSubmatch(content, -1) {
			target := strings.Trim(match[1], "<>")
			if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			target, fragment, _ := strings.Cut(target, "#")
			target, _, _ = strings.Cut(target, "?")
			resolved := path
			if target != "" {
				resolved = filepath.Join(filepath.Dir(path), filepath.FromSlash(target))
			}
			if _, err := os.Stat(resolved); err != nil {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("%s links missing %s", relative, target)
				continue
			}
			if fragment != "" && strings.HasSuffix(resolved, ".md") && !hasAnchor(readFile(t, resolved), fragment) {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("%s links missing anchor %s#%s", relative, target, fragment)
			}
		}
	}
}

func hasAnchor(content string, want string) bool {
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "#") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimLeft(line, "#"))
		var anchor strings.Builder
		for _, character := range strings.ToLower(heading) {
			switch {
			case unicode.IsLetter(character), unicode.IsNumber(character), character == '-':
				anchor.WriteRune(character)
			case unicode.IsSpace(character):
				anchor.WriteByte('-')
			}
		}
		if anchor.String() == want {
			return true
		}
	}
	return false
}

func checkSkills(t *testing.T, root string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, ".agents/skills/*/SKILL.md"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("discover skills: matches=%d err=%v", len(matches), err)
	}
	for _, path := range matches {
		content := readFile(t, path)
		if !strings.HasPrefix(content, "---\nname: ") || !strings.Contains(content, "\ndescription: ") {
			t.Errorf("skill lacks name/description frontmatter: %s", path)
		}
		for _, temporary := range []string{"PLAN.md", "STANDARDS.md", "docs/implementation"} {
			if strings.Contains(content, temporary) {
				t.Errorf("skill references temporary source %s: %s", temporary, path)
			}
		}
	}
}

func checkPinnedDelivery(t *testing.T, root string) {
	t.Helper()
	for _, line := range strings.Split(readFile(t, filepath.Join(root, "Dockerfile")), "\n") {
		if strings.HasPrefix(line, "FROM ") && !strings.Contains(line, "@sha256:") {
			t.Errorf("unpinned Dockerfile base: %s", line)
		}
	}
	workflowRoot := filepath.Join(root, ".github/workflows")
	err := filepath.WalkDir(workflowRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".yml") {
			return err
		}
		for _, line := range strings.Split(readFile(t, path), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- uses:") || strings.HasPrefix(line, "uses:") {
				fields := strings.Fields(line)
				value := fields[1]
				if fields[0] == "-" {
					value = fields[2]
				}
				_, ref, found := strings.Cut(value, "@")
				if !found || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(ref) {
					t.Errorf("workflow action is not pinned by commit: %s", line)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve conformance test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
