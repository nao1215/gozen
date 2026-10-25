package event_test

import (
	"errors"
	"math/rand/v2"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// version2 is a small game using the spec version 2 vocabulary (ADR 0021).
const version2Game = `schema_version: 2
id: version-two
start: {location: camp, party: [scout], gold: 10, items: {tonic: 2, blade: 1}}
equip_slots: [{id: weapon, name: Weapon}]
battle: {tactic_presets: {careful: {name: Careful, rows: {}}}}
verbs: {roll: sub}
screens: {album: {title: Notes, entries: [{label: Entry, text: Welcome}]}}
`

const version2Data = `classes:
  walker:
    stats: {mhp: "20 + level * 10", mmp: "5", atk: "5", def: "1", mat: "1", mdf: "1", agi: "1", luk: "1"}
    exp_for: "level * 10"
actors:
  scout: {name: Scout, class: walker}
  guide: {name: Guide, class: walker}
items:
  tonic: {name: Tonic, use: {scope: one_ally, where: [menu], effects: [{heal: "5"}]}}
  blade: {name: Blade, slot: weapon, stats: {atk: 3}}
skills:
  attack: {scope: one_enemy, effects: [{damage: "1"}]}
  guard: {scope: self}
locations:
  camp:
    name: Camp
    actions: [{label: Rest, do: [{set: {var.rested: 1}}]}]
    exits: [{to: deep, label: Down}]
  deep:
    name: Deep
    dungeon:
      map: |
        #####
        #S..#
        ###G#
        #####
      tiles:
        S: {start: true, facing: east}
        G: {name: Goal, do: [{set: {var.goal: "var.goal + 1"}}]}
  far:
    name: Far
    dungeon:
      map: |
        ##############################################################################################
        #S..........................................................................................G#
        ##############################################################################################
      tiles:
        S: {start: true, facing: east}
        G: {name: Goal, look: door, solid: "true", do: [{set: {var.goal: "var.goal + 1"}}]}
events:
  sub: {do: [{set: {var.rolled: 6}}]}
  walkfar: {do: [{move: far}, {walk: G}, {set: {var.at: "pos.x"}}]}
  walk: {do: [{move: deep}, {walk: G}, {set: {var.at: "pos.x * 10 + pos.y"}}]}
  step: {do: [{move: deep}, {step: 2}, {turn: right}, {step: 1}, {set: {var.at: "pos.x * 10 + pos.y", var.facing: "pos.facing"}}]}
  look: {do: [{move: deep}, {set: {var.row: "map[1]", var.wall: "cell(0, 0).solid", var.floor: "cell(2, 1).passable"}}]}
  rest: {do: [{pick: Rest}, {set: {var.menu: "len(menu)", var.first: "menu[0].label"}}]}
  gear: {do: [{equip: {actor: scout, item: blade}}, {use: tonic}, {set: {var.left: "item.tonic", var.worn: "actor.scout.equip.weapon"}}]}
  write: {do: [{set: {gold: 99, item.tonic: 7, party: [scout, guide], location: deep, actor.scout.tactics: careful}}, {set: {var.mode: "actor.scout.tactics.mode", var.preset: "actor.scout.tactics.preset"}}]}
  stats: {do: [{set: {actor.scout.level: 3}}, {set: {actor.scout.hp: 1}}]}
  fight: {do: [{battle: none}, {set: {var.result: "result"}}]}
  custom: {do: [{roll: true}, {open: album}]}
`

func version2(t *testing.T) (*data.DB, *rpg.State) {
	t.Helper()
	db, err := data.LoadDir(fstest.MapFS{
		"game.yaml":      {Data: []byte(version2Game)},
		"assets/x.png":   {Data: []byte("synthetic file sentinel")},
		"data/main.yaml": {Data: []byte(version2Data + "troops: {none: {members: [mote]}}\nenemies: {mote: {image: x.png, stats: {mhp: 1, mmp: 0, atk: 1, def: 0, mat: 0, mdf: 0, agi: 1, luk: 1}, ai: [{use: attack}]}}\n")},
	}, ".")
	if err != nil {
		t.Fatal(err)
	}
	return db, rpg.New(db, rand.New(rand.NewPCG(1, 2))) //nolint:gosec // Deterministic test.
}

// run2 runs the event commands to the end (choices are 0, battles are wins).
func run2(t *testing.T, db *data.DB, st *rpg.State, id string, dev bool) ([]event.Request, []error) {
	t.Helper()
	var errs []error
	r := event.New(db, st, event.HostFunc(func(e event.Effect) {
		if m, ok := e.(event.MoveEffect); ok {
			st.Location = m.Location
		}
	}))
	r.Log = func(err error) { errs = append(errs, err) }
	r.Dev = dev
	r.UseItem = func(item, target string) ([]string, error) {
		if st.Items[item] <= 0 {
			return nil, errors.New("none left")
		}
		st.AddItem(item, -1)
		return []string{"used " + item + " on " + target}, nil
	}
	r.Start(db.Events[id].Do)
	var seen []event.Request
	for range 1000 {
		q := r.Next()
		if q == nil {
			if r.Running() {
				continue
			}
			return seen, errs
		}
		seen = append(seen, q)
		switch q.(type) {
		case event.Choice:
			r.Answer(0)
		case event.Battle:
			r.Answer(event.BattleWon)
		}
	}
	t.Fatal("the event does not end")
	return nil, nil
}

func TestVersion2Walk(t *testing.T) {
	db, st := version2(t)
	if _, errs := run2(t, db, st, "walk", false); len(errs) > 0 {
		t.Fatal(errs)
	}
	if st.Vars["at"] != 32.0 || st.Vars["goal"] != 1.0 {
		t.Fatalf("walk did not reach the goal and step on it: at %v goal %v", st.Vars["at"], st.Vars["goal"])
	}
}

// A walk longer than the events' nesting limit (64) goes on: each step is the walk's
// continuation, not a frame nested in the last one.
func TestVersion2WalkFar(t *testing.T) {
	db, st := version2(t)
	if _, errs := run2(t, db, st, "walkfar", false); len(errs) > 0 {
		t.Fatal(errs)
	}
	if st.Vars["at"] != 91.0 || st.Vars["goal"] != 1.0 {
		t.Fatalf("a far walk did not reach the door: at %v goal %v", st.Vars["at"], st.Vars["goal"])
	}
}

func TestVersion2StepAndTurn(t *testing.T) {
	db, st := version2(t)
	if _, errs := run2(t, db, st, "step", false); len(errs) > 0 {
		t.Fatal(errs)
	}
	if st.Vars["at"] != 32.0 || st.Vars["facing"] != "south" || st.Vars["goal"] != 1.0 {
		t.Fatalf("step and turn went wrong: at %v facing %v goal %v", st.Vars["at"], st.Vars["facing"], st.Vars["goal"])
	}
}

func TestVersion2MapAndCell(t *testing.T) {
	db, st := version2(t)
	if _, errs := run2(t, db, st, "look", false); len(errs) > 0 {
		t.Fatal(errs)
	}
	if st.Vars["row"] != "#S..#" || st.Vars["wall"] != 1.0 || st.Vars["floor"] != 1.0 { // boolean vars are 1 and 0
		t.Fatalf("map and cell: %v %v %v", st.Vars["row"], st.Vars["wall"], st.Vars["floor"])
	}
}

// menu covers Rest, Down, and the place's default menu items.
func TestVersion2PickAndMenu(t *testing.T) {
	db, st := version2(t)
	if _, errs := run2(t, db, st, "rest", false); len(errs) > 0 {
		t.Fatal(errs)
	}
	if st.Vars["rested"] != 1.0 || st.Vars["menu"] != 3.0 || st.Vars["first"] != "Rest" {
		t.Fatalf("pick and menu: %v", st.Vars)
	}
}

func TestVersion2EquipAndUse(t *testing.T) {
	db, st := version2(t)
	seen, errs := run2(t, db, st, "gear", false)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	line, ok := event.Say{}, len(seen) == 1
	if ok {
		line, ok = seen[0].(event.Say)
	}
	if st.Vars["worn"] != "blade" || st.Vars["left"] != 1.0 || !ok || !strings.Contains(line.Text, "used tonic") {
		t.Fatalf("equip and use: %v %v", st.Vars, seen)
	}
}

func TestVersion2SetLeftSides(t *testing.T) {
	db, st := version2(t)
	if _, errs := run2(t, db, st, "write", false); len(errs) > 0 {
		t.Fatal(errs)
	}
	if st.Gold != 99 || st.Items["tonic"] != 7 || strings.Join(st.Party, ",") != "scout,guide" || st.Location != "deep" {
		t.Fatalf("set: gold %d tonic %d party %v location %s", st.Gold, st.Items["tonic"], st.Party, st.Location)
	}
	if st.Vars["mode"] != "preset" || st.Vars["preset"] != "careful" {
		t.Fatalf("tactics: %v %v", st.Vars["mode"], st.Vars["preset"])
	}
}

// Setting abilities and levels writes only with Dev (author's decision 5).
func TestVersion2StatsOnlyInDev(t *testing.T) {
	db, st := version2(t)
	_, errs := run2(t, db, st, "stats", false)
	a := st.Actors["scout"]
	if len(errs) != 2 || a.Level != 1 || a.HP != a.Stat("mhp") {
		t.Fatalf("stats were set outside the development mode: level %d hp %d errs %v", a.Level, a.HP, errs)
	}
	if _, errs := run2(t, db, st, "stats", true); len(errs) > 0 || a.Level != 3 || a.HP != 1 {
		t.Fatalf("stats in the development mode: level %d hp %d errs %v", a.Level, a.HP, errs)
	}
}

func TestVersion2ResultAndCustom(t *testing.T) {
	db, st := version2(t)
	if _, errs := run2(t, db, st, "fight", false); len(errs) > 0 || st.Vars["result"] != "win" {
		t.Fatalf("result: %v %v", st.Vars["result"], errs)
	}
	seen, errs := run2(t, db, st, "custom", false)
	if len(errs) != 0 || st.Vars["rolled"] != float64(6) {
		t.Fatalf("a reusable command executes its event: %v", errs)
	}
	if len(seen) != 1 || seen[0] != (event.Open{Screen: "album"}) {
		t.Fatalf("a declared screen opens: %v", seen)
	}
}
