// Package actioncheck analyzes action controls in templ source files.
package actioncheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	templparser "github.com/a-h/templ/parser/v2"
	"github.com/a-h/templ/parser/v2/visitor"
)

type Violation struct {
	Line   uint32
	Reason string
}

type Exception struct {
	Path    string
	OpenTag string
	Line    uint32
	Reason  string
}

// Real import paths trust in a package qualifier (actionattrs.X, atoms.X,
// molecules.X) is tied to, resolved per file via resolveImports. A file
// that aliases some other import to one of these names, or that imports
// nothing by that name, gets none of the corresponding trust: see
// resolveImports and trustedPackage.
const (
	actionattrsImportPath = "example.com/dynamis-code/apps-template/internal/web/components/actionattrs"
	atomsImportPath       = "example.com/dynamis-code/apps-template/internal/web/components/atoms"
	moleculesImportPath   = "example.com/dynamis-code/apps-template/internal/web/components/molecules"
)

// dangerousKinds names the component constructors whose Class/Attrs must be
// bounded. scanGoSource looks for calls to these by name anywhere in a
// file's Go code, not only directly under an `@` component expression, so
// that building one in a `{{ }}` block or a helper function and rendering
// it via a bare `@value` reference cannot skip the check that bare
// references otherwise extend (see safeComponent).
var dangerousKinds = map[string]bool{
	"Anchor": true, "Link": true, "Button": true,
	"ConfirmButton": true, "SubmitButton": true, "SubmitButtonStyled": true,
}

// Check fails closed on dynamic classes, attribute spreads, and unresolved props.
func Check(source string) ([]Violation, error) {
	return CheckPath(source, "", nil)
}

// CheckPath accepts only exact, uniquely occurring opening tags in the named file.
func CheckPath(source, path string, exceptions []Exception) ([]Violation, error) {
	file, err := templparser.ParseString(source)
	if err != nil {
		return nil, err
	}
	imports := resolveImports(source)
	var violations []Violation
	matches := make([]int, len(exceptions))
	v := visitor.New()
	visitElement := v.Element
	v.Element = func(element *templparser.Element) error {
		if element.Name == "a" || element.Name == "button" {
			if !safeElement(element, imports) {
				from, to := element.OpenTagRange.From.Index, element.OpenTagRange.To.Index
				excepted := false
				if from >= 0 && to >= from && to <= int64(len(source)) {
					opening := source[from:to]
					for i, exception := range exceptions {
						if exception.Path == path && exception.OpenTag == opening && exception.Line == element.Range.From.Line+1 && strings.TrimSpace(exception.Reason) != "" {
							matches[i]++
							excepted = true
						}
					}
				}
				if !excepted {
					violations = append(violations, Violation{element.Range.From.Line + 1, "unbounded or unresolved action element"})
				}
			}
		}
		return visitElement(element)
	}
	visitCall := v.TemplElementExpression
	v.TemplElementExpression = func(call *templparser.TemplElementExpression) error {
		expr, err := parser.ParseExpr(call.Expression.Value)
		if err != nil {
			violations = append(violations, Violation{call.Range.From.Line + 1, "unsupported component expression"})
		} else if !safeComponent(expr, imports) {
			violations = append(violations, Violation{call.Range.From.Line + 1, "unbounded or unresolved action component"})
		}
		return visitCall(call)
	}
	// Go code living in `{{ }}` blocks or in plain helper functions (outside
	// any `@` expression) can construct one of dangerousKinds and hand it to
	// a bare component reference elsewhere, which safeComponent trusts
	// unconditionally (see its doc comment). Scan every such block directly
	// so that path cannot bypass the Class/Attrs checks.
	visitGoCode := v.GoCode
	v.GoCode = func(code *templparser.GoCode) error {
		if unsafe := scanStatements(code.Expression.Value, imports); unsafe {
			violations = append(violations, Violation{code.Range.From.Line + 1, "unbounded or unresolved action component built in Go code"})
		}
		return visitGoCode(code)
	}
	visitFileGo := v.TemplateFileGoExpression
	v.TemplateFileGoExpression = func(code *templparser.TemplateFileGoExpression) error {
		if unsafe := scanDecls(code.Expression.Value, imports); unsafe {
			violations = append(violations, Violation{code.Expression.Range.From.Line + 1, "unbounded or unresolved action component built in Go code"})
		}
		return visitFileGo(code)
	}
	if err := file.Visit(v); err != nil {
		return nil, err
	}
	for i, exception := range exceptions {
		if exception.Path == path && (strings.TrimSpace(exception.Reason) == "" || exception.OpenTag == "" || exception.Line == 0 || matches[i] != 1) {
			return nil, fmt.Errorf("invalid or ambiguous action exception %q: matched %d controls", exception.OpenTag, matches[i])
		}
	}
	return violations, nil
}

// resolveImports extracts "local identifier -> import path" from a templ
// file's Go import declarations (single-line and grouped). Trust in a
// package qualifier is tied to this map (see trustedPackage), not to the
// identifier text alone: a file that imports some other package under an
// alias matching a trusted name (for example `import actionattrs
// "github.com/attacker/evil"`) gets no trust from that alias, because its
// resolved path will not match the real one.
func resolveImports(source string) map[string]string {
	imports := map[string]string{}
	inBlock := false
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case !inBlock && trimmed == "import (":
			inBlock = true
		case !inBlock && strings.HasPrefix(trimmed, "import "):
			addImport(imports, strings.TrimPrefix(trimmed, "import "))
		case inBlock && trimmed == ")":
			inBlock = false
		case inBlock:
			addImport(imports, trimmed)
		}
	}
	return imports
}

var importLineRe = regexp.MustCompile(`^(?:([A-Za-z_]\w*|\.|_)\s+)?"([^"]+)"$`)

func addImport(imports map[string]string, line string) {
	m := importLineRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return
	}
	path := m[2]
	name := m[1]
	if name == "" {
		segments := strings.Split(path, "/")
		name = segments[len(segments)-1]
	}
	if name == "_" || name == "." {
		return
	}
	imports[name] = path
}

// trustedPackage reports whether ident resolves, in this file's imports, to
// wantPath. An ident with no matching import, or one that resolves
// elsewhere, is not trusted: this is the only source of truth a qualifier
// like `actionattrs.` or `atoms.` gets.
func trustedPackage(imports map[string]string, ident, wantPath string) bool {
	return imports[ident] == wantPath
}

// scanStatements parses code as a function body (the shape of a `{{ }}`
// block) and reports whether it contains a call to a dangerousKinds
// constructor that safeComponent would reject. A code block that fails to
// parse this way is not flagged (it is not valid standalone Go), consistent
// with the rest of this checker only acting on syntax it can parse.
func scanStatements(code string, imports map[string]string) bool {
	return scanGoSource("package p\nfunc _() {\n"+code+"\n}\n", imports)
}

// scanDecls parses code as top-level declarations (the shape of Go source
// outside any templ block: imports, types, plain functions) and reports
// whether it contains a call to a dangerousKinds constructor that
// safeComponent would reject.
func scanDecls(code string, imports map[string]string) bool {
	return scanGoSource("package p\n"+code+"\n", imports)
}

func scanGoSource(wrapped string, imports map[string]string) bool {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", wrapped, 0)
	if err != nil {
		return false
	}
	unsafe := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var kind string
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			kind = fn.Sel.Name
		case *ast.Ident:
			kind = fn.Name
		}
		if !dangerousKinds[kind] {
			return true
		}
		if !safeComponent(call, imports) {
			unsafe = true
		}
		return true
	})
	return unsafe
}

// VerifyTemplUniqueness fails if any type other than actionattrs.Safe
// defines a method literally named Templ. safeAttrs trusts any bare
// `<expr>.Templ()` call as a bounded attribute source precisely because,
// today, only actionattrs.Safe has that method; this function makes that a
// checked invariant at scan time (in scripts/actioncheck/cmd), not only in
// `go test`, so a second Templ() method fails the actual check run rather
// than depending on a separate test having been run first.
func VerifyTemplUniqueness(root string) error {
	fset := token.NewFileSet()
	var found []string
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "node_modules", ".git", "vendor":
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
			found = append(found, fmt.Sprintf("%s: %s", path, recvTypeName(fn.Recv)))
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	if len(found) != 1 {
		return fmt.Errorf("expected exactly one Templ() method in the module, found %d: %v", len(found), found)
	}
	if !strings.Contains(filepath.ToSlash(strings.SplitN(found[0], ":", 2)[0]), "/components/actionattrs/") || !strings.HasSuffix(found[0], " Safe") {
		return fmt.Errorf("the one Templ() method in the module is not actionattrs.Safe.Templ: %s", found[0])
	}
	return nil
}

func recvTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return "?"
	}
	expr := recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return "?"
}

func safeElement(element *templparser.Element, imports map[string]string) bool {
	boundary := false
	classSeen := false
	for _, attr := range element.Attributes {
		switch attr := attr.(type) {
		case *templparser.SpreadAttributes:
			expr, err := parser.ParseExpr(attr.Expression.Value)
			if err != nil || !safeAttrs(expr, false, imports) {
				return false
			}
		case *templparser.ConditionalAttribute:
			if !safeConditionalAttributes(attr.Then) || !safeConditionalAttributes(attr.Else) {
				return false
			}
		case *templparser.ConstantAttribute:
			key, ok := attr.Key.(templparser.ConstantAttributeKey)
			if !ok {
				return false
			}
			if strings.EqualFold(key.Name, "class") {
				if classSeen {
					return false
				}
				classSeen = true
				if !safeTokens(attr.Value) {
					return false
				}
				boundary = hasBoundary(attr.Value)
			}
		case *templparser.ExpressionAttribute:
			key, ok := attr.Key.(templparser.ConstantAttributeKey)
			if !ok {
				return false
			}
			if strings.EqualFold(key.Name, "class") {
				if classSeen {
					return false
				}
				classSeen = true
				expr, err := parser.ParseExpr("[]any{" + attr.Expression.Value + "}")
				if err != nil {
					return false
				}
				for _, item := range expr.(*ast.CompositeLit).Elts {
					value, unconditional, ok := classValue(item, imports)
					if !ok || !safeTokens(value) {
						return false
					}
					boundary = boundary || (unconditional && hasBoundary(value))
				}
			}
		}
	}
	return boundary
}

func safeConditionalAttributes(attrs []templparser.Attribute) bool {
	for _, attr := range attrs {
		switch attr := attr.(type) {
		case *templparser.ConstantAttribute:
			if strings.EqualFold(attr.Key.String(), "class") {
				return false
			}
		case *templparser.ExpressionAttribute:
			if strings.EqualFold(attr.Key.String(), "class") {
				return false
			}
		case *templparser.BoolConstantAttribute, *templparser.BoolExpressionAttribute:
		case *templparser.ConditionalAttribute:
			if !safeConditionalAttributes(attr.Then) || !safeConditionalAttributes(attr.Else) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func classValue(expr ast.Expr, imports map[string]string) (string, bool, bool) {
	if value, ok := stringValue(expr); ok {
		return value, true, true
	}
	// actionattrs.LinkClass enforces the boundary token itself at runtime;
	// actionattrs.ButtonClass does not (it relies on its caller's element
	// already carrying an unconditional "btn" literal), so it contributes
	// an empty, trivially-safe, non-boundary token. A raw element has no
	// such caller to rely on, but ButtonClass is still accepted here
	// because safeElement's own boundary accumulation still requires some
	// other item in the same class expression to supply "btn"/"border";
	// ButtonClass alone, with no such item, correctly leaves boundary false
	// and the element rejected.
	if isActionattrsCall(expr, "LinkClass", imports) {
		return "btn", true, true
	}
	if isActionattrsCall(expr, "ButtonClass", imports) {
		return "", true, true
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return "", false, false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "KV" {
		return "", false, false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "templ" {
		return "", false, false
	}
	value, ok := stringValue(call.Args[0])
	return value, false, ok
}

// isActionattrsCall reports whether expr is a one-argument call to the
// named function, qualified by an identifier that this file's imports
// actually resolve to the real actionattrs package (see resolveImports and
// trustedPackage). An identifier named "actionattrs" that resolves
// elsewhere, or that resolves nowhere, is not trusted.
func isActionattrsCall(expr ast.Expr, name string, imports map[string]string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && trustedPackage(imports, pkg.Name, actionattrsImportPath)
}

// safeClassExpr reports whether expr is a known-safe class source: a string
// literal that passes the static checks, or a runtime-guarded
// actionattrs.ButtonClass/LinkClass call (see that package's doc comments).
// requireBoundary applies the hasBoundary literal check on the literal
// path; on the actionattrs-call path, only LinkClass is accepted when a
// boundary is required, because only LinkClass enforces one at runtime.
// ButtonClass does not, and is only safe where the caller's own element
// already carries "btn" unconditionally (atoms.Button's own definition),
// which requireBoundary=false signals.
func safeClassExpr(expr ast.Expr, requireBoundary bool, imports map[string]string) bool {
	if value, ok := stringValue(expr); ok {
		return safeTokens(value) && (!requireBoundary || hasBoundary(value))
	}
	if isActionattrsCall(expr, "LinkClass", imports) {
		return true
	}
	return !requireBoundary && isActionattrsCall(expr, "ButtonClass", imports)
}

func safeComponent(expr ast.Expr, imports map[string]string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		// A bare component value reference (`@props.Icon`, `@tab.Content`, a
		// loop variable, an indexed slice element, ...) has no call syntax to
		// carry a caller-controlled Class or Attrs override: there is no
		// composite literal here for an override to live in. Whatever
		// templ.Component value it names was already type-checked where it
		// was constructed; scanStatements/scanDecls check every place in this
		// file's Go code where such a value could have been constructed, so
		// this trust is not extended to code this checker never looked at.
		// Reject anything that is not a plain identifier/selector/index
		// chain.
		switch expr.(type) {
		case *ast.Ident, *ast.SelectorExpr, *ast.IndexExpr, *ast.StarExpr, *ast.TypeAssertExpr:
			return true
		}
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		if name, ok := call.Fun.(*ast.Ident); ok {
			switch name.Name {
			case "Anchor", "Link", "Button", "ConfirmButton", "SubmitButton", "SubmitButtonStyled":
				return false
			}
		}
		return true
	}
	kind := selector.Sel.Name
	if kind == "SubmitButton" {
		pkg, ok := selector.X.(*ast.Ident)
		return ok && trustedPackage(imports, pkg.Name, moleculesImportPath) && len(call.Args) == 2 && safeAttrs(call.Args[1], false, imports)
	}
	if kind == "ConfirmButton" || kind == "SubmitButtonStyled" {
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || !trustedPackage(imports, pkg.Name, moleculesImportPath) {
			return false
		}
		classIndex, attrsIndex := 2, 3
		if kind == "SubmitButtonStyled" {
			classIndex, attrsIndex = 1, 2
		}
		if len(call.Args) != attrsIndex+1 {
			return false
		}
		return safeClassExpr(call.Args[classIndex], false, imports) && safeAttrs(call.Args[attrsIndex], false, imports)
	}
	if kind != "Anchor" && kind != "Link" && kind != "Button" {
		return true
	}
	if len(call.Args) != 1 {
		return false
	}
	props, ok := call.Args[0].(*ast.CompositeLit)
	if !ok {
		return false
	}
	propsType, ok := props.Type.(*ast.SelectorExpr)
	if !ok || propsType.Sel.Name != kind+"Props" {
		return false
	}
	componentPackage, componentOK := selector.X.(*ast.Ident)
	propsPackage, propsOK := propsType.X.(*ast.Ident)
	if !componentOK || !propsOK || !trustedPackage(imports, componentPackage.Name, atomsImportPath) || !trustedPackage(imports, propsPackage.Name, atomsImportPath) {
		return false
	}
	hasClass := false
	for _, field := range props.Elts {
		kv, ok := field.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return false
		}
		switch key.Name {
		case "Class":
			if !safeClassExpr(kv.Value, kind != "Button", imports) {
				return false
			}
			hasClass = true
		case "Attrs":
			if !safeAttrs(kv.Value, false, imports) {
				return false
			}
		}
	}
	return hasClass || kind == "Button"
}

func safeAttrs(expr ast.Expr, allowClass bool, imports map[string]string) bool {
	if id, ok := expr.(*ast.Ident); ok && id.Name == "nil" {
		return true
	}
	// A bare `<expr>.Templ()` call is trusted: in this module only
	// actionattrs.Safe defines a Templ() method (VerifyTemplUniqueness
	// checks this at scan time; TestTemplMethodIsUnique checks it in `go
	// test`), and its only constructors enforce an attribute key allow-list.
	// See the actionattrs package doc comment.
	if call, ok := expr.(*ast.CallExpr); ok && len(call.Args) == 0 {
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Templ" {
			return true
		}
	}
	// actionattrs.From(expr) validates every key of its argument against the
	// same allow-list at runtime and panics before render if one is missing;
	// see the actionattrs package doc comment.
	if isActionattrsCall(expr, "From", imports) {
		return true
	}
	literal, ok := expr.(*ast.CompositeLit)
	if !ok {
		return false
	}
	typ, ok := literal.Type.(*ast.SelectorExpr)
	if !ok || typ.Sel.Name != "Attributes" {
		return false
	}
	pkg, ok := typ.X.(*ast.Ident)
	if !ok || pkg.Name != "templ" {
		return false
	}
	for _, entry := range literal.Elts {
		kv, ok := entry.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key, ok := stringValue(kv.Key)
		if !ok {
			return false
		}
		if strings.EqualFold(key, "class") {
			value, ok := stringValue(kv.Value)
			if !allowClass || !ok || !safeTokens(value) {
				return false
			}
		}
	}
	return true
}

func stringValue(expr ast.Expr) (string, bool) {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func safeTokens(value string) bool {
	for _, token := range strings.Fields(value) {
		base := token[strings.LastIndex(token, ":")+1:]
		// Tailwind's "important" modifier prefixes the utility with "!"
		// (e.g. "!border-0", "md:!border-0"); strip it before comparing.
		base = strings.TrimPrefix(base, "!")
		if base == "btn-ghost" || base == "btn-link" || base == "border-0" {
			return false
		}
	}
	return true
}

func hasBoundary(value string) bool {
	for _, token := range strings.Fields(value) {
		if token == "btn" || token == "border" {
			return true
		}
	}
	return false
}
