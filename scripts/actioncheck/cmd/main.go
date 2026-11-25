// Command actioncheck reports action controls that cannot be proven bounded.
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"example.com/dynamis-code/apps-template/internal/platform/sourceaudit"
	"example.com/dynamis-code/apps-template/scripts/actioncheck"
)

func main() {
	if err := sourceaudit.Verify("."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := sourceaudit.VerifyBuildEnvironment("."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	failed := false
	if err := actioncheck.VerifyTemplUniqueness("."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		failed = true
	}
	if err := actioncheck.VerifyTypedComponentArguments("."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		failed = true
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
	err = filepath.WalkDir("internal/web", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		// Plain .go files are scanned too (CheckGoFile), not only .templ
		// ones: a dangerousKinds constructor can be built in an ordinary Go
		// helper file and handed across files as a templ.Component value,
		// which is exactly what a bare component reference elsewhere trusts
		// (safeComponent's doc comment). Generated _templ.go files are
		// skipped: they are templ's own compiled output of the .templ file
		// already scanned, not a second source of truth.
		switch {
		case strings.HasSuffix(path, ".templ"):
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			seen[path] = true
			violations, err := actioncheck.CheckPath(string(source), filepath.ToSlash(path), exceptions)
			if err != nil {
				// Report and keep walking: one file's error (an invalid
				// exception, an unparseable expression) must not hide every
				// later file's violations and exception-path checks behind it.
				fmt.Fprintf(os.Stderr, "%s: %s\n", path, err)
				failed = true
				return nil
			}
			for _, violation := range violations {
				fmt.Fprintf(os.Stderr, "%s:%d: %s\n", path, violation.Line, violation.Reason)
				failed = true
			}
		// _test.go is excluded: test files deliberately construct both safe
		// and rejected-at-runtime inputs to exercise actionattrs' own panics
		// (see button_test.go), and are never rendered in production.
		// components/actionattrs is excluded: it is the trust boundary
		// itself, not code this checker validates against it -- its own
		// Scanned function must call templ.ComponentFunc to build the safe
		// wrapper CheckGoFile elsewhere requires everyone else to use
		// instead of calling templ.ComponentFunc/Raw/FromGoHTML directly.
		case strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_templ.go") && !strings.HasSuffix(path, "_test.go") && !strings.Contains(filepath.ToSlash(path), "/components/actionattrs/"):
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			violations, err := actioncheck.CheckGoFile(string(source), path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %s\n", path, err)
				failed = true
				return nil
			}
			for _, violation := range violations {
				fmt.Fprintf(os.Stderr, "%s:%d: %s\n", path, violation.Line, violation.Reason)
				failed = true
			}
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
