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
	"strings"
	"testing"
	"time"

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

// blockingComponent writes first, then waits for either resume to be
// closed (continuing with second) or ctx to be cancelled.
type blockingComponent struct {
	first, second string
	resume        chan struct{}
}

func (b blockingComponent) Render(ctx context.Context, w io.Writer) error {
	if _, err := io.WriteString(w, b.first); err != nil {
		return err
	}
	select {
	case <-b.resume:
	case <-ctx.Done():
		return ctx.Err()
	}
	_, err := io.WriteString(w, b.second)
	return err
}

// TestScannedStreamsWithoutBufferingWholeSlot regresses a functional
// review finding: Scanned used to render the whole slot into a
// bytes.Buffer before validating or writing anything, so a large slot (a
// whole list's results, not a single row) held its entire output in
// memory at once and never streamed. Scanned now forwards each validated
// HTML token as soon as it is read, so the first chunk of a slot that
// blocks partway through must reach the real writer before that slot
// finishes rendering, not after.
func TestScannedStreamsWithoutBufferingWholeSlot(t *testing.T) {
	resume := make(chan struct{})
	comp := blockingComponent{first: `<svg class="icon"></svg>`, second: `<span>done</span>`, resume: resume}
	pr, pw := io.Pipe()
	renderDone := make(chan error, 1)
	go func() {
		err := Scanned(comp).Render(context.Background(), pw)
		pw.CloseWithError(err)
		renderDone <- err
	}()
	readFirst := make(chan []byte, 1)
	readErr := make(chan error, 1)
	go func() {
		buf := make([]byte, len(comp.first))
		if _, err := io.ReadFull(pr, buf); err != nil {
			readErr <- err
			return
		}
		readFirst <- buf
	}()
	select {
	case buf := <-readFirst:
		if string(buf) != comp.first {
			t.Fatalf("got %q, want %q", buf, comp.first)
		}
	case err := <-readErr:
		t.Fatalf("reading the first chunk failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Scanned did not forward the first token before the blocked component finished: it is buffering the whole slot instead of streaming")
	}
	close(resume)
	rest, err := io.ReadAll(pr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rest), "done") {
		t.Fatalf("missing the rest of the output: %q", rest)
	}
	if err := <-renderDone; err != nil {
		t.Fatal(err)
	}
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

// TestScannedPreservesValidatedPrefixOnChildError regresses a functional
// review finding: the previous buffer-everything-then-write
// implementation discarded a child's entire output, including the part
// already produced before it failed, instead of the already-validated
// prefix reaching the caller. That prefix was independently validated
// token by token as it streamed through, so there is nothing unsafe about
// forwarding it; this is a deliberate, documented behavior (see Scanned's
// doc comment), not an oversight.
func TestScannedPreservesValidatedPrefixOnChildError(t *testing.T) {
	wantErr := errors.New("boom")
	comp := errThenComponent{prefix: `<svg class="icon"></svg>`, err: wantErr}
	var buf bytes.Buffer
	err := Scanned(comp).Render(context.Background(), &buf)
	if !errors.Is(err, wantErr) {
		t.Fatalf("got err %v, want %v", err, wantErr)
	}
	if !strings.Contains(buf.String(), "<svg") {
		t.Fatalf("expected the validated prefix to have reached the writer before the error, got %q", buf.String())
	}
}

// TestScannedStopsOnContextCancellation regresses a functional review
// finding: Scanned wrote its buffered output unconditionally, without
// checking whether ctx had already been cancelled (the client
// disconnected, the request timed out). It now checks ctx between tokens
// and tears down the underlying render via the pipe if cancelled.
func TestScannedStopsOnContextCancellation(t *testing.T) {
	resume := make(chan struct{})
	defer close(resume) // let the blocked render goroutine exit; it must not leak
	comp := blockingComponent{first: `<svg class="icon"></svg>`, second: `<span>done</span>`, resume: resume}
	ctx, cancel := context.WithCancel(context.Background())
	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- Scanned(comp).Render(ctx, &buf)
	}()
	time.Sleep(20 * time.Millisecond) // let the first chunk flow through
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got err %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Scanned did not stop promptly after ctx was cancelled")
	}
}

// TestScannedNestingDoesNotDeadlock proves Scanned(Scanned(c)) still works
// correctly: each layer only adds its own one-token pipe/tokenizer
// overhead, not another full copy of the slot's output (see Scanned's doc
// comment on nesting).
func TestScannedNestingDoesNotDeadlock(t *testing.T) {
	got, _ := renderScanned(t, Scanned(rawHTMLComponent(`<button class="btn btn-outline">ok</button>`)))
	if !strings.Contains(got, "btn-outline") {
		t.Fatalf("expected the nested-safe button to render unchanged, got %q", got)
	}
}
