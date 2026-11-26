package actioncheck

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriterCapabilityReviewRejectsDelegationAndChanges(t *testing.T) {
	const approved = "func approved(w Writer) { w.Header() }"
	const source = `package fixture
type Writer interface{ Header() }
type Alias = Writer
type Narrow interface{ Something() }
type Handler interface{ Serve(Alias) }
type Registry struct{ handler Handler }
func emit(any) {}
func registration(reg Registry) { emit(reg.handler) }
func callback() { emit(delegated) }
func stored(w Writer, ch chan any) any { var value any; value = w; _ = value; var capture any = w; _ = capture; _ = []any{w}; ch <- w; return w }
func projected(w Writer) { emit(w.(Narrow)); switch w.(type) { case Narrow: } }
func delegated(w Alias) { emit(any(w)) }
func changed(w Writer) { w.Header(); emit(w) }
` + approved
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue), Defs: make(map[*ast.Ident]types.Object)}
	pkg, err := (&types.Config{}).Check("fixture", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	writer := pkg.Scope().Lookup("Writer").Type().Underlying().(*types.Interface)
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(approved)))
	found := strings.Join(reviewWriterCapability(file, info, []byte(source), writer, map[string]string{"approved": digest, "changed": digest}), "\n")
	if !strings.Contains(found, "delegated: unreviewed") || !strings.Contains(found, "changed: unreviewed") ||
		!strings.Contains(found, "registration: unreviewed") || !strings.Contains(found, "callback: unreviewed") ||
		strings.Contains(found, "approved: unreviewed") {
		t.Fatal(found)
	}
	var opaqueTransfers int
	var opaqueStorage int
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			opaqueTransfers += len(unresolvedWriterTransfers(fn.Body, info, writer, func(*ast.CallExpr) bool { return false }))
			opaqueStorage += len(opaqueWriterStorage(fn.Body, info, writer))
		}
	}
	if opaqueTransfers != 4 {
		t.Fatalf("got %d unresolved transfers, want 4", opaqueTransfers)
	}
	if opaqueStorage != 7 {
		t.Fatalf("got %d unresolved storage paths, want 7", opaqueStorage)
	}
}

func TestWriterInventoryIncludesOutsideWebRegistration(t *testing.T) {
	dir := t.TempDir()
	entries := map[string]string{
		"go.mod": "module example.test/fixture\n\ngo 1.25\n",
		"cmd/app/main.go": `package main
import ("io"; "net/http")
func main() { http.HandleFunc("/preview", outsideWeb) }
func outsideWeb(writer http.ResponseWriter, _ *http.Request) { delegated(io.Writer(writer)) }
func delegated(writer io.Writer) { _, _ = io.WriteString(writer, "<button>Next</button>") }
`,
	}
	for path, source := range entries {
		path = filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	findings, err := inventoryWriterCapabilities(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(findings, "\n")
	for _, want := range []string{"cmd/app/main.go: main: unreviewed", "cmd/app/main.go: outsideWeb: unreviewed"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}
