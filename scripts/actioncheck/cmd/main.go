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
	if err := actioncheck.VerifyTemplUniqueness("."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
			// Report and keep walking: one file's error (an invalid exception,
			// an unparseable expression) must not hide every later file's
			// violations and exception-path checks behind it.
			fmt.Fprintf(os.Stderr, "%s: %s\n", path, err)
			failed = true
			return nil
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
