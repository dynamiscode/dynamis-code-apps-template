package actioncheck

import (
	"go/ast"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// This checks first-party packages compiled into cmd/server in this build.
// Other build configurations and third-party implementations are separate
// review boundaries. Merely passing this diagnostic is not ingress closure.
func TestIngressNoDefaultOrAlternateServer(t *testing.T) {
	roots, err := packages.Load(&packages.Config{
		Dir: "../..", Mode: packages.LoadAllSyntax | packages.NeedModule,
	}, "./cmd/server")
	if err != nil || len(roots) != 1 || roots[0].Module == nil {
		t.Fatalf("load production server: %v", err)
	}
	module := roots[0].Module.Path
	explicitServe, serverLiterals := 0, 0
	packages.Visit(roots, func(pkg *packages.Package) bool {
		if !strings.HasPrefix(pkg.PkgPath, module+"/") {
			return false
		}
		if len(pkg.Errors) != 0 || pkg.TypesInfo == nil {
			t.Errorf("type check %s: %v", pkg.PkgPath, pkg.Errors)
			return false
		}
		for _, file := range pkg.Syntax {
			for _, declaration := range file.Decls {
				fn, _ := declaration.(*ast.FuncDecl)
				ast.Inspect(declaration, func(node ast.Node) bool {
					literal, ok := node.(*ast.CompositeLit)
					if !ok || types.TypeString(pkg.TypesInfo.TypeOf(literal), func(other *types.Package) string { return other.Path() }) != "net/http.Server" {
						return true
					}
					serverLiterals++
					if fn == nil || fn.Name.Name != "NewServer" || pkg.PkgPath != module+"/internal/httpapi" {
						t.Errorf("unapproved Server literal at %s", pkg.Fset.Position(literal.Pos()))
						return true
					}
					signature := pkg.TypesInfo.Defs[fn.Name].(*types.Func).Signature()
					found := false
					for _, element := range literal.Elts {
						entry, ok := element.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						key, ok := entry.Key.(*ast.Ident)
						if !ok || key.Name != "Handler" {
							continue
						}
						value, ok := entry.Value.(*ast.Ident)
						found = ok && signature.Params().Len() == 2 && pkg.TypesInfo.Uses[value] == signature.Params().At(1)
					}
					if !found {
						t.Error("NewServer must carry its incoming handler to Server.Handler")
					}
					return true
				})
			}
			for id, obj := range pkg.TypesInfo.Uses {
				if id.Pos() < file.Pos() || id.Pos() > file.End() || obj.Pkg() == nil || obj.Pkg().Path() != "net/http" {
					continue
				}
				switch obj.Name() {
				case "DefaultServeMux", "ListenAndServe", "ListenAndServeTLS", "ServeTLS":
					t.Errorf("unsupported HTTP ingress %s at %s", obj.Name(), pkg.Fset.Position(id.Pos()))
				case "Handle", "HandleFunc":
					if fn, ok := obj.(*types.Func); !ok || fn.Signature().Recv() == nil {
						t.Errorf("unsupported default mux registration at %s", pkg.Fset.Position(id.Pos()))
					}
				case "Serve":
					fn, ok := obj.(*types.Func)
					enclosing := ""
					for _, declaration := range file.Decls {
						if enclosingFn, ok := declaration.(*ast.FuncDecl); ok && declaration.Pos() <= id.Pos() && id.Pos() <= declaration.End() {
							enclosing = enclosingFn.Name.Name
							break
						}
					}
					if ok && fn.FullName() == "(*net/http.Server).Serve" && pkg.PkgPath == module+"/internal/bootstrap" && enclosing == "Run" {
						explicitServe++
						continue
					}
					t.Errorf("unapproved server entry at %s", pkg.Fset.Position(id.Pos()))
				}
			}
		}
		return true
	}, nil)
	if explicitServe != 1 {
		t.Fatalf("found %d supported explicit server references, want 1", explicitServe)
	}
	if serverLiterals != 1 {
		t.Fatalf("found %d HTTP server literals, want 1", serverLiterals)
	}
}
