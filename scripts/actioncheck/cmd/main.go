// Command actioncheck reports action controls that cannot be proven bounded.
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"example.com/dynamis-code/apps-template/scripts/actioncheck"
)

func main() {
	data, err := os.ReadFile("scripts/actioncheck/exceptions.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var exceptions []actioncheck.Exception
	if err := json.Unmarshal(data, &exceptions); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	seen := make(map[string]bool)
	failed := false
	err = filepath.WalkDir("internal/web", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".templ") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		seen[path] = true
		violations, err := actioncheck.CheckPath(string(source), filepath.ToSlash(path), exceptions)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for _, violation := range violations {
			fmt.Fprintf(os.Stderr, "%s:%d: %s\n", path, violation.Line, violation.Reason)
			failed = true
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		failed = true
	}
	for _, exception := range exceptions {
		if !seen[filepath.FromSlash(exception.Path)] {
			fmt.Fprintf(os.Stderr, "exception path not found: %s\n", exception.Path)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}
