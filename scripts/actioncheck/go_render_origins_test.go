package actioncheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestGoRenderOrigins(t *testing.T) {
	const source = `package fixture
	type Component interface { Render() }
	type body struct{}
	func (body) Render() {}
	func checked(any) Component { return body{} }
	func openTemplate(any) Component { return body{} }
	func opaque(any) Component { return body{} }
	func opaqueValue() any { return body{} }
	func accept(factory func(any) Component) { factory(nil).Render() }
	func reject(factory func(any) Component) { factory(nil).Render() }
	func Exported(factory func(any) Component) { factory(nil).Render() }
	func addressTaken() { factory := checked; _ = &factory; factory(nil).Render() }
	func reassigned() { factory := checked; factory = opaque; factory(nil).Render() }
	func aliased() { factory := checked; alias := factory; alias(nil).Render() }
	func indirect(factory func(any) Component) { factory(nil).Render() }
	func methodAlias(component Component) { render := component.Render; render() }
	func badClosure() { factory := func() Component { return openTemplate(opaqueValue()) }; factory().Render() }
	func goodClosure() { factory := func() Component { return openTemplate("label") }; factory().Render() }
	func openAlias() { factory := openTemplate; factory(nil).Render() }
	type responder struct{}
	type open interface { dispatch(func(any) Component) }
	func (responder) dispatch(factory func(any) Component) { factory(nil).Render() }
	func calls(r responder) {
		accept(func(data any) Component { return checked(data) })
		reject(opaque)
		Exported(checked)
		f := indirect
		f(checked)
		r.dispatch(checked)
	}`
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
	component := pkg.Scope().Lookup("Component").Type().Underlying().(*types.Interface)
	escapes := 0
	for _, use := range goRenderSelectors(file, info, component) {
		if !use.direct {
			escapes++
		}
	}
	if escapes != 1 {
		t.Fatalf("got %d indirect method uses, want 1", escapes)
	}
	index := indexRenderOrigins([]*packages.Package{{Syntax: []*ast.File{file}, TypesInfo: info}}, map[string]map[string]bool{"fixture": {"checked": true, "openTemplate": true}})
	index.component = component
	index.closed = func(function *types.Func) bool { return function != nil && function.Name() == "checked" }
	want := map[string]bool{"accept": true, "reject": false, "Exported": false, "addressTaken": false, "reassigned": false, "aliased": true, "indirect": false, "dispatch": false, "badClosure": false, "goodClosure": true, "openAlias": false}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		expected, checked := want[function.Name.Name]
		if !checked {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "Render" {
				if got := index.safeGoRenderReceiver(selector.X, info); got != expected {
					t.Errorf("%s: got %v, want %v", function.Name.Name, got, expected)
				}
				delete(want, function.Name.Name)
			}
			return true
		})
	}
	if len(want) != 0 {
		t.Fatalf("missing render expressions: %v", want)
	}
}

func TestForwardedTemplateInputsRejectMutationAndOpaqueSources(t *testing.T) {
	const source = `package fixture
	type Box struct { Value any; Items []any }
	func sink(any) {}
	func opaque() any { return nil }
	func valid(input Box) { sink(input.Value); for _, item := range input.Items { sink(item) } }
	func fieldWrite(input Box) { input.Value = opaque(); sink(input.Value) }
	func fieldAddress(input Box) { _ = &input.Value; sink(input.Value) }
	func swapped(input Box) { input = Box{Value: opaque()}; sink(input.Value) }
	func mutatedRange(input Box) { for _, item := range input.Items { item = opaque(); sink(item) } }
	func unproven() { sink(opaque()) }
	`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	if _, err := (&types.Config{}).Check("fixture", fset, []*ast.File{file}, info); err != nil {
		t.Fatal(err)
	}
	index := indexRenderOrigins([]*packages.Package{{Syntax: []*ast.File{file}, TypesInfo: info}}, nil)
	count := 0
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		owner := info.Defs[fn.Name].(*types.Func)
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok && len(call.Args) == 1 && callIdentifier(call.Fun).Name == "sink" {
				count++
				if got, want := index.forwardedTemplateInput(call.Args[0], info, owner), fn.Name.Name == "valid"; got != want {
					t.Errorf("%s: got %v, want %v", fn.Name.Name, got, want)
				}
			}
			return true
		})
	}
	if count != 7 {
		t.Fatalf("checked %d calls, want 7", count)
	}
}
