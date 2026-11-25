package actioncheck

import (
	"go/ast"
	"go/token"
	"go/types"
)

// A forwarding contract is scoped to one source declaration. Its params
// must be established by the calling edge. Any wrapper field in the contract
// belongs to an independently audited transport type, not a structural match.
type forwardingEdgeContract struct {
	callback      *types.Var
	writer        *types.Var
	method        *types.Func
	wrapperFields map[*types.Var]bool
}

func (index ingressAliases) transparentForward(call *ast.CallExpr, contract forwardingEdgeContract) bool {
	selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || len(call.Args) != 2 || contract.callback == nil || contract.writer == nil || contract.method == nil {
		return false
	}
	method := index.info.Selections[selector]
	if method == nil || method.Kind() != types.MethodVal || method.Obj() != contract.method {
		return false
	}
	if index.aliasRoot(selector.X) != contract.callback {
		return false
	}
	if index.aliasRoot(call.Args[0]) == contract.writer {
		return true
	}
	// Direct pointer construction with exactly one explicitly bound field.
	// Mutable, inferred, returned and composite aliases are unsupported.
	address, ok := ast.Unparen(call.Args[0]).(*ast.UnaryExpr)
	if !ok || address.Op != token.AND {
		return false
	}
	wrapper, ok := ast.Unparen(address.X).(*ast.CompositeLit)
	if !ok || len(wrapper.Elts) != 1 {
		return false
	}
	field, ok := wrapper.Elts[0].(*ast.KeyValueExpr)
	if !ok {
		return false
	}
	key, ok := field.Key.(*ast.Ident)
	if !ok {
		return false
	}
	variable, _ := index.info.ObjectOf(key).(*types.Var)
	return variable != nil && contract.wrapperFields[variable] && index.aliasRoot(field.Value) == contract.writer
}

func (index ingressAliases) aliasRoot(expr ast.Expr) types.Object {
	seen := map[types.Object]bool{}
	for expr != nil {
		id, ok := ast.Unparen(expr).(*ast.Ident)
		if !ok {
			return nil
		}
		obj := index.info.ObjectOf(id)
		if obj == nil || seen[obj] || index.blocked[obj] {
			return nil
		}
		seen[obj] = true
		if expr = index.targets[obj]; expr == nil {
			return obj
		}
	}
	return nil
}
