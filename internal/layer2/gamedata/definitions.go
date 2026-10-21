package gamedata

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"sync"

	"github.com/nao1215/gozen/internal/diag"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"

	"github.com/nao1215/gozen/internal/layer1/decodeguard"
)

// definitionShape checks first for nulls that would be lost in conversion to the type.
// It follows the YAML types without listing sections, so the same check applies when definitions are added.
func (doc *sourceDocument) definitionShape(raw any, typ reflect.Type, where string) []error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	bad := func(want string) []error {
		return []error{coded(doc.diagnostic(where, fmt.Sprintf("expected %s, got %T", want, raw)), diag.Shape)}
	}
	// A nil expression and a null for an arbitrary number or boolean keep the previous interpretation.
	if typ == reflect.TypeFor[Expr]() || typ == reflect.TypeFor[Color]() {
		if _, ok := raw.(yaml.MapSlice); ok {
			return bad("a scalar")
		}
		if _, ok := raw.([]any); ok {
			return bad("a scalar")
		}
		return nil
	}
	if typ == reflect.TypeFor[Assignments]() { // a left side that takes only strings may also be written as a list (party: [a, b])
		if m, ok := raw.(yaml.MapSlice); ok {
			var errs []error
			for _, it := range m {
				if key := fmt.Sprint(it.Key); !LiteralSet(key) {
					errs = append(errs, doc.definitionShape(it.Value, reflect.TypeFor[Expr](), numberPath(where, key))...)
				}
			}
			return errs
		}
	}
	if typ == reflect.TypeFor[AIRef]() {
		if _, ok := raw.(string); ok {
			return nil
		}
		return doc.definitionShape(raw, reflect.TypeFor[[]Rule](), where)
	}
	switch typ {
	case reflect.TypeFor[Picture](), reflect.TypeFor[CG](), reflect.TypeFor[Say](), reflect.TypeFor[MusicCue](), reflect.TypeFor[Target](), reflect.TypeFor[Bubble](), reflect.TypeFor[Token](),
		reflect.TypeFor[Stand](), reflect.TypeFor[EffectCue](), reflect.TypeFor[SFXCue]():
		if _, ok := raw.(string); ok {
			return nil
		}
	case reflect.TypeFor[WaitCue](): // seconds, or a map (schema_version 4)
		switch raw.(type) {
		case uint64, int64, int, float64:
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
			// A number whose contract is fixed at this stage is not filled in as omitted when an explicit null is given.
			if it.Value == nil && ((typ == reflect.TypeFor[BattleCfg]() && (key == "variance" || key == "critical_rate" || key == "guard_rate" || key == "front_weight" || key == "back_weight")) || (typ == reflect.TypeFor[Music]() && key == "volume")) {
				errs = append(errs, coded(doc.diagnostic(numberPath(where, key), "expected a number, got null"), diag.Shape))
				continue
			}
			if typ == reflect.TypeFor[StateNames]() && key == "strict" {
				if _, ok := it.Value.(bool); !ok {
					errs = append(errs, coded(doc.diagnostic(numberPath(where, key), "expected a boolean"), diag.Shape))
					continue
				}
			}
			// Newly added tactics items are not implicitly converted even under the old spec; typos in numbers and booleans are diagnosed.
			if typ == reflect.TypeFor[TacticCondition]() {
				if key == "weight" || key == "priority" {
					if !numericScalar(it.Value, fields[key]) {
						errs = append(errs, coded(doc.diagnostic(numberPath(where, key), "expected a number"), diag.Shape))
						continue
					}
				}
				if key == "once" {
					if _, ok := it.Value.(bool); !ok {
						errs = append(errs, coded(doc.diagnostic(numberPath(where, key), "expected a boolean"), diag.Shape))
						continue
					}
				}
			}
			if typ == reflect.TypeFor[TacticRow]() {
				if key == "enabled" {
					if _, ok := it.Value.(bool); !ok {
						errs = append(errs, coded(doc.diagnostic(numberPath(where, key), "expected a boolean"), diag.Shape))
						continue
					}
				}
				if key == "percent" && !numericScalar(it.Value, reflect.TypeFor[int]()) {
					errs = append(errs, coded(doc.diagnostic(numberPath(where, key), "expected an integer"), diag.Shape))
					continue
				}
			}
			if field, ok := fields[key]; ok {
				errs = append(errs, doc.definitionShape(it.Value, field, numberPath(where, key))...)
			}
		}
	case reflect.Map:
		if raw == nil { // if the definition list itself is empty, there is no ID to add
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
		if raw == nil { // an empty list is allowed, same as omitted; distinct from an empty definition inside the list
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

// definitionFieldsOf keeps definitionFields of each type: the shape check asks for the
// fields of every definition it visits, thousands of times in a game.
var definitionFieldsOf sync.Map // reflect.Type -> map[string]reflect.Type

// definitionFields maps YAML fields, including inline ones, to types. The map is shared:
// the callers only read it.
func definitionFields(typ reflect.Type) map[string]reflect.Type {
	if v, ok := definitionFieldsOf.Load(typ); ok {
		if fields, ok := v.(map[string]reflect.Type); ok {
			return fields
		}
	}
	fields := collectDefinitionFields(typ)
	definitionFieldsOf.Store(typ, fields)
	return fields
}

func collectDefinitionFields(typ reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	for field := range typ.Fields() {
		tag, ok := field.Tag.Lookup("yaml")
		if !ok || tag == "-" {
			continue
		}
		name, options, _ := strings.Cut(tag, ",")
		if options == "inline" {
			maps.Copy(fields, collectDefinitionFields(field.Type))
		} else {
			fields[name] = field.Type
		}
	}
	return fields
}

// resolveMerges writes out each YAML merge of one mapping (<<: *base, or <<: {...}) beside
// the keys of its own mapping, so a key written there overrides the merged one wherever it
// is written, as YAML means. The decoder would take the override for a key written twice
// (or let the merge win when it comes after). A merge of a sequence of mappings is left to
// the decoder. anchors holds the mappings of the anchors seen so far, in the order of the
// document, already resolved.
func resolveMerges(n ast.Node, anchors map[string]ast.Node) {
	switch x := n.(type) {
	case *ast.AnchorNode:
		resolveMerges(x.Value, anchors)
		anchors[x.Name.GetToken().Value] = x.Value
	case *ast.TagNode:
		resolveMerges(x.Value, anchors)
	case *ast.SequenceNode:
		for _, v := range x.Values {
			resolveMerges(v, anchors)
		}
	case *ast.MappingValueNode:
		resolveMerges(x.Value, anchors)
	case *ast.MappingNode:
		for _, e := range x.Values {
			resolveMerges(e.Value, anchors)
		}
		mergeInto(x, anchors)
	}
}

// mergeInto replaces the merges of one mapping by the entries they bring that the mapping
// does not write itself.
func mergeInto(m *ast.MappingNode, anchors map[string]ast.Node) {
	source := func(v ast.Node) *ast.MappingNode {
		for {
			switch x := v.(type) {
			case *ast.AliasNode:
				v = anchors[x.Value.GetToken().Value]
			case *ast.AnchorNode:
				v = x.Value
			case *ast.TagNode:
				v = x.Value
			case *ast.MappingNode:
				return x
			case *ast.MappingValueNode:
				return &ast.MappingNode{Values: []*ast.MappingValueNode{x}}
			default:
				return nil
			}
		}
	}
	written := map[string]bool{}
	merges := false
	for _, e := range m.Values {
		if k := sourceKey(e.Key); k != "<<" {
			written[k] = true
		} else if source(e.Value) != nil {
			merges = true
		}
	}
	if !merges {
		return
	}
	out := make([]*ast.MappingValueNode, 0, len(m.Values))
	for _, e := range m.Values {
		src := source(e.Value)
		if sourceKey(e.Key) != "<<" || src == nil {
			out = append(out, e)
			continue
		}
		for _, me := range src.Values {
			if k := sourceKey(me.Key); !written[k] {
				written[k] = true
				out = append(out, me)
			}
		}
	}
	m.Values = out
}

// decodeDefinitions checks shape and types on the parsed node and ties diagnostics to the same source.
func (doc *sourceDocument) decodeDefinitions(into any) (yaml.MapSlice, error) {
	typ := reflect.TypeOf(into).Elem()
	var raw any
	if doc.root == nil {
		if typ == reflect.TypeFor[file]() {
			return nil, nil
		}
	} else {
		resolveMerges(doc.root, map[string]ast.Node{})
		if err := decodeguard.Run(func() error { return yaml.NodeToValue(doc.root, &raw, yaml.UseOrderedMap()) }); err != nil {
			return nil, doc.yamlError(err)
		}
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
	// Keep only positions, so the parse tree is not held while the game runs.
	doc.root, doc.tokenPaths = nil, nil
	mapping, ok := raw.(yaml.MapSlice)
	if !ok {
		return nil, errors.New("definition root must be a mapping")
	}
	return mapping, nil
}

// definitionDecodeError maps the original child node to a type only for errors from custom decoders that carry no token.
// It does not convert back to a YAML string, so original positions are kept even for blocks, quotes and nesting.
func (doc *sourceDocument) definitionDecodeError(node ast.Node, typ reflect.Type, where string, err error) error {
	if _, ok := errors.AsType[yaml.Error](err); ok {
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
	return &diagnostic{nodePosition(doc.name, node.GetToken()), where, err.Error(), diag.Shape}
}
