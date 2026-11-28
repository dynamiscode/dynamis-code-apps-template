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

	"example.com/dynamis-code/apps-template/internal/web/components/actionattrs"
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
	"Anchor": true, "Link": true, "Button": true, "Dropdown": true,
	"ConfirmButton": true, "SubmitButton": true, "SubmitButtonStyled": true,
}

// unprovableSelectors names templ package calls that can embed arbitrary
// content -- a raw HTML string, a hand-written render function -- this
// checker has no AST left to inspect once it is built. A call to any of
// these, anywhere, is always a violation: there is no amount of static
// analysis that proves a raw string or an opaque function is bounded.
var unprovableSelectors = map[string]bool{"Raw": true, "ComponentFunc": true, "FromGoHTML": true}

// isUnprovableCall reports whether call invokes one of unprovableSelectors,
// qualified by an identifier literally named "templ" (the same convention,
// not import-path-verified, this checker already uses for templ.Attributes/
// templ.KV/templ.SafeURL; spoofing "templ" itself is a materially bigger
// and more visible change than spoofing actionattrs/atoms/molecules, which
// import-path verification covers).
func isUnprovableCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "templ" && unprovableSelectors[selector.Sel.Name]
}

// isDangerousCall reports whether call is a genuine candidate for
// safeComponentCall: a call to one of dangerousKinds, qualified by a
// package this file's imports actually resolve to the real atoms or
// molecules package (see trustedPackage), or -- the same-package-
// forwarding shape safeComponentCall always rejects -- an unqualified call
// to one of those names in a file that is itself part of the atoms or
// molecules package. Matching by name alone, with no package or package-
// membership check, would also flag any unrelated function or method that
// merely happens to share one of these names (an unrelated type's own
// Button() method, say); this is what keeps that firmly out of scope.
func isDangerousCall(call *ast.CallExpr, imports map[string]string, pkgName string) bool {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		if !dangerousKinds[fn.Sel.Name] {
			return false
		}
		qualifier, ok := fn.X.(*ast.Ident)
		if !ok {
			return false
		}
		return trustedPackage(imports, qualifier.Name, atomsImportPath) || trustedPackage(imports, qualifier.Name, moleculesImportPath)
	case *ast.Ident:
		return dangerousKinds[fn.Name] && (pkgName == "atoms" || pkgName == "molecules")
	}
	return false
}

// findUnsafeCalls walks root for any call that is either unprovable
// (isUnprovableCall) or a dangerous construction (isDangerousCall) that
// fails safeComponentCall, and reports whether it found at least one.
func isOpenListCall(call *ast.CallExpr, imports map[string]string, pkgName string) bool {
	name := callKindName(call)
	if name != "ListScreen" && name != "ListResults" {
		return false
	}
	switch function := call.Fun.(type) {
	case *ast.Ident:
		return pkgName == "organisms"
	case *ast.SelectorExpr:
		qualifier, ok := function.X.(*ast.Ident)
		return ok && imports[qualifier.Name] == "example.com/dynamis-code/apps-template/internal/web/components/organisms"
	}
	return false
}

func findUnsafeCalls(root ast.Node, imports map[string]string, pkgName string) bool {
	unsafe := false
	ast.Inspect(root, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isUnprovableCall(call) || isOpenListCall(call, imports, pkgName) {
			unsafe = true
			return true
		}
		if isDangerousCall(call, imports, pkgName) && !safeComponentCall(call, imports) {
			unsafe = true
		}
		return true
	})
	return unsafe
}

// packageName extracts a templ or Go file's own package name from its
// source text (the first "package X" line), for isDangerousCall's
// same-package-forwarding check. An absent or unparseable package clause
// yields "", which isDangerousCall's bare-Ident branch never matches --
// fail-closed in the sense that it narrows what counts as dangerous,
// consistent with isDangerousCall's purpose of avoiding false positives on
// code with no real relationship to atoms/molecules.
var packageClauseRe = regexp.MustCompile(`(?m)^\s*package\s+(\w+)`)

func packageName(source string) string {
	m := packageClauseRe.FindStringSubmatch(source)
	if m == nil {
		return ""
	}
	return m[1]
}

// Check fails closed on dynamic classes, attribute spreads, and unresolved props.
func Check(source string) ([]Violation, error) {
	return CheckPath(source, "", nil)
}

// CheckGoFile reports dangerousKinds constructions in a plain .go file (not
// a .templ file). CheckPath only ever sees one .templ file's own Go-like
// text; a plain .go helper -- for example a file alongside a .templ one in
// the same package, building and returning a templ.Component -- is never
// scanned at all unless something calls CheckGoFile on it too, yet its
// result can flow into any .templ file's bare component reference
// (safeComponent's doc comment) exactly as if it had been built in that
// file's own {{ }} block.
func CheckGoFile(source, path string) ([]Violation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, 0)
	if err != nil {
		return nil, err
	}
	imports := map[string]string{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := ""
		if imp.Name != nil {
			name = imp.Name.Name
		} else {
			segments := strings.Split(path, "/")
			name = segments[len(segments)-1]
		}
		if name == "_" || name == "." {
			continue
		}
		imports[name] = path
	}
	if names := trustedIdents(imports); len(names) > 0 && astDeclaresAnyName(file, names) {
		for name := range names {
			delete(imports, name)
		}
	}
	pkgName := ""
	if file.Name != nil {
		pkgName = file.Name.Name
	}
	var violations []Violation
	type packageFile struct {
		file    *ast.File
		imports map[string]string
	}
	files := []packageFile{{file, imports}}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_templ.go") || name == filepath.Base(path) {
			continue
		}
		other, err := parser.ParseFile(token.NewFileSet(), filepath.Join(filepath.Dir(path), name), nil, 0)
		if err != nil {
			return nil, err
		}
		if other.Name.Name != pkgName {
			continue
		}
		otherImports := map[string]string{}
		for _, imp := range other.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return nil, err
			}
			qualifier := filepath.Base(importPath)
			if imp.Name != nil {
				qualifier = imp.Name.Name
			}
			otherImports[qualifier] = importPath
		}
		files = append(files, packageFile{other, otherImports})
	}
	aliases := map[string]bool{}
	componentType := func(expr ast.Expr, imports map[string]string) bool {
		if name, ok := expr.(*ast.Ident); ok {
			return aliases[name.Name]
		}
		selector, ok := expr.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Component" {
			return false
		}
		qualifier, ok := selector.X.(*ast.Ident)
		return ok && imports[qualifier.Name] == "github.com/a-h/templ"
	}
	for changed := true; changed; {
		changed = false
		for _, source := range files {
			for _, declaration := range source.file.Decls {
				group, ok := declaration.(*ast.GenDecl)
				if !ok || group.Tok != token.TYPE {
					continue
				}
				for _, spec := range group.Specs {
					typeSpec := spec.(*ast.TypeSpec)
					if !aliases[typeSpec.Name.Name] && componentType(typeSpec.Type, source.imports) {
						aliases[typeSpec.Name.Name], changed = true, true
					}
				}
			}
		}
	}
	templates, err := templateNames(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	packageASTs := make([]*ast.File, 0, len(files))
	for _, source := range files {
		packageASTs = append(packageASTs, source.file)
	}
	if line, ok := dynamicTemplateRenderViolation(file, packageASTs, imports, templates, fset); ok {
		violations = append(violations, Violation{line, "unverified dynamic component renderer"})
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Type.Results == nil || function.Body == nil {
			continue
		}
		var componentIndexes []int
		index := 0
		for _, result := range function.Type.Results.List {
			count := len(result.Names)
			if count == 0 {
				count = 1
			}
			if componentType(result.Type, imports) {
				for i := 0; i < count; i++ {
					componentIndexes = append(componentIndexes, index+i)
				}
			}
			index += count
		}
		if len(componentIndexes) == 0 {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			statement, ok := node.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			for _, index := range componentIndexes {
				if index >= len(statement.Results) {
					violations = append(violations, Violation{uint32(fset.Position(statement.Pos()).Line), "unverified component returned by Go constructor"})
					continue
				}
				result := statement.Results[index]
				if name, ok := result.(*ast.Ident); ok && name.Name == "nil" {
					continue
				}
				if call, ok := result.(*ast.CallExpr); ok {
					if name, ok := call.Fun.(*ast.Ident); ok && templates[name.Name] {
						nestedCall := false
						for _, argument := range call.Args {
							ast.Inspect(argument, func(node ast.Node) bool {
								if _, ok := node.(*ast.CallExpr); ok {
									nestedCall = true
								}
								return true
							})
						}
						if !nestedCall {
							continue
						}
					}
				}
				violations = append(violations, Violation{uint32(fset.Position(result.Pos()).Line), "unverified component returned by Go constructor"})
			}
			return true
		})
	}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		unsafe := isUnprovableCall(call) || isOpenListCall(call, imports, pkgName) || (isDangerousCall(call, imports, pkgName) && !safeComponentCall(call, imports))
		if unsafe {
			violations = append(violations, Violation{uint32(fset.Position(call.Pos()).Line), "unbounded or unresolved action component built in Go code"})
		}
		return true
	})
	return violations, nil
}

// dynamicTemplateRenderViolation verifies function-valued template renderers
// from every same-package callsite. It trusts only callbacks that resolve to
// local templ declarations, so a callback cannot hide a raw component behind
// a function parameter.
func dynamicTemplateRenderViolation(current *ast.File, files []*ast.File, imports map[string]string, templates map[string]bool, fset *token.FileSet) (uint32, bool) {
	for _, declaration := range current.Decls {
		renderer, ok := declaration.(*ast.FuncDecl)
		if !ok || renderer.Body == nil {
			continue
		}
		paramName, paramIndex, dataName, matches, malformed := templateRendererParam(renderer, imports)
		if !matches {
			continue
		}
		if malformed || !callsTemplateRenderer(renderer.Body, paramName, dataName) {
			return uint32(fset.Position(renderer.Pos()).Line), true
		}
		callCount := 0
		safe := true
		for _, file := range files {
			for _, declaration := range file.Decls {
				caller, ok := declaration.(*ast.FuncDecl)
				if !ok || caller.Body == nil {
					continue
				}
				ast.Inspect(caller.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok || !callsNamedFunction(call, renderer) {
						return true
					}
					callCount++
					if paramIndex >= len(call.Args) || !trustedTemplateValue(call.Args[paramIndex], caller.Body, templates, make(map[string]bool)) {
						safe = false
					}
					return true
				})
			}
		}
		if callCount == 0 || !safe {
			return uint32(fset.Position(renderer.Pos()).Line), true
		}
	}
	return 0, false
}

func templateRendererParam(function *ast.FuncDecl, imports map[string]string) (string, int, string, bool, bool) {
	if function.Type.Params == nil {
		return "", 0, "", false, false
	}
	var callbackName, dataName string
	callbackIndex := 0
	index := 0
	for _, field := range function.Type.Params.List {
		if callbackName == "" && isTemplateCallbackType(field.Type, imports) && len(field.Names) == 1 {
			callbackName = field.Names[0].Name
			callbackIndex = index
		}
		if dataName == "" && isViewmodelPageData(field.Type, imports) && len(field.Names) == 1 {
			dataName = field.Names[0].Name
		}
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		index += count
	}
	if callbackName == "" {
		return "", 0, "", false, false
	}
	return callbackName, callbackIndex, dataName, true, dataName == ""
}

func isTemplateCallbackType(expr ast.Expr, imports map[string]string) bool {
	function, ok := expr.(*ast.FuncType)
	if !ok || function.Results == nil || len(function.Results.List) != 1 || function.Params == nil || len(function.Params.List) != 1 {
		return false
	}
	return isViewmodelPageData(function.Params.List[0].Type, imports) && isTemplComponentType(function.Results.List[0].Type, imports)
}

func isTemplComponentType(expr ast.Expr, imports map[string]string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Component" {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && imports[pkg.Name] == "github.com/a-h/templ"
}

func isViewmodelPageData(expr ast.Expr, imports map[string]string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "PageData" {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && imports[pkg.Name] == "example.com/dynamis-code/apps-template/internal/web/viewmodel"
}

func callsTemplateRenderer(body *ast.BlockStmt, component, data string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Render" {
			return true
		}
		rendered, ok := selector.X.(*ast.CallExpr)
		if !ok || len(rendered.Args) != 1 {
			return true
		}
		fn, ok := rendered.Fun.(*ast.Ident)
		arg, argOK := rendered.Args[0].(*ast.Ident)
		if ok && fn.Name == component && argOK && arg.Name == data {
			found = true
		}
		return true
	})
	return found
}

func callsNamedFunction(call *ast.CallExpr, function *ast.FuncDecl) bool {
	switch callee := call.Fun.(type) {
	case *ast.Ident:
		return function.Recv == nil && callee.Name == function.Name.Name
	case *ast.SelectorExpr:
		return function.Recv != nil && callee.Sel.Name == function.Name.Name
	}
	return false
}

func trustedTemplateValue(expr ast.Expr, body *ast.BlockStmt, templates map[string]bool, visiting map[string]bool) bool {
	switch value := expr.(type) {
	case *ast.Ident:
		if templates[value.Name] && !declaresLocalName(body, value.Name) {
			return true
		}
		if visiting[value.Name] {
			return false
		}
		visiting[value.Name] = true
		defer delete(visiting, value.Name)
		return trustedTemplateVariable(value.Name, body, templates, visiting)
	case *ast.FuncLit:
		return trustedTemplateReturns(value.Body, value.Type.Params, templates)
	}
	return false
}

func trustedTemplateVariable(name string, body *ast.BlockStmt, templates map[string]bool, visiting map[string]bool) bool {
	found, safe := false, true
	ast.Inspect(body, func(node ast.Node) bool {
		switch statement := node.(type) {
		case *ast.AssignStmt:
			for i, lhs := range statement.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok || id.Name != name {
					continue
				}
				found = true
				if i >= len(statement.Rhs) || !trustedTemplateValue(statement.Rhs[i], body, templates, visiting) {
					safe = false
				}
			}
		case *ast.ValueSpec:
			for i, id := range statement.Names {
				if id.Name != name {
					continue
				}
				found = true
				if i >= len(statement.Values) || !trustedTemplateValue(statement.Values[i], body, templates, visiting) {
					safe = false
				}
			}
		}
		return true
	})
	return found && safe
}

func trustedTemplateReturns(body *ast.BlockStmt, params *ast.FieldList, templates map[string]bool) bool {
	if body == nil {
		return false
	}
	for _, param := range params.List {
		for _, name := range param.Names {
			if declaresLocalName(body, name.Name) {
				return false
			}
		}
	}
	found, safe := false, true
	ast.Inspect(body, func(node ast.Node) bool {
		statement, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		found = true
		if len(statement.Results) != 1 || !trustedTemplateCall(statement.Results[0], templates, body) {
			safe = false
		}
		return true
	})
	return found && safe
}

func trustedTemplateCall(expr ast.Expr, templates map[string]bool, body *ast.BlockStmt) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	name, ok := call.Fun.(*ast.Ident)
	if !ok || !templates[name.Name] || declaresLocalName(body, name.Name) {
		return false
	}
	safe := true
	for _, arg := range call.Args {
		ast.Inspect(arg, func(node ast.Node) bool {
			nested, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := callKindName(nested)
			switch name {
			case "Raw", "ComponentFunc", "FromGoHTML", "WithChildren":
				safe = false
			default:
				if templates[name] {
					safe = false
				}
			}
			return safe
		})
	}
	return safe
}

func declaresLocalName(body *ast.BlockStmt, want string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		switch declaration := node.(type) {
		case *ast.ValueSpec:
			for _, name := range declaration.Names {
				found = found || name.Name == want
			}
		case *ast.TypeSpec:
			found = found || declaration.Name.Name == want
		case *ast.AssignStmt:
			if declaration.Tok == token.DEFINE {
				for _, lhs := range declaration.Lhs {
					if name, ok := lhs.(*ast.Ident); ok {
						found = found || name.Name == want
					}
				}
			}
		case *ast.RangeStmt:
			if declaration.Tok == token.DEFINE {
				for _, expr := range []ast.Expr{declaration.Key, declaration.Value} {
					if name, ok := expr.(*ast.Ident); ok {
						found = found || name.Name == want
					}
				}
			}
		}
		return !found
	})
	return found
}

func localGoFunctions(dir string) (map[string]bool, error) {
	names := map[string]bool{"len": true, "cap": true, "string": true, "int": true, "int64": true, "uint": true, "uint64": true}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return names, nil
	}
	if err != nil {
		return nil, err
	}
	collect := func(source string) error {
		file, err := parser.ParseFile(token.NewFileSet(), "", source, 0)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && function.Recv == nil {
				names[function.Name.Name] = true
			}
		}
		return nil
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_templ.go") {
			continue
		}
		if !strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, ".templ") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(name, ".go") {
			if err := collect(string(source)); err != nil {
				return nil, err
			}
			continue
		}
		file, err := templparser.ParseString(string(source))
		if err != nil {
			return nil, err
		}
		v := visitor.New()
		visit := v.TemplateFileGoExpression
		v.TemplateFileGoExpression = func(code *templparser.TemplateFileGoExpression) error {
			if err := collect("package p\n" + code.Expression.Value); err != nil {
				return err
			}
			return visit(code)
		}
		if err := file.Visit(v); err != nil {
			return nil, err
		}
	}
	return names, nil
}

func externalGoFunctions(dir string) (map[string]bool, error) {
	names := map[string]bool{}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return names, nil
	}
	if err != nil {
		return nil, err
	}
	type parsedFile struct {
		file    *ast.File
		imports map[string]string
	}
	var files []parsedFile
	unsafeTypes := map[string]bool{"any": true}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_templ.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, 0)
		if err != nil {
			return nil, err
		}
		imports := map[string]string{}
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return nil, err
			}
			name := filepath.Base(path)
			if imp.Name != nil {
				name = imp.Name.Name
			}
			imports[name] = path
		}
		files = append(files, parsedFile{file, imports})
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || function.Name.Name != "Render" {
				continue
			}
			receiver := function.Recv.List[0].Type
			if pointer, ok := receiver.(*ast.StarExpr); ok {
				receiver = pointer.X
			}
			if ident, ok := receiver.(*ast.Ident); ok {
				unsafeTypes[ident.Name] = true
			}
		}
	}
	unsafeType := func(expr ast.Expr, imports map[string]string) bool {
		unsafe := false
		ast.Inspect(expr, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.InterfaceType:
				unsafe = true
			case *ast.Ident:
				unsafe = unsafe || unsafeTypes[value.Name]
			case *ast.SelectorExpr:
				if qualifier, ok := value.X.(*ast.Ident); ok && imports[qualifier.Name] != "" && !strings.HasPrefix(imports[qualifier.Name], "example.com/dynamis-code/apps-template/") {
					unsafe = true
				}
			}
			return true
		})
		return unsafe
	}
	for changed := true; changed; {
		changed = false
		for _, source := range files {
			for _, declaration := range source.file.Decls {
				group, ok := declaration.(*ast.GenDecl)
				if !ok || group.Tok != token.TYPE {
					continue
				}
				for _, spec := range group.Specs {
					typeSpec := spec.(*ast.TypeSpec)
					if !unsafeTypes[typeSpec.Name.Name] && unsafeType(typeSpec.Type, source.imports) {
						unsafeTypes[typeSpec.Name.Name], changed = true, true
					}
				}
			}
		}
	}
	for _, source := range files {
		for _, declaration := range source.file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || function.Type.Results == nil {
				continue
			}
			for _, result := range function.Type.Results.List {
				if unsafeType(result.Type, source.imports) {
					names[function.Name.Name] = true
				}
			}
		}
	}
	return names, nil
}

func templateNames(dir string) (map[string]bool, error) {
	names := make(map[string]bool)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return names, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".templ") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		file, err := templparser.ParseString(string(source))
		if err != nil {
			return nil, err
		}
		v := visitor.New()
		visit := v.HTMLTemplate
		v.HTMLTemplate = func(template *templparser.HTMLTemplate) error {
			declarations, err := parser.ParseFile(token.NewFileSet(), "", "package p\nfunc "+template.Expression.Value+" {}", 0)
			if err != nil {
				return err
			}
			names[declarations.Decls[0].(*ast.FuncDecl).Name.Name] = true
			return visit(template)
		}
		if err := file.Visit(v); err != nil {
			return nil, err
		}
	}
	return names, nil
}

// CheckPath accepts only exact, uniquely occurring opening tags in the named file.
func CheckPath(source, path string, exceptions []Exception) ([]Violation, error) {
	file, err := templparser.ParseString(source)
	if err != nil {
		return nil, err
	}
	imports := resolveImports(source)
	revokeShadowedTrust(file, imports)
	pkgName := packageName(source)
	var packageTemplates, externalFactories, localFunctions map[string]bool
	if _, err := os.Stat(path); path != "" && err == nil {
		packageTemplates, err = templateNames(filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		externalFactories, err = externalGoFunctions(filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		localFunctions, err = localGoFunctions(filepath.Dir(path))
		if err != nil {
			return nil, err
		}
	}
	importedTemplates := make(map[string]map[string]bool)
	var violations []Violation
	matches := make([]int, len(exceptions))
	v := visitor.New()
	executableAttrs := make(map[string]int)
	checkAttribute := func(key templparser.AttributeKey, source string, line uint32) {
		constant, ok := key.(templparser.ConstantAttributeKey)
		if !ok {
			violations = append(violations, Violation{line + 1, "unverified dynamic HTML attribute key"})
			return
		}
		if actionattrs.ExecutableAttribute(constant.Name) {
			executableAttrs[source]++
			if executableAttrs[source] > reviewedExecutableAttributes[path][source] {
				violations = append(violations, Violation{line + 1, "unreviewed executable attribute " + constant.Name})
			}
		}
	}
	v.ConstantAttribute = func(node *templparser.ConstantAttribute) error {
		checkAttribute(node.Key, node.String(), node.Range.From.Line)
		return nil
	}
	v.ExpressionAttribute = func(node *templparser.ExpressionAttribute) error {
		checkAttribute(node.Key, node.String(), node.Range.From.Line)
		return nil
	}
	v.BoolConstantAttribute = func(node *templparser.BoolConstantAttribute) error {
		checkAttribute(node.Key, node.String(), node.Range.From.Line)
		return nil
	}
	v.BoolExpressionAttribute = func(node *templparser.BoolExpressionAttribute) error {
		checkAttribute(node.Key, node.String(), node.Range.From.Line)
		return nil
	}
	visitElement := v.Element
	var elementParents []string
	seenScannedSlots := make(map[string]int)
	visitScript := v.ScriptElement
	seenReviewedScripts := make(map[string]bool)
	v.ScriptElement = func(element *templparser.ScriptElement) error {
		if reason := checkTemplateScript(source, path, element, seenReviewedScripts); reason != "" {
			violations = append(violations, Violation{element.Range.From.Line + 1, reason})
		}
		return visitScript(element)
	}
	v.ScriptTemplate = func(script *templparser.ScriptTemplate) error {
		violations = append(violations, Violation{script.Range.From.Line + 1, "unreviewed templ script declaration"})
		return nil
	}
	v.Element = func(element *templparser.Element) error {
		if strings.EqualFold(element.Name, "script") {
			violations = append(violations, Violation{element.Range.From.Line + 1, "unverified browser script element"})
		}
		if strings.EqualFold(element.Name, "a") || strings.EqualFold(element.Name, "button") {
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
		elementParents = append(elementParents, element.Name)
		err := visitElement(element)
		elementParents = elementParents[:len(elementParents)-1]
		return err
	}
	visitCall := v.TemplElementExpression
	v.TemplElementExpression = func(call *templparser.TemplElementExpression) error {
		expr, err := parser.ParseExpr(call.Expression.Value)
		for _, parent := range elementParents {
			if strings.EqualFold(parent, "svg") || strings.EqualFold(parent, "math") {
				violations = append(violations, Violation{call.Range.From.Line + 1, "unverified component in foreign HTML context"})
				break
			}
		}
		if err == nil && path != "" && isActionattrsCall(expr, "Scanned", imports) {
			slot := strings.Join(elementParents, "/") + " @ " + strings.TrimSpace(call.Expression.Value)
			seenScannedSlots[slot]++
			if seenScannedSlots[slot] > reviewedScannedSlots[path][slot] {
				violations = append(violations, Violation{call.Range.From.Line + 1, "unreviewed opaque component slot " + slot})
			}
		}
		if err != nil {
			violations = append(violations, Violation{call.Range.From.Line + 1, "unsupported component expression"})
		} else if !safeComponent(expr, imports, pkgName) {
			violations = append(violations, Violation{call.Range.From.Line + 1, "unbounded or unresolved action component"})
		} else if packageTemplates != nil {
			if component, ok := expr.(*ast.CallExpr); ok {
				for _, argument := range component.Args {
					ast.Inspect(argument, func(node ast.Node) bool {
						nested, ok := node.(*ast.CallExpr)
						if !ok {
							return true
						}
						if name, ok := nested.Fun.(*ast.Ident); ok && (externalFactories[name.Name] || !localFunctions[name.Name] && !packageTemplates[name.Name]) {
							violations = append(violations, Violation{call.Range.From.Line + 1, "unverified nested component constructor " + name.Name})
						}
						return true
					})
				}
				proven := false
				switch function := component.Fun.(type) {
				case *ast.Ident:
					proven = packageTemplates[function.Name]
				case *ast.SelectorExpr:
					if qualifier, ok := function.X.(*ast.Ident); ok {
						importPath := imports[qualifier.Name]
						if importPath == actionattrsImportPath && (function.Sel.Name == "Scanned" || function.Sel.Name == "Markdown") || qualifier.Name == "templ" && function.Sel.Name == "JSONScript" && importPath == "" {
							proven = true
						} else if strings.HasPrefix(importPath, "example.com/dynamis-code/apps-template/internal/web/") {
							if _, ok := importedTemplates[importPath]; !ok {
								var err error
								importedTemplates[importPath], err = templateNames(strings.TrimPrefix(importPath, "example.com/dynamis-code/apps-template/"))
								if err != nil {
									return err
								}
							}
							proven = importedTemplates[importPath][function.Sel.Name]
						}
					}
				}
				if !proven {
					violations = append(violations, Violation{call.Range.From.Line + 1, "unverified component constructor"})
				}
			}
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
		if unsafe := scanStatements(code.Expression.Value, imports, pkgName); unsafe {
			violations = append(violations, Violation{code.Range.From.Line + 1, "unbounded or unresolved action component built in Go code"})
		}
		if hasRogueTemplField("package p\nfunc _() {\n" + code.Expression.Value + "\n}\n") {
			violations = append(violations, Violation{code.Range.From.Line + 1, "struct field literally named Templ bypasses the action-attribute guard"})
		}
		return visitGoCode(code)
	}
	visitFileGo := v.TemplateFileGoExpression
	v.TemplateFileGoExpression = func(code *templparser.TemplateFileGoExpression) error {
		if unsafe := scanDecls(code.Expression.Value, imports, pkgName); unsafe {
			violations = append(violations, Violation{code.Expression.Range.From.Line + 1, "unbounded or unresolved action component built in Go code"})
		}
		if hasRogueTemplField("package p\n" + code.Expression.Value + "\n") {
			violations = append(violations, Violation{code.Expression.Range.From.Line + 1, "struct field literally named Templ bypasses the action-attribute guard"})
		}
		return visitFileGo(code)
	}
	visitTemplate := v.HTMLTemplate
	v.HTMLTemplate = func(template *templparser.HTMLTemplate) error {
		// A templ function's own parameter list can declare an anonymous
		// struct type inline (`templ Demo(evil struct{ Templ func()
		// templ.Attributes })`); VerifyTemplUniqueness only ever scanned
		// plain .go files, so this shape -- otherwise identical to the
		// struct-field trick its doc comment describes -- was invisible to
		// anything.
		if hasRogueTemplField("package p\nfunc " + template.Expression.Value + " {}\n") {
			violations = append(violations, Violation{template.Range.From.Line + 1, "struct field literally named Templ bypasses the action-attribute guard"})
		}
		return visitTemplate(template)
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

// trustedPaths are the import paths safeComponent/safeAttrs place trust in,
// whatever local identifier a file imports them under.
var trustedPaths = map[string]bool{actionattrsImportPath: true, atomsImportPath: true, moleculesImportPath: true}

// trustedIdents returns the set of local identifiers that, in this file's
// own imports, resolve to one of trustedPaths -- not a fixed guess like
// {"actionattrs", "atoms", "molecules"}, which would miss a file that
// imports the real package under a different alias (`import aa
// "...actionattrs"`) and would then fail to notice a local shadow of "aa"
// specifically.
func trustedIdents(imports map[string]string) map[string]bool {
	idents := map[string]bool{}
	for name, path := range imports {
		if trustedPaths[path] {
			idents[name] = true
		}
	}
	return idents
}

// revokeShadowedTrust deletes every entry of imports resolving to a
// trustedPaths package (see trustedIdents) that this file also declares as
// a local variable, range variable, or function parameter anywhere -- a
// templ function parameter, a `{{ }}` block's `:=`, or a top-level
// var/const/type/func. Go's own scoping would let such a local binding
// shadow the package import within its block; this checker has no
// per-scope resolution to tell a shadowed use from a real one, so it fails
// closed for the whole file instead: once a file declares a local reusing
// one of its own trusted import names anywhere, nothing in that file gets
// the corresponding package's trust, even outside that binding's real
// scope. Legitimate code has no reason to name a local that way.
func revokeShadowedTrust(file *templparser.TemplateFile, imports map[string]string) {
	names := trustedIdents(imports)
	if len(names) == 0 {
		return
	}
	revoke := func() {
		for name := range names {
			delete(imports, name)
		}
	}
	v := visitor.New()
	visitGoCode := v.GoCode
	v.GoCode = func(code *templparser.GoCode) error {
		if declaresAnyName("package p\nfunc _() {\n"+code.Expression.Value+"\n}\n", names) {
			revoke()
		}
		return visitGoCode(code)
	}
	visitFileGo := v.TemplateFileGoExpression
	v.TemplateFileGoExpression = func(code *templparser.TemplateFileGoExpression) error {
		if declaresAnyName("package p\n"+code.Expression.Value+"\n", names) {
			revoke()
		}
		return visitFileGo(code)
	}
	visitTemplate := v.HTMLTemplate
	v.HTMLTemplate = func(template *templparser.HTMLTemplate) error {
		if declaresAnyName("package p\nfunc "+template.Expression.Value+" {}\n", names) {
			revoke()
		}
		return visitTemplate(template)
	}
	_ = file.Visit(v)
}

// declaresAnyName reports whether wrapped (already-valid Go source)
// declares a variable, range variable, parameter, result, type, or function
// named anything in names, anywhere -- including nested inside a function
// body, not only at top level. A parse error yields false, same as
// elsewhere in this checker.
func declaresAnyName(wrapped string, names map[string]bool) bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", wrapped, 0)
	if err != nil {
		return false
	}
	return astDeclaresAnyName(f, names)
}

// astDeclaresAnyName is declaresAnyName's logic over an already-parsed Go
// AST node, reused by CheckGoFile for real .go files parsed once with
// go/parser directly.
func astDeclaresAnyName(root ast.Node, names map[string]bool) bool {
	found := false
	idents := func(list []*ast.Ident) {
		for _, id := range list {
			if names[id.Name] {
				found = true
			}
		}
	}
	ast.Inspect(root, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			if node.Tok == token.DEFINE {
				for _, lhs := range node.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && names[id.Name] {
						found = true
					}
				}
			}
		case *ast.ValueSpec:
			idents(node.Names)
		case *ast.TypeSpec:
			if names[node.Name.Name] {
				found = true
			}
		case *ast.FuncDecl:
			if names[node.Name.Name] {
				found = true
			}
			if node.Recv != nil {
				for _, field := range node.Recv.List {
					idents(field.Names)
				}
			}
			for _, field := range node.Type.Params.List {
				idents(field.Names)
			}
			if node.Type.Results != nil {
				for _, field := range node.Type.Results.List {
					idents(field.Names)
				}
			}
		case *ast.FuncLit:
			for _, field := range node.Type.Params.List {
				idents(field.Names)
			}
		case *ast.RangeStmt:
			if node.Tok == token.DEFINE {
				for _, expr := range []ast.Expr{node.Key, node.Value} {
					if id, ok := expr.(*ast.Ident); ok && names[id.Name] {
						found = true
					}
				}
			}
		}
		return true
	})
	return found
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
func scanStatements(code string, imports map[string]string, pkgName string) bool {
	return scanGoSource("package p\nfunc _() {\n"+code+"\n}\n", imports, pkgName)
}

// scanDecls parses code as top-level declarations (the shape of Go source
// outside any templ block: imports, types, plain functions) and reports
// whether it contains a call to a dangerousKinds constructor that
// safeComponent would reject.
func scanDecls(code string, imports map[string]string, pkgName string) bool {
	return scanGoSource("package p\n"+code+"\n", imports, pkgName)
}

func scanGoSource(wrapped string, imports map[string]string, pkgName string) bool {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", wrapped, 0)
	if err != nil {
		return false
	}
	return findUnsafeCalls(file, imports, pkgName)
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
		// ast.Inspect walks the whole file, not just its top-level
		// declarations: a struct type (and the Templ-named field or method
		// trick above) can just as easily be declared inside a function body
		// as at package level, and would be invisible to safeAttrs' identical
		// trust either way.
		hasTemplField := func(struct_ *ast.StructType) bool {
			for _, field := range struct_.Fields.List {
				for _, name := range field.Names {
					if name.Name == "Templ" {
						return true
					}
				}
			}
			return false
		}
		namedStructs := map[*ast.StructType]bool{}
		ast.Inspect(src, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncDecl:
				if node.Recv != nil && node.Name.Name == "Templ" {
					found = append(found, fmt.Sprintf("%s: method on %s", path, recvTypeName(node.Recv)))
				}
			case *ast.TypeSpec:
				struct_, ok := node.Type.(*ast.StructType)
				if !ok {
					return true
				}
				// Recorded here, by pointer, so the generic *ast.StructType case
				// below (which ast.Inspect will also visit, as this node's
				// child) does not count the same named struct a second time.
				namedStructs[struct_] = true
				if hasTemplField(struct_) {
					found = append(found, fmt.Sprintf("%s: field on %s", path, node.Name.Name))
				}
			case *ast.StructType:
				// Matched directly, not only through a wrapping *ast.TypeSpec:
				// an anonymous struct type (`struct{ Templ func() templ.Attributes }`
				// used inline in a var or composite literal, with no `type X
				// struct` declaration at all) has no TypeSpec to match the case
				// above, but is callable with the exact same x.Templ() syntax
				// safeAttrs trusts.
				if namedStructs[node] {
					return true
				}
				if hasTemplField(node) {
					found = append(found, fmt.Sprintf("%s: field on an anonymous struct", path))
				}
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	if len(found) != 1 {
		return fmt.Errorf("expected exactly one Templ member (method or field) in the module, found %d: %v", len(found), found)
	}
	if !strings.Contains(filepath.ToSlash(strings.SplitN(found[0], ":", 2)[0]), "/components/actionattrs/") || !strings.HasSuffix(found[0], "on Safe") {
		return fmt.Errorf("the one Templ member in the module is not actionattrs.Safe.Templ: %s", found[0])
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
	classSeen := false
	for _, attr := range element.Attributes {
		switch attr := attr.(type) {
		case *templparser.SpreadAttributes:
			expr, err := parser.ParseExpr(attr.Expression.Value)
			if err != nil || !safeAttrs(expr, imports) {
				return false
			}
		case *templparser.ConditionalAttribute:
			if !safeConditionalAttributes(attr.Then) || !safeConditionalAttributes(attr.Else) {
				return false
			}
		case *templparser.ConstantAttribute:
			key, ok := attr.Key.(templparser.ConstantAttributeKey)
			if !ok || strings.EqualFold(key.Name, "style") {
				return false
			}
			if strings.EqualFold(key.Name, "class") {
				if classSeen {
					return false
				}
				classSeen = true
			}
		case *templparser.ExpressionAttribute:
			key, ok := attr.Key.(templparser.ConstantAttributeKey)
			if !ok || strings.EqualFold(key.Name, "style") {
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
					_, _, ok := classValue(item, imports)
					if !ok {
						return false
					}
				}
			}
		case *templparser.BoolConstantAttribute:
			if !safeActionAuxKey(attr.Key) {
				return false
			}
		case *templparser.BoolExpressionAttribute:
			if !safeActionAuxKey(attr.Key) {
				return false
			}
		}
	}
	return true
}

func safeActionAuxKey(key templparser.AttributeKey) bool {
	constant, ok := key.(templparser.ConstantAttributeKey)
	return ok && !strings.EqualFold(constant.Name, "class") && !strings.EqualFold(constant.Name, "style")
}

func safeConditionalAttributes(attrs []templparser.Attribute) bool {
	for _, attr := range attrs {
		switch attr := attr.(type) {
		case *templparser.ConstantAttribute:
			if !safeActionAuxKey(attr.Key) {
				return false
			}
		case *templparser.ExpressionAttribute:
			if !safeActionAuxKey(attr.Key) {
				return false
			}
		case *templparser.BoolConstantAttribute:
			if !safeActionAuxKey(attr.Key) {
				return false
			}
		case *templparser.BoolExpressionAttribute:
			if !safeActionAuxKey(attr.Key) {
				return false
			}
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
	if isActionattrsCall(expr, "LinkClass", imports) || isActionattrsCall(expr, "ButtonClass", imports) {
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
func safeClassExpr(expr ast.Expr, imports map[string]string) bool {
	if _, ok := stringValue(expr); ok {
		return true
	}
	return isActionattrsCall(expr, "LinkClass", imports) || isActionattrsCall(expr, "ButtonClass", imports)
}

// safeComponent reports whether expr -- the argument of an `@` component
// expression -- is safe. A bare component value reference (`@props.Icon`,
// `@tab.Content`, a loop variable, an indexed slice element, ...) has no
// call syntax anywhere in it to carry a caller-controlled Class or Attrs
// override; scanStatements/scanDecls separately check every place in this
// file's Go code where such a value could have been constructed, so this
// trust is not extended to code this checker never looked at.
//
// Otherwise, expr is inspected for every nested call to a dangerousKinds
// constructor, wherever it occurs -- as the direct `@` call itself, or
// buried inside an argument to some other, unrecognized wrapper call (for
// example `@organisms.Something(atoms.Anchor(atoms.AnchorProps{...}))`) --
// and each one is required to pass safeComponentCall. Only an unrecognized
// wrapper call with no such nested construction anywhere inside it is
// trusted, the same as an unrecognized bare call always was.
// safeComponent reports whether expr -- the argument of an `@` component
// expression -- is safe.
//
// A bare component value reference (`@props.Icon`, `@tab.Content`, a loop
// variable, an indexed slice element, ...) has no call syntax anywhere in
// it, which means this checker cannot see how the referenced
// templ.Component was actually built: that construction could be in this
// same file (which scanStatements/scanDecls do check), in another file in
// this module (CheckGoFile checks every .go file too), or in a package
// this module has no source for at all -- a hand-written
// templ.ComponentFunc, a templ.Raw call embedding an arbitrary HTML
// string, anything. No amount of AST analysis closes that last case, so a
// bare reference is never trusted by itself; see actionattrs.Scanned's
// doc comment for the one way to make it so: wrap the reference in a call
// to actionattrs.Scanned, which inspects the component's actual rendered
// HTML at render time instead of trying to prove its source is safe.
func safeComponent(expr ast.Expr, imports map[string]string, pkgName string) bool {
	if !containsCall(expr) {
		return false
	}
	unknownExternal := false
	ast.Inspect(expr, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		path, imported := imports[qualifier.Name]
		if !imported || strings.HasPrefix(path, "example.com/dynamis-code/apps-template/internal/web/") {
			return true
		}
		if path == "github.com/a-h/templ" && (selector.Sel.Name == "SafeURL" || selector.Sel.Name == "KV") ||
			path == "fmt" && selector.Sel.Name == "Sprintf" ||
			path == "strings" && selector.Sel.Name == "TrimSpace" ||
			path == "strconv" && (selector.Sel.Name == "Itoa" || selector.Sel.Name == "FormatInt") ||
			path == "example.com/dynamis-code/apps-template/internal/transactions" && selector.Sel.Name == "FormatAmount" {
			return true
		}
		unknownExternal = true
		return true
	})
	if unknownExternal {
		return false
	}
	if call, ok := expr.(*ast.CallExpr); ok {
		if isActionattrsCall(call, "Scanned", imports) {
			// Scanned's own runtime guard is the real verification here; still
			// recurse into its argument for a buried dangerous/unprovable
			// construction that would be a violation regardless.
			return !findUnsafeCalls(expr, imports, pkgName)
		}
		// The expression actually being rendered via `@` is held to the
		// strict, name-only rule regardless of package: if it is shaped like
		// one of dangerousKinds at all, it must pass safeComponentCall, so an
		// aliased/untrusted/wrong-package call at exactly this position is a
		// violation, not silently out of scope (unlike a buried nested call
		// found below, which uses the package-verified isDangerousCall
		// specifically to avoid flagging unrelated code -- see its doc
		// comment).
		if dangerousKinds[callKindName(call)] && !safeComponentCall(call, imports) {
			return false
		}
	}
	return !findUnsafeCalls(expr, imports, pkgName)
}

// hasRogueTemplField parses wrapped (already-valid Go source) and reports
// whether it declares any struct type -- named or anonymous, at any
// nesting depth, including a function or templ signature's own parameter
// list -- with a field literally named Templ. See VerifyTemplUniqueness's
// doc comment for why this matters; VerifyTemplUniqueness itself only
// scans plain .go files, so CheckPath applies the identical check to a
// .templ file's own Go-shaped text (a `{{ }}` block, a top-level
// declaration, or a templ function's parameter list) as well.
func hasRogueTemplField(wrapped string) bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", wrapped, 0)
	if err != nil {
		return false
	}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if struct_, ok := n.(*ast.StructType); ok && structHasTemplField(struct_) {
			found = true
		}
		return true
	})
	return found
}

func structHasTemplField(struct_ *ast.StructType) bool {
	for _, field := range struct_.Fields.List {
		for _, name := range field.Names {
			if name.Name == "Templ" {
				return true
			}
		}
	}
	return false
}

func containsCall(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if _, ok := n.(*ast.CallExpr); ok {
			found = true
		}
		return true
	})
	return found
}

func callKindName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fn.Sel.Name
	case *ast.Ident:
		return fn.Name
	}
	return ""
}

// safeComponentCall validates one call already known to be named like a
// dangerousKinds constructor (see safeComponent). An unqualified call --
// bare "Button(...)" rather than "atoms.Button(...)" -- is always rejected:
// there is no package qualifier here to check against imports at all.
func safeComponentCall(call *ast.CallExpr, imports map[string]string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	kind := selector.Sel.Name
	if kind == "SubmitButton" {
		pkg, ok := selector.X.(*ast.Ident)
		return ok && trustedPackage(imports, pkg.Name, moleculesImportPath) && len(call.Args) == 2 && safeAttrs(call.Args[1], imports)
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
		return safeClassExpr(call.Args[classIndex], imports) && safeAttrs(call.Args[attrsIndex], imports)
	}
	if kind == "Dropdown" {
		if len(call.Args) != 1 {
			return false
		}
		props, ok := call.Args[0].(*ast.CompositeLit)
		if !ok {
			return false
		}
		propsType, ok := props.Type.(*ast.SelectorExpr)
		if !ok || propsType.Sel.Name != "DropdownProps" {
			return false
		}
		componentPackage, componentOK := selector.X.(*ast.Ident)
		propsPackage, propsOK := propsType.X.(*ast.Ident)
		if !componentOK || !propsOK || !trustedPackage(imports, componentPackage.Name, atomsImportPath) || !trustedPackage(imports, propsPackage.Name, atomsImportPath) {
			return false
		}
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
			case "Class", "ListClass", "PanelClass", "SummaryClass":
				if !safeClassExpr(kv.Value, imports) {
					return false
				}
			case "SummaryAttrs", "PanelAttrs":
				if !safeAttrs(kv.Value, imports) {
					return false
				}
			case "Label", "Items", "Summary", "Panel", "Open":
				// Dropdown's template escapes Label and runtime-validates
				// Items, Summary, and Panel in its reviewed Scanned/From slots.
			case "PanelChildren":
				value, ok := kv.Value.(*ast.Ident)
				if !ok || value.Name != "false" {
					return false
				}
			default:
				return false
			}
		}
		return true
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
			if !safeClassExpr(kv.Value, imports) {
				return false
			}
		case "Attrs":
			if !safeAttrs(kv.Value, imports) {
				return false
			}
		}
	}
	return true
}

// safeAttrs validates an Attrs expression. A literal `templ.Attributes{...}`
// composite must pass exactly the rule actionattrs.From/With would apply at
// runtime -- every key on the allow-list, and href/onclick's value shape
// checked too -- rather than the historical, narrower rule that only
// restricted a "class" key: that narrower rule let a literal spread set
// "style", or an unchecked "href"/"onclick" value, straight past this
// checker, never touching actionattrs at all.
func safeAttrs(expr ast.Expr, imports map[string]string) bool {
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
		if !ok || !actionattrs.Allowed(key) {
			return false
		}
		if key == "href" || key == "onclick" {
			value, ok := stringValue(kv.Value)
			if !ok || actionattrs.ValidateValue(key, value) != nil {
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
