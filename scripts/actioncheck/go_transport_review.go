package actioncheck

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

// reviewedTransportFingerprint binds the reviewed source and the signature of
// its resolved references. It is not a transitive code fingerprint. Callers
// must separately close dependencies, builds and each incoming producer.
func reviewedTransportFingerprint(decl ast.Decl, fileset *token.FileSet, info *types.Info, source []byte, module string) (string, error) {
	file := fileset.File(decl.Pos())
	if file == nil || !decl.End().IsValid() || decl.End() > token.Pos(file.Base()+file.Size()) {
		return "", fmt.Errorf("transport declaration unavailable")
	}
	start, end := file.Offset(decl.Pos()), file.Offset(decl.End())
	if start >= end || end > len(source) {
		return "", fmt.Errorf("transport declaration source unavailable")
	}
	digest := sha256.New()
	fmt.Fprintf(digest, "%q\n", source[start:end])
	qualifier := func(pkg *types.Package) string {
		path := pkg.Path()
		switch {
		case path == module:
			return "$module"
		case strings.HasPrefix(path, module+"/"):
			return "$module" + strings.TrimPrefix(path, module)
		default:
			return path
		}
	}
	ast.Inspect(decl, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Ident:
			ref := info.Uses[n]
			if ref == nil {
				return true
			}
			if pkg, ok := ref.(*types.PkgName); ok {
				fmt.Fprintf(digest, "%d import %q\n", file.Offset(n.Pos())-start, qualifier(pkg.Imported()))
				return true
			}
			fmt.Fprintf(digest, "%d ref %q", file.Offset(n.Pos())-start, types.ObjectString(ref, qualifier))
			if c, ok := ref.(*types.Const); ok {
				fmt.Fprintf(digest, " const %q", c.Val().ExactString())
			}
			fmt.Fprintln(digest)
		case *ast.SelectorExpr:
			if selection := info.Selections[n]; selection != nil {
				fmt.Fprintf(digest, "%d selector %d %v %q\n", file.Offset(n.Pos())-start,
					selection.Kind(), selection.Index(), types.TypeString(selection.Recv(), qualifier))
			}
		}
		return true
	})
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}
