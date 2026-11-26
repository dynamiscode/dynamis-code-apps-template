package actioncheck

import (
	"go/ast"
	"go/types"
)

// uncheckedWriterMethods is the method counterpart to writer transfer checks.
// A direct call needs a reviewed effect. Bound method values and method
// expressions are rejected by default because they erase receiver provenance.
func uncheckedWriterMethods(body ast.Node, info *types.Info, writer *types.Interface, checked func(*types.Func) bool) []*ast.SelectorExpr {
	direct := map[*ast.SelectorExpr]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
				direct[selector] = true
			}
		}
		return true
	})
	var rejected []*ast.SelectorExpr
	ast.Inspect(body, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		member := info.Selections[selector]
		if member == nil || !types.Implements(member.Recv(), writer) {
			return true
		}
		method, ok := member.Obj().(*types.Func)
		if !ok {
			return true
		}
		if !direct[selector] || member.Kind() != types.MethodVal || !checked(method) {
			rejected = append(rejected, selector)
		}
		return true
	})
	return rejected
}
