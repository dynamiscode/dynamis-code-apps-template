package actionattrs

import (
	"bytes"
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestWithRejectsUnlistedKey(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for disallowed key")
		}
	}()
	New().With("class", "btn-ghost")
}

func TestWithAllowsListedKey(t *testing.T) {
	got := New().With("hx-get", "/x").Templ()
	if got["hx-get"] != "/x" {
		t.Fatalf("got %v", got)
	}
}

func TestMergeIsAdditive(t *testing.T) {
	a := New().With("id", "a")
	b := New().With("name", "n")
	got := a.Merge(b).Templ()
	if got["id"] != "a" || got["name"] != "n" {
		t.Fatalf("got %v", got)
	}
}

// TestTemplMethodIsUnique guards the trust assumption documented on Safe and
// in scripts/actioncheck: exactly one type in the module may define a
// Templ() method, because the action-control checker treats any
// `<expr>.Templ()` call as a bounded attribute source. If this test fails,
// a second Templ() method was added somewhere and can smuggle unchecked
// attributes past the checker; rename it instead.
func TestTemplMethodIsUnique(t *testing.T) {
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	count := 0
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "node_modules" || info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_templ.go") {
			return nil
		}
		src, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		for _, decl := range src.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "Templ" {
				continue
			}
			count++
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	if count != 1 {
		t.Fatalf("expected exactly one Templ() method in the module, found %d", count)
	}
}

type rawHTMLComponent string

func (c rawHTMLComponent) Render(ctx context.Context, w io.Writer) error {
	_, err := io.WriteString(w, string(c))
	return err
}

func renderScanned(t *testing.T, c templ.Component) (string, func()) {
	t.Helper()
	var buf bytes.Buffer
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		if err := Scanned(c).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
	}()
	return buf.String(), func() {
		if recovered == nil {
			t.Fatal("expected Scanned to panic")
		}
	}
}

func TestScannedRejectsUnboundedAnchor(t *testing.T) {
	_, assertPanicked := renderScanned(t, rawHTMLComponent(`<a class="link" onclick="evil()">click</a>`))
	assertPanicked()
}

func TestScannedRejectsGhostButton(t *testing.T) {
	_, assertPanicked := renderScanned(t, rawHTMLComponent(`<button class="btn btn-ghost">click</button>`))
	assertPanicked()
}

func TestScannedAcceptsPlainIcon(t *testing.T) {
	got, assertNoPanic := renderScanned(t, rawHTMLComponent(`<svg class="h-4 w-4"></svg>`))
	_ = assertNoPanic
	if !strings.Contains(got, "<svg") {
		t.Fatalf("expected the icon to render unchanged, got %q", got)
	}
}

// TestScannedAcceptsAlreadySafeNestedButton proves Scanned does not break
// content slots (a table row, a page section, ...) that legitimately
// contain an already-vetted action control: one built the same way
// scripts/actioncheck requires everywhere else (a "btn" boundary class, no
// forbidden override) passes untouched.
func TestScannedAcceptsAlreadySafeNestedButton(t *testing.T) {
	got, _ := renderScanned(t, rawHTMLComponent(`<tr><td>Row</td><td><button class="btn btn-sm btn-outline">Archive</button></td></tr>`))
	if !strings.Contains(got, "btn-outline") {
		t.Fatalf("expected the already-safe nested button to render unchanged, got %q", got)
	}
}

func TestScannedPassesThroughNil(t *testing.T) {
	if Scanned(nil) != nil {
		t.Fatal("expected Scanned(nil) to return nil unchanged")
	}
}
