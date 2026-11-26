package actioncheck

import (
	"go/ast"
	"go/types"
)

type writerFinding struct {
	node   ast.Node
	reason string
}

// strictWriterFlow checks one declaration. Local definitions in checked must
// themselves pass this check. Trusted foreign/opaque targets must have an
// edge-specific contract. An approved destination alone cannot erase a writer.
func strictWriterFlow(owner *ast.FuncDecl, info *types.Info, writer *types.Interface, checked map[*types.Func]bool, contract func(ast.Node) bool) []writerFinding {
	if owner.Body == nil {
		return nil
	}
	index := indexIngressAliases(owner.Body, info)
	allowed := func(node ast.Node) bool { return contract != nil && contract(node) }
	carries := func(typ types.Type) bool {
		return typ != nil && carriesResponseWriter(typ, writer, map[types.Type]bool{})
	}
	var findings []writerFinding
	for _, node := range opaqueWriterStorage(owner.Body, info, writer) {
		if !allowed(node) {
			findings = append(findings, writerFinding{node, "unapproved writer storage or projection"})
		}
	}
	for _, node := range uncheckedWriterMethods(owner.Body, info, writer, func(method *types.Func) bool {
		recv := method.Signature().Recv()
		return recv != nil && !types.IsInterface(recv.Type()) && checked[method]
	}) {
		if !allowed(node) {
			findings = append(findings, writerFinding{node, "unapproved writer method"})
		}
	}
	ast.Inspect(owner.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || allowed(node) {
			return true
		}
		capability := carries(info.TypeOf(call))
		for _, argument := range call.Args {
			capability = capability || carries(info.TypeOf(argument))
		}
		if !capability {
			return true
		}
		fn := index.resolve(call.Fun)
		if fn == nil || !checked[fn] {
			findings = append(findings, writerFinding{call, "unapproved writer destination"})
			return true
		}
		signature, ok := info.TypeOf(call.Fun).Underlying().(*types.Signature)
		if !ok || signature.Variadic() || len(call.Args) != signature.Params().Len() {
			findings = append(findings, writerFinding{call, "unsupported writer call signature"})
			return true
		}
		for i, argument := range call.Args {
			if carries(info.TypeOf(argument)) && !carries(signature.Params().At(i).Type()) {
				findings = append(findings, writerFinding{argument, "writer capability erased at argument"})
			}
		}
		return true
	})
	return findings
}
