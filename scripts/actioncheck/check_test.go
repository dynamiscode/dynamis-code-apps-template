package actioncheck

import (
	"os"
	"testing"
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
		{"safe component", `@atoms.Anchor(atoms.AnchorProps{Class: "btn", Attrs: templ.Attributes{"href": format(a, b)}})`, 0},
		{"bare component slot", `@props.Icon`, 0},
		{"bare indexed component slot", `@props[i].Icon`, 0},
		{"bare loop variable component", `@tab.Content`, 0},
		{"bare type-asserted component", `@props.Toggle.(templ.Component)`, 0},
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
