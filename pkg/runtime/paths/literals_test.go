package paths_test

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
// Test Setup
// =============================================================================

var sourceDirLiterals = []string{"terraform", "kustomize", "contexts"}

// exemptBases are the substrings of a Join base argument that mark a path outside the project
// source directories: the .windsor scratch path and the context directory, which holds its own terraform subdirectory.
var exemptBases = []string{"scratch", "configroot", "contextdir"}

// allowedFiles may hold the literals. The resolver defines the default names. The artifact layout
// file defines the fixed names of the bundle layout.
var allowedFiles = map[string]bool{
	"pkg/runtime/paths/paths.go":      true,
	"pkg/composer/artifact/layout.go": true,
}

// nonTestGoFiles parses each non-test Go file under pkg and cmd, keyed by its slash path from the repo root.
func nonTestGoFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	root := filepath.Join("..", "..", "..")
	files := make(map[string]*ast.File)
	fset := token.NewFileSet()
	for _, top := range []string{"pkg", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			parsed, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			files[filepath.ToSlash(rel)] = parsed
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", top, err)
		}
	}
	return files
}

// =============================================================================
// Test Public Methods
// =============================================================================

func TestSourceDirectoryLiterals(t *testing.T) {
	t.Run("NoJoinOutsideTheResolverUsesASourceDirectoryLiteral", func(t *testing.T) {
		// Given every non-test Go file under pkg and cmd
		files := nonTestGoFiles(t)

		// When each filepath.Join and path.Join call is checked
		var violations []string
		for name, file := range files {
			if allowedFiles[name] {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !isJoin(call) || len(call.Args) == 0 {
					return true
				}
				if hasLiteral(call.Args, ".windsor") || exemptBase(call.Args[0]) {
					return true
				}
				for _, arg := range call.Args {
					if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING && namesSourceDir(lit) {
						violations = append(violations, name+": "+lit.Value)
					}
				}
				return true
			})
		}

		// Then no call uses the literals
		if len(violations) > 0 {
			t.Fatalf("filepath.Join uses a source directory literal. Use the path resolver.\n%s", strings.Join(violations, "\n"))
		}
	})

	t.Run("NoArtifactStringUsesASourceDirectoryLiteral", func(t *testing.T) {
		// Given the files of the artifact package
		files := nonTestGoFiles(t)

		// When each string literal is checked
		var violations []string
		for name, file := range files {
			if !strings.HasPrefix(name, "pkg/composer/artifact/") || allowedFiles[name] {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				if _, isImport := n.(*ast.ImportSpec); isImport {
					return false
				}
				if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && namesSourceDir(lit) {
					violations = append(violations, name+": "+lit.Value)
				}
				return true
			})
		}

		// Then none holds a literal, because layout.go defines the names
		if len(violations) > 0 {
			t.Fatalf("The artifact package uses a source directory literal. Use the layout constants.\n%s", strings.Join(violations, "\n"))
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// isJoin reports whether call is filepath.Join or path.Join.
func isJoin(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Join" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && (pkg.Name == "filepath" || pkg.Name == "path")
}

// hasLiteral reports whether args holds the string literal value.
func hasLiteral(args []ast.Expr, value string) bool {
	for _, arg := range args {
		if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING && unquote(lit) == value {
			return true
		}
	}
	return false
}

// exemptBase reports whether the base argument of a Join names a scratch path or a context directory.
func exemptBase(arg ast.Expr) bool {
	var sb strings.Builder
	_ = printer.Fprint(&sb, token.NewFileSet(), arg)
	text := strings.ToLower(sb.String())
	for _, marker := range exemptBases {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// namesSourceDir reports whether a string literal is, or has a path element equal to, a source directory name.
func namesSourceDir(lit *ast.BasicLit) bool {
	for _, element := range strings.Split(unquote(lit), "/") {
		for _, name := range sourceDirLiterals {
			if element == name {
				return true
			}
		}
	}
	return false
}

// unquote returns the value of a string literal.
func unquote(lit *ast.BasicLit) string {
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return lit.Value
	}
	return value
}
