package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CalledMethod is one RPC method name found in a plugin's source. The
// position lets diagnostics point at the offending callsite.
type CalledMethod struct {
	Name string
	File string
	Line int
}

// AnalyzeGoSource walks the plugin's `src/` directory looking for calls
// to `<receiver>.Call("method.name", ...)`. The first argument must be
// a string literal — runtime-computed method names slip through silently.
//
// Returns nil (with no error) when the plugin has no Go source.
func AnalyzeGoSource(pluginDir string) ([]CalledMethod, error) {
	srcDir := filepath.Join(pluginDir, "src")
	if !dirExists(srcDir) {
		return nil, nil
	}

	var methods []CalledMethod
	fset := token.NewFileSet()

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip vendored deps and test fixtures; not interesting and pollutes results.
			name := info.Name()
			if name == "vendor" || name == "node_modules" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip generated files — actions_gen.go is ours, plus anything else
		// with a "generated" header would be noise.
		if strings.HasSuffix(path, "_gen.go") {
			return nil
		}

		f, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			// Don't fail the whole walk on one unparseable file.
			return nil
		}

		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name != "Call" {
				return true
			}
			// First argument must be a string literal.
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			name, err := strconv.Unquote(lit.Value)
			if err != nil || name == "" {
				return true
			}
			// Only collect names that look like RPC methods (contain a dot
			// or match the known platform pattern). Narrows false-positives
			// from incidental .Call methods on unrelated types.
			if !strings.Contains(name, ".") && !strings.HasPrefix(name, "_platform") {
				return true
			}
			pos := fset.Position(lit.Pos())
			methods = append(methods, CalledMethod{
				Name: name,
				File: pos.Filename,
				Line: pos.Line,
			})
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return methods, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
