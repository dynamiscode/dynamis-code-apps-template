package actioncheck

import (
	"bytes"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"example.com/dynamis-code/apps-template/internal/web/components/actionattrs"
	templparser "github.com/a-h/templ/parser/v2"
)

const templGeneratedPrefix = "templ_7745c5c3_"

// directRenderProducer checks provenance at the render receiver, independently
// of the arguments. A rendered component injected through arguments still must
// be identified at its own downstream sink before this can replace the gate.
func directRenderProducer(receiver ast.Expr, info *types.Info, known map[string]map[string]bool) bool {
	producer, ok := ast.Unparen(receiver).(*ast.CallExpr)
	if !ok {
		return false
	}
	fn := calledFunction(producer, info)
	return fn != nil && fn.Pkg() != nil && fn.Type().(*types.Signature).Recv() == nil && known[fn.Pkg().Path()][fn.Name()]
}

func handwrittenIdentifier(mapping *templparser.SourceMap, input []byte, position token.Position, name string) bool {
	origin, ok := mapping.TargetLinesToSource[uint32(position.Line-1)][uint32(position.Column-1)]
	return ok && 0 <= origin.Index && origin.Index < int64(len(input)) && bytes.HasPrefix(input[origin.Index:], []byte(name))
}

// reservedRenderObject identifies templ facilities that can bypass checked
// component calls even when imported under an alias or taken as a value.
func reservedRenderObject(object types.Object) bool {
	if object == nil || object.Pkg() == nil {
		return false
	}
	switch object.Pkg().Path() {
	case "github.com/a-h/templ/runtime":
		return true
	case "github.com/a-h/templ":
		switch object.Name() {
		case "Raw", "ComponentFunc", "FromGoHTML", "ToGoHTML", "Handler", "WithChildren", "GetChildren", "RenderCSSItems", "RenderScriptItems":
			return true
		}
	}
	return false
}

func compiledChildSlots(body *ast.BlockStmt, info *types.Info) map[types.Object]bool {
	slots := make(map[types.Object]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		name, ok := assignment.Lhs[0].(*ast.Ident)
		if !ok || !strings.HasPrefix(name.Name, templGeneratedPrefix) {
			return true
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if ok && packageFunction(call, info, "github.com/a-h/templ", "GetChildren") {
			slots[info.Defs[name]] = true
		}
		return true
	})
	return slots
}

func compiledChildInputs(root ast.Node, info *types.Info) (contexts, children map[types.Object]bool) {
	contexts = make(map[types.Object]bool)
	children = make(map[types.Object]bool)
	ast.Inspect(root, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != len(assignment.Rhs) {
			return true
		}
		for i, lhs := range assignment.Lhs {
			name, ok := lhs.(*ast.Ident)
			if !ok {
				continue
			}
			variable := info.Defs[name]
			if variable == nil {
				continue
			}
			if rhs, ok := assignment.Rhs[i].(*ast.SelectorExpr); ok && name.Name == "ctx" && rhs.Sel.Name == "Context" {
				if input, ok := rhs.X.(*ast.Ident); ok && input.Name == templGeneratedPrefix+"Input" {
					contexts[variable] = true
				}
			}
			if rhs, ok := assignment.Rhs[i].(*ast.CallExpr); ok && strings.HasPrefix(name.Name, templGeneratedPrefix) && len(rhs.Args) == 1 && packageFunction(rhs, info, "github.com/a-h/templ/runtime", "GeneratedTemplate") {
				if _, ok := rhs.Args[0].(*ast.FuncLit); ok {
					children[variable] = true
				}
			}
		}
		return true
	})
	return contexts, children
}

func packageFunction(call *ast.CallExpr, info *types.Info, path, name string) bool {
	function := calledFunction(call, info)
	return function != nil && function.Pkg() != nil && function.Pkg().Path() == path && function.Name() == name && function.Type().(*types.Signature).Recv() == nil
}

func namedTypeIs(typ types.Type, path, name string) bool {
	named, ok := types.Unalias(typ).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == path && named.Obj().Name() == name
}

func scalarAttributeType(typ types.Type) bool {
	if typ == nil {
		return false
	}
	if basic, ok := types.Unalias(typ).(*types.Basic); ok {
		return basic.Kind() == types.UntypedNil || basic.Info()&(types.IsBoolean|types.IsNumeric|types.IsString) != 0
	}
	if namedTypeIs(typ, "github.com/a-h/templ", "SafeURL") || namedTypeIs(typ, "github.com/a-h/templ", "SafeCSS") {
		return true
	}
	if named, ok := types.Unalias(typ).(*types.Named); ok && namedTypeIs(typ, "github.com/a-h/templ", "KeyValue") && named.TypeArgs().Len() == 2 {
		key, value := named.TypeArgs().At(0), named.TypeArgs().At(1)
		return (types.Identical(key, types.Typ[types.String]) || types.Identical(key, types.Typ[types.Bool])) && types.Identical(value, types.Typ[types.Bool])
	}
	return false
}

func checkedAttributeSpread(expr ast.Expr, info *types.Info) bool {
	switch expression := ast.Unparen(expr).(type) {
	case *ast.CallExpr:
		if packageFunction(expression, info, actionattrsImportPath, "From") || packageFunction(expression, info, actionattrsImportPath, "ScalarAttributes") {
			return true
		}
		function := calledFunction(expression, info)
		if function != nil && function.Name() == "Templ" {
			receiver := function.Type().(*types.Signature).Recv()
			return receiver != nil && namedTypeIs(receiver.Type(), actionattrsImportPath, "Safe")
		}
	case *ast.CompositeLit:
		if !namedTypeIs(info.TypeOf(expression), "github.com/a-h/templ", "Attributes") {
			return false
		}
		for _, entry := range expression.Elts {
			pair, ok := entry.(*ast.KeyValueExpr)
			if !ok || !scalarAttributeType(info.TypeOf(pair.Value)) {
				return false
			}
			key := info.Types[pair.Key].Value
			if key == nil || key.Kind() != constant.String || actionattrs.ExecutableAttribute(constant.StringVal(key)) {
				return false
			}
		}
		return true
	}
	return false
}

func compiledClassLists(body *ast.BlockStmt, info *types.Info) map[types.Object]bool {
	lists := make(map[types.Object]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		declaration, ok := node.(*ast.ValueSpec)
		if !ok || len(declaration.Names) != 1 || len(declaration.Values) != 1 || !strings.HasPrefix(declaration.Names[0].Name, templGeneratedPrefix) {
			return true
		}
		literal, ok := declaration.Values[0].(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, item := range literal.Elts {
			if !plainClassItem(info.TypeOf(item)) {
				return true
			}
		}
		lists[info.Defs[declaration.Names[0]]] = true
		return true
	})
	return lists
}

func plainClassItem(typ types.Type) bool {
	if basic, ok := types.Unalias(typ).(*types.Basic); ok {
		return basic.Info()&types.IsString != 0
	}
	if slice, ok := types.Unalias(typ).(*types.Slice); ok {
		return types.Identical(slice.Elem(), types.Typ[types.String])
	}
	if named, ok := types.Unalias(typ).(*types.Named); ok && namedTypeIs(typ, "github.com/a-h/templ", "KeyValue") && named.TypeArgs().Len() == 2 {
		return types.Identical(named.TypeArgs().At(0), types.Typ[types.String]) && types.Identical(named.TypeArgs().At(1), types.Typ[types.Bool])
	}
	return false
}

func writesDynamicHTML(call *ast.CallExpr, info *types.Info) bool {
	function := calledFunction(call, info)
	if function != nil && function.Pkg() != nil && function.Pkg().Path() == "github.com/a-h/templ/runtime" {
		switch function.Name() {
		case "NewSrcdocExpression", "ResolveDynamic":
			return true
		}
	}
	appendName, ok := call.Fun.(*ast.Ident)
	if !ok || info.Uses[appendName] != types.Universe.Lookup("append") || len(call.Args) < 2 {
		return false
	}
	list, ok := call.Args[0].(*ast.Ident)
	return ok && list.Name == templGeneratedPrefix+"ElementScripts"
}

// A closed consumer can accept opaque data without creating another component
// sink. Every rendered child must be a declared template closed by this same
// proof. The separate attribute and CSS checks still apply to every template.
func closedTemplateConsumers(functions map[*types.Func]typedFunction, known map[string]map[string]bool, component *types.Interface) func(*types.Func) bool {
	results := make(map[*types.Func]bool)
	var closed func(*types.Func) bool
	closed = func(function *types.Func) bool {
		if function == nil || function.Pkg() == nil || function.Type().(*types.Signature).Recv() != nil || !known[function.Pkg().Path()][function.Name()] {
			return false
		}
		if cached, checked := results[function]; checked {
			return cached
		}
		source, exists := functions[function]
		if !exists {
			return false
		}
		results[function] = false // A recursive render path is not proven closed.
		children := compiledChildSlots(source.declaration.Body, source.info)
		valid := true
		ast.Inspect(source.declaration.Body, func(node ast.Node) bool {
			if !valid {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if isScannedCall(call, source.info) {
				valid = false
				return false
			}
			method, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
			if !ok || method.Sel.Name != "Render" {
				return true
			}
			typ := source.info.TypeOf(method.X)
			if typ == nil || !types.Implements(typ, component) {
				return true
			}
			if child, ok := method.X.(*ast.Ident); ok && children[source.info.Uses[child]] {
				return true
			}
			producer, ok := ast.Unparen(method.X).(*ast.CallExpr)
			valid = ok && (packageFunction(producer, source.info, "github.com/a-h/templ", "JSONScript") || closed(calledFunction(producer, source.info)))
			return valid
		})
		results[function] = valid
		return valid
	}
	return closed
}
