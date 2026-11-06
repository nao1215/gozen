package hygiene_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestFitLinesSizeIsKept fails when a caller drops the size gfx.FitLines
// returns (`lines, _ := gfx.FitLines(...)`). FitLines wraps text that does not
// fit its lines at a smaller size; lines drawn at the size asked for instead
// run past the right edge (the phone feed's long captions were cut so). A
// caller that keeps the lines keeps their size and draws at it.
func TestFitLinesSizeIsKept(t *testing.T) {
	t.Parallel()
	err := filepath.WalkDir("../../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "build", "node_modules", "vendor":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 2 || len(as.Rhs) != 1 {
				return true
			}
			call, ok := as.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "FitLines" {
				return true
			}
			if id, ok := as.Lhs[1].(*ast.Ident); ok && id.Name == "_" {
				t.Errorf("%s: the size FitLines wrapped at is dropped; keep it and draw the lines at it", fset.Position(as.Pos()))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
