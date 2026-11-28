package actioncheck

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestTypedComponentArgumentProvenance(t *testing.T) {
	source := `package fixture
	type Component interface { Render() }
	type concrete struct{}
	func (concrete) Render() {}
	type Alias = concrete
	type factory struct{}
	func (factory) Method() Alias { return Alias{} }
	func (factory) checked() Alias { return Alias{} }
	func checked() Component { return concrete{} }
	func unchecked() Alias { return Alias{} }
	func sink(Component) {}
	type Box struct { Child Component }
	type OpaqueBox struct { Value any }
	type ScalarBox struct { Text string; Hidden any }
	func anyBox() any { return unchecked() }
	func box() Box { return Box{Child: unchecked()} }
	func boxes() []Box { return []Box{box()} }
	func sinkBox(Box) {}
	func sinkBoxes([]Box) {}
	func sinkAny(any) {}
	func sinkOpaque(OpaqueBox) {}
	func sinkText(string) {}
	func describe(box ScalarBox) string { return box.Text }
	func checkedFromBox(ScalarBox) Component { return concrete{} }
	func test(c Component, f factory, checked func() Alias) {
		sink(c)
		sink(f.Method())
		sink(f.checked())
		sink(unchecked())
		sink(checked())
	}
	func good() { sink(checked()); sinkBox(box()); sinkBoxes(boxes()); sinkBox(Box{Child: checked()}); sinkAny(anyBox()) }
	func opaque(box OpaqueBox) { sinkOpaque(box) }
	func projected(box ScalarBox) { sinkText(box.Text); sinkText(describe(box)); sinkAny(box.Hidden); sink(checkedFromBox(box)) }
	`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	pkg, err := (&types.Config{}).Check("fixture", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	component := pkg.Scope().Lookup("Component").Type().Underlying().(*types.Interface).Complete()
	known := map[string]map[string]bool{"fixture": {"checked": true, "checkedFromBox": true}}
	closed := func(function *types.Func) bool { return function != nil && function.Name() == "checkedFromBox" }
	var got []bool
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name, ok := call.Fun.(*ast.Ident)
		if ok && (name.Name == "sink" || name.Name == "sinkBox" || name.Name == "sinkBoxes" || name.Name == "sinkAny" || name.Name == "sinkOpaque" || name.Name == "sinkText") {
			got = append(got, typedComponentArgumentSafe(call.Args[0], info, component, known, true, closed, nil, nil))
		}
		return true
	})
	want := []bool{false, false, false, false, false, true, false, false, true, false, false, true, true, false, true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestApprovedDropdownScannedSlots(t *testing.T) {
	const source = `package atoms
	type Component interface { Render() }
	type DropdownProps struct { Summary Component; Panel Component }
	type DropdownItem struct { Item Component }
	type Other struct { Child Component }
	func Scanned(Component) {}
	func Dropdown(props DropdownProps, item DropdownItem, other Other) {
		Scanned(props.Summary)
		Scanned(props.Panel)
		Scanned(item.Item)
		Scanned(other.Child)
	}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "dropdown.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	pkg, err := (&types.Config{}).Check(atomsImportPath, fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	owner := pkg.Scope().Lookup("Dropdown").(*types.Func)
	var got []bool
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || callIdentifier(call.Fun) == nil || callIdentifier(call.Fun).Name != "Scanned" {
			return true
		}
		got = append(got, isApprovedDropdownScannedSlot(owner, call, info))
		return true
	})
	want := []bool{true, true, true, false}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slot %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestRealClientPageCallbackFactoriesResolve(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	const module = "example.com/dynamis-code/apps-template/"
	loaded, err := packages.Load(&packages.Config{
		Dir: root,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
	}, "./internal/web/...")
	if err != nil {
		t.Fatal(err)
	}
	known := make(map[string]map[string]bool)
	for _, pkg := range loaded {
		if len(pkg.Errors) != 0 {
			t.Fatal(pkg.Errors[0])
		}
		templates, err := templateNames(filepath.Join(root, strings.TrimPrefix(pkg.PkgPath, module)))
		if err != nil {
			t.Fatal(err)
		}
		known[pkg.PkgPath] = templates
	}
	roots, err := expectedListRoots(filepath.Join(root, "internal/web"))
	if err != nil {
		t.Fatal(err)
	}
	var component *types.Interface
	for _, pkg := range loaded {
		for _, imported := range pkg.Types.Imports() {
			if imported.Path() == "github.com/a-h/templ" {
				component = imported.Scope().Lookup("Component").Type().Underlying().(*types.Interface).Complete()
			}
		}
	}
	if component == nil {
		t.Fatal("templ.Component unavailable")
	}
	_, functions, err := reachableListFunctions(loaded, module+"internal/web", roots)
	if err != nil {
		t.Fatal(err)
	}
	closed := closedTemplateConsumers(functions, known, component)
	index := indexRenderOrigins(loaded, known)
	index.closed, index.component = closed, component
	for _, pkg := range loaded {
		if pkg.PkgPath != module+"internal/web" {
			continue
		}
		for _, file := range pkg.Syntax {
			for _, declaration := range file.Decls {
				method, ok := declaration.(*ast.FuncDecl)
				if !ok || method.Name.Name != "renderClientPage" {
					continue
				}
				target := pkg.TypesInfo.Defs[method.Name]
				ast.Inspect(file, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok || calledFunction(call, pkg.TypesInfo) != target {
						return true
					}
					if len(call.Args) < 4 {
						t.Errorf("%s: renderClientPage has %d args", pkg.Fset.Position(call.Pos()), len(call.Args))
						return true
					}
					if index.safeFactory(call.Args[3], pkg.TypesInfo, make(map[*types.Var]bool)) {
						return true
					}
					var expression bytes.Buffer
					_ = format.Node(&expression, pkg.Fset, call.Args[3])
					t.Errorf("%s: callback %s rejected", pkg.Fset.Position(call.Args[3].Pos()), expression.String())
					return true
				})
			}
		}
	}
}
