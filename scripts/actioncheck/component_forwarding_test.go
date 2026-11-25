package actioncheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestReachableListFunctions(t *testing.T) {
	source := `package fixture
	func clientListView() { downstream() }
	func clientResults() {}
	func memberListView() {}
	func memberResults() {}
	func fileListView() {}
	func fileResults() {}
	func downstream() { helper() }
	func helper() {}
	func unrelated() {}
	`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}}
	pkg, err := (&types.Config{}).Check("fixture", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	for _, filename := range []string{"clients.go", "files.go"} {
		if err := os.WriteFile(filepath.Join(directory, filename), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := expectedListRoots(directory)
	if err != nil {
		t.Fatal(err)
	}
	loaded := []*packages.Package{{PkgPath: "fixture", Syntax: []*ast.File{file}, TypesInfo: info}}
	reachable, _, err := reachableListFunctions(loaded, "fixture", expected)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		want bool
	}{{"clientListView", true}, {"downstream", true}, {"helper", true}, {"unrelated", false}} {
		function := pkg.Scope().Lookup(tc.name).(*types.Func)
		if got := reachable[function]; got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	pruned, err := expectedListRoots(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned) != 2 || !pruned["memberListView"] || !pruned["memberResults"] {
		t.Fatalf("pruned roots = %v", pruned)
	}
	if _, _, err := reachableListFunctions(loaded, "fixture", pruned); err == nil || !strings.Contains(err.Error(), "unexpected list entry point") {
		t.Fatalf("retained feature roots without feature source: %v", err)
	}
	for i, declaration := range file.Decls {
		if declaration.(*ast.FuncDecl).Name.Name == "clientResults" {
			file.Decls = append(file.Decls[:i], file.Decls[i+1:]...)
			break
		}
	}
	if _, _, err := reachableListFunctions(loaded, "fixture", expected); err == nil || !strings.Contains(err.Error(), "missing required list entry point clientResults") {
		t.Fatalf("missing feature root: %v", err)
	}
}

func TestIgnoresComponentFields(t *testing.T) {
	source := `package fixture
	type Component interface { Render() }
	type Box struct { Child Component; Text string }
	func consume(Box) {}
	func ignored(b Box) { _ = b.Text }
	func read(b Box) { _ = b.Child }
	func forwarded(b Box) { consume(b) }
	`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}}
	pkg, err := (&types.Config{}).Check("fixture", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	component := pkg.Scope().Lookup("Component").Type().Underlying().(*types.Interface).Complete()
	want := map[string]bool{"ignored": true, "read": false, "forwarded": false}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		expected, ok := want[function.Name.Name]
		if !ok {
			continue
		}
		if got := ignoresComponentFields(typedFunction{function, info}, 0, component); got != expected {
			t.Errorf("%s: got %v, want %v", function.Name.Name, got, expected)
		}
	}
}
