package actioncheck

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// inventoryWriterCapabilities loads production packages, including command
// entry points outside internal/web. It is an inventory prototype, not a
// proof of what already reviewed code can do after erasing the writer type.
func inventoryWriterCapabilities(root string, reviews map[string]map[string]string) ([]string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	config := &packages.Config{
		Dir:  absolute,
		Mode: packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
	}
	loaded, err := packages.Load(config, "./...", "net/http")
	if err != nil {
		return nil, err
	}
	var writer *types.Interface
	for _, pkg := range loaded {
		if len(pkg.Errors) != 0 {
			return nil, fmt.Errorf("load %s: %s", pkg.PkgPath, pkg.Errors[0])
		}
		if pkg.PkgPath == "net/http" {
			writer, _ = pkg.Types.Scope().Lookup("ResponseWriter").Type().Underlying().(*types.Interface)
		}
	}
	if writer == nil {
		return nil, fmt.Errorf("net/http.ResponseWriter unavailable")
	}
	var rejected []string
	for _, pkg := range loaded {
		for i, file := range pkg.Syntax {
			path, err := filepath.Rel(absolute, pkg.CompiledGoFiles[i])
			if err != nil {
				return nil, err
			}
			if path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) || strings.HasSuffix(path, "_templ.go") {
				continue
			}
			source, err := os.ReadFile(filepath.Join(absolute, path))
			if err != nil {
				return nil, err
			}
			path = filepath.ToSlash(path)
			for _, finding := range reviewWriterCapability(file, pkg.TypesInfo, source, writer, reviews[path]) {
				rejected = append(rejected, path+": "+finding)
			}
		}
	}
	return rejected, nil
}

// reviewWriterCapability finds explicit introductions of the HTTP response
// writer capability, including references through aliases and inferred types.
// Review is bound to the entire owning function (or package declaration).
func reviewWriterCapability(file *ast.File, info *types.Info, source []byte, writer *types.Interface, exact map[string]string) []string {
	var rejected []string
	for _, declaration := range file.Decls {
		name := ""
		switch decl := declaration.(type) {
		case *ast.FuncDecl:
			name = decl.Name.Name
			if decl.Recv != nil {
				name = types.TypeString(info.TypeOf(decl.Recv.List[0].Type), nil) + "." + name
			}
		case *ast.GenDecl:
			name = fmt.Sprintf("package declaration at byte %d", declaration.Pos()-file.FileStart)
		}
		usesWriter := false
		ast.Inspect(declaration, func(node ast.Node) bool {
			expression, ok := node.(ast.Expr)
			if !ok || usesWriter {
				return !usesWriter
			}
			typ := info.TypeOf(expression)
			usesWriter = typ != nil && carriesResponseWriter(typ, writer, make(map[types.Type]bool))
			return !usesWriter
		})
		if !usesWriter {
			continue
		}
		from, to := declaration.Pos()-file.FileStart, declaration.End()-file.FileStart
		if from < 0 || to <= from || int(to) > len(source) {
			rejected = append(rejected, name+": unavailable declaration source")
			continue
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(source[from:to]))
		if exact[name] != digest {
			rejected = append(rejected, name+": unreviewed writer capability ("+digest+")")
		}
	}
	return rejected
}

func carriesResponseWriter(typ types.Type, writer *types.Interface, seen map[types.Type]bool) bool {
	if seen[typ] {
		return false
	}
	seen[typ] = true
	if _, basic := typ.Underlying().(*types.Basic); !basic && types.Implements(typ, writer) {
		return true
	}
	switch t := typ.Underlying().(type) {
	case *types.Signature:
		return carriesResponseWriter(t.Params(), writer, seen) || carriesResponseWriter(t.Results(), writer, seen)
	case *types.Tuple:
		for i := 0; i < t.Len(); i++ {
			if carriesResponseWriter(t.At(i).Type(), writer, seen) {
				return true
			}
		}
	case *types.Interface:
		for i := 0; i < t.NumMethods(); i++ {
			if carriesResponseWriter(t.Method(i).Type(), writer, seen) {
				return true
			}
		}
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			if carriesResponseWriter(t.Field(i).Type(), writer, seen) {
				return true
			}
		}
	case *types.Pointer:
		return carriesResponseWriter(t.Elem(), writer, seen)
	case *types.Slice:
		return carriesResponseWriter(t.Elem(), writer, seen)
	case *types.Array:
		return carriesResponseWriter(t.Elem(), writer, seen)
	case *types.Chan:
		return carriesResponseWriter(t.Elem(), writer, seen)
	case *types.Map:
		return carriesResponseWriter(t.Key(), writer, seen) || carriesResponseWriter(t.Elem(), writer, seen)
	}
	return false
}

// unresolvedWriterTransfers does not allow opaque calls or conversions that
// erase the capability to proceed merely because the surrounding handler is
// reviewed. A caller must certify each destination separately.
func unresolvedWriterTransfers(body ast.Node, info *types.Info, writer *types.Interface, checked func(*ast.CallExpr) bool) []*ast.CallExpr {
	var unresolved []*ast.CallExpr
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, argument := range call.Args {
			typ := info.TypeOf(argument)
			if typ != nil && carriesResponseWriter(typ, writer, make(map[types.Type]bool)) {
				if !checked(call) {
					unresolved = append(unresolved, call)
				}
				break
			}
		}
		return true
	})
	return unresolved
}

// opaqueWriterStorage finds erasing writer assignments and structural storage.
// A separately audited adapter may preserve the capability in a typed field.
func opaqueWriterStorage(body ast.Node, info *types.Info, writer *types.Interface) []ast.Node {
	var unresolved []ast.Node
	carries := func(expression ast.Expr) bool {
		typ := info.TypeOf(expression)
		return typ != nil && carriesResponseWriter(typ, writer, make(map[types.Type]bool))
	}
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.TypeAssertExpr:
			// A narrowing assertion, including a type switch with opaque
			// branch bindings, cannot erase an incoming writer capability.
			if carries(n.X) && (n.Type == nil || !carries(n)) {
				unresolved = append(unresolved, n)
			}
		case *ast.ValueSpec:
			for i, value := range n.Values {
				if !carries(value) {
					continue
				}
				if len(n.Names) != len(n.Values) {
					unresolved = append(unresolved, n)
					continue
				}
				obj := info.Defs[n.Names[i]]
				if obj == nil || !carriesResponseWriter(obj.Type(), writer, make(map[types.Type]bool)) {
					unresolved = append(unresolved, n)
				}
			}
		case *ast.AssignStmt:
			for i, right := range n.Rhs {
				if !carries(right) {
					continue
				}
				if len(n.Rhs) != len(n.Lhs) || !carries(n.Lhs[i]) {
					unresolved = append(unresolved, n)
				}
			}
		case *ast.CompositeLit:
			for _, element := range n.Elts {
				if entry, ok := element.(*ast.KeyValueExpr); ok {
					element = entry.Value
				}
				if carries(element) {
					unresolved = append(unresolved, element)
				}
			}
		case *ast.SendStmt:
			if carries(n.Value) {
				unresolved = append(unresolved, n)
			}
		case *ast.ReturnStmt:
			for _, expression := range n.Results {
				if carries(expression) {
					unresolved = append(unresolved, expression)
				}
			}
		}
		return true
	})
	return unresolved
}
