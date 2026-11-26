package actionattrs

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"
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

func TestValidateValueRejectsRenderableAttributes(t *testing.T) {
	for _, value := range []any{templ.ComponentScript{}, rawHTMLComponent("text"), func() bool { return true }} {
		if err := ValidateValue("data-test", value); err == nil {
			t.Errorf("accepted %T", value)
		}
	}
	for _, value := range []any{"text", true, 1, templ.KV("text", true)} {
		if err := ValidateValue("data-test", value); err != nil {
			t.Errorf("rejected %T: %v", value, err)
		}
	}
}

func TestScalarAttributesPreservesNativeValuesAndRejectsRawContent(t *testing.T) {
	want := templ.Attributes{"class": "input w-full", "required": true, "tabindex": 0, "aria-label": templ.KV("Name", true)}
	if got := ScalarAttributes(want); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for _, attrs := range []templ.Attributes{{"SrcDoc": "text"}, {"data-label": templ.ComponentScript{}}, {"x-html": "action"}, {"@click": "action"}, {"data-hx-on:click": "action"}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected unverified attribute to be rejected")
				}
			}()
			ScalarAttributes(attrs)
		}()
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

func TestScannedRejectsSplitActionTag(t *testing.T) {
	for _, fragment := range []string{`<button`, `<a class="btn"`, `<button class="btn`} {
		t.Run(fragment, func(t *testing.T) {
			got, assertPanicked := renderScanned(t, rawHTMLComponent(fragment))
			assertPanicked()
			if got != "" {
				t.Fatalf("incomplete tag was written: %q", got)
			}
		})
	}
}

func TestScannedRejectsGhostButton(t *testing.T) {
	_, assertPanicked := renderScanned(t, rawHTMLComponent(`<button class="btn btn-ghost">click</button>`))
	assertPanicked()
}

func TestScannedRejectsDanglingCommentBeforeEmission(t *testing.T) {
	for _, fragment := range []string{"<!--", "<!-- unfinished >"} {
		got, assertPanicked := renderScanned(t, rawHTMLComponent(fragment))
		assertPanicked()
		if got != "" {
			t.Fatalf("emitted %q before rejecting the unclosed comment", got)
		}
	}
}

func TestScannedUsesBrowserEffectiveClass(t *testing.T) {
	for _, class := range []string{`class="btn-ghost" CLASS="btn"`, `class="text&nbsp;btn"`, `class="md:btn"`, `class="btn" style="border:0"`, `class="border border-transparent"`, `class="border border-none"`} {
		t.Run(class, func(t *testing.T) {
			_, assertPanicked := renderScanned(t, rawHTMLComponent(`<button `+class+`>click</button>`))
			assertPanicked()
		})
	}
}

func TestScannedRejectsActionInContextSensitiveTextElement(t *testing.T) {
	for _, fragment := range []string{`<svg><title><a>Link</a></title></svg>`, `<svg><textarea><a href="#example">Link</a></textarea></svg>`, `<noscript><button>Continue</button></noscript>`} {
		t.Run(fragment, func(t *testing.T) {
			_, assertPanicked := renderScanned(t, rawHTMLComponent(fragment))
			assertPanicked()
		})
	}
}

func TestScannedPreservesLiteralActionTextInRawContexts(t *testing.T) {
	for _, fragment := range []string{
		`<script>const sample = "<button>sample</button>";</script>`,
		`<textarea><button>sample</button></textarea>`,
		`<svg><![CDATA[<a>sample</a>]]></svg>`,
		`<svg><![CDATA[ > <a href="#sample">sample</a> ]]></svg>`,
	} {
		got, _ := renderScanned(t, rawHTMLComponent(fragment))
		if got != fragment {
			t.Errorf("got %q, want %q", got, fragment)
		}
	}
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

func TestScannedListRowLink(t *testing.T) {
	const valid = `<tr><td class="list-linked-cell"><a class="list-row-link" href="/items">Item</a></td></tr>`
	got, _ := renderScanned(t, rawHTMLComponent(valid))
	if got != valid {
		t.Fatalf("got %q, want %q", got, valid)
	}
	for _, fragment := range []string{
		`<a class="list-row-link btn-ghost">Item</a>`,
		`<a class="list-row-link" style="display:none">Item</a>`,
		`<a class="other" class="list-row-link">Item</a>`,
		`<button class="list-row-link">Item</button>`,
		`<svg><a class="list-row-link btn-ghost">Item</a></svg>`,
	} {
		t.Run(fragment, func(t *testing.T) {
			got, assertPanicked := renderScanned(t, rawHTMLComponent(fragment))
			assertPanicked()
			if got != "" {
				t.Fatalf("rejected output was written: %q", got)
			}
		})
	}
}

func TestScannedPassesThroughNil(t *testing.T) {
	if Scanned(nil) != nil {
		t.Fatal("expected Scanned(nil) to return nil unchanged")
	}
}

// panicComponent panics instead of rendering.
type panicComponent struct{ msg string }

func (p panicComponent) Render(context.Context, io.Writer) error {
	panic(p.msg)
}

// TestScannedChildPanicPropagatesToCaller regresses a functional review
// finding specific to the (reverted) streaming implementation: it ran the
// child's Render in its own goroutine, so a panic there crashed the whole
// process instead of reaching any recover() in the caller's goroutine.
// This synchronous implementation calls Render directly, so a panic
// propagates exactly like any other panic in a normal function call: it is
// recoverable by the caller.
func TestScannedChildPanicPropagatesToCaller(t *testing.T) {
	defer func() {
		r := recover()
		if r != "deliberate panic" {
			t.Fatalf("expected to recover the child's own panic in this goroutine, got %v", r)
		}
	}()
	var buf bytes.Buffer
	_ = Scanned(panicComponent{msg: "deliberate panic"}).Render(context.Background(), &buf)
	t.Fatal("expected Render to panic")
}

// errThenComponent writes prefix, then fails with err without closing its
// tags.
type errThenComponent struct {
	prefix string
	err    error
}

func (e errThenComponent) Render(_ context.Context, w io.Writer) error {
	if _, err := io.WriteString(w, e.prefix); err != nil {
		return err
	}
	return e.err
}

// TestScannedWritesNothingOnChildError regresses a functional review
// finding specific to the (reverted) streaming implementation: a token
// partially consumed by the tokenizer when the underlying stream ended in
// an error was silently discarded -- neither validated nor forwarded, nor
// accounted for in the returned error. This implementation only ever
// tokenizes a complete, fully-rendered buffer, so that failure mode does
// not exist; a child that fails partway through writes nothing at all
// (see Scanned's doc comment for why that is deliberate, not merely
// convenient).
func TestScannedWritesNothingOnChildError(t *testing.T) {
	wantErr := errors.New("boom")
	comp := errThenComponent{prefix: `<svg class="icon"></svg>`, err: wantErr}
	var buf bytes.Buffer
	err := Scanned(comp).Render(context.Background(), &buf)
	if !errors.Is(err, wantErr) {
		t.Fatalf("got err %v, want %v", err, wantErr)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected nothing to be written when the child errors, got %q", buf.String())
	}
}

// TestScannedChecksContextBeforeRendering and
// TestScannedChecksContextBeforeWriting regress a functional review
// finding: Scanned wrote its buffered output unconditionally, with no
// check for whether ctx had already been cancelled. The (reverted)
// streaming implementation then tried to check ctx *during* a blocked
// tokenizer.Next() read, which cannot actually interrupt a child Render
// that does not itself check ctx (ordinary templ-generated code does
// not), so cancellation silently did nothing and the render could still
// "succeed". This implementation is honest about what it can and cannot
// do: it cannot preempt a child Render already in progress (no code in
// this codebase can, without the child's own cooperation), so it checks
// ctx at its two real decision points instead -- before starting to
// render, and again before writing the validated result -- and returns
// ctx.Err() at either one if already cancelled.
func TestScannedChecksContextBeforeRendering(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	comp := rawHTMLComponent(`<svg class="icon"></svg>`)
	rendered := false
	spy := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		rendered = true
		return comp.Render(ctx, w)
	})
	var buf bytes.Buffer
	err := Scanned(spy).Render(ctx, &buf)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got err %v, want context.Canceled", err)
	}
	if rendered {
		t.Fatal("expected Scanned to check ctx before rendering an already-cancelled request")
	}
}

func TestScannedChecksContextBeforeWriting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	comp := templ.ComponentFunc(func(renderCtx context.Context, w io.Writer) error {
		cancel() // simulate the request being cancelled while this child renders
		_, err := io.WriteString(w, `<svg class="icon"></svg>`)
		return err
	})
	var buf bytes.Buffer
	err := Scanned(comp).Render(ctx, &buf)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got err %v, want context.Canceled", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected nothing to be written once ctx was found cancelled, got %q", buf.String())
	}
}

// shortWriter always reports writing one fewer byte than it was given,
// with a nil error -- a technically non-compliant io.Writer, but one this
// defensive check still needs to catch.
type shortWriter struct{ buf bytes.Buffer }

func (s *shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	s.buf.Write(p[:len(p)-1])
	return len(p) - 1, nil
}

// TestScannedDetectsShortWrite regresses a functional review finding: the
// final w.Write call's returned byte count was never checked against the
// length of what was asked to be written, so a writer that silently
// truncates (returns fewer bytes with a nil error, which io.Writer's
// contract discourages but does not forbid implementations from doing)
// went undetected.
func TestScannedDetectsShortWrite(t *testing.T) {
	w := &shortWriter{}
	err := Scanned(rawHTMLComponent(`<svg class="icon"></svg>`)).Render(context.Background(), w)
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("got err %v, want io.ErrShortWrite", err)
	}
}

// TestScannedNestingStillValidates proves Scanned(Scanned(c)) still
// validates correctly; see Scanned's doc comment on the memory cost of
// nesting (each layer holds its own full copy of the slot's output).
func TestScannedNestingStillValidates(t *testing.T) {
	got, _ := renderScanned(t, Scanned(rawHTMLComponent(`<button class="btn btn-outline">ok</button>`)))
	if !strings.Contains(got, "btn-outline") {
		t.Fatalf("expected the nested-safe button to render unchanged, got %q", got)
	}
}

func TestScannedRejectsSplitUnsafeTagBeforeEmission(t *testing.T) {
	var output bytes.Buffer
	component := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		for _, part := range []string{`<button`, ` class="btn-`, `ghost">bad</button>`} {
			if _, err := io.WriteString(w, part); err != nil {
				return err
			}
			if output.Len() != 0 {
				t.Fatalf("unsafe or incomplete tag was emitted: %q", output.String())
			}
		}
		return nil
	})
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for split unsafe tag")
		}
		if output.Len() != 0 {
			t.Fatalf("unsafe tag was emitted: %q", output.String())
		}
	}()
	_ = Scanned(Scanned(component)).Render(context.Background(), &output)
}
