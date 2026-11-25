package actioncheck

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

type renderOriginExpr struct {
	expr ast.Expr
	info *types.Info
}

type renderOriginParameter struct {
	function *types.Func
	index    int
}

type renderOriginIndex struct {
	known         map[string]map[string]bool
	closed        func(*types.Func) bool
	component     *types.Interface
	writes        map[*types.Var][]renderOriginExpr
	parameters    map[*types.Var]renderOriginParameter
	ranges        map[*types.Var]renderOriginExpr
	calls         map[*types.Func][]renderOriginExpr
	escaped       map[types.Object]bool
	interfaceName map[string]bool
}

// Index direct calls and all variable writes. The origin proof trusts only
// declared templates and private direct callback flows, not arbitrary Go code
// or exported callbacks whose callers are outside the checked package set.
func indexRenderOrigins(loaded []*packages.Package, known map[string]map[string]bool) renderOriginIndex {
	index := renderOriginIndex{
		known: known, writes: map[*types.Var][]renderOriginExpr{}, parameters: map[*types.Var]renderOriginParameter{},
		calls: map[*types.Func][]renderOriginExpr{}, ranges: map[*types.Var]renderOriginExpr{}, escaped: map[types.Object]bool{}, interfaceName: map[string]bool{},
	}
	for _, pkg := range loaded {
		info := pkg.TypesInfo
		for _, file := range pkg.Syntax {
			directUses := make(map[*ast.Ident]bool)
			ast.Inspect(file, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.FuncDecl:
					function, _ := info.Defs[n.Name].(*types.Func)
					position := 0
					for _, field := range n.Type.Params.List {
						if len(field.Names) == 0 {
							position++
						}
						for _, name := range field.Names {
							if variable, ok := info.Defs[name].(*types.Var); ok {
								index.parameters[variable] = renderOriginParameter{function, position}
							}
							position++
						}
					}
				case *ast.InterfaceType:
					for _, field := range n.Methods.List {
						for _, name := range field.Names {
							index.interfaceName[name.Name] = true
						}
					}
				case *ast.CallExpr:
					if name := callIdentifier(n.Fun); name != nil {
						if function, ok := info.Uses[name].(*types.Func); ok {
							directUses[name] = true
							index.calls[function] = append(index.calls[function], renderOriginExpr{n, info})
						}
					}
				case *ast.AssignStmt:
					for i, lhs := range n.Lhs {
						if name, ok := lhs.(*ast.Ident); ok {
							if variable, ok := info.ObjectOf(name).(*types.Var); ok {
								index.addWrite(variable, i, len(n.Lhs), n.Rhs, info)
							}
						} else if variable := rootedVariable(lhs, info); variable != nil {
							index.escaped[variable] = true
						}
					}
				case *ast.ValueSpec:
					for i, name := range n.Names {
						if variable, ok := info.Defs[name].(*types.Var); ok {
							index.addWrite(variable, i, len(n.Names), n.Values, info)
						}
					}
				case *ast.UnaryExpr:
					if n.Op == token.AND {
						if variable := rootedVariable(n.X, info); variable != nil {
							index.escaped[variable] = true
						}
					}
				case *ast.RangeStmt:
					if value, ok := n.Value.(*ast.Ident); ok && n.Tok == token.DEFINE {
						if variable, ok := info.Defs[value].(*types.Var); ok {
							index.ranges[variable] = renderOriginExpr{n.X, info}
						}
					}
					for _, expr := range []ast.Expr{n.Key, n.Value} {
						if name, ok := expr.(*ast.Ident); ok {
							if variable, ok := info.ObjectOf(name).(*types.Var); ok {
								if _, known := index.ranges[variable]; !known || n.Tok != token.DEFINE {
									index.escaped[variable] = true
								}
							}
						}
					}
				}
				return true
			})
			ast.Inspect(file, func(node ast.Node) bool {
				if name, ok := node.(*ast.Ident); ok && !directUses[name] {
					if fn, ok := info.Uses[name].(*types.Func); ok {
						index.escaped[fn] = true
					}
				}
				return true
			})
		}
	}
	return index
}

func rootedVariable(expr ast.Expr, info *types.Info) *types.Var {
	switch node := ast.Unparen(expr).(type) {
	case *ast.Ident:
		object, _ := info.ObjectOf(node).(*types.Var)
		return object
	case *ast.SelectorExpr:
		return rootedVariable(node.X, info)
	case *ast.IndexExpr:
		return rootedVariable(node.X, info)
	case *ast.StarExpr:
		return rootedVariable(node.X, info)
	}
	return nil
}

func callIdentifier(expr ast.Expr) *ast.Ident {
	switch expression := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return expression
	case *ast.SelectorExpr:
		return expression.Sel
	}
	return nil
}

func (index renderOriginIndex) addWrite(variable *types.Var, position, count int, expressions []ast.Expr, info *types.Info) {
	if count != len(expressions) {
		if len(expressions) != 0 {
			index.escaped[variable] = true
		}
		return
	}
	index.writes[variable] = append(index.writes[variable], renderOriginExpr{expressions[position], info})
}

func (index renderOriginIndex) safeGoRenderReceiver(expr ast.Expr, info *types.Info) bool {
	call, ok := ast.Unparen(expr).(*ast.CallExpr)
	return ok && (directRenderProducer(call, info, index.known) || index.safeFactory(call.Fun, info, make(map[*types.Var]bool)))
}

func (index renderOriginIndex) forwardedTemplateInput(expr ast.Expr, info *types.Info, owner *types.Func) bool {
	switch node := ast.Unparen(expr).(type) {
	case *ast.Ident:
		variable, ok := info.Uses[node].(*types.Var)
		if !ok || index.escaped[variable] || len(index.writes[variable]) != 0 {
			return false
		}
		if parameter, exists := index.parameters[variable]; exists {
			return parameter.function == owner
		}
		if origin, exists := index.ranges[variable]; exists {
			return index.forwardedTemplateInput(origin.expr, origin.info, owner)
		}
	case *ast.SelectorExpr:
		selection := info.Selections[node]
		return selection != nil && selection.Kind() == types.FieldVal && index.forwardedTemplateInput(node.X, info, owner)
	}
	return false
}

type goRenderUse struct {
	selector *ast.SelectorExpr
	direct   bool
}

func goRenderSelectors(file *ast.File, info *types.Info, component *types.Interface) []goRenderUse {
	var selectors []goRenderUse
	direct := make(map[*ast.SelectorExpr]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			if selector, ok := ast.Unparen(n.Fun).(*ast.SelectorExpr); ok {
				direct[selector] = true
			}
		case *ast.SelectorExpr:
			if n.Sel.Name == "Render" {
				typ := info.TypeOf(n.X)
				if typ != nil && types.Implements(typ, component) {
					selectors = append(selectors, goRenderUse{n, direct[n]})
				}
			}
		}
		return true
	})
	return selectors
}

func (index renderOriginIndex) safeFactory(expr ast.Expr, info *types.Info, seen map[*types.Var]bool) bool {
	if closure, ok := ast.Unparen(expr).(*ast.FuncLit); ok {
		if len(closure.Body.List) != 1 {
			return false
		}
		result, ok := closure.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(result.Results) != 1 || !directRenderProducer(result.Results[0], info, index.known) {
			return false
		}
		call := ast.Unparen(result.Results[0]).(*ast.CallExpr)
		if index.closed == nil || index.closed(calledFunction(call, info)) {
			return true
		}
		for _, argument := range call.Args {
			if !typedComponentArgumentSafe(argument, info, index.component, index.known, true, index.closed, nil) {
				return false
			}
		}
		return true
	}
	name := callIdentifier(expr)
	if name == nil {
		return false
	}
	switch object := info.Uses[name].(type) {
	case *types.Func:
		return object.Pkg() != nil && object.Type().(*types.Signature).Recv() == nil && index.known[object.Pkg().Path()][object.Name()] &&
			(index.closed == nil || index.closed(object))
	case *types.Var:
		if _, identifier := ast.Unparen(expr).(*ast.Ident); !identifier || index.escaped[object] || seen[object] || object.Pkg() == nil || object.Parent() == object.Pkg().Scope() {
			return false
		}
		seen[object] = true
		defer delete(seen, object)
		values := index.writes[object]
		if parameter, ok := index.parameters[object]; ok {
			fn := parameter.function
			if fn == nil || fn.Exported() || index.escaped[fn] || index.interfaceName[fn.Name()] || len(index.calls[fn]) == 0 {
				return false
			}
			values = append([]renderOriginExpr(nil), values...)
			for _, origin := range index.calls[fn] {
				call := origin.expr.(*ast.CallExpr)
				position := parameter.index
				if selection, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
					if selected := origin.info.Selections[selection]; selected != nil && selected.Kind() == types.MethodExpr {
						position++
					}
				}
				if position >= len(call.Args) || call.Ellipsis.IsValid() {
					return false
				}
				values = append(values, renderOriginExpr{call.Args[position], origin.info})
			}
		}
		for _, value := range values {
			if !index.safeFactory(value.expr, value.info, seen) {
				return false
			}
		}
		return len(values) != 0
	}
	return false
}
