package actioncheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

func TestWriterEffectsRejectUnreviewedReceiverAndBoundMethod(t *testing.T) {
	const source = `package fixture
type Writer interface { Write([]byte) (int,error); Header() }
type Alias = Writer
func fixture(w Alias) {
	w.Header()
	w.Write(nil)
	write := w.Write
	write(nil)
	_ = w.Header
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
	pkg, err := (&types.Config{}).Check("fixture", set, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	writer := pkg.Scope().Lookup("Writer").Type().Underlying().(*types.Interface)
	fn := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
	findings := uncheckedWriterMethods(fn.Body, info, writer, func(method *types.Func) bool { return method.Name() == "Header" })
	want := []int{6, 7, 9}
	if len(findings) != len(want) {
		t.Fatalf("found %d writer effects, want %d", len(findings), len(want))
	}
	for i, selector := range findings {
		if line := set.Position(selector.Pos()).Line; line != want[i] {
			t.Errorf("writer effect %d on line %d, want %d", i, line, want[i])
		}
	}
}
