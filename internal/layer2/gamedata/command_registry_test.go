package gamedata

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strconv"
	"testing"
)

// タグの未登録・存在しない許可先と、実行／意味検査の分岐の追加漏れを検出する。
// 文書との照合はリポジトリ側の同名テストで行い、エンジン単独の検査は文書に依存しない。
func TestCommandRegistryCoverage(t *testing.T) {
	verbs := map[string]bool{}
	for _, f := range commandFields {
		if f.role == "verb" || f.role == "internal" {
			verbs[f.name] = true
		}
	}
	for f := range reflect.TypeFor[Command]().Fields() {
		if !f.IsExported() {
			continue
		}
		if f.Tag.Get("command") == "" {
			t.Errorf("%s に命令契約が無い", f.Name)
		}
		if (f.Tag.Get("command") == "internal") != (f.Tag.Get("yaml") == "-") {
			t.Errorf("%s の内部命令と YAML の公開区分が一致しない", f.Name)
		}
	}
	for _, f := range commandFields {
		if f.role == "verb" || f.role == "internal" {
			continue
		}
		for _, owner := range f.owners {
			if !verbs[owner] {
				t.Errorf("%s の許可先 %s が未登録", f.name, owner)
			}
		}
	}
	for _, target := range []struct{ path, function string }{
		{"check.go", "commands"}, {"../../../event/event.go", "run"},
	} {
		t.Run(target.function, func(t *testing.T) {
			cases := commandCases(t, target.path, target.function)
			for name := range verbs {
				if !cases[name] {
					t.Errorf("%s に %s の処理が無い", target.function, name)
				}
			}
			for name := range cases {
				if !verbs[name] {
					t.Errorf("%s の %s は未登録", target.function, name)
				}
			}
		})
	}

	t.Run("スキーマ", func(t *testing.T) {
		raw, err := os.ReadFile("../../../data/schema/types.schema.json")
		if err != nil {
			t.Fatal(err)
		}
		var document struct {
			Defs map[string]struct {
				OneOf []struct {
					Required             []string
					Properties           map[string]json.RawMessage
					AdditionalProperties bool
				} `json:"oneOf"`
				Properties map[string]json.RawMessage
			} `json:"$defs"`
		}
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		branches := document.Defs["Command"].OneOf
		public := 0
		for _, field := range commandFields {
			if field.role == "verb" {
				public++
			}
		}
		if len(branches) != public {
			t.Fatalf("動詞の数が違う 登録%d スキーマ%d", public, len(branches))
		}
		seen := map[string]bool{}
		for _, branch := range branches {
			if len(branch.Required) != 1 || branch.AdditionalProperties {
				t.Fatal("命令の動詞・未知項目の制約がない")
			}
			verb := branch.Required[0]
			if !verbs[verb] || verb == "done" || seen[verb] {
				t.Fatalf("公開しない動詞か重複 %s", verb)
			}
			seen[verb] = true
			for _, field := range commandFields {
				_, got := branch.Properties[field.name]
				want := field.name == verb || (field.role != "verb" && field.role != "internal" && slices.Contains(field.owners, verb))
				if got != want {
					t.Errorf("%s の %s の配置が違う", verb, field.name)
				}
			}
		}
		if _, ok := document.Defs["Command"].Properties["done"]; ok {
			t.Fatal("内部命令を公開した")
		}
	})
	// 付随項目を登録するだけで実行・検査から使わない漏れも防ぐ。
	runtimeFields := commandSelectors(t, "../../../event/event.go", "")
	for _, path := range []string{"../../../event/stones.go", "../../../event/sp.go"} {
		for name := range commandSelectors(t, path, "") {
			runtimeFields[name] = true
		}
	}
	checkFields := commandSelectors(t, "check.go", "commands")
	for _, f := range commandFields {
		if f.role == "verb" || f.role == "internal" {
			continue
		}
		field := reflect.TypeFor[Command]().Field(f.index)
		if !runtimeFields[field.Name] {
			t.Errorf("付随項目 %s が Runner で使われていない", f.name)
		}
		// 真偽値は YAML の型検査だけで足りる。参照・式・子命令は意味検査が必要。
		if field.Type != reflect.TypeFor[*bool]() && !checkFields[field.Name] {
			t.Errorf("付随項目 %s が意味検査で使われていない", f.name)
		}
	}

	// 補助の実行器へ渡した動詞も、そこで処理されることを確かめる。
	for _, target := range []struct{ path, function string }{
		{"../../../event/stones.go", "stones"}, {"../../../event/sp.go", "sp"},
	} {
		cases := commandCases(t, target.path, target.function)
		for name := range cases {
			if !verbs[name] {
				t.Errorf("%s の %s は未登録", target.function, name)
			}
		}
		file, err := parser.ParseFile(token.NewFileSet(), "../../../event/event.go", nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			clause, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			delegates := false
			for _, stmt := range clause.Body {
				ast.Inspect(stmt, func(n ast.Node) bool {
					if call, ok := n.(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == target.function {
							delegates = true
						}
					}
					return true
				})
			}
			if delegates {
				for _, expr := range clause.List {
					lit, ok := expr.(*ast.BasicLit)
					if !ok {
						t.Fatal("動詞の分岐が文字列でない")
					}
					name, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatal(err)
					}
					if !cases[name] {
						t.Errorf("%s に委譲された %s の処理が無い", target.function, name)
					}
				}
			}
			return true
		})
	}
	if !slices.ContainsFunc(commandFields, func(f commandField) bool { return f.name == "done" && f.role == "internal" }) {
		t.Fatal("done の内部命令区分が失われた")
	}
}

func commandCases(t *testing.T, path, function string) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != function {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			id, ok := sw.Tag.(*ast.Ident)
			if !ok || id.Name != "verb" {
				return true
			}
			for _, stmt := range sw.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if !ok {
					t.Fatal("expected case clause")
				}
				for _, expr := range clause.List {
					lit, ok := expr.(*ast.BasicLit)
					if !ok {
						t.Fatal("動詞の分岐が文字列でない")
					}
					name, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatal(err)
					}
					cases[name] = true
				}
			}
			return false
		})
	}
	if len(cases) == 0 {
		t.Fatalf("%s の %s に動詞の switch が無い", path, function)
	}
	return cases
}

func commandSelectors(t *testing.T, path, function string) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || (function != "" && fn.Name.Name != function) {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				fields[sel.Sel.Name] = true
			}
			return true
		})
	}
	return fields
}
