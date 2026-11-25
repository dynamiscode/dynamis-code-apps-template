package actioncheck

import (
	"go/ast"
	"go/token"
	"go/types"
)

// ingressAliases resolves static destinations and local single-assignment
// function aliases. It does not trust resolved destinations or track what
// flows through their parameters. Callers must prove those contracts.
type ingressAliases struct {
	info    *types.Info
	targets map[types.Object]ast.Expr
	blocked map[types.Object]bool
}

func indexIngressAliases(body ast.Node, info *types.Info) ingressAliases {
	index := ingressAliases{info: info, targets: map[types.Object]ast.Expr{}, blocked: map[types.Object]bool{}}
	block := func(expr ast.Expr) {
		if id, ok := ast.Unparen(expr).(*ast.Ident); ok {
			index.blocked[info.ObjectOf(id)] = true
		}
	}
	put := func(obj types.Object, value ast.Expr) {
		if obj == nil {
			return
		}
		if index.targets[obj] != nil {
			index.blocked[obj] = true
		}
		index.targets[obj] = value
	}
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				id, ok := ast.Unparen(lhs).(*ast.Ident)
				if !ok {
					continue
				}
				if n.Tok == token.DEFINE && info.Defs[id] != nil && len(n.Lhs) == len(n.Rhs) {
					put(info.Defs[id], n.Rhs[i])
				} else {
					block(id)
				}
			}
		case *ast.ValueSpec:
			if len(n.Names) == len(n.Values) {
				for i, name := range n.Names {
					put(info.Defs[name], n.Values[i])
				}
			}
		case *ast.UnaryExpr:
			if n.Op == token.AND {
				block(n.X)
			}
		case *ast.RangeStmt:
			if n.Tok != token.DEFINE {
				if n.Key != nil {
					block(n.Key)
				}
				if n.Value != nil {
					block(n.Value)
				}
			}
		}
		return true
	})
	return index
}

func (index ingressAliases) resolve(expr ast.Expr) *types.Func {
	seen := map[types.Object]bool{}
	for expr != nil {
		switch expr := ast.Unparen(expr).(type) {
		case *ast.SelectorExpr:
			if selection := index.info.Selections[expr]; selection != nil {
				fn, _ := selection.Obj().(*types.Func)
				if fn == nil {
					return nil
				}
				receiver := fn.Signature().Recv().Type()
				if types.IsInterface(receiver) {
					return nil
				}
				return fn
			}
			fn, _ := index.info.ObjectOf(expr.Sel).(*types.Func)
			return fn
		case *ast.Ident:
			obj := index.info.ObjectOf(expr)
			if fn, ok := obj.(*types.Func); ok {
				return fn
			}
			if index.blocked[obj] || seen[obj] {
				return nil
			}
			seen[obj] = true
		default:
			return nil
		}
		id := ast.Unparen(expr).(*ast.Ident)
		expr = index.targets[index.info.ObjectOf(id)]
	}
	return nil
}

type ingressCallContract struct {
	// Function parameters carrying a destination must resolve to an approved
	// function independently of approval of the registration function.
	callbackArguments []int
	producesCallback  bool
	isCallback        bool
}

// Limit total callback expansions per call, including repeated subtrees.
// Exhaustion is an unsupported origin and never falls back to approval.
const ingressOriginBudget = 32

func (index ingressAliases) approvedOrigin(expr ast.Expr, contracts map[*types.Func]ingressCallContract, remaining *int) bool {
	if *remaining == 0 {
		return false
	}
	*remaining = *remaining - 1
	if direct := index.resolve(expr); direct != nil {
		return contracts[direct].isCallback
	}
	switch expr := ast.Unparen(expr).(type) {
	case *ast.Ident:
		obj := index.info.ObjectOf(expr)
		if alias := index.targets[obj]; alias != nil && !index.blocked[obj] {
			return index.approvedOrigin(alias, contracts, remaining)
		}
	case *ast.CallExpr:
		contract, ok := contracts[index.resolve(expr.Fun)]
		return ok && contract.producesCallback && index.approvedArguments(expr, contract, contracts, remaining)
	}
	return false
}

func (index ingressAliases) approvedArguments(call *ast.CallExpr, contract ingressCallContract, contracts map[*types.Func]ingressCallContract, remaining *int) bool {
	for _, position := range contract.callbackArguments {
		if position < 0 || position >= len(call.Args) || !index.approvedOrigin(call.Args[position], contracts, remaining) {
			return false
		}
	}
	return true
}

func uncheckedIngressCalls(body ast.Node, info *types.Info, writer *types.Interface, contracts map[*types.Func]ingressCallContract) []*ast.CallExpr {
	index := indexIngressAliases(body, info)
	carries := func(expr ast.Expr) bool {
		typ := info.TypeOf(expr)
		return typ != nil && carriesResponseWriter(typ, writer, map[types.Type]bool{})
	}
	var rejected []*ast.CallExpr
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		capability := carries(call.Fun)
		if selector, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
			capability = capability || carries(selector.X)
		}
		for _, arg := range call.Args {
			capability = capability || carries(arg)
		}
		target := index.resolve(call.Fun)
		contract, checked := contracts[target]
		remaining := ingressOriginBudget
		approved := checked && index.approvedArguments(call, contract, contracts, &remaining)
		if !approved && (capability || checked) {
			rejected = append(rejected, call)
		}
		return true
	})
	return rejected
}
