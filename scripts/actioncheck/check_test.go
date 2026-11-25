package actioncheck

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/web/components/actionattrs"
)

// testImports gives test fixtures real imports for atoms, molecules, and
// actionattrs so safeComponent's import-path checks (trustedPackage) have
// something genuine to resolve against, matching how real .templ files are
// structured. Tests that must prove a spoofed/aliased import is rejected
// build their own source instead of using this constant.
const testImports = "package demo\n\n" +
	"import atoms \"example.com/dynamis-code/apps-template/internal/web/components/atoms\"\n" +
	"import molecules \"example.com/dynamis-code/apps-template/internal/web/components/molecules\"\n" +
	"import \"example.com/dynamis-code/apps-template/internal/web/components/actionattrs\"\n"

func TestCheck(t *testing.T) {
	cases := []struct {
		name, body string
		violations int
	}{
		{"bounded element", `<a class="btn" href={ path }>Open</a>`, 0},
		{"unbounded element", `<a class="link">Open</a>`, 1},
		{"nested expression", `<a class={ "btn", "btn-sm" } href={ format(call(a, b)) }>Open</a>`, 0},
		{"dynamic class", `<button class={ "btn", extra }>Save</button>`, 1},
		{"dynamic class via unwrapped call", `<button class={ "btn", unknown() }>Save</button>`, 1},
		{"conditional with dynamic class", `<button class={ "btn", templ.KV(extra, active) }>Save</button>`, 1},
		{"conditional with forbidden token", `<a class={ "btn", templ.KV("btn-ghost", active) }>Open</a>`, 1},
		{"unwrapped helper call rejected even if always-safe", "func listViewOptionClass(applied bool) string {\n\tif applied {\n\t\treturn \"btn btn-active\"\n\t}\n\treturn \"btn\"\n}\n<a class={ listViewOptionClass(true) }>View</a>", 1},
		{"literal class missing entirely", `<a href={ path }>Open</a>`, 1},
		{"literal class with boundary and extra tokens", `<a class="link rounded-field border border-base-300 px-1" href={ path }>Clients</a>`, 0},
		{"literal class with boundary but forbidden token", `<button class="btn btn-ghost">Save</button>`, 1},
		{"conditional safe class", `<button class={ "btn", templ.KV("btn-primary", enabled) }>Save</button>`, 0},
		{"conditional boundary only", `<button class={ templ.KV("btn", enabled) }>Save</button>`, 1},
		{"conditional boundary removal", `<button class={ "btn", templ.KV("border-0", enabled) }>Save</button>`, 1},
		{"variant boundary removal", `<button class="btn md:border-0">Save</button>`, 1},
		{"conditional unrelated attr", `<button class="btn" if enabled { disabled }>Save</button>`, 0},
		{"conditional class override", `<button class="btn" if enabled { class="link" }>Save</button>`, 1},
		{"uppercase conditional override", `<button class="btn" if enabled { CLASS="link" }>Save</button>`, 1},
		{"uppercase attribute override", `<button class="btn" CLASS="link">Save</button>`, 1},
		{"spread", `<button class="btn" { attrs... }>Save</button>`, 1},
		{"literal safe spread", `<button class="btn" { templ.Attributes{"name": nested(a, b)}... }>Save</button>`, 0},
		{"literal class spread", `<button class="btn" { templ.Attributes{"class": "btn-ghost"}... }>Save</button>`, 1},
		{"dynamic key spread", `<button class="btn" { templ.Attributes{key: "value"}... }>Save</button>`, 1},
		{"wrong type spread", `<button class="btn" { other.Attributes{"name": "value"}... }>Save</button>`, 1},
		{"aliased unsafe component", `@ui.Anchor(ui.AnchorProps{Class: "link"})`, 1},
		{"prebuilt props", `@atoms.Button(props)`, 1},
		{"wrong props type", `@atoms.Button(other.ButtonProps{Class: "btn"})`, 1},
		{"untrusted same-name package", `@other.Button(other.ButtonProps{Class: "btn"})`, 1},
		{"unqualified button", `@Button(ButtonProps{Class: "btn-ghost"})`, 1},
		{"unqualified confirm", `@ConfirmButton("Delete", "Sure?", "btn-ghost", nil)`, 1},
		{"safe component", `@atoms.Anchor(atoms.AnchorProps{Class: "btn", Attrs: templ.Attributes{"aria-label": format(a, b)}})`, 0},
		{"literal href must be a literal value", `@atoms.Anchor(atoms.AnchorProps{Class: "btn", Attrs: templ.Attributes{"href": format(a, b)}})`, 1},
		{"literal href with javascript scheme rejected", `@atoms.Anchor(atoms.AnchorProps{Class: "btn", Attrs: templ.Attributes{"href": "javascript:alert(1)"}})`, 1},
		{"literal style key rejected", `@atoms.Anchor(atoms.AnchorProps{Class: "btn", Attrs: templ.Attributes{"style": "display:none"}})`, 1},
		{"anchor literal class without boundary", `@atoms.Anchor(atoms.AnchorProps{Href: "/clients", Class: "link"})`, 1},
		{"anchor literal class with boundary", `@atoms.Anchor(atoms.AnchorProps{Href: "/clients", Class: "btn btn-primary"})`, 0},
		{"qualified button forbidden token", `@atoms.Button(atoms.ButtonProps{Class: "btn-ghost"})`, 1},
		{"qualified button safe token", `@atoms.Button(atoms.ButtonProps{Class: "btn-primary"})`, 0},
		{"button bare attrs var rejected", `@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: attrs})`, 1},
		{"button non-literal key rejected", `@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: templ.Attributes{key: value}})`, 1},
		{"button safe nested-call attrs value", `@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: templ.Attributes{"type": "submit", "aria-label": format(nested(a, b), []string{"x", "y"})}})`, 0},
		// Bare component-slot references are always rejected now (adversarial
		// review: a bare reference has no call syntax to show this checker
		// how the component was built, including across a package boundary it
		// has no source for at all). actionattrs.Scanned is the only way to
		// make one trusted again; see its doc comment.
		{"bare component slot", `@props.Icon`, 1},
		{"bare indexed component slot", `@props[i].Icon`, 1},
		{"bare loop variable component", `@tab.Content`, 1},
		{"bare type-asserted component", `@props.Toggle.(templ.Component)`, 1},
		{"scanned component slot", `@actionattrs.Scanned(props.Icon)`, 0},
		{"scanned indexed component slot", `@actionattrs.Scanned(props[i].Icon)`, 0},
		{"scanned loop variable component", `@actionattrs.Scanned(tab.Content)`, 0},
		{"scanned type-asserted component", `@actionattrs.Scanned(props.Toggle.(templ.Component))`, 0},
		{"guarded button class on raw button", `<button class={ "btn", actionattrs.ButtonClass(extra) }>Go</button>`, 0},
		{"guarded link class on raw anchor", `<a class={ actionattrs.LinkClass(extra) }>Go</a>`, 0},
		{"guarded safe attrs", `@atoms.Anchor(atoms.AnchorProps{Class: "btn", Attrs: props.Attrs.Templ()})`, 0},
		{"guarded safe attrs with args rejected", `@atoms.Anchor(atoms.AnchorProps{Class: "btn", Attrs: props.Attrs.Templ(extra)})`, 1},
		{"class override", `@atoms.Button(atoms.ButtonProps{Attrs: templ.Attributes{"class": "btn-ghost"}})`, 1},
		{"confirm safe", `@molecules.ConfirmButton("Delete", "Sure?", "btn-sm", nil)`, 0},
		{"confirm aliased package", `@other.ConfirmButton("Delete", "Sure?", "", nil)`, 1},
		{"submit aliased package", `@other.SubmitButtonStyled("Save", "w-full", nil)`, 1},
		{"confirm injection", `@molecules.ConfirmButton("Delete", "Sure?", "", templ.Attributes{"class": "border-0"})`, 1},
		{"confirm dynamic class", `@molecules.ConfirmButton("Delete", "Sure?", extra, nil)`, 1},
		{"confirm unsafe class", `@molecules.ConfirmButton("Delete", "Sure?", "md:border-0", nil)`, 1},
		{"confirm bare attrs var rejected", `@molecules.ConfirmButton("Delete", "Sure?", "", attrs)`, 1},
		{"confirm dynamic value on non-class key is safe", `@molecules.ConfirmButton("Delete", "Sure?", "", templ.Attributes{"name": "action", "value": action})`, 0},
		{"submit safe", `@molecules.SubmitButton("Save", templ.Attributes{"name": "action"})`, 0},
		{"submit dynamic attrs", `@molecules.SubmitButton("Save", attrs)`, 1},
		{"submit guarded attrs", `@molecules.SubmitButton("Save", actionattrs.From(attrs))`, 0},
		{"submit class override", `@molecules.SubmitButton("Save", templ.Attributes{"CLASS": "btn-ghost"})`, 1},
		{"submit alias", `@other.SubmitButton("Save", nil)`, 1},
		{"styled submit safe", `@molecules.SubmitButtonStyled("Save", "w-full", nil)`, 0},
		{"styled submit dynamic class", `@molecules.SubmitButtonStyled("Save", extra, nil)`, 1},
		{"styled submit guarded class", `@molecules.SubmitButtonStyled("Save", actionattrs.ButtonClass(extra), nil)`, 0},
		{"styled submit class override", `@molecules.SubmitButtonStyled("Save", "w-full", templ.Attributes{"class": "btn-ghost"})`, 1},
		{"comment ignored", `<!-- <a class="link"> --> <a class="btn">Open</a>`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Check(testImports + "\ntempl Demo() {\n" + tc.body + "\n}")
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.violations {
				t.Fatalf("got %v, want %d violations", got, tc.violations)
			}
		})
	}
}

func TestListColumnHeadingActions(t *testing.T) {
	source, err := os.ReadFile("../../internal/web/components/molecules/list_column_heading.templ")
	if err != nil {
		t.Fatal(err)
	}
	violations, err := Check(string(source))
	if err != nil || len(violations) != 0 {
		t.Fatalf("column actions: %v %v", violations, err)
	}
}

func TestExactExceptions(t *testing.T) {
	const source = "package demo\ntempl Demo() { <a class=\"skip-link\" href=\"#main\">Skip</a> }"
	exceptions := []Exception{{Path: "skip.templ", OpenTag: `<a class="skip-link" href="#main">`, Line: 2, Reason: "focus-visible skip link"}}
	got, err := CheckPath(source, "skip.templ", exceptions)
	if err != nil || len(got) != 0 {
		t.Fatalf("reviewed control: %v %v", got, err)
	}
	got, err = CheckPath(source, "other.templ", exceptions)
	if err != nil || len(got) != 1 {
		t.Fatalf("different path: %v %v", got, err)
	}
	_, err = CheckPath("package demo\ntempl Demo() { <a data-other-class=\"skip-link\" href=\"#main\">Skip</a> }", "skip.templ", exceptions)
	if err == nil {
		t.Fatal("changed control accepted")
	}
	_, err = CheckPath("package demo\ntempl Demo() { <a class=\"skip-link\" href=\"#main\">1</a><a class=\"skip-link\" href=\"#main\">2</a> }", "skip.templ", exceptions)
	if err == nil {
		t.Fatal("duplicate control accepted")
	}
}

func TestInvalidTemplateFails(t *testing.T) {
	if _, err := Check("package demo\ntempl Demo() { <a class=\"btn\"> }"); err == nil {
		t.Fatal("invalid template accepted")
	}
}

// TestUnsafeComponentBuiltInGoCodeIsCaught regresses a bypass found in
// adversarial review: building an unresolved Anchor/Button inside a `{{ }}`
// Go code block, then rendering it with a bare `@value` reference, used to
// pass unnoticed because safeComponent trusts bare references and nothing
// scanned the Go code block itself.
func TestUnsafeComponentBuiltInGoCodeIsCaught(t *testing.T) {
	source := testImports + `
templ Demo(class string) {
	{{ unsafe := atoms.Anchor(atoms.AnchorProps{Class: class}) }}
	@unsafe
}
`
	got, err := Check(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("unsafe component built in a {{ }} block and bare-rendered was not caught")
	}
}

// TestUnsafeComponentBuiltInHelperFuncIsCaught is the same bypass, but the
// unresolved construction lives in a plain top-level helper function
// instead of a `{{ }}` block.
func TestUnsafeComponentBuiltInHelperFuncIsCaught(t *testing.T) {
	source := testImports + `
func buildIcon(class string) templ.Component {
	return atoms.Button(atoms.ButtonProps{Class: class})
}

templ Demo() {
	@buildIcon("whatever")
}
`
	got, err := Check(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("unsafe component built in a helper function was not caught")
	}
}

// TestButtonClassDoesNotSatisfyAnchorBoundary regresses a bug found in
// adversarial review: safeClassExpr accepted actionattrs.ButtonClass as a
// Class source for Anchor (and any other non-Button component), even though
// ButtonClass enforces no boundary token at runtime and Anchor injects no
// "btn" literal of its own the way atoms.Button does. Only LinkClass may
// satisfy a boundary requirement.
func TestButtonClassDoesNotSatisfyAnchorBoundary(t *testing.T) {
	source := testImports + `
templ Demo(extra string) {
	@atoms.Anchor(atoms.AnchorProps{Class: actionattrs.ButtonClass(extra)})
}
`
	got, err := Check(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("actionattrs.ButtonClass on atoms.Anchor must not satisfy the boundary requirement")
	}
}

// TestSpoofedActionattrsImportIsRejected regresses a bug found in
// adversarial review: isActionattrsCall trusted any call to a function
// literally named actionattrs.From/.Templ/.ButtonClass/.LinkClass by
// identifier text alone, so a file that imports an unrelated package under
// the alias "actionattrs" (or "atoms"/"molecules") got the same trust as
// the real package.
func TestSpoofedActionattrsImportIsRejected(t *testing.T) {
	source := "package demo\n\n" +
		`import actionattrs "github.com/attacker/evil"` + "\n" +
		`import atoms "example.com/dynamis-code/apps-template/internal/web/components/atoms"` + "\n" + `
templ Demo(extra templ.Attributes) {
	@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: actionattrs.From(extra)})
}
`
	got, err := Check(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("a spoofed actionattrs import alias must not be trusted")
	}
}

func TestSpoofedAtomsImportIsRejected(t *testing.T) {
	source := "package demo\n\n" +
		`import atoms "github.com/attacker/evil"` + "\n" + `
templ Demo() {
	@atoms.Button(atoms.ButtonProps{Class: "btn-primary"})
}
`
	got, err := Check(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("a spoofed atoms import alias must not be trusted")
	}
}

// TestImportantModifierCannotSmuggleBorderZero regresses a bug found in
// adversarial review: the forbidden-token check compared tokens like
// "!border-0" literally, so Tailwind's "important" prefix ("!") let a
// caller remove the boundary border without matching "border-0" exactly.
func TestImportantModifierCannotSmuggleBorderZero(t *testing.T) {
	for _, body := range []string{
		`<a class="btn !border-0">Open</a>`,
		`<a class="btn md:!border-0">Open</a>`,
	} {
		got, err := Check(testImports + "\ntempl Demo() {\n" + body + "\n}")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatalf("%q: important-prefixed border-0 override was not caught", body)
		}
	}
}

// TestVerifyTemplUniquenessRejectsASecondMethod regresses the gap where the
// Templ()-method uniqueness invariant that safeAttrs' `.Templ()` trust
// depends on was checked only by a separate `go test`, never by the
// actioncheck scan itself (scripts/actioncheck/cmd now calls this at
// startup).
func TestVerifyTemplUniquenessRejectsWrongLocation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(root+"/real.go", []byte("package real\ntype Safe struct{}\nfunc (s Safe) Templ() string { return \"\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTemplUniqueness(root); err == nil {
		t.Fatal("a Templ() method outside components/actionattrs on a type named Safe must still fail")
	}
}

func TestVerifyTemplUniquenessPassesForActionattrs(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/components/actionattrs", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/components/actionattrs/actionattrs.go", []byte("package actionattrs\ntype Safe struct{}\nfunc (s Safe) Templ() string { return \"\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTemplUniqueness(root); err != nil {
		t.Fatalf("expected the single, correctly-located Templ() method to pass: %v", err)
	}
}

// TestRealListViewsFileIsClean ports half of the legacy JS scanner's
// "checks real shared helper and component calls" case for
// list_views.templ: the real file has zero static violations.
//
// The other half of that JS test mutated the shared class helper's return
// value and expected the *static* scanner to catch it. This checker's
// design deliberately does not: list_views.templ wraps the helper's result
// in actionattrs.LinkClass (see list_views.templ), which is a runtime
// guard, not a static one -- the point of actionattrs.ButtonClass/LinkClass
// is to accept exactly this kind of helper-computed value without the
// static checker needing to prove every branch of the helper is safe.
// TestLinkClassCatchesWhatTheHelperMutationWouldProduce below proves the
// runtime side of that guard instead: the same "link" value the JS
// mutation introduced is rejected by actionattrs.LinkClass itself.
func TestRealListViewsFileIsClean(t *testing.T) {
	source, err := os.ReadFile("../../internal/web/components/organisms/list_views.templ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "actionattrs.LinkClass(listViewOptionClass(") {
		t.Fatal("expected list_views.templ to still route listViewOptionClass through actionattrs.LinkClass; update this test if that changed")
	}
	got, err := CheckPath(string(source), "internal/web/components/organisms/list_views.templ", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected the real file to be clean, got %d violations: %v", len(got), got)
	}
}

func TestLinkClassCatchesWhatTheHelperMutationWouldProduce(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected actionattrs.LinkClass(\"link\") to panic: it lacks the btn/border boundary token")
		}
	}()
	actionattrs.LinkClass("link")
}

// TestRealExceptionsResolveRealFiles ports the legacy JS scanner's
// "repository demo and accessibility exceptions accept their real
// controls" case: every file named in scripts/actioncheck/exceptions.json
// has a real violation without its exception, and none with it.
func TestRealExceptionsResolveRealFiles(t *testing.T) {
	data, err := os.ReadFile("exceptions.json")
	if err != nil {
		t.Fatal(err)
	}
	var exceptions []Exception
	if err := json.Unmarshal(data, &exceptions); err != nil {
		t.Fatal(err)
	}
	if len(exceptions) == 0 {
		t.Fatal("expected at least one real exception to test against")
	}
	for _, exception := range exceptions {
		source, err := os.ReadFile("../../" + exception.Path)
		if err != nil {
			t.Fatalf("%s: %v", exception.Path, err)
		}
		without, err := CheckPath(string(source), exception.Path, nil)
		if err != nil {
			t.Fatalf("%s: %v", exception.Path, err)
		}
		if len(without) == 0 {
			t.Errorf("%s: expected a violation without its exception (otherwise the exception is dead)", exception.Path)
		}
		with, err := CheckPath(string(source), exception.Path, exceptions)
		if err != nil {
			t.Fatalf("%s: %v", exception.Path, err)
		}
		for _, violation := range with {
			if violation.Line == exception.Line {
				t.Errorf("%s:%d: exception did not resolve its own violation", exception.Path, violation.Line)
			}
		}
	}
}

func TestVerifyTemplUniquenessRejectsASecondMethodElsewhere(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/components/actionattrs", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/components/actionattrs/actionattrs.go", []byte("package actionattrs\ntype Safe struct{}\nfunc (s Safe) Templ() string { return \"\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/evil.go", []byte("package evil\ntype Evil struct{}\nfunc (e Evil) Templ() string { return \"\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTemplUniqueness(root); err == nil {
		t.Fatal("a second Templ() method anywhere in the tree must fail VerifyTemplUniqueness")
	}
}

func TestVerifyTemplUniquenessRejectsATemplFuncField(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/components/actionattrs", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/components/actionattrs/actionattrs.go", []byte("package actionattrs\ntype Safe struct{}\nfunc (s Safe) Templ() string { return \"\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A struct field of function type literally named Templ is callable with
	// the exact same `x.Templ()` syntax as a real method; safeAttrs cannot
	// tell them apart, so this must count toward the uniqueness invariant
	// the same as a second method would.
	if err := os.WriteFile(root+"/evil.go", []byte("package evil\ntype Evil struct{ Templ func() string }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTemplUniqueness(root); err == nil {
		t.Fatal("a struct field literally named Templ anywhere in the tree must fail VerifyTemplUniqueness")
	}
}

// TestNestedDangerousCallInsideUnrecognizedWrapperIsCaught regresses a bug
// found in adversarial review: safeComponent returned true immediately for
// any `@` call whose own name was not one of Anchor/Link/Button/..., never
// looking at that call's arguments. An unresolved atoms.Anchor/Button
// construction passed as an argument to some other, unrecognized wrapper
// component in the very same `@` expression was therefore never checked at
// all.
func TestNestedDangerousCallInsideUnrecognizedWrapperIsCaught(t *testing.T) {
	source := testImports + `
templ Demo(class string) {
	@organisms.Something(atoms.Anchor(atoms.AnchorProps{Class: class}))
}
`
	got, err := Check(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("an unresolved Anchor nested inside an unrecognized wrapper call's arguments was not caught")
	}
}

// TestShadowedActionattrsNameIsNotTrusted regresses a bug found in
// adversarial review: trust in the "actionattrs" qualifier was tied to the
// file's import declarations, but a local variable or parameter can shadow
// an import name within its scope, and this checker has no per-scope
// resolution to tell a shadowed use from a real one.
func TestShadowedActionattrsNameIsNotTrusted(t *testing.T) {
	source := testImports + `
templ Demo(extra templ.Attributes) {
	{{ actionattrs := extra }}
	@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: actionattrs.From(extra)})
}
`
	got, err := Check(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("a local variable named actionattrs must not get the real package's trust anywhere in the file")
	}
}

func TestShadowedActionattrsParamIsNotTrusted(t *testing.T) {
	source := testImports + `
templ Demo(actionattrs string, extra templ.Attributes) {
	@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: actionattrs.From(extra)})
}
`
	got, err := Check(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("a templ function parameter named actionattrs must not get the real package's trust")
	}
}

// TestTailwindV4TrailingImportantCannotSmuggleBorderZero regresses a bug
// found in adversarial review: only a leading "!" (Tailwind v3) was
// stripped before comparing tokens; Tailwind v4 moved the "important"
// marker to a trailing "!" ("border-0!").
func TestTailwindV4TrailingImportantCannotSmuggleBorderZero(t *testing.T) {
	for _, body := range []string{
		`<a class="btn border-0!">Open</a>`,
		`<a class="btn md:border-0!">Open</a>`,
	} {
		got, err := Check(testImports + "\ntempl Demo() {\n" + body + "\n}")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatalf("%q: Tailwind v4 trailing-important border-0 override was not caught", body)
		}
	}
}

// TestHrefRejectsOpenRedirectShapes and TestHrefAcceptsLegitimateRelative
// regress a bug found in adversarial review: the href validator treated any
// leading "/" as safe, including "//evil.com" (protocol-relative: same
// scheme, attacker host) and "/\\evil.com" (some browsers normalize a
// leading backslash the same way); it also rejected legitimate
// scheme-less relative references with no leading slash at all.
func TestHrefRejectsOpenRedirectShapes(t *testing.T) {
	for _, href := range []string{"//evil.com", "/\\evil.com", "\\\\evil.com", "\\evil.com"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("expected href %q to be rejected", href)
				}
			}()
			actionattrs.New().With("href", href)
		}()
	}
}

func TestHrefAcceptsLegitimateRelative(t *testing.T) {
	for _, href := range []string{"/docs", "/", "", "#main", "?x=1", "settings/export", "https://example.com/x"} {
		href := href
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("expected href %q to be accepted: %v", href, r)
				}
			}()
			actionattrs.New().With("href", href)
		}()
	}
}

// TestHrefRejectsControlCharacterObfuscation regresses a bug found in
// adversarial review: a tab/newline/CR inserted inside a scheme name
// ("jav\tascript:") defeated the scheme regex, and the value then fell
// through to the "plain relative reference" branch and was accepted --
// exactly the normalization a real browser performs before it ever looks at
// the scheme (WHATWG URL spec). A leading space before "//evil.com" had the
// same effect on the protocol-relative check.
func TestHrefRejectsControlCharacterObfuscation(t *testing.T) {
	for _, href := range []string{
		"jav\tascript:alert(1)",
		"jav\nascript:alert(1)",
		"jav\rascript:alert(1)",
		" //evil.com",
		"\t//evil.com",
	} {
		href := href
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("expected href %q to be rejected", href)
				}
			}()
			actionattrs.New().With("href", href)
		}()
	}
}

// TestTypeLocalTemplFieldIsCaught regresses a bug found in adversarial
// review: VerifyTemplUniqueness only inspected a file's top-level
// declarations, so a struct type (and its Templ-named field) declared
// *inside a function body* was invisible to it, even though it is callable
// with the exact same x.Templ() syntax safeAttrs trusts.
func TestTypeLocalTemplFieldIsCaught(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/components/actionattrs", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/components/actionattrs/actionattrs.go", []byte("package actionattrs\ntype Safe struct{}\nfunc (s Safe) Templ() string { return \"\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/evil.go", []byte("package evil\nfunc build() {\n\ttype Evil struct{ Templ func() string }\n\t_ = Evil{}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTemplUniqueness(root); err == nil {
		t.Fatal("a function-local type with a field literally named Templ must still fail VerifyTemplUniqueness")
	}
}

// TestCheckGoFileCatchesUnresolvedConstructionInAPlainGoFile regresses a
// bug found in adversarial review: scripts/actioncheck/cmd only scanned
// .templ files, so a dangerousKinds construction built in an ordinary .go
// helper -- handed to a .templ file as a bare templ.Component value exactly
// as safeComponent's doc comment describes -- was never scanned at all, in
// any file.
func TestCheckGoFileCatchesUnresolvedConstructionInAPlainGoFile(t *testing.T) {
	source := `package molecules

import atoms "example.com/dynamis-code/apps-template/internal/web/components/atoms"

func buildIcon(class string) templ.Component {
	return atoms.Button(atoms.ButtonProps{Class: class})
}
`
	got, err := CheckGoFile(source, "internal/web/components/molecules/helper.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("an unresolved Button construction in a plain .go file was not caught")
	}
}

func TestCheckGoFileAcceptsSafeConstruction(t *testing.T) {
	source := `package molecules

import atoms "example.com/dynamis-code/apps-template/internal/web/components/atoms"

func buildIcon() templ.Component {
	return atoms.Button(atoms.ButtonProps{Class: "btn-primary"})
}
`
	got, err := CheckGoFile(source, "internal/web/components/molecules/helper.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected a safe construction to be clean, got %v", got)
	}
}

// TestShadowedAliasedActionattrsNameIsNotTrusted regresses a bug found in
// adversarial review: the fixed names {"actionattrs", "atoms", "molecules"}
// missed a file that imports the real actionattrs package under a
// different alias ("aa") and then locally shadows that alias specifically.
func TestShadowedAliasedActionattrsNameIsNotTrusted(t *testing.T) {
	source := "package demo\n\n" +
		`import aa "example.com/dynamis-code/apps-template/internal/web/components/actionattrs"` + "\n" +
		`import atoms "example.com/dynamis-code/apps-template/internal/web/components/atoms"` + "\n" + `
templ Demo(extra templ.Attributes) {
	{{ aa := extra }}
	@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: aa.From(extra)})
}
`
	got, err := Check(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("a local shadow of a non-default actionattrs import alias must not be trusted")
	}
}

// TestVerifyTemplUniquenessRejectsAnonymousStructField regresses a bug
// found in adversarial review: a struct literal type with no `type X
// struct` declaration at all (`struct{ Templ func() templ.Attributes }`
// used directly in a var or composite literal) has no *ast.TypeSpec, so
// the TypeSpec-only scan missed it entirely, even though it is callable
// with the exact same x.Templ() syntax safeAttrs trusts.
func TestVerifyTemplUniquenessRejectsAnonymousStructField(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/components/actionattrs", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/components/actionattrs/actionattrs.go", []byte("package actionattrs\ntype Safe struct{}\nfunc (s Safe) Templ() string { return \"\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/evil.go", []byte(`package evil

func build() any {
	return struct {
		Templ func() string
	}{Templ: func() string { return "style=\"display:none\"" }}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTemplUniqueness(root); err == nil {
		t.Fatal("an anonymous struct literal's field literally named Templ must still fail VerifyTemplUniqueness")
	}
}

// TestCheckGoFileIgnoresUnrelatedSameNamedMethod regresses a false positive
// found in adversarial review: dangerousKinds matched by name alone, so an
// entirely unrelated type's own Button() method (returning an int, say, for
// a gamepad button index) was treated as a candidate atoms.Button call and
// flagged, because it could never satisfy safeComponentCall's shape. A
// same-named call is only in scope when its package qualifier actually
// resolves to atoms or molecules.
func TestCheckGoFileIgnoresUnrelatedSameNamedMethod(t *testing.T) {
	source := `package input

type Device struct{}

func (d Device) Button() int { return 0 }

func read(d Device) int {
	return d.Button()
}
`
	got, err := CheckGoFile(source, "internal/web/input/device.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("an unrelated Device.Button() int method must not be flagged, got %v", got)
	}
}

// TestCheckGoFileCatchesTemplRaw and
// TestNestedTemplRawInsideUnrecognizedWrapperIsCaught regress a bug found
// in adversarial review: templ.Raw (and templ.ComponentFunc/FromGoHTML)
// embeds an arbitrary string as HTML with no AST left for this checker to
// inspect at all; a bare @icon reference could originate one of these,
// undetected, anywhere upstream.
func TestCheckGoFileCatchesTemplRaw(t *testing.T) {
	source := `package molecules

import "github.com/a-h/templ"

func buildIcon(class string) templ.Component {
	return templ.Raw("<a class=\"" + class + "\">click</a>")
}
`
	got, err := CheckGoFile(source, "internal/web/components/molecules/helper.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("templ.Raw building raw HTML in a plain .go file was not caught")
	}
}

func TestNestedTemplRawInsideUnrecognizedWrapperIsCaught(t *testing.T) {
	source := testImports + `
templ Demo(class string) {
	@organisms.Something(templ.Raw("<a class=\"" + class + "\">click</a>"))
}
`
	got, err := Check(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("templ.Raw nested inside an unrecognized wrapper call's arguments was not caught")
	}
}

func TestBareTemplRawReferenceIsCaughtViaItsConstruction(t *testing.T) {
	source := testImports + `
templ Demo(class string) {
	{{ icon := templ.Raw("<a class=\"" + class + "\">click</a>") }}
	@icon
}
`
	got, err := Check(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("a bare @icon reference to a templ.Raw value built in the same {{ }} block was not caught")
	}
}

// TestExternalPackageTemplRawSlotIsRejected regresses the first bug
// reported in the fifth round of adversarial review exactly: a bare @icon
// reference to a templ.Component supplied by an external package
// (reviewexternal.Unsafe(), wrapping templ.Raw internally) was previously
// trusted, because the bare-reference branch trusted any such reference
// unconditionally and this checker has no source for -- and so no way to
// inspect -- a function in a package outside this module. Bare references
// are now always rejected; actionattrs.Scanned (verified at render time
// against the component's actual HTML output, regardless of where it was
// built) is the only way to make one trusted again.
func TestExternalPackageTemplRawSlotIsRejected(t *testing.T) {
	source := testImports + `
import reviewexternal "example.com/reviewexternal"

templ Demo() {
	{{ icon := reviewexternal.Unsafe() }}
	@icon
}
`
	got, err := Check(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("a bare @icon reference to an externally-supplied component must be rejected")
	}
}

// TestAnonymousTemplParamTemplFieldIsRejected regresses the second bug
// reported in the fifth round exactly: an anonymous struct type used
// directly as a *templ function's own parameter* (`templ Demo(evil
// struct{ Templ func() templ.Attributes })`), not a plain .go file or a
// `{{ }}` block, whose Templ-named field returns attacker-controlled
// attributes (style="display:none"). VerifyTemplUniqueness only ever
// scanned plain .go files; a templ function's own signature was invisible
// to it.
func TestAnonymousTemplParamTemplFieldIsRejected(t *testing.T) {
	source := testImports + `
templ Demo(evil struct{ Templ func() templ.Attributes }) {
	@atoms.Button(atoms.ButtonProps{Class: "btn-primary", Attrs: evil.Templ()})
}
`
	got, err := Check(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("an anonymous struct templ-function parameter with a field literally named Templ must be rejected")
	}
}
