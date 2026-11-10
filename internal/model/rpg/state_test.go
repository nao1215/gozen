package rpg_test

import (
	"math/rand/v2"
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestPartyRatios: HP and MP ratios count a fallen member as zero, levels are averaged
// down, and an empty party divides by one instead of by zero.
func TestPartyRatios(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Join("guide")
	members := st.Members()
	if len(members) < 2 {
		t.Fatalf("want a party of two, got %d", len(members))
	}
	for i, a := range members {
		a.Level = 3 + i*2 // stats depend on the level, so set it before summing them
	}
	mhp, mmp := 0, 0
	for _, a := range members {
		mhp += a.Stat("mhp")
		mmp += a.Stat("mmp")
	}
	members[0].HP, members[0].MP = -5, 0
	members[1].HP, members[1].MP = members[1].Stat("mhp"), members[1].Stat("mmp")
	if got, want := st.PartyHPRatio(), float64(members[1].Stat("mhp"))/float64(mhp); got != want {
		t.Errorf("PartyHPRatio = %v, want %v", got, want)
	}
	if got, want := st.PartyMPRatio(), float64(members[1].Stat("mmp"))/float64(max(1, mmp)); got != want {
		t.Errorf("PartyMPRatio = %v, want %v", got, want)
	}
	if got := st.AverageLevel(); got != 4 {
		t.Errorf("AverageLevel = %d, want 4", got)
	}
	empty := &rpg.State{}
	if empty.PartyHPRatio() != 0 || empty.PartyMPRatio() != 1 || empty.AverageLevel() != 0 {
		t.Error("an empty party must report no HP, no MP spent and level zero")
	}
	// A game without magic has no MP to spend: its party is never worn by MP.
	magicless := testgame.Load(t, map[string]string{"data/z.yaml": "items: {mute: {name: Mute cap, kind: hat, slot: armor, stats: {mmp: -1000}}}\n"})
	quiet := testgame.State(magicless, 1)
	for _, a := range quiet.Members() {
		a.Equip["armor"], a.MP = "mute", 0
	}
	if got := quiet.PartyMPRatio(); got != 1 {
		t.Errorf("PartyMPRatio without MP = %v, want 1", got)
	}
}

// TestRecruitIDIsIndependentOfOtherStates: two games on the same loaded definitions
// number their monsters alone, and neither writes its instances into the definitions.
func TestRecruitIDIsIndependentOfOtherStates(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	first, second := testgame.State(db, 1), testgame.State(db, 1)
	a, _ := first.Recruit("mote", 1, 0, nil)
	b, _ := second.Recruit("mote", 1, 0, nil)
	if a != b {
		t.Fatalf("the same new game recruited %q then %q: the first game's monster leaked into the shared DB", a, b)
	}
	if db.Actors[a] != nil {
		t.Fatalf("the loaded definitions hold the instance %q", a)
	}
	if first.Actors[a].Name() != "Mote" || !first.ReleaseMonster(a) || second.Actors[b] == nil || second.Actors[b].Name() != "Mote" {
		t.Fatal("releasing one game's monster changed the other's")
	}
}

// TestLoadedMonsterKeepsItsOwnSpecies: a save's monster is defined by its own species,
// not by a monster of the same id another game on the same definitions recruited.
func TestLoadedMonsterKeepsItsOwnSpecies(t *testing.T) {
	earlier := testgame.Load(t, testgame.MonsterFiles())
	old := testgame.State(earlier, 1)
	id, _ := old.Recruit("boss", 1, 0, nil)
	saved, err := old.Save()
	if err != nil {
		t.Fatal(err)
	}
	db := testgame.Load(t, testgame.MonsterFiles())
	now := testgame.State(db, 1)
	if other, _ := now.Recruit("mote", 1, 0, nil); other != id {
		t.Fatalf("setup: %q != %q", other, id)
	}
	got, err := rpg.Parse(db, saved, rand.New(rand.NewPCG(1, 1))) //nolint:gosec // Fixed seed.
	if err != nil {
		t.Fatal(err)
	}
	if name := got.Actors[id].Name(); name != "Sentinel" {
		t.Fatalf("the loaded boss is named %q: it took the definition of the other game's monster", name)
	}
}

// TestLoadedHPAndMPAreWithinTheirLimits: a save whose HP is above the member's maximum
// (after an update lowered the stats or dropped equipment) or whose MP is negative
// loads clamped.
func TestLoadedHPAndMPAreWithinTheirLimits(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	st := testgame.State(db, 1)
	id := st.Party[0]
	st.Actors[id].HP = st.Actors[id].Stat("mhp") + 9000
	st.Actors[id].MP = -5
	saved, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	got, err := rpg.Parse(db, saved, rand.New(rand.NewPCG(1, 1))) //nolint:gosec // Fixed seed.
	if err != nil {
		t.Fatal(err)
	}
	if a := got.Actors[id]; a.HP != a.Stat("mhp") || a.MP != 0 {
		t.Fatalf("loaded HP %d of %d, MP %d", a.HP, a.Stat("mhp"), a.MP)
	}
}

// TestSetPathRefusesNonFiniteVariables: NaN and the infinities (the square root of a
// negative, an overflowing power) are refused, since JSON cannot keep them and every
// later Save would fail.
func TestSetPathRefusesNonFiniteVariables(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	for _, src := range []string{"sqrt(0 - 1)", "pow(10, 400)"} {
		st := testgame.State(db, 1)
		v, err := testgame.Expr(src).Eval(st.Env())
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SetPath("var.x", v); err == nil {
			t.Errorf("set var.x: %s was accepted", src)
		}
		if _, ok := st.Vars["x"]; ok {
			t.Errorf("set var.x: %s was kept", src)
		}
		if _, err := st.Save(); err != nil {
			t.Errorf("after set var.x: %s the game cannot be saved: %v", src, err)
		}
	}
}

// TestTakeEmptiesSlotsInTheirOrder: taking one of an item worn in two slots empties the
// first slot of game.yaml, not whichever the map gives first, so a replay takes the same.
func TestTakeEmptiesSlotsInTheirOrder(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Slots = []data.Slot{{ID: "weapon1"}, {ID: "weapon2"}, {ID: "weapon3"}, {ID: "weapon4"}}
	st := testgame.State(db, 1)
	a := st.Actors[st.Party[0]]
	for range 40 {
		a.Equip = map[string]string{"weapon1": "stick", "weapon2": "stick", "weapon3": "stick", "weapon4": "stick"}
		st.Take("stick", 2)
		if len(a.Equip) != 2 || a.Equip["weapon3"] != "stick" || a.Equip["weapon4"] != "stick" {
			t.Fatalf("after taking two, the slots left are %v, want weapon3 and weapon4", a.Equip)
		}
	}
}
