package hygiene_test

import (
	"context"
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// sourceGOROOT fills build.Default.GOROOT once for the source importer.
var sourceGOROOT sync.Once

// fillGOROOT gives the source importer a GOROOT: a binary built with -trimpath
// (as the dango E2E builds it) knows none, and the importer reads the standard
// library from there.
func fillGOROOT() {
	sourceGOROOT.Do(func() {
		if build.Default.GOROOT == "" {
			out, err := exec.CommandContext(context.Background(), "go", "env", "GOROOT").Output()
			if err == nil {
				build.Default.GOROOT = strings.TrimSpace(string(out))
			}
		}
	})
}

// floatToIntConversions lists the conversions of a decimal number to an integer type
// in the packages under dir (a path from the repository root), as "file:line: text".
// Go leaves such a conversion to the platform when the number is NaN or beyond the
// integer: amd64 gives the lowest integer, arm64 and wasm saturate, so the same game
// plays differently on a desktop and in a browser. A constant is exempt: the compiler
// refuses one that does not fit.
func floatToIntConversions(t *testing.T, dir string) []string {
	t.Helper()
	const root = "../../.."
	fillGOROOT()
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "source", nil)
	var found []string
	err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		pkg, err := build.ImportDir(p, 0)
		if err != nil {
			return nil //nolint:nilerr // A directory without Go files for this platform holds nothing to check.
		}
		var files []*ast.File
		for _, name := range pkg.GoFiles {
			f, err := parser.ParseFile(fset, filepath.Join(p, name), nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			files = append(files, f)
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
		conf := types.Config{Importer: imp}
		if _, err := conf.Check("github.com/nao1215/gozen/"+filepath.ToSlash(rel), fset, files, info); err != nil {
			return err
		}
		for _, f := range files {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) != 1 {
					return true
				}
				to, arg := info.Types[call.Fun], info.Types[call.Args[0]]
				if !to.IsType() || arg.Value != nil || !basic(to.Type, types.IsInteger) || !basic(arg.Type, types.IsFloat) {
					return true
				}
				pos := fset.Position(call.Pos())
				name, err := filepath.Rel(root, pos.Filename)
				if err != nil {
					name = pos.Filename
				}
				found = append(found, fmt.Sprintf("%s:%d: %s", filepath.ToSlash(name), pos.Line, types.ExprString(call)))
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func basic(t types.Type, flag types.BasicInfo) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&flag != 0
}

// TestModelFloatToInt keeps the definitions and the state free of bare conversions of
// a decimal number to an integer: internal/base/safeint.From gives NaN and numbers
// beyond an int one value on every platform.
func TestModelFloatToInt(t *testing.T) {
	if _, err := os.Stat("../../../internal/model"); err != nil {
		t.Fatal(err)
	}
	for _, at := range floatToIntConversions(t, "internal/model") {
		t.Errorf("%s converts a decimal number to an integer by the platform's rule; use safeint.From", at)
	}
}
