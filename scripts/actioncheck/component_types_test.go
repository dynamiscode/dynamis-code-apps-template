package actioncheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
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
			got = append(got, typedComponentArgumentSafe(call.Args[0], info, component, known, true, closed, nil))
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
