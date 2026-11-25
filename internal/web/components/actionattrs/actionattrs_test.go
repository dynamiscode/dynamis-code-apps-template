package actionattrs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithRejectsUnlistedKey(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for disallowed key")
		}
	}()
	New().With("class", "btn-ghost")
}

func TestWithAllowsListedKey(t *testing.T) {
	got := New().With("hx-get", "/x").Templ()
	if got["hx-get"] != "/x" {
		t.Fatalf("got %v", got)
	}
}

func TestMergeIsAdditive(t *testing.T) {
	a := New().With("id", "a")
	b := New().With("name", "n")
	got := a.Merge(b).Templ()
	if got["id"] != "a" || got["name"] != "n" {
		t.Fatalf("got %v", got)
	}
}

// TestTemplMethodIsUnique guards the trust assumption documented on Safe and
// in scripts/actioncheck: exactly one type in the module may define a
// Templ() method, because the action-control checker treats any
// `<expr>.Templ()` call as a bounded attribute source. If this test fails,
// a second Templ() method was added somewhere and can smuggle unchecked
// attributes past the checker; rename it instead.
func TestTemplMethodIsUnique(t *testing.T) {
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	count := 0
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "node_modules" || info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_templ.go") {
			return nil
		}
		src, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		for _, decl := range src.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "Templ" {
				continue
			}
			count++
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	if count != 1 {
		t.Fatalf("expected exactly one Templ() method in the module, found %d", count)
	}
}
