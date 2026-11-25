// Package actioncheck analyzes action controls in templ source files.
package actioncheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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
	var violations []Violation
	matches := make([]int, len(exceptions))
	v := visitor.New()
	visitElement := v.Element
	v.Element = func(element *templparser.Element) error {
		if element.Name == "a" || element.Name == "button" {
			if !safeElement(element) {
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
		} else if !safeComponent(expr) {
			violations = append(violations, Violation{call.Range.From.Line + 1, "unbounded or unresolved action component"})
		}
		return visitCall(call)
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

func safeElement(element *templparser.Element) bool {
	boundary := false
	classSeen := false
	for _, attr := range element.Attributes {
		switch attr := attr.(type) {
		case *templparser.SpreadAttributes:
			expr, err := parser.ParseExpr(attr.Expression.Value)
			if err != nil || !safeAttrs(expr, false) {
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
					value, unconditional, ok := classValue(item)
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

func classValue(expr ast.Expr) (string, bool, bool) {
	if value, ok := stringValue(expr); ok {
		return value, true, true
	}
	// actionattrs.LinkClass enforces the boundary token itself at runtime;
	// actionattrs.ButtonClass does not (its caller's own element already
	// carries an unconditional "btn" literal), so it contributes an empty,
	// trivially-safe, non-boundary token.
	if isActionattrsCall(expr, "LinkClass") {
		return "btn", true, true
	}
	if isActionattrsCall(expr, "ButtonClass") {
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
// named function in the actionattrs package.
func isActionattrsCall(expr ast.Expr, name string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "actionattrs"
}

// safeClassExpr reports whether expr is a known-safe class source: a string
// literal that passes the static checks, or a runtime-guarded
// actionattrs.ButtonClass/LinkClass call (see that package's doc comments).
// requireBoundary applies the hasBoundary literal check only to the literal
// path; the actionattrs calls enforce their own boundary rule at runtime.
func safeClassExpr(expr ast.Expr, requireBoundary bool) bool {
	if value, ok := stringValue(expr); ok {
		return safeTokens(value) && (!requireBoundary || hasBoundary(value))
	}
	return isActionattrsCall(expr, "ButtonClass") || isActionattrsCall(expr, "LinkClass")
}

func safeComponent(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		// A bare component value reference (`@props.Icon`, `@tab.Content`, a
		// loop variable, an indexed slice element, ...) has no call syntax to
		// carry a caller-controlled Class or Attrs override: there is no
		// composite literal here for an override to live in. Whatever
		// templ.Component value it names was already type-checked where it
		// was constructed, the same trust already extended below to a call
		// of an unrecognized function (go/types, not this checker, is what
		// proves that value really is a templ.Component). Reject anything
		// that is not a plain identifier/selector/index chain.
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
		return ok && pkg.Name == "molecules" && len(call.Args) == 2 && safeAttrs(call.Args[1], false)
	}
	if kind == "ConfirmButton" || kind == "SubmitButtonStyled" {
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || pkg.Name != "molecules" {
			return false
		}
		classIndex, attrsIndex := 2, 3
		if kind == "SubmitButtonStyled" {
			classIndex, attrsIndex = 1, 2
		}
		if len(call.Args) != attrsIndex+1 {
			return false
		}
		return safeClassExpr(call.Args[classIndex], false) && safeAttrs(call.Args[attrsIndex], false)
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
	if !componentOK || !propsOK || componentPackage.Name != "atoms" || propsPackage.Name != "atoms" {
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
			if !safeClassExpr(kv.Value, kind != "Button") {
				return false
			}
			hasClass = true
		case "Attrs":
			if !safeAttrs(kv.Value, false) {
				return false
			}
		}
	}
	return hasClass || kind == "Button"
}

func safeAttrs(expr ast.Expr, allowClass bool) bool {
	if id, ok := expr.(*ast.Ident); ok && id.Name == "nil" {
		return true
	}
	// A bare `<expr>.Templ()` call is trusted: in this module only
	// actionattrs.Safe defines a Templ() method (guarded by
	// internal/web/components/actionattrs.TestTemplMethodIsUnique), and its
	// only constructors enforce an attribute key allow-list. See the
	// actionattrs package doc comment.
	if call, ok := expr.(*ast.CallExpr); ok && len(call.Args) == 0 {
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Templ" {
			return true
		}
	}
	// actionattrs.From(expr) validates every key of its argument against the
	// same allow-list at runtime and panics before render if one is missing;
	// see the actionattrs package doc comment.
	if isActionattrsCall(expr, "From") {
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
