package actioncheck

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

// This review covers the replay mechanism, not the producer behind next.
// Related route/consumer and dependency checks must still close that edge.
func TestTransportReviewExistingTimeoutReplay(t *testing.T) {
	packagesFound, err := packages.Load(&packages.Config{
		Dir: "../..", Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedCompiledGoFiles | packages.NeedModule,
	}, "./internal/httpapi")
	if err != nil || len(packagesFound) != 1 {
		t.Fatalf("load HTTP API package: %v", err)
	}
	pkg := packagesFound[0]
	if len(pkg.Errors) != 0 || pkg.Module == nil {
		t.Fatalf("type check HTTP API package: %v", pkg.Errors)
	}
	want := map[string]string{
		"bufferedResponse":             "a01ef61694d9e40a551d818eeceb91df27162ded0ba7521cc9e0f62192de9024",
		"handlerResult":                "6aaacd8cdc4df4b2d79b8fb2522bd36cd4eadf9bc35c02a9e62b5f5eda040a96",
		"newBufferedResponse":          "196d879a86dc7ddfdeafb999567069acdc72ce0257f8202cc3e8c8c48796149c",
		"bufferedResponse.Header":      "f7e20f251428341fa0ec1bf7c14aadccf39f94072f937f614f9d9d28828b4d34",
		"bufferedResponse.WriteHeader": "ad50d6c4f6cb7f7d102d9e3cc254d465eddd4cc21acc70fbd979379cab43d2c2",
		"bufferedResponse.Write":       "5f11960fcff6318f7563f8e155cb8d2a09199fe8448abe4f5b6031f80eaa21d2",
		"copyHeaders":                  "137e5a25ad30ca9e5ad0e32170a057a11f6e0897b6e8b46a42558444e7ac8388",
		"timeoutMiddleware":            "d80d6e5ecd305dde53850a77de57cf2051f794138715c8a0cf19085c3b0a5905",
	}
	for i, file := range pkg.Syntax {
		path := pkg.CompiledGoFiles[i]
		if filepath.Base(path) != "middleware.go" {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			name := ""
			switch decl := declaration.(type) {
			case *ast.FuncDecl:
				name = decl.Name.Name
				if fn, ok := pkg.TypesInfo.Defs[decl.Name].(*types.Func); ok && fn.Signature().Recv() != nil {
					receiver := fn.Signature().Recv().Type()
					if pointer, ok := receiver.(*types.Pointer); ok {
						receiver = pointer.Elem()
					}
					if named, ok := receiver.(*types.Named); ok {
						name = named.Obj().Name() + "." + name
					}
				}
			case *ast.GenDecl:
				if len(decl.Specs) == 1 {
					if spec, ok := decl.Specs[0].(*ast.TypeSpec); ok {
						name = spec.Name.Name
					}
				}
			}
			expected, found := want[name]
			if !found {
				continue
			}
			digest, err := reviewedTransportFingerprint(declaration, pkg.Fset, pkg.TypesInfo, source, pkg.Module.Path)
			if err != nil {
				t.Fatal(err)
			}
			if digest != expected {
				t.Errorf("audited timeout transport %s changed (%s)", name, digest)
			}
			delete(want, name)
		}
	}
	for name := range want {
		t.Errorf("audited timeout transport %s missing", name)
	}
}

func TestTransportReviewBindsImportedDestinationAndBody(t *testing.T) {
	fingerprint := func(backend, literal string) string {
		t.Helper()
		const module = "example.test/fixture"
		source := "package fixture\nimport sink " + backend + "\nfunc send() { _, _ = sink.New(), " + literal + " }"
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "fixture.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		info := &types.Info{
			Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{},
		}
		if _, err := (&types.Config{Importer: importer.Default()}).Check(module, fset, []*ast.File{file}, info); err != nil {
			t.Fatal(err)
		}
		result, err := reviewedTransportFingerprint(file.Decls[1], fset, info, []byte(source), module)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	base := fingerprint(`"crypto/md5"`, `"outcome"`)
	if base == fingerprint(`"crypto/sha256"`, `"outcome"`) {
		t.Fatal("changing import destination preserved transport fingerprint")
	}
	if base == fingerprint(`"crypto/md5"`, `"different"`) {
		t.Fatal("changing transport body preserved fingerprint")
	}
}
