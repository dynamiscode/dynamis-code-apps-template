package actioncheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func TestStrictWriterFlowRejectsErasureAndOpenDispatch(t *testing.T) {
	const source = `package fixture
type Writer interface { Header(); Write([]byte) }
type Alias = Writer
type Callback interface { Run(Writer) }
func audited(w Alias) {}
func unknown(w Alias) {}
func erased(any) {}
func fixture(w Writer, cb Callback) {
	alias := audited
	alias(w)
	audited(w)
	erased(w)
	unknown(w)
	cb.Run(w)
	w.Write(nil)
	write := w.Write
	write(nil)
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
	owner := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
	findings := strictWriterFlow(owner, info, writer, map[*types.Func]bool{
		pkg.Scope().Lookup("audited").(*types.Func): true,
		pkg.Scope().Lookup("erased").(*types.Func):  true,
	}, nil)
	for _, want := range []string{"fixture.go:12", "fixture.go:13", "fixture.go:14", "fixture.go:15", "fixture.go:16"} {
		matched := false
		for _, finding := range findings {
			matched = matched || strings.HasPrefix(set.Position(finding.node.Pos()).String(), want+":")
		}
		if !matched {
			t.Errorf("no diagnostic on %s", want)
		}
	}
	for _, finding := range findings {
		if line := set.Position(finding.node.Pos()).Line; line < 12 {
			t.Errorf("approved direct edge failed at %d (%s)", line, finding.reason)
		}
	}
}
