package actioncheck

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func TestExportedRouterReturnsOnlyProvenRegistrations(t *testing.T) {
	const source = `package fixture
import h "net/http"
func page(h.ResponseWriter, *h.Request) {}
func other(h.ResponseWriter, *h.Request) {}
func stores(any) {}
func Routes(opaque h.Handler) h.Handler {
	mux := h.NewServeMux()
	add := mux.HandleFunc
	add("/ok", page)
	mux.Handle("/unknown", opaque)
	mux.HandleFunc("/other", other)
	stores(mux)
	var hidden any = add
	_ = hidden
	stores(mux.HandleFunc)
	stores(func() h.Handler { return mux })
	stores(func() { add("/late", page) })
	return mux
}
`
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{},
		Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	pkg, err := (&types.Config{Importer: importer.Default()}).Check("fixture", set, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	fn := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
	httpPkg := pkg.Imports()[0]
	mux := httpPkg.Scope().Lookup("ServeMux").Type()
	pointerMethods := types.NewMethodSet(types.NewPointer(mux))
	newMux := httpPkg.Scope().Lookup("NewServeMux").(*types.Func)
	handle := pointerMethods.Lookup(httpPkg, "Handle").Obj().(*types.Func)
	handleFunc := pointerMethods.Lookup(httpPkg, "HandleFunc").Obj().(*types.Func)
	page := pkg.Scope().Lookup("page").(*types.Func)
	writer := httpPkg.Scope().Lookup("ResponseWriter").Type().Underlying().(*types.Interface)
	rejected := uncheckedIngressCalls(fn.Body, info, writer, map[*types.Func]ingressCallContract{
		newMux:     {producesCallback: true},
		handle:     {callbackArguments: []int{1}},
		handleFunc: {callbackArguments: []int{1}},
		page:       {isCallback: true},
	})
	if len(rejected) != 5 || set.Position(rejected[0].Pos()).Line != 10 ||
		set.Position(rejected[1].Pos()).Line != 11 || set.Position(rejected[2].Pos()).Line != 12 ||
		set.Position(rejected[3].Pos()).Line != 15 || set.Position(rejected[4].Pos()).Line != 16 {
		t.Fatalf("must reject unknown and unapproved routes, identified %v", rejected)
	}
	index := indexIngressAliases(fn.Body, info)
	def := fn.Body.List[0].(*ast.AssignStmt).Lhs[0].(*ast.Ident)
	muxVariable := info.Defs[def]
	escapes := index.localMuxEscapes(fn.Body, muxVariable, newMux, map[*types.Func]bool{handle: true, handleFunc: true})
	if len(escapes) != 5 || set.Position(escapes[0].Pos()).Line != 12 ||
		set.Position(escapes[1].Pos()).Line != 13 || set.Position(escapes[2].Pos()).Line != 15 ||
		set.Position(escapes[3].Pos()).Line != 16 || set.Position(escapes[4].Pos()).Line != 17 {
		t.Fatalf("must reject mux and registration-method escapes, identified %v", escapes)
	}
}

func TestExportedHandlerConsumerMustUseProvenOrigins(t *testing.T) {
	source := `package fixture
import h "net/http"
func Routes() h.Handler { return h.NewServeMux() }
func RogueRoutes() h.Handler { return h.NewServeMux() }
func wrap(in h.Handler) h.Handler { return in }
func start(in h.Handler) {}
func elsewhere(server func(h.Handler)) {
	alias := Routes()
	start(wrap(alias))
	start(wrap(RogueRoutes()))
	server(alias)
}
`
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{},
		Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	pkg, err := (&types.Config{Importer: importer.Default()}).Check("fixture", set, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	httpPkg := pkg.Imports()[0]
	writer := httpPkg.Scope().Lookup("ResponseWriter").Type().Underlying().(*types.Interface)
	fn := func(name string) *types.Func { return pkg.Scope().Lookup(name).(*types.Func) }
	body := file.Decls[len(file.Decls)-1].(*ast.FuncDecl).Body
	rejected := uncheckedIngressCalls(body, info, writer, map[*types.Func]ingressCallContract{
		fn("Routes"): {producesCallback: true},
		fn("wrap"):   {producesCallback: true, callbackArguments: []int{0}},
		fn("start"):  {callbackArguments: []int{0}},
	})
	var lines []string
	for _, call := range rejected {
		lines = append(lines, set.Position(call.Pos()).String())
	}
	if len(rejected) != 4 || !strings.Contains(strings.Join(lines, " "), ":11:") {
		t.Fatalf("unproved exported-handler consumers: %v", lines)
	}
}
