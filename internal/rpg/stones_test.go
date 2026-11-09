package rpg_test

import (
	"slices"
	"testing"

	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

const stoneGameData = `
classes:
  mage:
    name: Mage
    stats: {mhp: "30", mmp: "10", atk: "5", def: "5", mat: "10", mdf: "5", agi: "10", luk: "5"}
    exp_for: "level * level * 10"
    equip: [sword, stone]
  clerk:
    name: Clerk
    stats: {mhp: "20", mmp: "5", atk: "2", def: "2", mat: "2", mdf: "2", agi: "5", luk: "2"}
    exp_for: "level * 10"
    equip: [sword]
actors:
  sage: {name: Sage, class: mage}
  monk: {name: Monk, class: mage}
  clerk: {name: Clerk, class: clerk}
items:
  white: {name: White Stone, kind: stone, slot: stone, stats: {def: 2}}
  white2: {name: Pure Stone, kind: stone, slot: stone, stats: {def: 6}}
  blue: {name: Blue Stone, kind: stone, slot: stone, stats: {mat: 3, mhp: 10}}
  red: {name: Red Stone, kind: stone, slot: stone, stats: {atk: 3}}
stone_sets:
  frost: {family: frost, name: Frost, stones: [white, blue], stats: {mdf: 4}}
  frost_up: {family: frost, name: Frost, stones: [white2, blue], stats: {mdf: 4}}
  ember: {name: Ember, stones: [red, red], stats: {atk: 5}, for: [sage], only: true}
`

func stoneState(t *testing.T) *rpg.State {
	t.Helper()
	db := testgame.Load(t, map[string]string{
		"game.yaml":        testgame.Game + "equip_slots:\n  - {id: weapon, name: W}\n  - {id: armor, name: A}\n  - {id: stone1, name: S1}\n  - {id: stone2, name: S2}\n",
		"data/stones.yaml": stoneGameData,
	})
	st := testgame.State(db, 1)
	st.Join("sage")
	st.Join("monk")
	st.Join("clerk")
	return st
}

func TestSetFamilyAndWearers(t *testing.T) {
	st := stoneState(t)
	if got := st.SetFamily("frost"); !slices.Equal(got, []string{"frost", "frost_up"}) {
		t.Errorf("a family is its sets in data order: %v", got)
	}
	if got := st.SetFamily("ember"); !slices.Equal(got, []string{"ember"}) {
		t.Errorf("%v", got)
	}
	if st.SetFamily("nothing") != nil {
		t.Error("an unknown set has no family")
	}
	if st.SetHolds("frost") {
		t.Error("nobody wears stones yet")
	}
	st.AddItem("white", 1)
	st.AddItem("blue", 1)
	got := st.SetWearers("frost")
	ids := make([]string, len(got))
	for i, a := range got {
		ids[i] = a.ID
	}
	if !slices.Contains(ids, "sage") || !slices.Contains(ids, "monk") || slices.Contains(ids, "clerk") {
		t.Errorf("the mages can wear it, the clerk cannot: %v", ids)
	}
	for _, a := range got {
		if a.ID == "clerk" {
			t.Error("a class that wears no stones cannot wear a set")
		}
	}
	if w := st.SetWearers("ember"); len(w) != 0 {
		t.Errorf("without two red stones nobody can wear ember: %v", w)
	}
	st.AddItem("red", 2)
	if w := st.SetWearers("ember"); len(w) != 1 || w[0].ID != "sage" {
		t.Errorf("a dedicated set is for its members only: %v", w)
	}
	if st.SetWearers("nothing") != nil {
		t.Error("an unknown set has no wearers")
	}
}

func TestEquipSetMovesStonesAndRaisesHP(t *testing.T) {
	st := stoneState(t)
	sage, monk := st.Actors["sage"], st.Actors["monk"]
	st.AddItem("white", 1)
	st.AddItem("blue", 1)
	hp, mhp := sage.HP, sage.Stat("mhp")
	if err := st.EquipSet(sage, "frost"); err != nil {
		t.Fatal(err)
	}
	if sage.Equip["stone1"] == "" || sage.Equip["stone2"] == "" {
		t.Fatalf("the stones are on: %v", sage.Equip)
	}
	if st.Items["white"] != 0 || st.Items["blue"] != 0 {
		t.Errorf("the stones left the bag: %v", st.Items)
	}
	if sage.Stat("mhp") != mhp+10 || sage.HP != hp+10 {
		t.Errorf("max HP %d -> %d, HP %d -> %d: HP rises by what the maximum rose", mhp, sage.Stat("mhp"), hp, sage.HP)
	}
	if !st.SetHolds("frost") || !slices.Contains(sage.Sets(), "frost") {
		t.Error("the set now works")
	}
	// the monk wants it too: the sage's stones count as locked in a working set, so the monk cannot take them
	if err := st.EquipSet(monk, "frost"); err == nil {
		t.Error("a stone worn in a working set cannot be taken")
	}
	// stones worn outside a working set can be taken by another member
	st.AddItem("red", 1)
	if err := st.Equip(monk, "stone1", "red"); err != nil {
		t.Fatal(err)
	}
	st.AddItem("red", 1)
	if err := st.EquipSet(st.Actors["clerk"], "frost"); err == nil {
		t.Error("the clerk wears no stones")
	}
	if n, taken, ok := st.StonePlanFor(sage, "frost"); !ok || n != 0 || taken != 0 {
		t.Errorf("already wearing the set: puts %d taken %d ok %v", n, taken, ok)
	}
	if _, _, ok := st.StonePlanFor(sage, "nothing"); ok {
		t.Error("a set that does not exist cannot be planned")
	}
	if _, _, ok := st.StonePlanFor(st.Actors["clerk"], "frost"); ok {
		t.Error("the clerk cannot wear stones")
	}
}

func TestEquipSetTakesSpareStonesFromOthers(t *testing.T) {
	st := stoneState(t)
	sage, monk := st.Actors["sage"], st.Actors["monk"]
	st.AddItem("red", 2)
	if err := st.Equip(monk, "stone1", "red"); err != nil {
		t.Fatal(err)
	}
	if err := st.Equip(monk, "stone2", "red"); err != nil {
		t.Fatal(err)
	}
	// ember is Ember: red + red, for the sage; the monk wears them outside a working set (ember is the sage's)
	if st.Items["red"] != 0 {
		t.Fatalf("the reds are worn: %v", st.Items)
	}
	puts, taken, ok := st.StonePlanFor(sage, "ember")
	if !ok || puts != 2 || taken != 2 {
		t.Fatalf("plan: puts %d taken %d ok %v", puts, taken, ok)
	}
	if err := st.EquipSet(sage, "ember"); err != nil {
		t.Fatal(err)
	}
	if monk.Equip["stone1"] != "" || monk.Equip["stone2"] != "" {
		t.Errorf("the monk's spare stones were taken: %v", monk.Equip)
	}
	if sage.Equip["stone1"] != "red" || sage.Equip["stone2"] != "red" {
		t.Errorf("the sage's: %v", sage.Equip)
	}
}

func TestStoneOffersAreOnePerFamilyBestFirst(t *testing.T) {
	st := stoneState(t)
	if offers := st.StoneOffers(); len(offers) != 0 {
		t.Errorf("no stones, no offers: %v", offers)
	}
	st.AddItem("white", 1)
	st.AddItem("white2", 1)
	st.AddItem("blue", 1)
	st.AddItem("red", 2)
	offers := st.StoneOffers()
	if len(offers) != 2 {
		t.Fatalf("one offer per family: %+v", offers)
	}
	for _, o := range offers {
		if o.Set == "" || o.Actor == "" {
			t.Errorf("%+v", o)
		}
	}
	if offers[0].Score < offers[1].Score {
		t.Errorf("the best offer comes first: %+v", offers)
	}
	var frost string
	for _, o := range offers {
		if st.SetFamily(o.Set)[0] == "frost" {
			frost = o.Set
		}
	}
	if frost != "frost_up" {
		t.Errorf("of the family the set with the stronger stones is offered, got %q", frost)
	}
	// once somebody works a set of the family, it is not offered again
	if err := st.EquipSet(st.Actors["sage"], frost); err != nil {
		t.Fatal(err)
	}
	for _, o := range st.StoneOffers() {
		if o.Set == "frost" || o.Set == "frost_up" {
			t.Errorf("a family that is worked is not offered: %+v", o)
		}
	}
}
