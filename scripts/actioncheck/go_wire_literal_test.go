package actioncheck

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

func TestWireMarshalRequiresUnescapedLocalPayload(t *testing.T) {
	const code = `package fixture
import "encoding/json"
type Event struct { Detail string }
type Unreviewed struct { Detail string }
func target([]byte) {}
func Good(event Event) {
	data, _ := json.Marshal(event)
	target(data)
}
func Mutated(event Event) {
	data, _ := json.Marshal(event)
	data[0] = ' '
	target(data)
}
func WrongType(event Unreviewed) {
	data, _ := json.Marshal(event)
	target(data)
}
`
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "wire.go", code, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{},
		Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	if _, err := (&types.Config{Importer: importer.Default()}).Check("fixture", set, []*ast.File{file}, info); err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls[4:] {
		fn := decl.(*ast.FuncDecl)
		target := fn.Body.List[len(fn.Body.List)-1].(*ast.ExprStmt).X.(*ast.CallExpr)
		want := fn.Name.Name == "Good"
		if got := matchesWireMarshal(target.Args[0], fn.Body, indexIngressAliases(fn.Body, info), "fixture", "Event"); got != want {
			t.Errorf("%s matched %t, want %t", fn.Name.Name, got, want)
		}
	}
}

func TestWireOutputFormatRejectsUnknownAndChangedBodies(t *testing.T) {
	const code = `package fixture
const eventFormat = "event: close\ndata: {\"reason\":\"lifetime\"}\n\n"
func Fixture(untrusted string) {
	_ = eventFormat
	_ = "event: close\ndata: changed\n\n"
	_ = untrusted
}
`
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "wire.go", code, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	if _, err := (&types.Config{}).Check("fixture", set, []*ast.File{file}, info); err != nil {
		t.Fatal(err)
	}
	statements := file.Decls[1].(*ast.FuncDecl).Body.List
	for i, expected := range []bool{true, false, false} {
		expr := statements[i].(*ast.AssignStmt).Rhs[0]
		got := matchesWireLiteral(expr, info, "event: close\ndata: {\"reason\":\"lifetime\"}\n\n")
		if got != expected {
			t.Errorf("expression %d matched %t, want %t", i, got, expected)
		}
	}
}

func TestWireJSONRequiresFixedPrimitivePayload(t *testing.T) {
	const code = `package fixture
type Status int
func Fixture(dynamic string) {
	_ = map[string]any{"status": 429, "detail": "fixed"}
	_ = map[string]any{"status": 429, "detail": dynamic}
	_ = map[string]any{"status": Status(429), "detail": "fixed"}
	_ = map[string]any{"status": 430, "detail": "fixed"}
}
`
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "wire.go", code, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	if _, err := (&types.Config{}).Check("fixture", set, []*ast.File{file}, info); err != nil {
		t.Fatal(err)
	}
	for i, stmt := range file.Decls[1].(*ast.FuncDecl).Body.List {
		expr := stmt.(*ast.AssignStmt).Rhs[0]
		want := i == 0
		if got := matchesFixedWireJSON(expr, info, map[string]string{"status": "429", "detail": `"fixed"`}); got != want {
			t.Errorf("map %d matched %t, want %t", i, got, want)
		}
	}
}
