package gamedata_test

import (
	"maps"
	"math"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/base/expr"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// finiteAllowed are the decimal numbers of the definitions Check need not refuse when
// they are NaN, each with the reason. Add a field only with one: the sweep exists so
// that a new number is refused without anyone remembering to write its check.
var finiteAllowed = map[string]string{}

// floatSlot is one decimal number of a loaded game: the field that holds it and how to
// write it.
type floatSlot struct {
	field string // Type.yaml_name
	set   func(float64)
	old   float64
}

// floatSlots finds one decimal number for each YAML field that holds any, under v: the
// first one a game wrote (not 0) when there is one, since a number left out belongs
// to a definition that may never read it (the seconds of a command that is no wait).
func floatSlots(v reflect.Value, field string, set func(reflect.Value), seen map[string]int, out *[]floatSlot) {
	switch v.Kind() { //nolint:exhaustive // Only the kinds definitions are made of.
	case reflect.Float64:
		slot := floatSlot{field: field, old: v.Float(), set: func(f float64) { set(reflect.ValueOf(f).Convert(v.Type())) }}
		at, found := seen[field]
		switch {
		case !found:
			seen[field] = len(*out)
			*out = append(*out, slot)
		case (*out)[at].old == 0 && slot.old != 0:
			(*out)[at] = slot
		}
	case reflect.Pointer:
		if !v.IsNil() {
			floatSlots(v.Elem(), field, func(x reflect.Value) { v.Elem().Set(x) }, seen, out)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			floatSlots(v.Index(i), field, func(x reflect.Value) { v.Index(i).Set(x) }, seen, out)
		}
	case reflect.Map:
		keys := v.MapKeys()
		slices.SortFunc(keys, func(a, b reflect.Value) int { return strings.Compare(a.String(), b.String()) })
		for _, key := range keys {
			// A map's value cannot be written in place: write a copy and put it back.
			value := reflect.New(v.Type().Elem()).Elem()
			value.Set(v.MapIndex(key))
			floatSlots(value, field, func(x reflect.Value) {
				value.Set(x)
				v.SetMapIndex(key, value)
			}, seen, out)
		}
	case reflect.Struct:
		if v.Type() == reflect.TypeFor[expr.Expr]() || v.Type() == reflect.TypeFor[data.Color]() {
			return
		}
		top := v.Type() == reflect.TypeFor[data.DB]()
		for f := range v.Type().Fields() {
			tag, tagged := f.Tag.Lookup("yaml")
			if !f.IsExported() || tag == "-" || (!tagged && !top) {
				continue
			}
			name, _, _ := strings.Cut(tag, ",")
			child := v.FieldByIndex(f.Index)
			floatSlots(child, v.Type().Name()+"."+name, func(x reflect.Value) {
				child.Set(x)
				// Writing a field of a struct held by value changes this copy: hand it up.
				set(v)
			}, seen, out)
		}
	}
}

// TestNumbersAreFinite is the finite-number sweep: every decimal number the example and
// fixture games define is set to NaN in turn, and Check must report it. A number Check
// misses loads from YAML as .nan and then draws, compares or saves wrongly.
func TestNumbersAreFinite(t *testing.T) {
	t.Parallel()
	games := map[string]func() (*data.DB, error){
		"fixture":  func() (*data.DB, error) { return testgame.TryLoad(nil) },
		"monsters": func() (*data.DB, error) { return testgame.TryLoad(testgame.MonsterFiles()) },
		"scenes":   func() (*data.DB, error) { return data.LoadDir(testgame.ScenesFS(nil), ".") },
		"campaign": func() (*data.DB, error) { return testgame.TryLoad(testgame.CampaignFiles(nil)) },
		"streams":  func() (*data.DB, error) { return testgame.TryLoad(streamGame()) },
	}
	examples, err := os.ReadDir("../../../examples")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range examples {
		for _, dir := range []string{e.Name(), e.Name() + "/game"} {
			if _, err := os.Stat("../../../examples/" + dir + "/game.yaml"); err == nil {
				games["examples/"+dir] = func() (*data.DB, error) { return data.LoadDir(os.DirFS("../../../examples"), dir) }
			}
		}
	}
	swept := map[string]bool{}    // field -> whether some game's Check reported it
	missed := map[string]string{} // field -> the last game whose Check did not
	for _, name := range slices.Sorted(maps.Keys(games)) {
		db, err := games[name]()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var slots []floatSlot
		floatSlots(reflect.ValueOf(db).Elem(), "", func(reflect.Value) {}, map[string]int{}, &slots)
		before := map[string]bool{}
		for _, e := range db.Check() {
			before[e.Error()] = true
		}
		for _, slot := range slots {
			if swept[slot.field] {
				continue
			}
			slot.set(math.NaN())
			errs := db.Check()
			slot.set(slot.old)
			if slices.ContainsFunc(errs, func(e error) bool { return !before[e.Error()] }) {
				swept[slot.field] = true
				delete(missed, slot.field)
			} else {
				swept[slot.field] = false
				missed[slot.field] = name
			}
		}
	}
	for _, field := range slices.Sorted(maps.Keys(swept)) {
		_, allowed := finiteAllowed[field]
		switch {
		case !swept[field] && !allowed:
			t.Errorf("%s: %s set to NaN adds no diagnostic to Check", missed[field], field)
		case swept[field] && allowed:
			t.Errorf("%s is reported now; remove it from finiteAllowed", field)
		}
	}
	t.Logf("%d decimal fields swept", len(swept))
	if len(swept) < 40 {
		t.Errorf("the sweep reached only %d decimal fields: the walk or the games shrank", len(swept))
	}
	for field := range finiteAllowed {
		if _, reached := swept[field]; !reached {
			t.Errorf("finiteAllowed names %s, which no game defines", field)
		}
	}
}
