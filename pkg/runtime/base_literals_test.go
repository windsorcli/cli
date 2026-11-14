package runtime_test

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// =============================================================================
// Test Public Methods
// =============================================================================

func TestSourceDirectoriesJoinTheBasePath(t *testing.T) {
	t.Run("NoJoinOfASourceDirectoryStartsAtTheProjectRoot", func(t *testing.T) {
		// Given every non-test Go file under pkg and cmd
		root := filepath.Join("..", "..")
		fset := token.NewFileSet()
		var violations []string

		// When each filepath.Join call with a terraform, kustomize, or contexts literal is checked
		for _, top := range []string{"pkg", "cmd"} {
			err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return err
				}
				file, err := parser.ParseFile(fset, path, nil, 0)
				if err != nil {
					return err
				}
				ast.Inspect(file, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok || !isFilepathJoin(call) || len(call.Args) < 2 {
						return true
					}
					if hasSourceDirLiteral(call.Args) && !hasLiteral(call.Args, ".windsor") && startsAtProjectRoot(call.Args[0]) {
						violations = append(violations, path+": "+fset.Position(call.Pos()).String())
					}
					return true
				})
				return nil
			})
			if err != nil {
				t.Fatalf("walk %s: %v", top, err)
			}
		}

		// Then none starts at the project root, because the directories sit under the base path
		if len(violations) > 0 {
			t.Fatalf("filepath.Join starts a terraform, kustomize, or contexts path at the project root. Start it at the base path.\n%s", strings.Join(violations, "\n"))
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// isFilepathJoin reports whether call is filepath.Join.
func isFilepathJoin(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Join" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "filepath"
}

// hasSourceDirLiteral reports whether args holds a string literal that names a source directory.
func hasSourceDirLiteral(args []ast.Expr) bool {
	for _, name := range []string{"terraform", "kustomize", "contexts"} {
		if hasLiteral(args, name) {
			return true
		}
	}
	return false
}

// hasLiteral reports whether args holds the string literal value.
func hasLiteral(args []ast.Expr, value string) bool {
	for _, arg := range args {
		lit, ok := arg.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		if got, err := strconv.Unquote(lit.Value); err == nil && got == value {
			return true
		}
	}
	return false
}

// startsAtProjectRoot reports whether the first argument of a Join is a variable or field that
// names the project root. A call, such as a base path lookup that takes the project root, does not count.
func startsAtProjectRoot(arg ast.Expr) bool {
	switch arg.(type) {
	case *ast.Ident, *ast.SelectorExpr:
	default:
		return false
	}
	var sb strings.Builder
	_ = printer.Fprint(&sb, token.NewFileSet(), arg)
	return strings.Contains(strings.ToLower(sb.String()), "projectroot")
}
