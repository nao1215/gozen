package hygiene_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The checks of this file keep the game's rules (internal/rules) and the runs
// without a screen (internal/drive) repeatable and unable to hang or to read a
// number the platform decides: three classes of defect a review found by hand.

// listedPackage is what `go list -json` tells of one package.
type listedPackage struct {
	ImportPath string
	Dir        string
	Export     string
	GoFiles    []string
}

// goList runs `go list -export -deps -json` from the repository root, so the
// export data of every dependency is at hand for the type checker.
func goList(t *testing.T, patterns ...string) []listedPackage {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "go", append([]string{"list", "-export", "-deps", "-json"}, patterns...)...) //nolint:gosec // Fixed go list of repository packages.
	cmd.Dir = "../../.."
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.String())
	}
	var pkgs []listedPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p listedPackage
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

// guardedConversions are functions, by package path and name, that convert a
// float to an integer only after ruling out NaN and the values outside the
// target's range themselves, where safeint.From would change the answer.
var guardedConversions = map[string]string{
	// A seed keeps all 64 bits; safeint.From would cap it at 2^53 and change every floor.
	"github.com/nao1215/gozen/internal/rules/dungeon.seedBits": "checks NaN, the sign and 2^64 first",
}

// unsafeIntConversions type-checks one package from its files and returns the
// conversions of a float value that is not a constant to an integer type.
// Such a conversion of NaN, an infinity or a value outside the integer's range
// gives a number that depends on the platform; safeint.From gives one answer.
func unsafeIntConversions(fset *token.FileSet, path string, files []*ast.File, imp types.Importer) ([]string, error) {
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	if _, err := (&types.Config{Importer: imp}).Check(path, fset, files, info); err != nil {
		return nil, err
	}
	var out []string
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if fd, ok := n.(*ast.FuncDecl); ok && guardedConversions[path+"."+fd.Name.Name] != "" {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			to, arg := info.Types[call.Fun], info.Types[call.Args[0]]
			if !to.IsType() || arg.Value != nil || to.Type == nil || arg.Type == nil {
				return true
			}
			target, okTo := to.Type.Underlying().(*types.Basic)
			source, okFrom := arg.Type.Underlying().(*types.Basic)
			if okTo && okFrom && target.Info()&types.IsInteger != 0 && source.Info()&types.IsFloat != 0 {
				out = append(out, fmt.Sprintf("%s: %s of a float: convert with safeint.From", fset.Position(call.Pos()), target.Name()))
			}
			return true
		})
	}
	return out, nil
}

// TestRulesConvertNumbersSafely rejects int(float) in the rules and the runs
// without a screen: a YAML expression may yield NaN, an infinity or 1e30, and
// Go leaves the integer such a conversion gives to the platform.
func TestRulesConvertNumbersSafely(t *testing.T) {
	t.Parallel()
	pkgs := goList(t, "./internal/rules/...", "./internal/drive/...")
	exports := map[string]string{}
	for _, p := range pkgs {
		exports[p.ImportPath] = p.Export
	}
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		file := exports[path]
		if file == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(file) //nolint:gosec // Export data that go list named.
	})
	checked := 0
	for _, p := range pkgs {
		rel, ok := strings.CutPrefix(p.ImportPath, module+"internal/")
		if !ok || !strings.HasPrefix(rel, "rules/") && !strings.HasPrefix(rel, "drive/") {
			continue
		}
		var files []*ast.File
		for _, name := range p.GoFiles {
			b, err := os.ReadFile(filepath.Join(p.Dir, name)) //nolint:gosec // A source file that go list named.
			if err != nil {
				t.Fatal(err)
			}
			f, err := parser.ParseFile(fset, "internal/"+rel+"/"+name, b, 0) // named from the root, as the other checks name files
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, f)
		}
		found, err := unsafeIntConversions(fset, p.ImportPath, files, imp)
		if err != nil {
			t.Fatalf("%s: %v", p.ImportPath, err)
		}
		for _, msg := range found {
			t.Error(msg)
		}
		checked++
	}
	if checked < 10 {
		t.Fatalf("only %d packages of internal/rules and internal/drive were checked", checked)
	}
}

// TestUnsafeIntConversionGuard checks the checker: it reports a float variable
// and a float call converted to an integer, and accepts constants, integers
// and a conversion to a float.
func TestUnsafeIntConversionGuard(t *testing.T) {
	t.Parallel()
	const src = `package p

import "math"

func f(x float64, n int) (int, int64, int, int, float64) {
	return int(x), int64(math.Floor(x)), int(3.0), int(int64(n)), float64(n)
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	fillGOROOT()
	found, err := unsafeIntConversions(fset, "p", []*ast.File{file}, importer.ForCompiler(fset, "source", nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || !strings.Contains(found[0], "int of a float") || !strings.Contains(found[1], "int64 of a float") {
		t.Fatalf("found %q, want the two conversions of a float value", found)
	}
}

// unboundedRunningLoops are the loops of the shape `for x.Running() {`: they
// go on for as long as the events do, and events a game writes may not end.
func unboundedRunningLoops(fset *token.FileSet, f *ast.File) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		loop, ok := n.(*ast.ForStmt)
		if !ok || loop.Init != nil || loop.Post != nil {
			return true
		}
		call, ok := loop.Cond.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Running" {
			out = append(out, fmt.Sprintf("%s: a loop that lasts while the events run has no limit: count its rounds (event.Runner.RunUnattended does) and report the events that do not end", fset.Position(loop.Pos())))
		}
		return true
	})
	return out
}

// TestRunnerLoopsAreBounded rejects `for x.Running() {` outside tests: a
// troop's event with a phone that reopened itself hung every automatic battle
// through two such loops.
func TestRunnerLoopsAreBounded(t *testing.T) {
	t.Parallel()
	sourceFiles(t, func(name string, b []byte) {
		if strings.HasSuffix(name, "_test.go") || !bytes.Contains(b, []byte(".Running()")) {
			return
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, b, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, msg := range unboundedRunningLoops(fset, f) {
			t.Error(msg)
		}
	})
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", "package p\nfunc f(r interface{ Running() bool; Next() }) {\n\tfor r.Running() {\n\t\tr.Next()\n\t}\n\tfor range 10 {\n\t\tif !r.Running() {\n\t\t\treturn\n\t\t}\n\t}\n}\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	if found := unboundedRunningLoops(fset, f); len(found) != 1 {
		t.Fatalf("the guard found %q in a file with one unbounded loop and one counted loop", found)
	}
}

// ambientCalls are the uses of process-wide randomness and of the clock in a
// file: a function of math/rand or math/rand/v2 other than a constructor of a
// generator, and time.Now, time.Since and time.Until.
func ambientCalls(fset *token.FileSet, f *ast.File) []string {
	names := map[string]string{} // the file's name of the import -> its path
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path != "math/rand" && path != "math/rand/v2" && path != "time" {
			continue
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if name == "v2" {
			name = "rand"
		}
		if imp.Name != nil {
			name = imp.Name.Name
		}
		names[name] = path
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Obj != nil {
			return true
		}
		switch path := names[pkg.Name]; {
		case path == "time" && (sel.Sel.Name == "Now" || sel.Sel.Name == "Since" || sel.Sel.Name == "Until"):
			out = append(out, fmt.Sprintf("%s: time.%s: a rule's outcome must not depend on the clock; take the time as a value", fset.Position(call.Pos()), sel.Sel.Name))
		case strings.HasPrefix(path, "math/rand") && !strings.HasPrefix(sel.Sel.Name, "New"):
			out = append(out, fmt.Sprintf("%s: rand.%s draws from the process's generator: draw from the *rand.Rand of the state, which a seed repeats", fset.Position(call.Pos()), sel.Sel.Name))
		}
		return true
	})
	return out
}

// TestRulesUseGivenRandomnessAndTime keeps internal/rules repeatable from a
// seed: no function of math/rand's process-wide generator and no clock.
func TestRulesUseGivenRandomnessAndTime(t *testing.T) {
	t.Parallel()
	sourceFiles(t, func(name string, b []byte) {
		if !strings.HasPrefix(name, "internal/rules/") || strings.HasSuffix(name, "_test.go") {
			return
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, b, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, msg := range ambientCalls(fset, f) {
			t.Error(msg)
		}
	})
	const src = `package p

import (
	"math/rand/v2"
	"time"
)

func f(r *rand.Rand) (int, int, time.Time, *rand.Rand) {
	return rand.IntN(3), r.IntN(3), time.Now(), rand.New(rand.NewPCG(1, 2))
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if found := ambientCalls(fset, f); len(found) != 2 {
		t.Fatalf("the guard found %q, want rand.IntN and time.Now only", found)
	}
}
