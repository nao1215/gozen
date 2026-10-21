package rpg_test

import (
	"math"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestSetPathRefusals: every left side of set refuses a wrong value with a plain message and leaves the state as it was.
func TestSetPathRefusals(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Join("guide")
	before, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path string
		v    any
		want string
	}{
		{"gold", "rich", "want a number"},
		{"steps", "many", "want a number"},
		{"pos.x", "left", "want a number"},
		{"pos.y", true, ""}, // booleans are numbers, so this is accepted
		{"pos.facing", "up-left", "is not north, east, south or west"},
		{"roamers", "all", "only none"},
		{"location", "nowhere", "is not a location"},
		{"location", 7.0, "is not a location"},
		{"spot", "nowhere", "is not a point or a spot"},
		{"spot", 3.0, "is not a point or a spot"},
		{"item.ghost", 1.0, `no item "ghost"`},
		{"item.potion", "lots", "want a number"},
		{"actor.ghost.hp", 1.0, "has not joined"},
		{"actor.scout.name", "Bob", `cannot set "actor.scout.name"`},
		{"actor.scout.hp", "full", "want a number"},
		{"actor.scout.tactics", 5.0, "want auto, manual or a preset"},
		{"actor.scout.tactics", "nonsense", "is not auto, manual or a preset"},
		{"actor.scout.tactics", "{not json", "set actor.scout.tactics"},
		{"party", 3.0, "want actor ids"},
		{"party", "", "1 to"},
		{"party", []any{}, "1 to"},
		{"party", []any{"scout", "scout"}, "written twice"},
		{"party", []any{"scout", 3.0}, "is not an actor"},
		{"party", []any{"ghost"}, "is not an actor"},
		{"nodot", 1.0, `cannot set "nodot"`},
		{"flag.", true, `cannot set "flag."`},
		{"thing.x", 1.0, "only flag.* and var.*"},
	}
	for _, c := range cases {
		err := st.SetPath(c.path, c.v)
		if c.want == "" {
			if err != nil {
				t.Errorf("SetPath(%q, %v): %v", c.path, c.v, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("SetPath(%q, %v) = %v, want an error with %q", c.path, c.v, err, c.want)
		}
	}
	// the refused ones changed nothing (compare after undoing the one accepted case)
	st.Pos = rpg.Pos{}
	after, _ := st.Save()
	if string(before) != string(after) {
		t.Error("a refused set changed the state")
	}
}

func TestSetPathAcceptedValues(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Location = "field"
	set := func(path string, v any) {
		t.Helper()
		if err := st.SetPath(path, v); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	set("gold", -50.0)
	if st.Gold != 0 {
		t.Errorf("gold is never negative, got %d", st.Gold)
	}
	set("gold", 1e30)
	if st.Gold <= 0 {
		t.Errorf("a huge gold is capped, not wrapped: %d", st.Gold)
	}
	set("steps", -3.0)
	if st.Steps != 0 {
		t.Errorf("steps %d", st.Steps)
	}
	set("pos.x", 4.0)
	set("pos.y", 2.0)
	set("pos.facing", "south")
	if st.Pos.X != 4 || st.Pos.Y != 2 || st.Pos.Facing != 2 || st.Pos.In != "field" {
		t.Errorf("pos %+v", st.Pos)
	}
	st.Roamers = []rpg.Roamer{{Troop: "one"}}
	set("roamers", "none")
	if st.Roamers != nil {
		t.Error("set roamers none removes them")
	}
	set("calm", true)
	if !st.Calm {
		t.Error("calm")
	}
	set("location", "vault")
	if st.Location != "vault" || st.Spot != "" {
		t.Errorf("location %q spot %q", st.Location, st.Spot)
	}
	set("item.potion", 5.0)
	set("item.key", 2.0)
	set("item.key", 0.0)
	if st.Items["potion"] != 5 {
		t.Error("potion")
	}
	if _, ok := st.Items["key"]; ok {
		t.Error("an item set to 0 leaves the bag")
	}
	set("party", "scout, guide")
	if len(st.Party) != 2 || st.Party[1] != "guide" || st.Actors["guide"] == nil {
		t.Errorf("party %v", st.Party)
	}
	set("party", []any{"guide"})
	if len(st.Party) != 1 || st.Party[0] != "guide" {
		t.Errorf("party %v", st.Party)
	}
	set("party", "scout")
	// tactics: a preset-free word and a JSON body
	set("actor.scout.tactics", "manual")
	if m := st.Actors["scout"].Tactics.Mode; m != rpg.TacticsManual {
		t.Errorf("mode %q", m)
	}
	set("actor.scout.tactics", `{"mode":"manual","rows":[{"enabled":true,"condition":"any","action":"attack","target":""}]}`)
}

// TestSetActorValuesDirectly: the development-only values of an actor are written directly, kept in their ranges.
func TestSetActorValuesDirectly(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	a := st.Actors["scout"]
	set := func(field string, v float64) {
		t.Helper()
		if err := st.SetPath("actor.scout."+field, v); err != nil {
			t.Fatal(err)
		}
	}
	set("hp", 9999)
	if a.HP != a.Stat("mhp") {
		t.Errorf("hp is capped at the maximum: %d", a.HP)
	}
	set("hp", -5)
	if a.HP != 0 {
		t.Errorf("hp is never negative: %d", a.HP)
	}
	set("mp", 9999)
	if a.MP != a.Stat("mmp") {
		t.Errorf("mp is capped: %d", a.MP)
	}
	set("sp", 7)
	if a.SP != 7 {
		t.Errorf("sp %d", a.SP)
	}
	set("exp", 100)
	if a.Level < 2 || a.Exp < 40 {
		t.Errorf("setting more experience levels up: level %d exp %d", a.Level, a.Exp)
	}
	lv := a.Level
	set("exp", 0)
	if a.Exp != 0 || a.Level != lv {
		t.Errorf("setting less experience only lowers the number: level %d exp %d", a.Level, a.Exp)
	}
	set("level", 5)
	if a.Level != 5 {
		t.Errorf("level %d", a.Level)
	}
	set("level", 2)
	if a.Level != 2 || a.Exp != a.ExpFor(2) {
		t.Errorf("lowering the level resets the experience to its start: level %d exp %d", a.Level, a.Exp)
	}
	set("level", 0)
	if a.Level != 1 {
		t.Errorf("level 0 is level 1, got %d", a.Level)
	}
	set("level", 9999)
	if a.Level != a.Class().Max {
		t.Errorf("level is capped at the class maximum %d, got %d", a.Class().Max, a.Level)
	}
	if a.HP > a.Stat("mhp") || a.MP > a.Stat("mmp") {
		t.Error("a level change leaves HP and MP in range")
	}
}

func TestPartyLimitsAndWipe(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": testgame.Game + "max_party: 2\n", "data/extra.yaml": "actors:\n  third: {name: Third, class: ranger}\n"})
	st := testgame.State(db, 1)
	if !st.Join("guide") {
		t.Fatal("room for a second member")
	}
	if st.Join("guide") {
		t.Error("joining twice")
	}
	if st.Join("third") {
		t.Error("the party is full")
	}
	if len(st.Party) != 2 || st.Actors["third"] != nil {
		t.Errorf("party %v", st.Party)
	}
	if st.Wiped() {
		t.Error("a standing party is not wiped")
	}
	for _, a := range st.Members() {
		a.HP = 0
	}
	if !st.Wiped() {
		t.Error("everyone has fallen")
	}
	st.Leave("guide")
	st.Leave("guide")
	if len(st.Party) != 1 || st.Actors["guide"] == nil {
		t.Errorf("leaving keeps the actor's progress: %v", st.Party)
	}
	if all := rpg.AllActors(st); len(all) != 2 || all[0].ID != "scout" {
		t.Errorf("AllActors in data order: %v", all)
	}
}

func TestShopRefusals(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/shop.yaml": `
shops:
  open: {goods: [potion, blade]}
  closed: {goods: [potion], sell: false}
  tally: {goods: [potion], spent: var.spent}
`})
	st := testgame.State(db, 1)
	st.Gold = 25
	if st.Buy("open", "potion", 3) || st.Gold != 25 {
		t.Error("buying more than the gold pays for")
	}
	if st.Buy("open", "ghost", 1) || st.Buy("open", "potion", 0) || st.Buy("open", "potion", -1) {
		t.Error("an unknown item or a count of 0 or less")
	}
	if !st.Buy("tally", "potion", 2) || st.Gold != 5 || st.Vars["spent"] != 20.0 {
		t.Errorf("gold %d spent %v", st.Gold, st.Vars["spent"])
	}
	if !st.Buy("tally", "potion", 0+0) && st.Gold != 5 {
		t.Error("gold changed on a refusal")
	}
	if st.Sell("closed", "potion", 1) || st.Sell("nowhere", "potion", 1) {
		t.Error("a shop that does not buy, or does not exist")
	}
	st.AddItem("key", 1)
	if st.Sell("open", "key", 1) {
		t.Error("a key cannot be sold")
	}
	if st.Sell("open", "potion", 99) || st.Sell("open", "potion", 0) {
		t.Error("selling more than the bag holds, or none")
	}
	n := st.Items["potion"]
	if !st.Sell("open", "potion", 1) || st.Items["potion"] != n-1 || st.Gold != 10 {
		t.Errorf("a sale pays half the price: gold %d", st.Gold)
	}
}

func TestExpressionNames(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	call := func(name string, args ...any) (any, error) {
		t.Helper()
		v, ok := st.Env().Lookup(name)
		if !ok {
			t.Fatalf("no name %q", name)
		}
		f, ok := v.(expr.Func)
		if !ok {
			t.Fatalf("%s is %T", name, v)
		}
		return f(args)
	}
	for _, c := range []struct {
		fn   string
		args []any
		want string
	}{
		{"pages", nil, "takes 1 argument"},
		{"pages", []any{3.0}, "file name or pattern"},
		{"chance", nil, "takes 1 argument"},
		{"rand", []any{1.0, 2.0, 3.0}, "0 to 2 arguments"},
		{"rand", []any{math.NaN()}, "not a number"},
	} {
		if _, err := call(c.fn, c.args...); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s%v: %v", c.fn, c.args, err)
		}
	}
	if v, _ := call("pages", "cg/none"); v != 0.0 {
		t.Errorf("pages of nothing %v", v)
	}
	if v, _ := call("chance", 1.0); v != true {
		t.Error("chance(1) always holds")
	}
	if v, _ := call("chance", 0.0); v != false {
		t.Error("chance(0) never holds")
	}
	for range 200 {
		v, err := call("rand", 3.0)
		if f := v.(float64); err != nil || f < 0 || f > 2 || f != math.Trunc(f) {
			t.Fatalf("rand(3) = %v %v", v, err)
		}
		v, _ = call("rand", 5.0, 7.0)
		if f := v.(float64); f < 5 || f > 7 || f != math.Trunc(f) {
			t.Fatalf("rand(5, 7) = %v", v)
		}
		v, _ = call("rand")
		if f := v.(float64); f < 0 || f >= 1 {
			t.Fatalf("rand() = %v", v)
		}
	}
	if v, _ := call("rand", 0.0); v != 0.0 {
		t.Errorf("rand(0) = %v", v)
	}
	if v, _ := call("rand", -4.0); v != 0.0 {
		t.Errorf("rand(-4) = %v", v)
	}
	if v, _ := call("rand", 9.0, 2.0); v != 9.0 {
		t.Errorf("rand(9, 2) = %v (a reversed range gives the first)", v)
	}
	if v, err := call("rand", math.Inf(1)); err != nil || v.(float64) < 0 {
		t.Errorf("rand(inf) = %v %v", v, err)
	}
	if v, err := call("rand", -1e300, 1e300); err != nil || math.IsNaN(v.(float64)) {
		t.Errorf("rand over an enormous range = %v %v", v, err)
	}
}
