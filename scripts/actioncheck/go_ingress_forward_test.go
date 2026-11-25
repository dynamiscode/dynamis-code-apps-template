package actioncheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/packages"
)

// Check the prototype against real typed middleware in both template and
// generated profiles, rather than assuming the fixture matches net/http.
func TestIngressForwardExistingMiddlewareEdges(t *testing.T) {
	pkgs, err := packages.Load(&packages.Config{
		Dir: "../..", Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
	}, "./internal/httpapi", "net/http")
	if err != nil {
		t.Fatal(err)
	}
	var api *packages.Package
	var serve *types.Func
	for _, pkg := range pkgs {
		if len(pkg.Errors) != 0 {
			t.Fatalf("load %s: %v", pkg.PkgPath, pkg.Errors[0])
		}
		if pkg.PkgPath == "net/http" {
			handler := pkg.Types.Scope().Lookup("Handler").Type().Underlying().(*types.Interface)
			serve = handler.Method(0)
		} else {
			api = pkg
		}
	}
	if api == nil || serve == nil {
		t.Fatal("HTTP middleware or net/http unavailable")
	}
	want := map[string]int{
		"concurrencyMiddleware":     2,
		"requestIDMiddleware":       1,
		"securityHeadersMiddleware": 1,
		"bodyLimitMiddleware":       1,
	}
	for _, source := range api.Syntax {
		for _, decl := range source.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || want[fn.Name.Name] == 0 {
				continue
			}
			info := api.TypesInfo
			callback := info.Defs[fn.Type.Params.List[0].Names[0]].(*types.Var)
			found := 0
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				literal, ok := node.(*ast.FuncLit)
				if !ok {
					return true
				}
				signature, ok := info.TypeOf(literal).(*types.Signature)
				if !ok || signature.Params().Len() != 2 {
					return true
				}
				writer := signature.Params().At(0)
				index := indexIngressAliases(fn.Body, info)
				ast.Inspect(literal.Body, func(item ast.Node) bool {
					call, ok := item.(*ast.CallExpr)
					if ok && index.transparentForward(call, forwardingEdgeContract{callback: callback, writer: writer, method: serve}) {
						found++
					}
					return true
				})
				return false
			})
			if expected := want[fn.Name.Name]; found != expected {
				t.Errorf("%s: proved %d direct forwards, expected %d", fn.Name.Name, found, expected)
			}
			delete(want, fn.Name.Name)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing middleware contracts %v", want)
	}
}

func TestIngressForwardRetainsWriterAndDestination(t *testing.T) {
	const source = `package fixture
type Writer interface { Header() }
type Handler interface { Serve(Writer, *int) }
type wrapper struct { Writer }
type otherWrapper struct { Writer }
func forwarding(next Handler, writer Writer, request *int, unknown Writer, other Handler) {
	w := writer
	h := next
	h.Serve(w, request)
	next.Serve(&wrapper{Writer: writer}, request)
	next.Serve(&otherWrapper{Writer: writer}, request)
	next.Serve(&wrapper{Writer: unknown}, request)
	other.Serve(writer, request)
	mutable := writer
	mutable = unknown
	next.Serve(mutable, request)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{},
		Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	pkg, err := (&types.Config{}).Check("fixture", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	fn := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
	params := info.Defs[fn.Name].(*types.Func).Signature().Params()
	handler := pkg.Scope().Lookup("Handler").Type().Underlying().(*types.Interface)
	wrapper := pkg.Scope().Lookup("wrapper").Type().Underlying().(*types.Struct)
	contract := forwardingEdgeContract{
		callback: params.At(0), writer: params.At(1), method: handler.Method(0),
		wrapperFields: map[*types.Var]bool{wrapper.Field(0): true},
	}
	index := indexIngressAliases(fn.Body, info)
	want := []bool{true, true, false, false, false, false}
	i := 0
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if got := index.transparentForward(call, contract); i >= len(want) || got != want[i] {
				t.Errorf("forward %d: got %t", i, got)
			}
			i++
		}
		return true
	})
	if i != len(want) {
		t.Fatalf("%d forwards found, want %d", i, len(want))
	}
}
