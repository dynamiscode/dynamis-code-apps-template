package actioncheck

import (
	"os"
	"testing"
)

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
			got, err := Check("package demo\n\ntempl Demo() {\n" + tc.body + "\n}")
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
