package actioncheck

import (
	"go/ast"
	"go/types"
	"reflect"
	"testing"

	"golang.org/x/tools/go/packages"
)

// Diagnostic only: the live root mount inventory is checked separately from
// the downstream leaf handlers, their raw transports and their consumers.
func TestIngressExistingBootstrapMounts(t *testing.T) {
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
	remaining := map[string]func(*testing.T, *packages.Package, *ast.BlockStmt){
		"New": testBootstrapMounts,
		"Run": testBootstrapServe,
	}
	for _, file := range pkg.Syntax {
		for _, node := range file.Decls {
			if fn, ok := node.(*ast.FuncDecl); ok && fn.Recv == nil {
				if check := remaining[fn.Name.Name]; check != nil {
					check(t, pkg, fn.Body)
					delete(remaining, fn.Name.Name)
				}
			}
		}
	}
	for name := range remaining {
		t.Errorf("bootstrap.%s missing", name)
	}
}

func testBootstrapServe(t *testing.T, pkg *packages.Package, body *ast.BlockStmt) {
	t.Helper()
	index := indexIngressAliases(body, pkg.TypesInfo)
	count := 0
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !bootstrapCall(index, call, "net/http", "Serve") {
			return true
		}
		count++
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			t.Error("bootstrap Serve must be a direct server method")
			return true
		}
		id, ok := method.X.(*ast.Ident)
		if !ok || index.blocked[pkg.TypesInfo.Uses[id]] {
			t.Error("bootstrap Serve receiver is not a stable local")
			return true
		}
		producer, ok := index.targets[pkg.TypesInfo.Uses[id]].(*ast.CallExpr)
		if !ok || !bootstrapCall(index, producer, pkg.Module.Path+"/internal/httpapi", "NewServer") || len(producer.Args) != 2 {
			t.Error("bootstrap Serve receiver lacks typed NewServer producer")
			return true
		}
		field, ok := producer.Args[1].(*ast.SelectorExpr)
		if !ok {
			t.Error("NewServer handler must be a selected App.Handler")
			return true
		}
		selection := pkg.TypesInfo.Selections[field]
		if selection == nil || selection.Kind() != types.FieldVal || selection.Obj().Name() != "Handler" ||
			types.TypeString(selection.Recv(), func(other *types.Package) string { return other.Path() }) != "*"+pkg.PkgPath+".App" {
			t.Error("NewServer handler is not bootstrap.App.Handler")
		}
		return true
	})
	if count != 1 {
		t.Errorf("found %d Serve destinations in bootstrap.Run, want 1", count)
	}
}

func testBootstrapMounts(t *testing.T, pkg *packages.Package, body *ast.BlockStmt) {
	t.Helper()
	index := indexIngressAliases(body, pkg.TypesInfo)
	observed := []string{}
	var mountedMux, agentMux types.Object
	agents, limiterCalls := 0, 0
	allowedUses := map[*ast.Ident]bool{}
	record := func(expr ast.Expr) types.Object {
		if id, ok := expr.(*ast.Ident); ok {
			allowedUses[id] = true
			obj := pkg.TypesInfo.Uses[id]
			if !index.blocked[obj] {
				return obj
			}
		}
		return nil
	}
	var limitedMux types.Object
	appHandlers := 0
	ast.Inspect(body, func(node ast.Node) bool {
		if field, ok := node.(*ast.KeyValueExpr); ok {
			if key, ok := field.Key.(*ast.Ident); ok && key.Name == "Handler" {
				appHandlers++
				outer, ok := field.Value.(*ast.CallExpr)
				if !ok || !bootstrapCall(index, outer, pkg.Module.Path+"/internal/platform/telemetry", "HTTPHandler") || len(outer.Args) != 1 {
					t.Error("app handler must enter through the typed telemetry wrapper")
					return true
				}
				local, ok := outer.Args[0].(*ast.CallExpr)
				if !ok || !bootstrapCall(index, local, pkg.PkgPath, "localHTTPOnly") || len(local.Args) != 2 {
					t.Error("app handler must enter through the local HTTP guard")
					return true
				}
				inner, ok := local.Args[0].(*ast.CallExpr)
				if !ok || !bootstrapCall(index, inner, pkg.Module.Path+"/internal/httpapi", "LimitPublicIntake") {
					t.Error("app handler must enter through the typed intake limiter")
				}
			}
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		target := index.resolve(call.Fun)
		if bootstrapCall(index, call, pkg.PkgPath, "registerAgent") {
			agents++
			agentMux = record(call.Args[0])
		}
		if bootstrapCall(index, call, pkg.Module.Path+"/internal/httpapi", "LimitPublicIntake") {
			limiterCalls++
			limitedMux = record(call.Args[0])
		}
		if target == nil || target.Pkg() == nil || target.Pkg().Path() != "net/http" {
			return true
		}
		if target.Name() != "Handle" && target.Name() != "HandleFunc" {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			t.Error("unsupported indirect bootstrap registration")
			return true
		}
		mux := record(selector.X)
		if mountedMux == nil {
			mountedMux = mux
		}
		if mux == nil || mux != mountedMux {
			t.Error("bootstrap route does not use the shared mux")
		}
		route, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			t.Error("nonliteral bootstrap route")
			return true
		}
		observed = append(observed, route.Value)
		if route.Value == `"/api/"` || route.Value == `"/health/"` {
			id, ok := call.Args[1].(*ast.Ident)
			if !ok {
				t.Errorf("nonlocal API producer for %s", route.Value)
				return true
			}
			producers := 0
			ast.Inspect(body, func(node ast.Node) bool {
				assign, ok := node.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for i, lhs := range assign.Lhs {
					dest, ok := lhs.(*ast.Ident)
					if !ok || pkg.TypesInfo.ObjectOf(dest) != pkg.TypesInfo.ObjectOf(id) {
						continue
					}
					producers++
					if i != 0 || len(assign.Rhs) != 1 {
						t.Errorf("unsupported API producer for %s", route.Value)
						continue
					}
					producer, ok := assign.Rhs[0].(*ast.CallExpr)
					api := pkg.Module.Path + "/internal/httpapi"
					if !ok || !bootstrapCall(index, producer, api, "NewHandlerWithCatalogAndEngagement") &&
						!bootstrapCall(index, producer, api, "NewHandlerWithEngagementAndIntake") {
						t.Errorf("unapproved API producer for %s", route.Value)
					}
				}
				return true
			})
			if producers != 1 {
				t.Errorf("API route %s producer count %d, want 1", route.Value, producers)
			}
		}
		if route.Value == `"/"` {
			wrapped, ok := call.Args[1].(*ast.CallExpr)
			if !ok || !bootstrapCall(index, wrapped, pkg.Module.Path+"/internal/httpapi", "Wrap") {
				t.Error("web route missing typed HTTP wrapper")
				return true
			}
			origin, ok := wrapped.Args[0].(*ast.CallExpr)
			if !ok || !bootstrapCall(index, origin, pkg.Module.Path+"/internal/web", "Routes") {
				t.Error("web route missing typed Routes producer")
			}
		}
		return true
	})
	if want := []string{`"/api/"`, `"/health/"`, `"/"`}; !reflect.DeepEqual(observed, want) {
		t.Fatalf("bootstrap route inventory = %v, want %v", observed, want)
	}
	if agents != 1 || agentMux == nil || agentMux != mountedMux {
		t.Error("registerAgent must use the same mux as the direct bootstrap mounts")
	}
	if appHandlers != 1 || limiterCalls != 1 || limitedMux == nil || limitedMux != mountedMux {
		t.Error("expected the public-intake limiter to consume the shared bootstrap mux")
	}
	if initializer, ok := index.targets[mountedMux].(*ast.CallExpr); !ok || !bootstrapCall(index, initializer, "net/http", "NewServeMux") {
		t.Error("bootstrap mux must come from net/http.NewServeMux")
	}
	if mountedMux == nil {
		return
	}
	ast.Inspect(body, func(node ast.Node) bool {
		if id, ok := node.(*ast.Ident); ok && pkg.TypesInfo.Uses[id] == mountedMux && !allowedUses[id] {
			t.Errorf("unapproved bootstrap mux escape at %s", pkg.Fset.Position(id.Pos()))
		}
		return true
	})
}

func bootstrapCall(index ingressAliases, call *ast.CallExpr, pkg, name string) bool {
	target := index.resolve(call.Fun)
	return target != nil && target.Pkg() != nil && target.Pkg().Path() == pkg && target.Name() == name
}
