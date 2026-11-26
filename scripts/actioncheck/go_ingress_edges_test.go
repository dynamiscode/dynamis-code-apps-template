package actioncheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

func TestIngressDestinationProof(t *testing.T) {
	const source = `package fixture
type Writer interface { Header() }
type Handler interface { Serve(Writer) }
type concrete struct{}
func (concrete) Serve(Writer) {}
func first(Writer) {}
func second(Writer) {}
func use(Handler) {}
func fixture(w Writer, unknown Handler, receiver concrete) {
	alias := first
	alias(w)
	first(w)
	mutable := first
	mutable = second
	mutable(w)
	addressed := first
	*(&addressed) = second
	addressed(w)
	receiver.Serve(w)
	unknown.Serve(w)
	use(unknown)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue), Defs: make(map[*ast.Ident]types.Object),
		Uses: make(map[*ast.Ident]types.Object), Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	if _, err := (&types.Config{}).Check("example.test/fixture", fset, []*ast.File{file}, info); err != nil {
		t.Fatal(err)
	}
	body := file.Decls[len(file.Decls)-1].(*ast.FuncDecl).Body
	index := indexIngressAliases(body, info)
	want := []string{
		"example.test/fixture.first", "example.test/fixture.first", "", "",
		"(example.test/fixture.concrete).Serve", "", "example.test/fixture.use",
	}
	i := 0
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		var got string
		if resolved := index.resolve(call.Fun); resolved != nil {
			got = resolved.FullName()
		}
		if i >= len(want) || got != want[i] {
			t.Errorf("call %d at %s resolved to %q", i, fset.Position(call.Pos()), got)
		}
		i++
		return true
	})
	if i != len(want) {
		t.Fatalf("checked %d calls, expected %d", i, len(want))
	}
}

func TestIngressEdgesRequireRegistrationOriginAndWriterContract(t *testing.T) {
	const source = `package fixture
type Writer interface { Header() }
type Handler interface { Serve(Writer) }
type Callback func(Writer)
func register(string, Callback) {}
func page(Writer) {}
func wrap(Writer) {}
func compose(next Callback) Callback { return next }
func opaque(any) {}
func fixture(w Writer, h Handler, unknown Callback) {
	alias := page
	register("/", alias)
	register("/unknown", unknown)
	register("/opaque", func(w Writer) { opaque(w) })
	register("/composed", compose(alias))
	register("/unsafe-composed", compose(unknown))
	wrap(w)
	h.Serve(w)
	opaque(any(w))
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue), Defs: make(map[*ast.Ident]types.Object),
		Uses: make(map[*ast.Ident]types.Object), Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := (&types.Config{}).Check("fixture", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	fn := func(name string) *types.Func { return pkg.Scope().Lookup(name).(*types.Func) }
	writer := pkg.Scope().Lookup("Writer").Type().Underlying().(*types.Interface)
	body := file.Decls[len(file.Decls)-1].(*ast.FuncDecl).Body
	contracts := map[*types.Func]ingressCallContract{
		fn("register"): {callbackArguments: []int{1}},
		fn("page"):     {isCallback: true},
		fn("wrap"):     {},
		fn("compose"):  {callbackArguments: []int{0}, producesCallback: true},
	}
	rejected := uncheckedIngressCalls(body, info, writer, contracts)
	wantLines := []int{13, 14, 14, 16, 16, 18, 19}
	if len(rejected) != len(wantLines) {
		t.Fatalf("rejected %d edges, expected %d", len(rejected), len(wantLines))
	}
	for i, call := range rejected {
		if line := fset.Position(call.Pos()).Line; line != wantLines[i] {
			t.Errorf("edge %d found at %d, want %d", i, line, wantLines[i])
		}
	}
	accepted := body.List[4].(*ast.ExprStmt).X.(*ast.CallExpr)
	index := indexIngressAliases(body, info)
	for _, tc := range []struct {
		budget int
		want   bool
	}{{0, false}, {1, false}, {2, true}} {
		remaining := tc.budget
		if got := index.approvedOrigin(accepted.Args[1], contracts, &remaining); got != tc.want {
			t.Errorf("origin budget %d: approved %t, want %t", tc.budget, got, tc.want)
		}
	}
}
