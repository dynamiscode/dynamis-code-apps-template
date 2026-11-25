package actioncheck

import (
	"go/ast"
	"go/types"
	"strconv"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestIngressExistingAgentMounts(t *testing.T) {
	found, err := packages.Load(&packages.Config{
		Dir: "../..", Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedModule,
	}, "./internal/bootstrap")
	if err != nil || len(found) != 1 {
		t.Fatalf("load bootstrap: %v", err)
	}
	pkg := found[0]
	if len(pkg.Errors) != 0 || pkg.Module == nil {
		t.Fatalf("type check bootstrap: %v", pkg.Errors)
	}
	for _, file := range pkg.Syntax {
		for _, node := range file.Decls {
			fn, ok := node.(*ast.FuncDecl)
			if ok && fn.Name.Name == "registerAgent" && fn.Recv == nil {
				checkAgentMounts(t, pkg, fn)
				return
			}
		}
	}
	t.Fatal("registerAgent missing")
}

func checkAgentMounts(t *testing.T, pkg *packages.Package, fn *ast.FuncDecl) {
	t.Helper()
	root := pkg.Module.Path + "/internal/"
	agentEnabled := false
	for _, dependency := range pkg.Types.Imports() {
		agentEnabled = agentEnabled || dependency.Path() == root+"mcpserver"
	}
	if !agentEnabled {
		if len(fn.Body.List) != 0 {
			t.Fatal("pruned profile must have an empty registerAgent")
		}
		return
	}
	want := map[string]string{
		"/mcp": "NewHandler",
		"/.well-known/oauth-protected-resource/mcp": "NewProtectedResourceMetadataHandler",
		"/oauth/token":    "NewOAuthTokenEndpoint",
		"/oauth/revoke":   "NewOAuthRevocationEndpoint",
		"/oauth/register": "NewOAuthRegistrationEndpoint",
		"/.well-known/oauth-authorization-server": "NewAuthorizationServerMetadataHandler",
	}
	info := pkg.TypesInfo
	mux := info.Defs[fn.Type.Params.List[0].Names[0]]
	index := indexIngressAliases(fn.Body, info)
	muxUses := map[*ast.Ident]bool{}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !bootstrapCall(index, call, "net/http", "Handle") || len(call.Args) != 2 {
			return true
		}
		if id, ok := method.X.(*ast.Ident); ok && info.Uses[id] == mux {
			muxUses[id] = true
		} else {
			t.Error("agent route receiver is not its incoming mux")
		}
		constant, ok := info.TypeOf(call.Args[0]).Underlying().(*types.Basic)
		route := ""
		if value := info.Types[call.Args[0]].Value; ok && constant.Info()&types.IsString != 0 && value != nil {
			route, _ = strconv.Unquote(value.ExactString())
		}
		name, declared := want[route]
		if !declared {
			t.Errorf("unapproved or duplicate agent route %q", route)
			return true
		}
		delete(want, route)
		wrapper, ok := call.Args[1].(*ast.CallExpr)
		if !ok || !bootstrapCall(index, wrapper, root+"httpapi", "Wrap") {
			t.Errorf("agent route %q missing checked wrapper", route)
			return true
		}
		origin := wrapper.Args[0]
		if id, ok := origin.(*ast.Ident); ok && !index.blocked[info.Uses[id]] {
			origin = index.targets[info.Uses[id]]
		}
		producer, ok := origin.(*ast.CallExpr)
		if !ok || !bootstrapCall(index, producer, root+"mcpserver", name) {
			t.Errorf("agent route %q missing checked producer %s", route, name)
		}
		return true
	})
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if id, ok := node.(*ast.Ident); ok && info.Uses[id] == mux && !muxUses[id] {
			t.Errorf("unapproved use of agent mux at %s", pkg.Fset.Position(id.Pos()))
		}
		return true
	})
	if len(want) != 0 {
		t.Errorf("missing agent routes: %v", want)
	}
}
