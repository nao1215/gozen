package docs

import (
	"bytes"
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
	"sort"
	"strings"
	"testing"
)

// snippetImports are the packages a Go example of the documents may use without
// writing its imports, by the name it uses.
var snippetImports = map[string]string{
	"context": "context",
	"embed":   "embed",
	"errors":  "errors",
	"fmt":     "fmt",
	"fs":      "io/fs",
	"log":     "log",
	"os":      "os",
	"signal":  "os/signal",
	"rand":    "math/rand/v2",
	"strings": "strings",
	"testing": "testing",
	"ebiten":  "github.com/hajimehoshi/ebiten/v2",
	"gozen":   "github.com/nao1215/gozen",
	"play":    "github.com/nao1215/gozen/play",
	"puppet":  "github.com/nao1215/gozen/puppet",
	"react":   "github.com/nao1215/gozen/react",
	"chat":    "github.com/nao1215/gozen/chat",
}

// snippetParams are the names an example of statements may use without declaring
// them: what the surrounding code of a game has at hand.
const snippetParams = "ctx context.Context, t *testing.T, files fs.FS, fsys fs.FS, screen *ebiten.Image, s *gozen.Session, g *gozen.Game"

// goSnippet turns an example into a Go file of package main: a whole file as it is;
// declarations (func, var, const, type) with the imports added; statements inside a
// function whose parameters are snippetParams, in a block of their own so that the
// example may declare the same names again.
func goSnippet(body string) string {
	if strings.HasPrefix(strings.TrimSpace(body), "package ") || strings.Contains(body, "\npackage ") {
		return body
	}
	var imports strings.Builder
	imports.WriteString("package main\n\nimport (\n")
	for _, name := range sortedKeys(snippetImports) {
		fmt.Fprintf(&imports, "\t%s %q\n", name, snippetImports[name])
	}
	imports.WriteString(")\n\n")
	declarations := true
	for _, line := range strings.Split(body, "\n") {
		if line == "" || strings.HasPrefix(line, "//") || line[0] == '\t' || line[0] == ' ' || line == "}" || line == ")" {
			continue
		}
		if f := strings.Fields(line); len(f) == 0 || (f[0] != "func" && f[0] != "var" && f[0] != "const" && f[0] != "type") {
			declarations = false
			break
		}
	}
	if declarations {
		return imports.String() + body
	}
	return imports.String() + "func example(" + snippetParams + ") error {\n\t{\n" + body + "\n\t}\n\treturn nil\n}\n"
}

// exportData lists the compiled export data of the packages the examples may import.
func exportData(t *testing.T) map[string]string {
	t.Helper()
	paths := []string{}
	for _, p := range snippetImports {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	cmd := exec.Command("go", append([]string{"list", "-export", "-deps", "-f", "{{.ImportPath}}\t{{.Export}}"}, paths...)...) //nolint:gosec // go list of fixed packages.
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -export: %v\n%s", err, stderr.String())
	}
	data := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		path, file, ok := strings.Cut(line, "\t")
		if ok && file != "" {
			data[path] = file
		}
	}
	return data
}

// checkGo type-checks an example. Unused variables and imports are allowed (an
// example shows a part of a program); anything else is an error.
func checkGo(src string, data map[string]string) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "example.go", src, parser.ParseComments)
	if err != nil {
		return []string{err.Error()}
	}
	var bad []string
	conf := types.Config{
		Importer: importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
			f, ok := data[path]
			if !ok {
				return nil, fmt.Errorf("no export data for %s", path)
			}
			return os.Open(f) //nolint:gosec // Export data from go list.
		}),
		Error: func(err error) {
			var te types.Error
			if errors.As(err, &te) && te.Soft {
				return // declared and not used, imported and not used
			}
			bad = append(bad, err.Error())
		},
	}
	if _, err := conf.Check("main", fset, []*ast.File{file}, nil); err != nil && len(bad) == 0 {
		// Error saw every error; only soft ones reach here, and they are allowed.
		var te types.Error
		if !errors.As(err, &te) || !te.Soft {
			bad = append(bad, err.Error())
		}
	}
	return bad
}

// TestDocGo type-checks the Go examples of README.md and the website against this
// checkout. An example without a mark is checked; `file PATH` examples are checked
// as whole files too; `skip REASON` leaves one out.
func TestDocGo(t *testing.T) {
	data := exportData(t)
	checked := 0
	for _, doc := range stepDocuments(t) {
		src, err := os.ReadFile(filepath.Join(root, doc)) //nolint:gosec // A document of this repository.
		if err != nil {
			t.Fatal(err)
		}
		blocks, _ := parseBlocks(doc, src)
		for _, b := range blocks {
			if b.lang != "go" || b.mark != nil && b.mark.kind == "skip" {
				continue
			}
			checked++
			for _, e := range checkGo(goSnippet(b.body), data) {
				t.Errorf("%s: %s", b.where, e)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no Go example was checked")
	}
}

func TestCheckGoFindsErrors(t *testing.T) {
	data := exportData(t)
	good := []string{
		"g, err := gozen.Load(os.DirFS(\"game\"), \".\")\nvar gold int\nerr = s.Query(\"gold\", &gold)\n",
		"//go:embed game\nvar files embed.FS\n\nfunc main() {\n\tlog.Fatal(play.Main(context.Background(), files, \"game\", os.Args[1:], play.Config{Output: os.Stdout}))\n}\n",
		"package main\n\nfunc main() {}\n",
	}
	for _, src := range good {
		if bad := checkGo(goSnippet(src), data); len(bad) > 0 {
			t.Errorf("%q: %v", src, bad)
		}
	}
	for src, want := range map[string]string{
		"s.NoSuchMethod()\n":                        "NoSuchMethod",
		"x := gozen.Load(os.DirFS(\".\"), \".\")\n": "assignment mismatch",
		"func main() { play.Run() }\n":              "Run",
	} {
		if bad := strings.Join(checkGo(goSnippet(src), data), "\n"); !strings.Contains(bad, want) {
			t.Errorf("%q: want an error with %q, got %q", src, want, bad)
		}
	}
}
