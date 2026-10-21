package battle_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/testgame"
)

const unitData = `
elements: {wind: Wind}
items:
  gale_blade: {name: Gale Blade, kind: sword, slot: weapon, element: wind, stats: {atk: 1}}
states:
  fury: {name: Fury, turns: 3, stats: {atk: 2}}
  bleed: {name: Bleed, turns: 3}
enemies:
  mask:
    name: Mask
    image: x.png
    faces: {calm: x.png, hurt: x.png}
    face_rules: [{when: "self.hp_rate < 0.5", face: hurt}, {when: "true", face: calm}]
    resist: {wind: 0.5}
    state_resist: {bleed: 0.25}
    stats: {mhp: 100, mmp: 10, atk: 4, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}
    level: 7
    ai: [{use: guard}]
troops:
  mask: {members: [mask]}
`

func unitSetup(t *testing.T) *battle.Battle {
	t.Helper()
	db := testgame.Load(t, map[string]string{"data/units.yaml": unitData})
	st := testgame.State(db, 1)
	return battle.New(db, st, "mask")
}

func field(t *testing.T, u *battle.Unit, name string) any {
	t.Helper()
	v, ok := u.Field(name)
	if !ok {
		t.Fatalf("no field %q", name)
	}
	return v
}

// TestUnitFieldsAsExpressionsSeeThem: every field a formula can read from a battler has its documented value.
func TestUnitFieldsAsExpressionsSeeThem(t *testing.T) {
	b := unitSetup(t)
	scout, mask := b.Party[0], b.Foes[0]
	mask.HP = 20
	scout.Guard = true
	scout.MP = scout.MMP() / 2
	cases := []struct {
		u    *battle.Unit
		name string
		want any
	}{
		{scout, "name", "Scout"},
		{scout, "id", "scout"},
		{mask, "id", "mask"},
		{mask, "name", "Mask"},
		{mask, "hp", 20.0},
		{mask, "hp_rate", 0.2},
		{scout, "mp_rate", float64(scout.MP) / float64(scout.MMP())},
		{mask, "mp_rate", 1.0},
		{mask, "alive", true},
		{mask, "dead", false},
		{mask, "level", 7.0},
		{scout, "level", 1.0},
		{mask, "is_enemy", true},
		{scout, "is_enemy", false},
		{scout, "guarding", true},
		{mask, "guarding", false},
		{mask, "index", 0.0},
		{mask, "boss", false},
		{scout, "boss", false},
		{scout, "class", "ranger"},
		{mask, "class", ""},
		{mask, "element", ""},
		{scout, "back", false},
		{mask, "back", false},
		{mask, "atk", 4.0},
		{mask, "resist_wind", 0.5},
		{mask, "resist_fire", 1.0},
		{scout, "resist_wind", 1.0},
		{scout, "lv", 1.0},
		{scout, "state_count", 0.0},
	}
	for _, c := range cases {
		if got := field(t, c.u, c.name); got != c.want {
			t.Errorf("%s.%s = %#v, want %#v", c.u.Name, c.name, got, c.want)
		}
	}
	if _, ok := scout.Field("nothing"); ok {
		t.Error("an unknown field must not resolve")
	}
	if scout.ID() != "scout" || mask.ID() != "mask" || scout.StateList() != nil {
		t.Error("ID and StateList")
	}
	mask.HP = 0
	if field(t, mask, "dead") != true || field(t, mask, "alive") != false {
		t.Error("a battler with no HP is dead")
	}
	mask.HP, mask.Fled = 20, true
	if field(t, mask, "alive") != false {
		t.Error("a battler that ran away is not alive")
	}
}

func TestStatesChangeStatsAndAreListedInDataOrder(t *testing.T) {
	b := unitSetup(t)
	scout := b.Party[0]
	atk := scout.Stat("atk")
	scout.States["bleed"] = 2
	scout.States["fury"] = 1
	scout.States["sleep"] = 0 // a zero is not a state
	if got := scout.Stat("atk"); got != atk*2 {
		t.Errorf("fury doubles attack: %d -> %d", atk, got)
	}
	if got := field(t, scout, "state_count"); got != 2.0 {
		t.Errorf("state_count %v", got)
	}
	list := scout.StateList()
	if len(list) != 2 || list[0] != "fury" && list[0] != "bleed" {
		t.Fatalf("list %v", list)
	}
	view := field(t, scout, "states").(expr.Object)
	if v, _ := view.Field("fury"); v != 1.0 {
		t.Errorf("states.fury = %v", v)
	}
	if v, ok := view.Field("unknown"); !ok || v != 0.0 {
		t.Errorf("a state the unit is not in reads as 0: %v %v", v, ok)
	}
}

func TestWeaponElementAndWornGear(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/units.yaml": unitData})
	st := testgame.State(db, 1)
	st.AddItem("gale_blade", 1)
	if err := st.Equip(st.Actors["scout"], "weapon", "gale_blade"); err != nil {
		t.Fatal(err)
	}
	b := battle.New(db, st, "mask")
	scout := b.Party[0]
	if got := field(t, scout, "element"); got != "wind" {
		t.Errorf("element %v", got)
	}
	wears := field(t, scout, "wears").(expr.Object)
	if v, _ := wears.Field("gale_blade"); v != true && v != 1.0 {
		t.Errorf("wears.gale_blade = %v", v)
	}
}

func TestEnemyFaceFollowsItsRules(t *testing.T) {
	b := unitSetup(t)
	mask := b.Foes[0]
	if got := mask.Face(); got != "calm" {
		t.Errorf("a healthy mask is calm, got %q", got)
	}
	mask.HP = 10
	if got := mask.Face(); got != "hurt" {
		t.Errorf("a hurt mask shows hurt, got %q", got)
	}
	if got := b.Party[0].Face(); got != "" {
		t.Errorf("a party member has no enemy face, got %q", got)
	}
}

func TestBackRowIsAimedAtLessOften(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/units.yaml": unitData, "data/hitter.yaml": `
enemies:
  hitter:
    name: Hitter
    image: x.png
    stats: {mhp: 9999, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}
    ai: [{use: slap}]
troops:
  hitter: {members: [hitter]}
`})
	count := func(front, back *float64) map[string]int {
		hits := map[string]int{}
		for seed := uint64(1); seed <= 300; seed++ {
			st := testgame.State(db, seed)
			st.Join("guide")
			st.Actors["guide"].Back = true
			db.Game.Battle.FrontWeight, db.Game.Battle.BackWeight = front, back
			b := battle.New(db, st, "hitter")
			b.Rand = st.Rand
			steps := run(b, battle.Action{User: b.Party[0], Skill: "guard"}, battle.Action{User: b.Party[1], Skill: "guard"})
			for _, s := range steps {
				if s.Kind == battle.Damage && s.Target != nil && s.Target.Actor != nil {
					hits[s.Target.Actor.ID]++
				}
			}
		}
		return hits
	}
	def := count(nil, nil)
	if def["scout"] < 2*def["guide"] || def["guide"] == 0 {
		t.Errorf("by default the back row is aimed at about a quarter as often: front %d back %d", def["scout"], def["guide"])
	}
	zero, one := 0.0, 1.0
	only := count(&one, &zero)
	if only["guide"] != 0 || only["scout"] != 300 {
		t.Errorf("a back weight of 0 means the back row is never aimed at: %v", only)
	}
	neg := -3.0
	if got := count(&neg, &neg); got["scout"]+got["guide"] != 300 {
		t.Errorf("negative weights act like 0 and the foe still hits someone: %v", got)
	}
	even := count(&one, &one)
	if even["guide"] < 100 || even["scout"] < 100 {
		t.Errorf("equal weights hit both rows: %v", even)
	}
}
