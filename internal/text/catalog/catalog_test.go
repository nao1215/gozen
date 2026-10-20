package catalog

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// indirect lists the calls of T whose key is not a string literal, by file and argument,
// with the keys they can receive. Each key must still be written in that file.
var indirect = map[string]struct {
	args []string
	keys []string
}{
	"internal/event/stones.go":          {[]string{"data.StatLabels[st]"}, nil},
	"internal/play/menu.go":             {[]string{"data.StatLabels[st]"}, nil},
	"internal/play/shop.go":             {[]string{"data.StatLabels[d.stat]"}, nil},
	"cmd/gozen/memos.go":                {[]string{"n"}, []string{"人", "LLM", "自動の遊び", "ボット"}},
	"internal/play/novel.go":            {[]string{"novelButtons[i].label"}, []string{"ログ", "オート", "非表示", "設定"}},
	"internal/layer2/gamedata/load.go":  {nil, []string{"武器", "防具", "装飾品"}},
	"internal/layer2/gamedata/types.go": {nil, []string{"最大HP", "最大MP", "攻撃力", "防御力", "魔法力", "魔法防御", "素早さ", "運"}},
}

// sourceKeys reads every key the module's code passes to T.
func sourceKeys(t *testing.T) map[string]string {
	t.Helper()
	root := "../../.."
	keys := map[string]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "website", "build", "bin", "node_modules", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		if in, ok := indirect[rel]; ok && in.args == nil {
			for _, k := range in.keys {
				keys[k] = rel
			}
		}
		name := ""
		for _, im := range f.Imports {
			if im.Path.Value == `"github.com/nao1215/gozen/internal/text"` {
				name = "text"
				if im.Name != nil {
					name = im.Name.Name
				}
			}
		}
		if name == "" {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "T" {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); !ok || id.Name != name {
				return true
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				k, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("%s: %v", fset.Position(lit.Pos()), err)
				}
				keys[k] = rel
				return true
			}
			var b strings.Builder
			if err := printer.Fprint(&b, fset, call.Args[0]); err != nil {
				t.Fatal(err)
			}
			in := indirect[rel]
			if !slices.Contains(in.args, b.String()) {
				t.Errorf("%s: T(%s) is not a string literal; add it and its keys to indirect", fset.Position(call.Pos()), b.String())
			}
			for _, k := range in.keys {
				keys[k] = rel
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

// Every key the code shows has an English word, and every English word has a key in the code.
func TestEnglishHasEveryKey(t *testing.T) {
	keys := sourceKeys(t)
	if len(keys) < 500 {
		t.Fatalf("found only %d keys; is the source walk broken?", len(keys))
	}
	var missing, stale []string
	for k, at := range keys {
		if _, ok := english[k]; !ok {
			missing = append(missing, strconv.Quote(k)+" ("+at+")")
		}
	}
	for k := range english {
		if _, ok := keys[k]; !ok {
			stale = append(stale, strconv.Quote(k))
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("%d keys have no English word:\n%s", len(missing), strings.Join(missing, "\n"))
	}
	if len(stale) > 0 {
		t.Errorf("%d English words have no key in the code:\n%s", len(stale), strings.Join(stale, "\n"))
	}
	ja, err := Words(Japanese)
	if err != nil {
		t.Fatal(err)
	}
	for k := range keys {
		if ja[k] != k {
			t.Errorf("Japanese lacks %q", k)
		}
	}
}

var (
	verb  = regexp.MustCompile(`%[-+# 0]*(?:\[(\d+)\])?(?:\d+|\*)?(?:\.(?:\d+|\*)?)?(?:\[(\d+)\])?([a-zA-Z%])`)
	brace = regexp.MustCompile(`\{[a-z_]+\}`)
	japan = regexp.MustCompile(`[\p{Hiragana}\p{Katakana}\p{Han}]`)
)

// args is the verb each argument of format s takes, by its position from 1.
func args(t *testing.T, s string) map[int]byte {
	t.Helper()
	out := map[int]byte{}
	n := 0
	for _, m := range verb.FindAllStringSubmatch(s, -1) {
		if m[3] == "%" {
			continue
		}
		for _, ix := range []string{m[1], m[2]} {
			if ix != "" {
				i, err := strconv.Atoi(ix)
				if err != nil {
					t.Fatalf("%q: argument index %q: %v", s, ix, err)
				}
				n = i - 1
			}
		}
		n++
		out[n] = m[3][0]
	}
	return out
}

// The English words take the same arguments with the same verbs and the same {names},
// and are written without Japanese.
func TestEnglishFormatsMatch(t *testing.T) {
	for k, w := range english {
		if w == "" && k != "" {
			t.Errorf("%q has an empty English word", k)
			continue
		}
		ka, wa := args(t, k), args(t, w)
		for i, v := range ka {
			if wa[i] != v {
				t.Errorf("%q -> %q: argument %d takes %%%c, the English %%%c", k, w, i, v, wa[i])
			}
		}
		for i := range wa {
			if _, ok := ka[i]; !ok {
				t.Errorf("%q -> %q: the English reads argument %d, which the key does not have", k, w, i)
			}
		}
		kb, wb := brace.FindAllString(k, -1), brace.FindAllString(w, -1)
		sort.Strings(kb)
		sort.Strings(wb)
		if !slices.Equal(slices.Compact(kb), slices.Compact(wb)) {
			t.Errorf("%q -> %q: names %v, the English %v", k, w, kb, wb)
		}
		if japan.MatchString(w) || strings.ContainsAny(w, "　「」『』（）、。！？：・～") {
			t.Errorf("%q -> %q: the English has Japanese characters", k, w)
		}
	}
}

func TestWordsIsACopy(t *testing.T) {
	en, err := Words(English)
	if err != nil {
		t.Fatal(err)
	}
	en["はい"] = "changed"
	if english["はい"] == "changed" {
		t.Fatal("Words gave out the built-in map")
	}
	ja, err := Words(Japanese)
	if err != nil {
		t.Fatal(err)
	}
	if len(ja) != len(english) || ja["はい"] != "はい" {
		t.Fatalf("Japanese words: %d keys, はい=%q", len(ja), ja["はい"])
	}
	if _, err := Words("xx"); err == nil {
		t.Fatal("an unknown language was accepted")
	}
}
