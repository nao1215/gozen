package gamedata

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/expr"
	lang "github.com/nao1215/gozen/text"
)

// StateNames は任意の変数宣言と、未知名を拒否する設定。
type StateNames struct {
	Strict bool                 `yaml:"strict"`
	Names  map[string]StateName `yaml:"names"`
}

// StateName の Initial は式でなくリテラル。生成名の初期値は状態へ書かない。
type StateName struct {
	Type    string `yaml:"type"`
	Initial any    `yaml:"initial"`
}

// UnmarshalYAML は YAML の整数型だけを式と同じ float64 へ揃える。文字列・真偽は変換しない。
func (d *StateName) UnmarshalYAML(b []byte) error {
	type raw StateName
	var value raw
	if err := yaml.UnmarshalWithOptions(b, &value, yaml.Strict()); err != nil {
		return err
	}
	v := reflect.ValueOf(value.Initial)
	if v.IsValid() {
		switch v.Kind() { //nolint:exhaustive // Only schema container/scalar kinds need handling; other reflection kinds are intentionally ignored.
		case reflect.Int, reflect.Int64:
			value.Initial = float64(v.Int())
		case reflect.Uint, reflect.Uint64:
			value.Initial = float64(v.Uint())
		}
	}
	*d = StateName(value)
	return nil
}

// ValueType は宣言の型を式の型に対応させる。
func (d StateName) ValueType() expr.ValueType {
	switch d.Type {
	case "bool":
		return expr.BoolType
	case "number":
		return expr.NumberType
	case "string":
		return expr.StringType
	}
	return expr.UnknownType
}

// StateRegistry は実在する ID と命令から生成名を列挙した、独立した名前表を返す。
func (db *DB) StateRegistry() map[string]StateName {
	out, _ := db.generatedStateNames()
	maps.Copy(out, db.Game.State.Names)
	return out
}

func (db *DB) generatedStateNames() (map[string]StateName, []string) {
	out := map[string]StateName{}
	var conflicts []string
	add := func(name, typ string) {
		key := "var." + name
		if old, ok := out[key]; ok && old.Type != typ {
			conflicts = append(conflicts, key)
			typ = min(old.Type, typ)
		}
		out[key] = StateName{Type: typ}
	}
	for _, n := range []string{"floor", "dive_seed", "deepest", "gauge", "explored", "floor_left", "roamer_beaten", "roamer_fled", "roam_calm", "hunt_from", "hunt_near"} {
		add(n, "number")
	}
	for _, n := range []string{"battle_advice_reason", "battle_advice_actor"} {
		add(n, "string")
	}
	add("battle_advice_row", "number")
	for id := range db.Enemies {
		add("kill_"+id, "number")
		add("last_kill_"+id, "number")
		for el := range db.Elements {
			add("known_"+id+"_"+el, "number")
		}
	}
	for id, l := range db.Locations {
		if l.TalkOnEnter {
			add("talk_turn_"+id, "number")
		}
	}
	db.EachCommand(func(c *Command) {
		if c.SPDecline != "" {
			add(c.SPDecline+"_no", "string")
		}
		if c.StoneDecline != "" {
			add(c.StoneDecline+"_no", "string")
		}
		sp := c.SPSuggest
		stone := c.StoneSuggest
		for _, name := range []string{sp, stone} {
			if name == "" {
				continue
			}
			add(name, "number")
			add(name+"_pages", "number")
			add(name+"_no", "string")
			for i := 1; i <= min(db.Game.MaxParty, len(db.Actors)); i++ {
				add(fmt.Sprintf("%s_page%d", name, i), "string")
			}
		}
		// 選択・着用者選択も提案と同じ出力欄を使う。
		for _, name := range []string{stone, c.StoneChoose, c.StoneWearer} {
			if name == "" {
				continue
			}
			for _, suffix := range []string{"set", "name", "stones", "skill", "stats", "who", "who_name", "no"} {
				add(name+"_"+suffix, "string")
			}
			for _, suffix := range []string{"sets", "wearers"} {
				add(name+"_"+suffix, "number")
			}
		}
	})
	slices.Sort(conflicts)
	return out, slices.Compact(conflicts)
}

func validStatePath(path string) bool {
	kind, name, ok := strings.Cut(path, ".")
	return ok && (kind == "flag" || kind == "var") && varName.MatchString(name)
}

func (c *checker) stateDeclarations() {
	generated, conflicts := c.db.generatedStateNames()
	if len(conflicts) > 0 && c.db.Game.State.Strict {
		for _, name := range conflicts {
			c.errorf("game.yaml state", lang.T("生成名 %q の型が別の命令と衝突する"), name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(c.db.Game.State.Names)) {
		d := c.db.Game.State.Names[name]
		w := "game.yaml state.names." + name
		if !validStatePath(name) {
			c.errorf(w, "%s", lang.T("宣言名は flag.<名前> または var.<名前> にする"))
		}
		if _, ok := generated[name]; ok {
			c.errorf(w, lang.T("生成名 %q は宣言で上書きできない"), name)
		}
		if d.ValueType() == expr.UnknownType || (strings.HasPrefix(name, "flag.") && d.Type != "bool") {
			c.errorf(w+".type", "%s", lang.T("型は bool・number・string（flag は bool のみ）にする"))
		}
		if expr.TypeOfValue(d.Initial) != d.ValueType() || d.Initial == nil {
			c.errorf(w+".initial", lang.T("初期値は型 %s のリテラルが必要"), d.Type)
		}
	}
}

func (c *checker) stateType(kind string) *vtype {
	fields := map[string]*vtype{}
	if c.stateNames == nil {
		c.stateNames = c.db.StateRegistry()
	}
	for path, d := range c.stateNames {
		if name, ok := strings.CutPrefix(path, kind+"."); ok {
			fields[name] = scalarType(d.ValueType())
		}
	}
	return objType(kind, fields, nil)
}

func scalarType(t expr.ValueType) *vtype {
	switch t {
	case expr.UnknownType, expr.NilType, expr.InvalidType:
		return tAny
	case expr.NumberType:
		return tNum
	case expr.BoolType:
		return tBool
	case expr.StringType:
		return tStr
	}
	return tAny
}

func (c *checker) resultType(e Expr, names []string) expr.ValueType {
	return e.ResultType(func(r expr.Ref) expr.ValueType {
		t := typeOf(r.Name, names)
		if r.Name == "flag" || r.Name == "var" {
			t = c.stateType(r.Name)
		}
		for _, step := range r.Path {
			next, problem := c.step(t, step)
			if problem != "" {
				return expr.UnknownType
			}
			t = next
		}
		switch t.kind {
		case vAny, vFunc, vObj, vDyn, vList:
			return expr.UnknownType
		case vNum:
			return expr.NumberType
		case vBool:
			return expr.BoolType
		case vStr:
			return expr.StringType
		}
		return expr.UnknownType
	})
}

func (c *checker) assignments(w string, set map[string]Expr, names []string) {
	for _, k := range slices.Sorted(maps.Keys(set)) {
		e := set[k]
		if !strings.HasPrefix(k, "flag.") && !strings.HasPrefix(k, "var.") {
			c.errorf(w, "%q: only flag.<name> and var.<name> can be set", k)
		}
		c.expr(w+"."+k, e, names)
		d, known := c.db.Game.State.Names[k]
		if c.db.Game.State.Strict {
			if c.stateNames == nil {
				c.stateNames = c.db.StateRegistry()
			}
			d, known = c.stateNames[k]
			if !known {
				c.errorf(w+"."+k, lang.T("未宣言の代入先 %q"), k)
			}
		}
		if known && e.Set() {
			got := c.resultType(e, names)
			if got != d.ValueType() && (c.db.Game.State.Strict || got&expr.UnknownType == 0) {
				c.errorf(w+"."+k, lang.T("代入式 %q は型 %s と一致しない"), e.String(), d.Type)
			}
		}
	}
}

// SetWarnings は同じ set の別の代入先を読む右辺を、元位置付きで報告する。
// 自分自身の加算は順序に依存しないので含めない。
func (db *DB) SetWarnings() []error {
	c := &checker{db: db}
	check := func(w string, set map[string]Expr) {
		for _, k := range slices.Sorted(maps.Keys(set)) {
			seen := map[string]bool{}
			for _, ref := range set[k].Refs() {
				for other := range set {
					kind, name, _ := strings.Cut(other, ".")
					if other == k || seen[other] || ref.Name != kind {
						continue
					}
					if len(ref.Path) == 0 || ref.Path[0].Name == "" || ref.Path[0].Name == name {
						seen[other] = true
						c.errorf(w+"."+k, lang.T("同じ set 内の %s を参照する。辞書順の逐次代入に依存するため別命令へ分ける"), other)
					}
				}
			}
		}
	}
	var walk func(reflect.Value, string)
	walk = func(v reflect.Value, w string) {
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return
			}
			v = v.Elem()
		}
		if v.Type() == reflect.TypeFor[Expr]() {
			return
		}
		if v.CanInterface() {
			switch x := v.Interface().(type) {
			case Command:
				check(w+".set", x.SetV)
			case GalleryScene:
				check(w+".set", x.Set)
			}
		}
		switch v.Kind() { //nolint:exhaustive // Only schema container/scalar kinds need handling; other reflection kinds are intentionally ignored.
		case reflect.Struct:
			for f := range v.Type().Fields() {
				tag := strings.Split(f.Tag.Get("yaml"), ",")[0]
				if tag != "" && tag != "-" {
					walk(v.FieldByIndex(f.Index), numberPath(w, tag))
				}
			}
		case reflect.Map:
			for _, k := range v.MapKeys() {
				walk(v.MapIndex(k), numberPath(w, fmt.Sprint(k.Interface())))
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i), fmt.Sprintf("%s[%d]", w, i))
			}
		}
	}
	walk(reflect.ValueOf(&db.Game), "game.yaml")
	walk(reflect.ValueOf(&file{Events: db.Events, Locations: db.Locations, Troops: db.Troops}), "")
	return sortedDiagnostics(c.errs)
}

func (c *checker) stateFlag(w, name string) {
	if name != "" && c.db.Game.State.Strict {
		if _, ok := c.stateType("flag").fields[name]; !ok {
			c.errorf(w, lang.T("未宣言のフラグ %q"), name)
		}
	}
}
