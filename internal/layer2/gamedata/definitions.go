package gamedata

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"

	"github.com/nao1215/gozen/internal/layer1/decodeguard"
)

// definitionShape は型への変換で失われる null を先に調べる。
// セクションを列挙せず YAML の型をたどるので、定義の追加時にも同じ検査が働く。
func (doc *sourceDocument) definitionShape(raw any, typ reflect.Type, where string) []error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	bad := func(want string) []error {
		return []error{doc.diagnostic(where, fmt.Sprintf("expected %s, got %T", want, raw))}
	}
	// 式の nil と任意の数値・真偽値の null は従来の解釈を保つ。
	if typ == reflect.TypeFor[Expr]() || typ == reflect.TypeFor[Color]() {
		if _, ok := raw.(yaml.MapSlice); ok {
			return bad("a scalar")
		}
		if _, ok := raw.([]any); ok {
			return bad("a scalar")
		}
		return nil
	}
	if typ == reflect.TypeFor[AIRef]() {
		if _, ok := raw.(string); ok {
			return nil
		}
		return doc.definitionShape(raw, reflect.TypeFor[[]Rule](), where)
	}
	switch typ {
	case reflect.TypeFor[Picture](), reflect.TypeFor[CG](), reflect.TypeFor[Say](), reflect.TypeFor[MusicCue](), reflect.TypeFor[Target]():
		if _, ok := raw.(string); ok {
			return nil
		}
	}
	var errs []error
	switch typ.Kind() { //nolint:exhaustive // Only schema container/scalar kinds need handling; other reflection kinds are intentionally ignored.
	case reflect.Struct:
		m, ok := raw.(yaml.MapSlice)
		if !ok {
			return bad("a definition map (not null)")
		}
		fields := definitionFields(typ)
		for _, it := range m {
			key := fmt.Sprint(it.Key)
			// この段で契約を定めた数値は、明示した null を省略として補完しない。
			if it.Value == nil && ((typ == reflect.TypeFor[BattleCfg]() && (key == "variance" || key == "critical_rate" || key == "guard_rate" || key == "front_weight" || key == "back_weight")) || (typ == reflect.TypeFor[Music]() && key == "volume")) {
				errs = append(errs, doc.diagnostic(numberPath(where, key), "expected a number, got null"))
				continue
			}
			if typ == reflect.TypeFor[StateNames]() && key == "strict" {
				if _, ok := it.Value.(bool); !ok {
					errs = append(errs, doc.diagnostic(numberPath(where, key), "expected a boolean"))
					continue
				}
			}
			// 作戦の新設項目は旧仕様でも暗黙変換せず、数値と真偽値の誤記を診断する。
			if typ == reflect.TypeFor[TacticCondition]() {
				if key == "weight" || key == "priority" {
					if !numericScalar(it.Value, fields[key]) {
						errs = append(errs, doc.diagnostic(numberPath(where, key), "expected a number"))
						continue
					}
				}
				if key == "once" {
					if _, ok := it.Value.(bool); !ok {
						errs = append(errs, doc.diagnostic(numberPath(where, key), "expected a boolean"))
						continue
					}
				}
			}
			if typ == reflect.TypeFor[TacticRow]() {
				if key == "enabled" {
					if _, ok := it.Value.(bool); !ok {
						errs = append(errs, doc.diagnostic(numberPath(where, key), "expected a boolean"))
						continue
					}
				}
				if key == "percent" && !numericScalar(it.Value, reflect.TypeFor[int]()) {
					errs = append(errs, doc.diagnostic(numberPath(where, key), "expected an integer"))
					continue
				}
			}
			if field, ok := fields[key]; ok {
				errs = append(errs, doc.definitionShape(it.Value, field, numberPath(where, key))...)
			}
		}
	case reflect.Map:
		if raw == nil { // 定義の一覧そのものが空なら、追加する ID は無い。
			return nil
		}
		m, ok := raw.(yaml.MapSlice)
		if !ok {
			return bad("a map")
		}
		for _, it := range m {
			errs = append(errs, doc.definitionShape(it.Value, typ.Elem(), numberPath(where, fmt.Sprint(it.Key)))...)
		}
	case reflect.Slice, reflect.Array:
		if raw == nil { // 省略と同じ空のリストは許す。リスト内の空の定義とは区別する。
			return nil
		}
		list, ok := raw.([]any)
		if !ok {
			return bad("a list")
		}
		for i, entry := range list {
			errs = append(errs, doc.definitionShape(entry, typ.Elem(), fmt.Sprintf("%s[%d]", where, i))...)
		}
	default:
		if _, ok := raw.(yaml.MapSlice); ok {
			return bad("a scalar")
		}
		if _, ok := raw.([]any); ok {
			return bad("a scalar")
		}
	}
	return errs
}

// definitionFields は inline を含む YAML 項目と型の対応。
func definitionFields(typ reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	for field := range typ.Fields() {
		tag, ok := field.Tag.Lookup("yaml")
		if !ok || tag == "-" {
			continue
		}
		name, options, _ := strings.Cut(tag, ",")
		if options == "inline" {
			for k, v := range definitionFields(field.Type) {
				fields[k] = v
			}
		} else {
			fields[name] = field.Type
		}
	}
	return fields
}

// decodeDefinitions は解析したノードで形と型を確かめ、同じ出典へ診断を結び付ける。
func (doc *sourceDocument) decodeDefinitions(into any) (yaml.MapSlice, error) {
	typ := reflect.TypeOf(into).Elem()
	var raw any
	if doc.root == nil {
		if typ == reflect.TypeFor[file]() {
			return nil, nil
		}
	} else if err := decodeguard.Run(func() error { return yaml.NodeToValue(doc.root, &raw, yaml.UseOrderedMap()) }); err != nil {
		return nil, doc.yamlError(err)
	}
	where := ""
	if doc.name == "game.yaml" {
		where = "game.yaml"
	}
	if errs := doc.definitionShape(raw, typ, where); len(errs) > 0 {
		return nil, joinDiagnostics(errs)
	}
	if err := decodeguard.Run(func() error { return yaml.NodeToValue(doc.root, into, yaml.Strict()) }); err != nil {
		return nil, doc.definitionDecodeError(doc.root, typ, where, err)
	}
	// 位置だけを残し、ゲームの実行中に解析木を抱え続けない。
	doc.root, doc.tokenPaths = nil, nil
	mapping, ok := raw.(yaml.MapSlice)
	if !ok {
		return nil, fmt.Errorf("definition root must be a mapping")
	}
	return mapping, nil
}

// definitionDecodeError はトークンを持たない独自デコーダのエラーだけ、元の子ノードを型に対応させる。
// 再び YAML 文字列へ変換しないので、ブロック・引用・入れ子でも元位置を保つ。
func (doc *sourceDocument) definitionDecodeError(node ast.Node, typ reflect.Type, where string, err error) error {
	var ye yaml.Error
	if errors.As(err, &ye) {
		return doc.yamlError(err)
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	child := func(n ast.Node, t reflect.Type, w string) error {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if e := decodeguard.Run(func() error { return yaml.NodeToValue(n, reflect.New(t).Interface(), yaml.Strict()) }); e != nil && e.Error() == err.Error() {
			return doc.definitionDecodeError(n, t, w, e)
		}
		return nil
	}
	switch n := node.(type) {
	case *ast.MappingNode:
		fields := map[string]reflect.Type{}
		if typ.Kind() == reflect.Struct {
			fields = definitionFields(typ)
		}
		for _, entry := range n.Values {
			key := sourceKey(entry.Key)
			t, ok := fields[key]
			if typ.Kind() == reflect.Map {
				t, ok = typ.Elem(), true
			}
			if ok {
				if e := child(entry.Value, t, numberPath(where, key)); e != nil {
					return e
				}
			}
		}
	case *ast.SequenceNode:
		if typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
			for i, entry := range n.Values {
				if e := child(entry, typ.Elem(), fmt.Sprintf("%s[%d]", where, i)); e != nil {
					return e
				}
			}
		}
	}
	return &diagnostic{nodePosition(doc.name, node.GetToken()), where, err.Error()}
}
