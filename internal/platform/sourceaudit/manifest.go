// Package sourceaudit records exact first-party Go and production build inputs
// after review. It complements analysis of templ and browser code. The
// manifest itself and the standard Go toolchain remain reviewer trust roots.
package sourceaudit

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const ManifestPath = "scripts/reviewed-go-output.json"
const maxInputBytes = 32 << 20

// Verify rejects drift. It never updates an existing manifest.
func Verify(root string) error {
	path := filepath.Join(root, ManifestPath)
	meta, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("approved Go output manifest: %w", err)
	}
	if !meta.Mode().IsRegular() || meta.Size() > maxInputBytes {
		return errors.New("unsupported approved Go output manifest")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("approved Go output manifest: %w", err)
	}
	var want map[string]string
	if err := json.Unmarshal(data, &want); err != nil || len(want) == 0 {
		return errors.New("approved Go output manifest is invalid or empty")
	}
	actual, err := snapshot(root)
	if err != nil {
		return err
	}
	var failures []string
	for name, expected := range want {
		digest, present := actual[name]
		switch {
		case !present:
			failures = append(failures, "missing "+name)
		case digest != expected:
			failures = append(failures, "changed "+name)
		}
	}
	for name := range actual {
		if _, ok := want[name]; !ok {
			failures = append(failures, "added "+name)
		}
	}
	if len(failures) == 0 {
		return nil
	}
	sort.Strings(failures)
	return fmt.Errorf("Go output input drift; review before updating %s:\n%s", ManifestPath, strings.Join(failures, "\n"))
}

// WriteReviewed is an explicit reviewer or verified-generation operation.
// Using this on a mutable application does not constitute a security audit.
func WriteReviewed(root string) error {
	files, err := snapshot(root)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(root, ManifestPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func snapshot(root string) (map[string]string, error) {
	inputs := map[string]string{}
	var goFiles []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".worktrees", "node_modules":
				return filepath.SkipDir
			case "vendor":
				return fmt.Errorf("unsupported vendored Go input %s", relative)
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 && relative != ".claude/skills" {
			return fmt.Errorf("unsupported symlink in Go output review scope %s", relative)
		}
		if !sourceInput(relative) {
			return nil
		}
		content, err := readInput(path, entry)
		if err != nil {
			return err
		}
		inputs[relative] = fmt.Sprintf("%x", sha256.Sum256(content))
		if strings.HasSuffix(relative, ".go") && strings.Contains(string(content), "//go:embed") {
			goFiles = append(goFiles, relative)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, input := range goFiles {
		if err := addEmbeds(root, input, inputs); err != nil {
			return nil, err
		}
	}
	for _, required := range []string{"go.mod", "go.sum"} {
		if _, ok := inputs[required]; !ok {
			return nil, fmt.Errorf("missing %s", required)
		}
	}
	return inputs, nil
}

func sourceInput(path string) bool {
	name := filepath.Base(path)
	if strings.HasSuffix(name, "_test.go") {
		return false
	}
	switch path {
	case "README.md", "VERSION", "package.json", "package-lock.json",
		".env.example", "scripts/action-exceptions.json", "scripts/actioncheck/exceptions.json",
		"scripts/reviewed-browser-scripts.json":
		return true
	}
	switch name {
	case "go.mod", "go.sum", "go.work", "go.work.sum", "Makefile", "Dockerfile":
		return true
	}
	if strings.HasPrefix(path, ".github/workflows/") || strings.HasPrefix(path, "api/") && filepath.Ext(path) == ".yaml" ||
		strings.HasPrefix(path, "internal/i18n/locales/") && filepath.Ext(path) == ".json" {
		return true
	}
	if strings.HasPrefix(path, "scripts/") && (filepath.Ext(path) == ".sh" || filepath.Ext(path) == ".mjs") {
		return true
	}
	switch filepath.Ext(path) {
	case ".go", ".templ", ".s", ".S", ".sx", ".syso", ".c", ".cc", ".cpp", ".cxx", ".m", ".h", ".hh", ".hpp", ".hxx", ".f", ".F", ".for", ".f90", ".swig", ".swigcxx":
		return true
	}
	return false
}

func readInput(path string, entry fs.DirEntry) ([]byte, error) {
	meta, err := entry.Info()
	if err != nil {
		return nil, err
	}
	if !meta.Mode().IsRegular() || meta.Size() > maxInputBytes {
		return nil, fmt.Errorf("unsupported Go output input %s", path)
	}
	return os.ReadFile(path)
}

func addEmbeds(root, input string, inputs map[string]string) error {
	full := filepath.Join(root, filepath.FromSlash(input))
	data, err := os.ReadFile(full)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimLeft(line, " \t")
		arguments, ok := strings.CutPrefix(line, "//go:embed")
		if !ok || len(arguments) == 0 || arguments[0] != ' ' && arguments[0] != '\t' {
			continue
		}
		for _, pattern := range strings.Fields(arguments) {
			if !fs.ValidPath(pattern) || strings.ContainsAny(pattern, "\x60\"\\:") || pattern == "." {
				return fmt.Errorf("unsupported embed pattern %q in %s", pattern, input)
			}
			paths, err := filepath.Glob(filepath.Join(filepath.Dir(full), filepath.FromSlash(pattern)))
			if err != nil || len(paths) == 0 {
				return fmt.Errorf("unresolved embed pattern %q in %s", pattern, input)
			}
			for _, path := range paths {
				meta, err := os.Lstat(path)
				if err != nil {
					return err
				}
				if !meta.Mode().IsRegular() || meta.Size() > maxInputBytes {
					return fmt.Errorf("unsupported embedded payload %s", path)
				}
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				relative, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				inputs[filepath.ToSlash(relative)] = fmt.Sprintf("%x", sha256.Sum256(content))
			}
		}
	}
	return nil
}
