package event_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen/event"
	"github.com/nao1215/gozen/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

const stoneData = `
classes:
  mage:
    name: Mage
    stats: {mhp: "30", mmp: "10", atk: "5", def: "5", mat: "10", mdf: "5", agi: "10", luk: "5"}
    exp_for: "level * level * 10"
    equip: [sword, stone]
actors:
  sage: {name: Sage, class: mage}
  monk: {name: Monk, class: mage}
items:
  white: {name: White Stone, short: White, kind: stone, slot: stone, stats: {def: 2}}
  white2: {name: Pure Stone, short: Pure, kind: stone, slot: stone, stats: {def: 6}}
  blue: {name: Blue Stone, short: Blue, kind: stone, slot: stone, stats: {mat: 3}}
  red: {name: Red Stone, short: Red, kind: stone, slot: stone, stats: {atk: 3}}
  gold: {name: Gold Stone, kind: stone, slot: stone, stats: {luk: 3}}
  vow: {name: Vow Stone, kind: stone, slot: stone, stats: {atk: 1}}
stone_sets:
  frost: {name: Frost, stones: [white, blue], skill: heal, stats: {mdf: 4}, for: [monk], only: true, pitch: "Frost guards."}
  frost_up: {name: Frost, stones: [white2, blue], skill: heal, stats: {mdf: 4}, for: [monk], only: true}
  ember: {name: Ember, stones: [red, gold], stats: {atk: 5}, for: [sage], only: true}
  oath: {name: Oath, stones: [vow], stats: {luk: 1}, for: [sage], only: true}
`

// stoneGame is the test game with two stone slots and the sage and the monk in the party.
func stoneGame(t *testing.T, bag map[string]int) (*data.DB, *rpg.State) {
	t.Helper()
	game := testgame.Game + `
equip_slots:
  - {id: weapon, name: W}
  - {id: armor, name: A}
  - {id: stone1, name: S1}
  - {id: stone2, name: S2}
`
	db := testgame.Load(t, map[string]string{"game.yaml": game, "data/stones.yaml": stoneData})
	st := testgame.State(db, 1)
	st.Join("sage")
	st.Join("monk")
	for id, n := range bag {
		st.AddItem(id, n)
	}
	return db, st
}

func run(t *testing.T, db *data.DB, st *rpg.State, cmds []data.Command) *event.Runner {
	t.Helper()
	r := event.New(db, st, &host{})
	r.Log = func(err error) { t.Error(err) }
	r.Start(cmds)
	return r
}

func suggest(t *testing.T, db *data.DB, st *rpg.State) {
	t.Helper()
	r := run(t, db, st, []data.Command{{StoneSuggest: "o"}})
	if req := r.Next(); req != nil {
		t.Fatalf("stone_suggest asked the screen: %+v", req)
	}
}

func equip(t *testing.T, db *data.DB, st *rpg.State) []string {
	t.Helper()
	cmds := []data.Command{{EquipStones: &data.EquipStones{Actor: mustExpr(t, "var.o_who"), Set: mustExpr(t, "var.o_set")}}}
	r := run(t, db, st, cmds)
	var notes []string
	for req := r.Next(); req != nil; req = r.Next() {
		if s, ok := req.(event.Say); ok {
			notes = append(notes, s.Text)
		}
	}
	return notes
}

func mustExpr(t *testing.T, src string) data.Expr {
	t.Helper()
	e, err := expr.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return data.Expr{Expr: e}
}

func TestStoneSuggestTheStrongestLoadout(t *testing.T) {
	db, st := stoneGame(t, map[string]int{"white": 1, "blue": 1, "red": 1, "gold": 1})
	// the sage wears the white and blue stones: Frost works only on the monk
	sage, monk := st.Actors["sage"], st.Actors["monk"]
	if err := st.Equip(sage, "stone1", "white"); err != nil {
		t.Fatal(err)
	}
	if err := st.Equip(sage, "stone2", "blue"); err != nil {
		t.Fatal(err)
	}
	if len(sage.Sets()) != 0 {
		t.Fatalf("a set only for the monk works on the sage: %v", sage.Sets())
	}
	suggest(t, db, st)
	if st.Vars["o"] != 2.0 || st.Vars["o_pages"] != 2.0 {
		t.Fatalf("vars = %v", st.Vars)
	}
	p1, p2 := st.Vars["o_page1"].(string), st.Vars["o_page2"].(string) //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
	if !strings.Contains(p1, "Sage　White・Blue → Red・Gold Stone") || !strings.Contains(p2, "Monk　なし → White・Blue") || !strings.Contains(p2, "Frost guards.") {
		t.Fatalf("pages = %q %q", p1, p2)
	}
	notes := applyStones(t, db, st)
	if len(sage.Sets()) != 1 || len(monk.Sets()) != 1 || monk.Sets()[0] != "frost" || sage.Sets()[0] != "ember" {
		t.Fatalf("sage %v monk %v", sage.Equip, monk.Equip)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "Restore") {
		t.Errorf("notes = %q", notes)
	}
	suggest(t, db, st)
	if st.Vars["o"] != 0.0 {
		t.Errorf("the strongest loadout is suggested again: %v", st.Vars)
	}
}

func applyStones(t *testing.T, db *data.DB, st *rpg.State) []string {
	t.Helper()
	r := run(t, db, st, []data.Command{{StoneApply: "o"}})
	var notes []string
	for req := r.Next(); req != nil; req = r.Next() {
		if s, ok := req.(event.Say); ok {
			notes = append(notes, s.Text)
		}
	}
	return notes
}

func TestStoneSetsOfOneNameCountOnce(t *testing.T) {
	_, st := stoneGame(t, map[string]int{"white": 1, "white2": 1, "blue": 1})
	db := st.DB
	if sets := rpg.SetsOf(db, "monk", []string{"white", "white2", "blue"}); len(sets) != 1 {
		t.Errorf("sets = %v", sets)
	}
	// monk に Frost（白か純白と蒼）、残りの白い石は sage の空いた枠に
	best, score, now := st.BestStones()
	worn := append(slices.Clone(best["monk"]), best["sage"]...)
	if score <= now || len(rpg.SetsOf(db, "monk", best["monk"])) != 1 || len(worn) != 3 {
		t.Errorf("best %v (%d, now %d)", best, score, now)
	}
}

func TestStoneSuggestKeepsALoadoutAsStrong(t *testing.T) {
	db, st := stoneGame(t, map[string]int{"white": 1, "blue": 1, "gold": 1})
	monk, sage := st.Actors["monk"], st.Actors["sage"]
	if err := st.Equip(monk, "stone1", "white"); err != nil {
		t.Fatal(err)
	}
	if err := st.Equip(monk, "stone2", "blue"); err != nil {
		t.Fatal(err)
	}
	if err := st.Equip(sage, "stone1", "gold"); err != nil {
		t.Fatal(err)
	}
	suggest(t, db, st)
	if st.Vars["o"] != 0.0 {
		t.Errorf("suggested when nothing is stronger: %v", st.Vars)
	}
	// the vow stone is the sage's alone
	st.AddItem("vow", 1)
	suggest(t, db, st)
	if st.Vars["o"] != 1.0 || !strings.Contains(st.Vars["o_page1"].(string), "Sage") { //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		t.Errorf("vars = %v", st.Vars)
	}
}

func TestStoneDeclineWaitsForTheStonesToChange(t *testing.T) {
	db, st := stoneGame(t, map[string]int{"white": 1, "blue": 1})
	r := run(t, db, st, []data.Command{{StoneSuggest: "o"}, {StoneDecline: "o"}, {StoneSuggest: "o"}})
	if req := r.Next(); req != nil {
		t.Fatal(req)
	}
	if st.Vars["o"] != 0.0 {
		t.Fatalf("suggested again after declining: %v", st.Vars)
	}
	st.AddItem("potion", 1) // not a stone
	suggest(t, db, st)
	if st.Vars["o"] != 0.0 {
		t.Fatalf("suggested again after an item that is no stone: %v", st.Vars)
	}
	st.AddItem("red", 1)
	suggest(t, db, st)
	if st.Vars["o"] == 0.0 {
		t.Errorf("not suggested after the stones changed: %v", st.Vars)
	}
}

func TestStoneEquipOneSet(t *testing.T) {
	db, st := stoneGame(t, map[string]int{"white": 1, "blue": 1, "gold": 2})
	monk := st.Actors["monk"]
	if err := st.Equip(monk, "stone1", "gold"); err != nil {
		t.Fatal(err)
	}
	if err := st.Equip(monk, "stone2", "gold"); err != nil {
		t.Fatal(err)
	}
	st.Vars["o_who"], st.Vars["o_set"] = "monk", "frost"
	equip(t, db, st)
	if len(monk.Sets()) != 1 || st.Items["gold"] != 2 {
		t.Errorf("monk %v bag %v", monk.Equip, st.Items)
	}
}

func TestStoneChooseAndWearer(t *testing.T) {
	db, st := stoneGame(t, map[string]int{"white": 1, "blue": 1, "red": 1, "gold": 1})
	r := run(t, db, st, []data.Command{{StoneChoose: "o", Prompt: "Which?"}})
	ch, ok := r.Next().(event.Choice)
	if !ok || len(ch.Options) != 2 || ch.Options[0] != "Frost（WhiteとBlue）" || ch.Prompt != "Which?" {
		t.Fatalf("choice = %+v", ch)
	}
	r.Answer(1)
	if req := r.Next(); req != nil || st.Vars["o_set"] != "ember" || st.Vars["o_who"] != "sage" {
		t.Fatalf("req %v vars %v", req, st.Vars)
	}
	r = run(t, db, st, []data.Command{{StoneWearer: "o"}})
	ch, ok = r.Next().(event.Choice)
	// Ember is only for the sage
	if !ok || len(ch.Options) != 1 || ch.Options[0] != "Sage" {
		t.Fatalf("wearers = %+v", ch)
	}
}

func TestStoneCommandsAreChecked(t *testing.T) {
	_, err := testgame.TryLoad(map[string]string{"data/s.yaml": `
events:
  bad:
    do:
      - stone_suggest: "a b"
      - equip_stones: {actor: "var.x"}
stone_sets:
  s: {name: S, stones: [stick], stats: {atk: 1}, for: [nobody]}
`})
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"stone_suggest", "actor and set", "nobody"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}
