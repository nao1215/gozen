// Package testgame builds an original in-memory camp expedition for contract tests.
// Its tiny text asset is a file-existence sentinel, not an image from a game.
package testgame

import (
	"math/rand/v2"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/rpg"
	data "github.com/nao1215/gozen/schema"
)

// Game defines a deterministic start for the test expedition.
const Game = `
title: Camp expedition
start: {location: camp, party: [scout], gold: 100, items: {potion: 2}}
battle:
  escape: "1"
  critical: "0"
  variance: 0.0001
  speed: "a.agi"
`

// Data defines synthetic equipment, encounters and a three-location journey.
const Data = `
elements: {fire: Warmth, ice: Frost}
classes:
  ranger:
    name: Ranger
    stats: {mhp: "40 + level * 10", mmp: "10 + level", atk: "10 + level * 2", def: "5 + level", mat: "5", mdf: "5", agi: "10", luk: "5"}
    exp_for: "level * level * 10"
    skills: [{level: 1, skill: fire}, {level: 3, skill: heal}]
    equip: [sword]
  hatter:
    name: Weaver
    stats: {mhp: "30", mmp: "10", atk: "5", def: "5", mat: "5", mdf: "5", agi: "10", luk: "5"}
    exp_for: "level * level * 10"
    equip: [hat]
actors:
  scout: {name: Scout, class: ranger, equip: {weapon: stick}}
  guide: {name: Guide, class: ranger, level: 2}
items:
  potion: {name: Tonic, price: 10, use: {scope: one_ally, effects: [{heal: "30"}]}}
  stick: {name: Branch, kind: sword, slot: weapon, stats: {atk: 3}, price: 10}
  blade: {name: Training blade, kind: sword, slot: weapon, stats: {atk: 10}, price: 100, element: fire}
  hat: {name: Woven cap, kind: hat, slot: armor, stats: {def: 2}}
  key: {name: Brass token, kind: key}
skills:
  attack: {name: Strike, scope: one_enemy, effects: [{damage: "a.atk * 2 - b.def"}]}
  guard: {name: Brace, scope: self, effects: []}
  fire: {name: Spark, mp: 3, scope: one_enemy, effects: [{damage: "20", element: fire}]}
  heal: {name: Restore, mp: 3, scope: one_ally, where: [battle, menu], effects: [{heal: "50"}]}
  big: {name: Sweep, scope: all_enemies, effects: [{damage: "999"}]}
  nap: {name: Lull, scope: one_enemy, effects: [{add_state: sleep}]}
  venom: {name: Spores, scope: one_enemy, effects: [{add_state: poison}]}
  slap: {name: Nudge, scope: one_enemy, effects: [{damage: "1", variance: 0}]}
  mend: {name: Repair, scope: one_ally, effects: [{heal: "10"}]}
states:
  sleep: {name: Drowsy, turns: 2, restrict: skip, bad: true}
  poison: {name: Poisoned, turns: 0, bad: true, persist: true, on_turn_end: [{damage: "5", variance: 0}]}
enemies:
  mote:
    name: Mote
    image: x.png
    stats: {mhp: 30, mmp: 0, atk: 8, def: 2, mat: 1, mdf: 1, agi: 1, luk: 1}
    exp: 25
    gold: 7
    drops: [{item: key, chance: 1}]
    resist: {fire: 2}
    ai: [{use: slap}]
  boss:
    name: Sentinel
    image: x.png
    stats: {mhp: 500, mmp: 50, atk: 8, def: 2, mat: 1, mdf: 1, agi: 1, luk: 1}
    ai:
      - {when: "self.hp_rate < 0.5", use: mend, target: self, priority: 5}
      - {when: "turn == 1", use: nap, priority: 3}
      - {use: slap, weight: 1}
troops:
  one: {members: [mote]}
  two: {members: [mote, mote]}
  boss:
    members: [boss]
    can_escape: false
    events: [{when: "turn == 2", do: [{say: "The bell rings."}, {set: {flag.saw_two: true}}]}]
locations:
  camp:
    name: Camp
    people:
      - {name: Courier, talk: [{say: "A clear trail today."}]}
      - {name: Watcher, when: "flag.ghost", talk: [{say: "A light in the distance."}]}
    actions: [{label: "Rest ({members * 5}G)", do: [{heal: all}]}]
    exits:
      - {to: field, label: Field}
      - {to: vault, label: Vault, when: "item.key > 0"}
  field:
    name: Field
    encounters: {rate: "1", search: Search, troops: [{troop: one}]}
    exits: [{to: camp}]
  vault:
    name: Vault
    exits: [{to: camp}]
events:
  greet: {trigger: enter, location: field, once: true, do: [{say: "The trail opens."}]}
  sub: {do: [{gold: "5"}]}
`

// Load loads an isolated game, replacing or adding the requested fixture files.
func Load(t testing.TB, extra map[string]string) *data.DB {
	t.Helper()
	db, err := TryLoad(extra)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// TryLoad exposes invalid fixture diagnostics to tests.
func TryLoad(extra map[string]string) (*data.DB, error) {
	fsys := fstest.MapFS{"game.yaml": {Data: []byte(Game)}, "data/main.yaml": {Data: []byte(Data)}, "assets/x.png": {Data: []byte(sentinel)}}
	for n, s := range extra {
		fsys[n] = &fstest.MapFile{Data: []byte(s)}
	}
	return data.Load(fsys, ".")
}

// sentinel is the content of assets/x.png: a file that exists, not a picture.
const sentinel = "synthetic file sentinel"

// State supplies an independent random source with the requested seed.
func State(db *data.DB, seed uint64) *rpg.State {
	st := rpg.New(db, rand.New(rand.NewPCG(seed, seed))) //nolint:gosec // Deterministic gameplay test, not a security token.
	st.SetRecruitSeed(seed)
	return st
}

// DeclaredGame enables strict variable typing for assignment tests.
const DeclaredGame = Game + `
state:
  strict: true
  names:
    flag.saw_two: {type: bool, initial: false}
    flag.ghost: {type: bool, initial: false}
    flag.ready: {type: bool, initial: true}
    var.a: {type: number, initial: 2}
    var.b: {type: number, initial: 7}
    var.text: {type: string, initial: 初期値}
    var.enabled: {type: bool, initial: false}
`

// Expr parses a literal owned by a test; invalid test definitions fail immediately.
func Expr(source string) *expr.Expr {
	e, err := expr.Parse(source)
	if err != nil {
		panic(err)
	}
	return e
}

// As returns v as a T. A fixture value of another type is a broken test, so it
// fails t.
func As[T any](t testing.TB, v any) T {
	t.Helper()
	x, ok := v.(T)
	if !ok {
		t.Fatalf("fixture: %T is not %T", v, x)
	}
	return x
}
