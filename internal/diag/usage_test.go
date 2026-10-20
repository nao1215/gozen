package diag

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// familyOf maps each package that raises codes to the exit values it reports.
// A package that starts raising codes has to be listed, so the choice is
// deliberate.
var familyOf = map[string][]int{
	"cmd/gozen":                     {0, 1, 2},
	"cmd/internal/cli":              {1, 2},
	"cmd/internal/checkdata":        {0, 1},
	"cmd/internal/trial":            {1},
	"internal/layer2/gamedata":      {0, 1},
	"internal/mosaic/command":       {0, 1, 2},
	"internal/promptgen/command":    {1, 2},
	"internal/clihelp":              {2},
	"cmd/gozen/internal/designeval": {1},
}

// untested lists the codes no E2E scenario can provoke, with the reason. A
// code listed here that a scenario does provoke fails the test, so the list
// only shrinks.
var untested = map[string]string{}

// TestCodes_AreReferenced fails for a code that exists only in the reference.
func TestCodes_AreReferenced(t *testing.T) {
	t.Parallel()
	used := usedCodes(t)
	for _, e := range All() {
		if _, ok := used[e.Name]; !ok {
			t.Errorf("%s (diag.%s) is registered but never raised; use it or remove it", e.Code, e.Name)
		}
	}
}

// TestCodes_MatchTheirPackageFamily checks the thousands digit against the
// exit values of the package that raises the code.
func TestCodes_MatchTheirPackageFamily(t *testing.T) {
	t.Parallel()
	byName := map[string]Entry{}
	for _, e := range All() {
		byName[e.Name] = e
	}
	for name, pkgs := range usedCodes(t) {
		e := byName[name]
		for _, pkg := range pkgs {
			want, ok := familyOf[pkg]
			if !ok {
				t.Errorf("%s raises diagnostics but is not in familyOf", pkg)
				continue
			}
			if !slices.Contains(want, e.Code.ExitCode()) {
				t.Errorf("%s raises %s, but it exits %v", pkg, e.Code, want)
			}
		}
	}
}

// TestCodes_AreProvokedByE2E requires every code to appear in an assertion of
// e2e/atago next to the exit value its first digit promises.
func TestCodes_AreProvokedByE2E(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join(repoRoot(t), "e2e", "atago", "*.atago.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatal("no E2E specs", err)
	}
	seen := map[Code]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // The repository's own specs.
		if err != nil {
			t.Fatal(err)
		}
		var spec struct {
			Scenarios []map[string]any `yaml:"scenarios"`
		}
		if err := yaml.Unmarshal(b, &spec); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, sc := range spec.Scenarios {
			for _, asserted := range assertions(t, sc) {
				for _, c := range reported(asserted.text) {
					seen[c] = true
					if asserted.exit != c.ExitCode() {
						t.Errorf("%s: %q asserts %s with exit_code %d", filepath.Base(f), sc["name"], c, asserted.exit)
					}
				}
			}
		}
	}
	for _, e := range All() {
		reason, skip := untested[e.Name]
		switch {
		case skip && seen[e.Code]:
			t.Errorf("%s (%s) is provoked by a scenario now; remove it from untested", e.Code, e.Name)
		case !skip && !seen[e.Code]:
			t.Errorf("%s (%s) is not provoked by any scenario of e2e/atago; add one to errors.atago.yaml", e.Code, e.Name)
		case skip && reason == "":
			t.Errorf("%s is untested without a reason", e.Code)
		}
	}
	for name := range untested {
		found := false
		for _, e := range All() {
			found = found || e.Name == name
		}
		if !found {
			t.Errorf("untested lists %s, which is not registered", name)
		}
	}
}

// reportedText is a code as a diagnostic reports it: at the end of a line, or
// in the code field of JSON. A code named elsewhere (gozen help GZN1101) is
// not one the command reported.
var reportedText = regexp.MustCompile(`\[(GZN\d{4})\]|"code": ?\\?"(GZN\d{4})`)

func reported(text string) []Code {
	var out []Code
	for _, m := range reportedText.FindAllStringSubmatch(text, -1) {
		out = append(out, Codes(m[1]+m[2])...)
	}
	return out
}

type asserted struct {
	text string
	exit int
}

// assertions expands a scenario's matrix and returns the text and the exit
// code of each assert step.
func assertions(t *testing.T, sc map[string]any) []asserted {
	t.Helper()
	rows, ok := sc["matrix"].([]any)
	if !ok {
		rows = nil
	}
	if len(rows) == 0 {
		rows = []any{map[string]any{}}
	}
	raw, err := yaml.Marshal(sc["steps"])
	if err != nil {
		t.Fatal(err)
	}
	var out []asserted
	for _, row := range rows {
		text := string(raw)
		vars, ok := row.(map[string]any)
		if !ok {
			vars = nil
		}
		keys := make([]string, 0, len(vars))
		for k := range vars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			text = strings.ReplaceAll(text, "${"+k+"}", fmt.Sprint(vars[k]))
		}
		var steps []map[string]any
		if err := yaml.Unmarshal([]byte(text), &steps); err != nil {
			t.Fatal(err)
		}
		for _, st := range steps {
			a, ok := st["assert"].(map[string]any)
			if !ok {
				continue
			}
			exit := -1
			if v, ok := a["exit_code"]; ok {
				fmt.Sscan(fmt.Sprint(v), &exit) //nolint:errcheck // -1 stays on a non-number.
			}
			delete(a, "exit_code")
			b, err := yaml.Marshal(a)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, asserted{string(b), exit})
		}
	}
	return out
}

// usedCodes maps every registered code referenced from non-test Go code
// outside this package to the packages that reference it.
func usedCodes(t *testing.T) map[string][]string {
	t.Helper()
	root := repoRoot(t)
	registered := map[string]bool{}
	for _, e := range All() {
		registered[e.Name] = true
	}
	used := map[string][]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", ".cache", "build", "bin", "dist", "node_modules", "public", "testdata", "diag":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly|parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		if !importsDiag(file) {
			return nil
		}
		if file, err = parser.ParseFile(fset, path, nil, parser.SkipObjectResolution); err != nil {
			return err
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(rel)
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "diag" && registered[sel.Sel.Name] && !slices.Contains(used[sel.Sel.Name], pkg) {
				used[sel.Sel.Name] = append(used[sel.Sel.Name], pkg)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return used
}

func importsDiag(file *ast.File) bool {
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) == "github.com/nao1215/gozen/internal/diag" {
			return true
		}
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
