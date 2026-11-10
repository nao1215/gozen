package hygiene_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// yamlDecoders are the entry points of the YAML library that read untrusted bytes or
// nodes into Go values.
var yamlDecoders = map[string]bool{
	"Unmarshal": true, "UnmarshalWithOptions": true, "UnmarshalContext": true,
	"NodeToValue": true, "NewDecoder": true,
}

// unguardedYAMLDecodes lists the YAML decodes of one Go source that can panic into the
// host: a call of the YAML library's decoder that is neither inside the function given
// to decodeguard.Run nor inside an UnmarshalYAML method, which only runs under the
// decode that called it.
func unguardedYAMLDecodes(name string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		return nil, err
	}
	yaml := ""
	for _, imp := range file.Imports {
		if path, err := strconv.Unquote(imp.Path.Value); err == nil && path == "github.com/goccy/go-yaml" {
			yaml = "yaml"
			if imp.Name != nil {
				yaml = imp.Name.Name
			}
		}
	}
	if yaml == "" {
		return nil, nil
	}
	selector := func(e ast.Expr, pkg string) (string, bool) {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok {
			return "", false
		}
		id, ok := sel.X.(*ast.Ident)
		return sel.Sel.Name, ok && id.Name == pkg
	}
	var found []string
	var stack []ast.Node
	guarded := func() bool {
		for i, n := range stack {
			if fn, ok := n.(*ast.FuncDecl); ok && fn.Recv != nil && fn.Name.Name == "UnmarshalYAML" {
				return true
			}
			call, ok := n.(*ast.CallExpr)
			if !ok || i+1 >= len(stack) {
				continue
			}
			if fn, ok := selector(call.Fun, "decodeguard"); ok && (fn == "Run" || fn == "RunNode") {
				if _, lit := stack[i+1].(*ast.FuncLit); lit {
					return true
				}
			}
		}
		return false
	}
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if call, ok := n.(*ast.CallExpr); ok {
			if fn, ok := selector(call.Fun, yaml); ok && yamlDecoders[fn] && !guarded() {
				found = append(found, fmt.Sprintf("%s: %s.%s outside decodeguard.Run", fset.Position(call.Pos()), yaml, fn))
			}
		}
		stack = append(stack, n)
		return true
	})
	return found, nil
}

// TestYAMLDecodesAreGuarded fails when a package of the base or model level decodes
// YAML outside decodeguard.Run. The YAML library has panicked on hostile input (a
// tagged scalar read into a slice), and game data, books, scripts and catalogs are
// untrusted: a decode outside the guard lets such a file crash the host.
func TestYAMLDecodesAreGuarded(t *testing.T) {
	t.Parallel()
	for _, root := range []string{"../../base", "../../model"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			src, err := os.ReadFile(path) //nolint:gosec // A source file of this repository.
			if err != nil {
				return err
			}
			found, err := unguardedYAMLDecodes(path, src)
			for _, f := range found {
				t.Errorf("%s; wrap the decode in decodeguard.Run(src, func() error { ... })", f)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestYAMLDecodeGuardRejectsBareDecodes proves the check sees each shape of a bare
// decode and accepts the two guarded ones, so an empty result is not a blind check.
func TestYAMLDecodeGuardRejectsBareDecodes(t *testing.T) {
	t.Parallel()
	const head = "package p\nimport (\n\t\"bytes\"\n\ty \"github.com/goccy/go-yaml\"\n\t\"example/decodeguard\"\n)\nvar _ = bytes.NewReader\n"
	for name, c := range map[string]struct {
		body string
		want int
	}{
		"bare":            {"func f(b []byte, v any) error { return y.Unmarshal(b, v) }", 1},
		"bare options":    {"func f(b []byte, v any) error { return y.UnmarshalWithOptions(b, v, y.Strict()) }", 1},
		"bare decoder":    {"func f(b []byte, v any) error { return y.NewDecoder(bytes.NewReader(b)).Decode(v) }", 1},
		"beside a guard":  {"func f(b []byte, v any) error { _ = decodeguard.Run(func() error { return nil }); return y.Unmarshal(b, v) }", 1},
		"argument of Run": {"func g() error { return nil }\nfunc f(b []byte, v any) error { return decodeguard.Run(g) }\nfunc h(b []byte, v any) error { return y.Unmarshal(b, v) }", 1},
		"guarded":         {"func f(b []byte, v any) error { return decodeguard.Run(func() error { return y.Unmarshal(b, v) }) }", 0},
		"guarded decoder": {"func f(b []byte, v any) error { return decodeguard.Run(func() error { return y.NewDecoder(bytes.NewReader(b)).Decode(v) }) }", 0},
		"unmarshaler":     {"type T struct{}\nfunc (t *T) UnmarshalYAML(b []byte) error { return y.Unmarshal(b, t) }", 0},
		"encoding":        {"func f(v any) ([]byte, error) { return y.Marshal(v) }", 0},
	} {
		found, err := unguardedYAMLDecodes(name+".go", []byte(head+c.body))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(found) != c.want {
			t.Errorf("%s: found %v, want %d", name, found, c.want)
		}
	}
}
