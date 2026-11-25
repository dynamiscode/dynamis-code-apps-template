package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
	"text/template"
)

func TestLocaleBrandingPreservesTemplateSyntax(t *testing.T) {
	const name = `Acme {{ .Untrusted }} "Quoted"`
	replace := strings.NewReplacer("Dynamis Code", name)
	branded, err := replaceBrandedSource("internal/i18n/locales/en.json", `{"greeting":"Hello {{.User}}, welcome to Dynamis Code"}`, replace, replace)
	var messages map[string]string
	if err != nil || json.Unmarshal([]byte(branded), &messages) != nil {
		t.Fatalf("locale generation: %v", err)
	}
	message, err := template.New("greeting").Parse(messages["greeting"])
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := message.Execute(&out, map[string]string{"User": "Example"}); err != nil || out.String() != "Hello Example, welcome to "+name {
		t.Fatalf("locale roundtrip = %q (%v)", out.String(), err)
	}
}

func TestBrandingMetadataRemainsData(t *testing.T) {
	name := `Acme "Example" \ draft <label>`
	url := `https://example.test/app?q="quoted"`
	replace := strings.NewReplacer("Dynamis Code", name, "https://old.example/app", url)
	goSource, err := replaceBrandedSource("app.go", "package app\nconst title = \"Dynamis Code\"\nconst url = `https://old.example/app`\n", replace, replace)
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "app.go", goSource, 0)
	if err != nil || len(file.Decls) != 2 {
		t.Fatalf("branding injured Go structure: %v", err)
	}
	for i, want := range []string{name, url} {
		literal := file.Decls[i].(*ast.GenDecl).Specs[0].(*ast.ValueSpec).Values[0].(*ast.BasicLit)
		value, err := strconv.Unquote(literal.Value)
		if err != nil || value != want {
			t.Errorf("metadata %d not preserved as literal data", i)
		}
	}
	jsonSource, err := replaceBrandedSource("app.json", `{"title":"Dynamis Code","url":"https://old.example/app"}`, replace, replace)
	var values map[string]string
	if err != nil || json.Unmarshal([]byte(jsonSource), &values) != nil || values["title"] != name || values["url"] != url {
		t.Fatal("branding injured JSON structure")
	}
	for path, source := range map[string]string{
		"app.js":    "const title = 'Dynamis Code'",
		"app.templ": "<body>Dynamis Code</body>",
		"app.go":    "package app\n//go:generate Dynamis Code\n",
	} {
		if _, err := replaceBrandedSource(path, source, replace, replace); err == nil {
			t.Errorf("unsupported dynamic context accepted in %s", path)
		}
	}
}
