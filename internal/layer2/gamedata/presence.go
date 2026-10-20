package gamedata

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/nao1215/gozen/internal/diag"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/layer1/optional"
)

// numericContract is the default and range of numbers in spec version 1. The type comes from the declared field.
// positive marks items that do not allow 0. Completion for the old spec stays in the existing defaults and on the user side.
type numericContract struct {
	fallback, lower, upper float64
	positive               bool
}

func nonnegative(fallback float64) numericContract {
	return numericContract{fallback: fallback, upper: math.Inf(1)}
}

func positive(fallback float64) numericContract {
	r := nonnegative(fallback)
	r.positive = true
	return r
}

func fraction(fallback float64) numericContract {
	return numericContract{fallback: fallback, upper: 1}
}

// The meaning of each item and the regression tests are in doc/numeric-presence.md.
var numericContracts = map[reflect.Type]map[string]numericContract{
	reflect.TypeFor[Game]():         {"max_party": positive(4)},
	reflect.TypeFor[Screen]():       {"width": positive(1280), "height": positive(720)},
	reflect.TypeFor[Class]():        {"sp": nonnegative(1), "start_sp": nonnegative(0), "max_level": positive(99)},
	reflect.TypeFor[TreeNode]():     {"max": positive(10)},
	reflect.TypeFor[Require]():      {"level": positive(1)},
	reflect.TypeFor[Learning]():     {"level": positive(1)},
	reflect.TypeFor[Actor]():        {"level": positive(1), "face_shade": fraction(0), "face_shade_top": fraction(0)},
	reflect.TypeFor[Enemy]():        {"level": positive(1), "scale": positive(1), "actions": nonnegative(1)},
	reflect.TypeFor[Usage]():        {"hits": positive(1)},
	reflect.TypeFor[ItemCount]():    {"count": nonnegative(1)},
	reflect.TypeFor[Affect]():       {"turns": nonnegative(0)},
	reflect.TypeFor[Command]():      {"wait": nonnegative(0)},
	reflect.TypeFor[State]():        {"turns": nonnegative(0), "damage_rate": nonnegative(1), "restrict_chance": fraction(0)},
	reflect.TypeFor[Effect]():       {"summon_max": positive(5)},
	reflect.TypeFor[BattleCfg]():    {"tempo": positive(1), "auto_tempo": {fallback: 1, lower: 1, upper: math.Inf(1)}},
	reflect.TypeFor[Picture]():      {"brightness": nonnegative(1)},
	reflect.TypeFor[CG]():           {"brightness": nonnegative(1), "fade": nonnegative(0.25), "over_size": positive(0.55), "auto": nonnegative(0), "page": nonnegative(0)},
	reflect.TypeFor[Dungeon]():      {"depth": positive(5), "darkness": fraction(0.5)},
	reflect.TypeFor[DungeonLook]():  {"darkness": fraction(0)},
	reflect.TypeFor[Generate]():     {"width": {lower: 9, upper: math.Inf(1)}, "height": {lower: 9, upper: math.Inf(1)}, "rooms": nonnegative(0), "loops": fraction(0.08)},
	reflect.TypeFor[Tile]():         {"size": positive(0.6)},
	reflect.TypeFor[Roamer]():       {"size": positive(0.8), "sight": positive(4), "far": nonnegative(0)},
	reflect.TypeFor[Rule]():         {"weight": positive(1)},
	reflect.TypeFor[TroopPick]():    {"weight": positive(1)},
	reflect.TypeFor[UIStyle]():      {"backlog": nonnegative(300)},
	reflect.TypeFor[Meter]():        {"max": positive(100)},
	reflect.TypeFor[TitleFigure]():  {"bottom": nonnegative(1), "height": positive(0.95)},
	reflect.TypeFor[CardBubble]():   {"count": nonnegative(4), "size": positive(3), "speed": positive(30), "rise": positive(50)},
	reflect.TypeFor[Music]():        {"bars": positive(8), "beats_per_bar": positive(4)},
	reflect.TypeFor[BattleFx]():     {"shake": nonnegative(1), "flash": nonnegative(1), "pop": nonnegative(1), "hitstop": nonnegative(1), "charge": nonnegative(1), "finish": nonnegative(1)},
	reflect.TypeFor[BattleFxAuto](): {"hitstop": nonnegative(0), "charge": nonnegative(0), "finish": nonnegative(0), "win": nonnegative(0.4)},
	reflect.TypeFor[SfxVary]():      {"pitch": fraction(0.03), "volume": fraction(0.1)},
}

// walkNumberDefinitions follows the YAML tags. It also fills in omitted structs and does not create nil optional definitions.
// Map values are written back, so definitions inside a non-pointer FloorPlan and the like are not missed.
func walkNumberDefinitions(v reflect.Value, raw any, where string, visit func(reflect.Value, yaml.MapSlice, string)) {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Type() == reflect.TypeFor[Expr]() || v.Type() == reflect.TypeFor[Color]() {
		return
	}
	if v.Type() == reflect.TypeFor[AIRef]() {
		walkNumberDefinitions(v.FieldByName("Rules"), raw, where, visit)
		return
	}
	switch v.Kind() { //nolint:exhaustive // Only schema container/scalar kinds need handling; other reflection kinds are intentionally ignored.
	case reflect.Struct:
		m := optional.Value[yaml.MapSlice](raw) // nil when the YAML is not a mapping
		visit(v, m, where)
		for _, field := range yamlFields(v.Type()) {
			if field.inline {
				walkNumberDefinitions(v.FieldByIndex(field.index), raw, where, visit)
			} else {
				entry, _ := numberEntry(m, field.name)
				walkNumberDefinitions(v.FieldByIndex(field.index), entry, numberPath(where, field.name), visit)
			}
		}
	case reflect.Map:
		m := optional.Value[yaml.MapSlice](raw) // nil when the YAML is not a mapping
		keys := v.MapKeys()
		slices.SortFunc(keys, func(a, b reflect.Value) int {
			return strings.Compare(fmt.Sprint(a.Interface()), fmt.Sprint(b.Interface()))
		})
		for _, key := range keys {
			value := reflect.New(v.Type().Elem()).Elem()
			value.Set(v.MapIndex(key))
			entry, _ := numberEntry(m, fmt.Sprint(key.Interface()))
			walkNumberDefinitions(value, entry, numberPath(where, fmt.Sprint(key.Interface())), visit)
			v.SetMapIndex(key, value)
		}
	case reflect.Slice, reflect.Array:
		list := optional.Value[[]any](raw) // nil when the YAML is not a list
		for i := range v.Len() {
			var entry any
			if i < len(list) {
				entry = list[i]
			}
			walkNumberDefinitions(v.Index(i), entry, fmt.Sprintf("%s[%d]", where, i), visit)
		}
	}
}

// yamlField is a struct field with a YAML tag (not "-"): its name in the YAML and whether
// it is inlined.
type yamlField struct {
	index  []int
	name   string
	inline bool
}

// yamlFieldsOf keeps the YAML fields of each struct type walked (the walk visits every
// definition of a game, and reading the tags again each time is most of its work).
var yamlFieldsOf sync.Map // reflect.Type -> []yamlField

func yamlFields(t reflect.Type) []yamlField {
	if v, ok := yamlFieldsOf.Load(t); ok {
		if fs, ok := v.([]yamlField); ok {
			return fs
		}
	}
	var fs []yamlField
	for field := range t.Fields() {
		tag, ok := field.Tag.Lookup("yaml")
		if !ok || tag == "-" {
			continue
		}
		name, options, _ := strings.Cut(tag, ",")
		fs = append(fs, yamlField{index: field.Index, name: name, inline: options == "inline"})
	}
	yamlFieldsOf.Store(t, fs)
	return fs
}

// contractField is a field of a type in numericContracts that has a contract.
type contractField struct {
	index []int
	name  string
	rule  numericContract
}

// contractFieldsOf keeps the fields with a contract of each type, in the type's order (nil
// for a type without contracts, which most of the walked ones are).
var contractFieldsOf sync.Map // reflect.Type -> []contractField

func contractFields(t reflect.Type) []contractField {
	if v, ok := contractFieldsOf.Load(t); ok {
		if fs, ok := v.([]contractField); ok {
			return fs
		}
	}
	var fs []contractField
	if rules := numericContracts[t]; rules != nil {
		for field := range t.Fields() {
			name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
			if rule, ok := rules[name]; ok {
				fs = append(fs, contractField{index: field.Index, name: name, rule: rule})
			}
		}
	}
	contractFieldsOf.Store(t, fs)
	return fs
}

func numberPath(where, name string) string {
	if where == "" {
		return name
	}
	return where + "." + name
}

func numberEntry(m yaml.MapSlice, name string) (any, bool) {
	for _, it := range m {
		if fmt.Sprint(it.Key) == name {
			return it.Value, true
		}
	}
	return nil, false
}

func (db *DB) resolveNumbers(raw yaml.MapSlice, into any, source string) error {
	var errs []error
	walkNumberDefinitions(reflect.ValueOf(into), raw, source, func(v reflect.Value, m yaml.MapSlice, where string) {
		if db.Game.SchemaVersion == LegacySchemaVersion {
			if v.Type() == reflect.TypeFor[ItemCount]() {
				if i, ok := reflect.TypeAssert[*ItemCount](v.Addr()); ok && i.Count != nil && *i.Count == 0 {
					i.Count = nil
				}
			}
			return
		}
		for _, field := range contractFields(v.Type()) {
			name, rule := field.name, field.rule
			value := v.FieldByIndex(field.index)
			entry, given := numberEntry(m, name)
			if given && !numericScalar(entry, value.Type()) {
				errs = append(errs, fmt.Errorf("%s.%s: expected %s number, got %T", where, name, value.Type(), entry))
				continue
			}
			if value.Kind() == reflect.Pointer {
				continue // an optional number stays nil and is passed to the user side's default or inheritance.
			}
			if !given {
				fallback := rule.fallback
				if v.Type() == reflect.TypeFor[Actor]() && name == "face_shade_top" {
					fallback = v.FieldByName("FaceShade").Float()
				}
				if value.Kind() == reflect.Int {
					value.SetInt(int64(fallback))
				} else {
					value.SetFloat(fallback)
				}
			}
		}
		if v.Type() == reflect.TypeFor[UIStyle]() {
			for _, it := range sectionMap(m, "stage_scale") {
				if !numericScalar(it.Value, reflect.TypeFor[float64]()) {
					errs = append(errs, fmt.Errorf("%s.stage_scale.%v: expected a number, got %T", where, it.Key, it.Value))
				}
			}
		}
		switch x := v.Addr().Interface().(type) {
		case *Picture:
			x.numbersResolved = true
		case *CG:
			x.numbersResolved = true
			_, x.fadeGiven = numberEntry(m, "fade")
		case *Generate:
			x.numbersResolved = true
		case *Roamer:
			_, x.farGiven = numberEntry(m, "far")
		case *Command:
			_, x.waitSet = numberEntry(m, "wait")
		}
	})
	if len(errs) > 0 {
		return db.sourceFiles[source].locateReported(joinDiagnostics(errs))
	}
	c := &checker{db: db}
	c.numericValues(into, source)
	return joinDiagnostics(c.errs)
}

// numericScalar prevents implicit conversion from quoted numbers, null and decimals to integers.
func numericScalar(raw any, typ reflect.Type) bool {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	v := reflect.ValueOf(raw)
	if !v.IsValid() {
		return false
	}
	return v.CanInt() || v.CanUint() || (typ.Kind() == reflect.Float64 && v.Kind() == reflect.Float64)
}

func (c *checker) numericValues(into any, where string) {
	if c.db.Game.SchemaVersion < SchemaVersion1 {
		return
	}
	walkNumberDefinitions(reflect.ValueOf(into), nil, where, func(v reflect.Value, _ yaml.MapSlice, w string) {
		for _, field := range contractFields(v.Type()) {
			name, rule := field.name, field.rule
			value := v.FieldByIndex(field.index)
			if value.Kind() == reflect.Pointer {
				if value.IsNil() {
					continue
				}
				value = value.Elem()
			}
			var n float64
			if value.Kind() == reflect.Int {
				n = float64(value.Int())
			} else {
				n = value.Float()
			}
			if rule.positive && n == 0 {
				c.errorf(diag.OutOfRange, w+"."+name, "want a positive number, got 0")
			} else {
				c.numberRange(w+"."+name, n, rule.lower, rule.upper)
			}
		}
		if v.Type() == reflect.TypeFor[UIStyle]() {
			if style, ok := reflect.TypeAssert[*UIStyle](v.Addr()); ok {
				for file, scale := range style.StageScale {
					if scale == 0 {
						c.errorf(diag.OutOfRange, w+".stage_scale."+file, "want a positive number, got 0")
					} else {
						c.numberRange(w+".stage_scale."+file, scale, 0, math.Inf(1))
					}
				}
			}
		}
	})
}
