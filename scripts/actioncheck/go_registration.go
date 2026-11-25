package actioncheck

import (
	"go/ast"
	"go/types"
)

// localMuxEscapes covers direct uses of a local NewServeMux value. The router's
// returned handler still requires proof at every external caller. This helper
// cannot certify the route callbacks.
func (index ingressAliases) localMuxEscapes(body ast.Node, mux types.Object, constructor *types.Func, methods map[*types.Func]bool) []ast.Node {
	init, ok := index.targets[mux].(*ast.CallExpr)
	if mux == nil || !ok || index.blocked[mux] || index.resolve(init.Fun) != constructor {
		return []ast.Node{body}
	}
	allowed := map[*ast.Ident]bool{}
	outer := map[ast.Node]bool{}
	allowedMethodUse := map[ast.Expr]bool{}
	methodValue := func(expr ast.Expr) bool {
		return methods[index.resolve(expr)]
	}
	// An immutable alias may reference a registration only when its type
	// remains callable. Interface conversions and stores are not aliases.
	mark := func(expr ast.Expr) {
		if id, ok := ast.Unparen(expr).(*ast.Ident); ok && index.info.Uses[id] == mux {
			allowed[id] = true
		}
	}
	ast.Inspect(body, func(node ast.Node) bool {
		outer[node] = true
		switch n := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			if methodValue(n.Fun) {
				allowedMethodUse[ast.Unparen(n.Fun)] = true
			}
		case *ast.SelectorExpr:
			if methodValue(n) {
				mark(n.X)
			}
		case *ast.ReturnStmt:
			for _, result := range n.Results {
				mark(result)
			}
		}
		return true
	})
	for object, target := range index.targets {
		if _, ok := object.Type().Underlying().(*types.Signature); ok && outer[target] && !index.blocked[object] && methodValue(target) {
			allowedMethodUse[ast.Unparen(target)] = true
		}
	}
	var rejected []ast.Node
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Ident:
			obj := index.info.Uses[n]
			var callable bool
			if obj != nil {
				_, callable = obj.Type().Underlying().(*types.Signature)
			}
			if obj == mux && !allowed[n] || callable && index.targets[obj] != nil && methodValue(n) && !allowedMethodUse[n] {
				rejected = append(rejected, n)
			}
		case *ast.SelectorExpr:
			if methodValue(n) && !allowedMethodUse[n] {
				rejected = append(rejected, n)
			}
		}
		return true
	})
	return rejected
}
