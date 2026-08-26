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
