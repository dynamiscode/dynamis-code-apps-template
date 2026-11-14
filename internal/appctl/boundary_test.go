package appctl

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestCLIImportsOnlyHTTPAndShortIDs keeps appctl a REST-only client. Production
// code may import the standard library and the short-ID codec, and nothing that
// reaches a database, a driver, or an application service. Test files may
// import bootstrap and items, but never the Clients package: a generated
// application without the Clients profile removes that package, and a test
// import of it breaks generation.
func TestCLIImportsOnlyHTTPAndShortIDs(t *testing.T) {
	const shortIDs = "example.com/dynamis-code/apps-template/internal/platform/id"
	forbidden := []string{"database/sql", "sqlite", "pgx", "lib/pq", "internal/clients",
		"internal/platform/database", "internal/items", "internal/identity", "internal/bootstrap"}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, name := range files {
		isTest := strings.HasSuffix(name, "_test.go")
		checked++
		parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(path, "internal/clients") {
				t.Fatalf("%s imports %q; no appctl file may import the Clients package", name, path)
			}
			if isTest {
				continue
			}
			for _, part := range forbidden {
				if strings.Contains(path, part) {
					t.Fatalf("%s imports %q, which the CLI must not use", name, path)
				}
			}
			firstElement, _, _ := strings.Cut(path, "/")
			if strings.Contains(firstElement, ".") && path != shortIDs {
				t.Fatalf("%s imports %q; appctl may import only the standard library and %s", name, path, shortIDs)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no CLI sources were checked")
	}
}
