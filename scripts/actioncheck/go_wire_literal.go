package actioncheck

import (
	"go/ast"
	"go/constant"
	"go/types"
)

// matchesWireLiteral checks the actual constant value, not its source-level
// name or its spelling. No runtime string qualifies as fixed wire framing.
func matchesWireLiteral(expr ast.Expr, info *types.Info, expected string) bool {
	value := info.Types[expr].Value
	return value != nil && value.Kind() == constant.String && constant.StringVal(value) == expected
}

// matchesFixedWireJSON is intentionally limited to the named-free map shape
// used by the stream limit branch. It rejects custom marshalers and dynamic
// data rather than inferring a generic JSON serialization contract.
func matchesFixedWireJSON(expr ast.Expr, info *types.Info, expected map[string]string) bool {
	literal, ok := ast.Unparen(expr).(*ast.CompositeLit)
	if !ok || len(literal.Elts) != len(expected) {
		return false
	}
	if !types.Identical(info.TypeOf(literal), types.NewMap(types.Typ[types.String], types.Universe.Lookup("any").Type())) {
		return false
	}
	seen := map[string]bool{}
	for _, item := range literal.Elts {
		entry, ok := item.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key := info.Types[entry.Key].Value
		value := info.Types[entry.Value].Value
		if key == nil || key.Kind() != constant.String || value == nil {
			return false
		}
		name := constant.StringVal(key)
		want, approved := expected[name]
		basic, plain := info.TypeOf(entry.Value).(*types.Basic)
		if seen[name] || !approved || !plain ||
			(value.Kind() != constant.String && value.Kind() != constant.Int) ||
			(basic.Info()&(types.IsString|types.IsInteger) == 0) || value.ExactString() != want {
			return false
		}
		seen[name] = true
	}
	return true
}

// matchesWireMarshal requires one local bytes binding and one use. Thus an
// alias, mutation, address escape or later formatter cannot change the data
// between the reviewed encoder and the staged SSE format.
func matchesWireMarshal(expr ast.Expr, body *ast.BlockStmt, index ingressAliases, packagePath, typeName string) bool {
	id, ok := ast.Unparen(expr).(*ast.Ident)
	if !ok {
		return false
	}
	info := index.info
	object, ok := info.Uses[id].(*types.Var)
	if !ok {
		return false
	}
	productions, uses := 0, 0
	valid := false
	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.Ident:
			if info.Uses[node] == object {
				uses++
			}
		case *ast.AssignStmt:
			if len(node.Lhs) != 2 || len(node.Rhs) != 1 {
				return true
			}
			name, ok := node.Lhs[0].(*ast.Ident)
			if !ok || info.Defs[name] != object {
				return true
			}
			productions++
			producer, ok := ast.Unparen(node.Rhs[0]).(*ast.CallExpr)
			if !ok || len(producer.Args) != 1 {
				return true
			}
			fn := index.resolve(producer.Fun)
			valid = fn != nil && fn.Pkg() != nil && fn.Pkg().Path() == "encoding/json" &&
				fn.Name() == "Marshal" && namedTypeIs(info.TypeOf(producer.Args[0]), packagePath, typeName)
		}
		return true
	})
	return productions == 1 && uses == 1 && valid
}
