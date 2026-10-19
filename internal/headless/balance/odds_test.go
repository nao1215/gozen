package balance

import (
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/nao1215/gozen/battle"
	"github.com/nao1215/gozen/headless"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

// 共有化の前の gozen sim の数え方（新しいゲームの一行、PCG(seed, i)、-level・-tree）と同じ数になること。
func TestSimMatchesTheFormerCommandLoop(t *testing.T) {
	db := testgame.Load(t, nil)
	for _, tc := range []struct {
		troop string
		level int
		party []string
	}{{"one", 0, nil}, {"two", 1, nil}, {"boss", 4, []string{"scout", "guide"}}} {
		const n, seed, turns = 40, 9, 30
		got, err := Sim(db, tc.troop, n, turns, FreshStart(db, seed, tc.party, Prep{Level: tc.level, Tree: true}), nil)
		if err != nil {
			t.Fatal(err)
		}
		var want Odds
		want.Troop = tc.troop
		for i := range n {
			st := rpg.New(db, rand.New(rand.NewPCG(seed, uint64(i)))) //nolint:gosec // Deterministic test.
			if tc.party != nil {
				st.Party = nil
				for _, id := range tc.party {
					st.Join(id)
				}
			}
			if tc.level > 0 {
				for _, a := range st.Members() {
					a.GainExp(a.ExpFor(tc.level) - a.Exp)
					a.Recover()
				}
			}
			for _, a := range st.Members() {
				a.SpendSP()
			}
			b := battle.New(db, st, tc.troop)
			headless.AutoBattle(db, st, b, nil, turns, nil)
			want.Battles++
			want.Turns += b.Turn
			switch b.Result {
			case battle.Won:
				want.Won++
				for _, u := range b.Party {
					if u.Alive() {
						want.Standing++
					}
				}
			case battle.Lost:
				want.Lost++
			case battle.Fled:
				want.Fled++
			case battle.Ongoing:
				want.Draw++
			}
		}
		got.HP = nil
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %+v want %+v", tc.troop, got, want)
		}
	}
}

func TestSavedStartHealsAndRepeats(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 3)
	st.Members()[0].HP = 1
	save, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	start := SavedStart(db, save, 1, Prep{})
	c, err := start(0)
	if err != nil {
		t.Fatal(err)
	}
	if a := c.Members()[0]; a.HP != a.Stat("mhp") {
		t.Fatalf("not healed: %d/%d", a.HP, a.Stat("mhp"))
	}
	a, err := Sim(db, "two", 30, 60, start, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Sim(db, "two", 30, 60, SavedStart(db, save, 1, Prep{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) || a.Battles != 30 || len(a.HP) != a.Won {
		t.Fatalf("%+v %+v", a, b)
	}
	if _, err := SavedStart(db, []byte("{"), 1, Prep{})(0); err == nil {
		t.Fatal("broken save accepted")
	}
	if _, err := FreshStart(db, 1, nil, Prep{Gear: []string{"absent"}})(0); err == nil {
		t.Fatal("unknown gear accepted")
	}
	if _, err := FreshStart(db, 1, []string{"absent"}, Prep{})(0); err == nil {
		t.Fatal("unknown actor accepted")
	}
}

func TestHereAndBand(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/more.yaml": `locations:
  ring:
    name: Ring
    encounters: {rate: "1", troops: [{troop: one, weight: 3}, {troop: two}, {troop: boss, when: "flag.late"}]}
    actions: [{label: Duel, do: [{battle: boss}]}, {label: Hidden, when: "flag.never", do: [{battle: two}]}]
    exits: [{to: camp}]
`})
	st := testgame.State(db, 1)
	st.Location = "ring"
	got := Here(db, st, "one", "absent", "two")
	want := []Encounter{{"one", 0.75}, {"two", 0.25}, {"boss", 0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
	st.Location = "vault"
	if got := Here(db, st, "boss"); !reflect.DeepEqual(got, []Encounter{{"boss", 0}}) {
		t.Fatalf("%+v", got)
	}
	if _, _, _, ok := (Odds{}).HPBand(); ok {
		t.Fatal("band without wins")
	}
	o := Odds{HP: []float64{0.9, 0.1, 0.5, 0.3, 0.7, 0.2, 0.4, 0.6, 0.8, 1, 0}}
	if lo, mid, hi, _ := o.HPBand(); lo != 0.1 || mid != 0.5 || hi != 0.9 {
		t.Fatalf("band %v %v %v", lo, mid, hi)
	}
	if (Odds{}).WinRate() != 0 || (Odds{}).AvgTurns() != 0 {
		t.Fatal("empty odds")
	}
}

func TestAsIsAndStreak(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 3)
	a := st.Members()[0]
	a.HP, a.MP = 9, 0
	a.States["poison"] = 3
	save, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	c, err := SavedStart(db, save, 1, Prep{AsIs: true})(0)
	if err != nil {
		t.Fatal(err)
	}
	if m := c.Members()[0]; m.HP != 9 || m.MP != 0 || m.States["poison"] != 3 {
		t.Fatalf("as-is start changed the party: %d %d %v", m.HP, m.MP, m.States)
	}
	hurt, err := Sim(db, "two", 40, 60, SavedStart(db, save, 1, Prep{AsIs: true}), nil)
	if err != nil {
		t.Fatal(err)
	}
	healed, err := Sim(db, "two", 40, 60, SavedStart(db, save, 1, Prep{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if hurt.WinRate() >= healed.WinRate() {
		t.Fatalf("hurt %v healed %v", hurt.WinRate(), healed.WinRate())
	}
	full, err := Streak(db, "one", 20, 60, 6, SavedStart(db, save, 1, Prep{}))
	if err != nil {
		t.Fatal(err)
	}
	low, err := Streak(db, "one", 20, 60, 6, SavedStart(db, save, 1, Prep{AsIs: true}))
	if err != nil {
		t.Fatal(err)
	}
	if Percentile(full, 0.5) <= Percentile(low, 0.5) || Percentile(full, 1) > 6 {
		t.Fatalf("full %v low %v", full, low)
	}
	if Percentile(nil, 0.5) != 0 || Percentile([]int{4, 1, 3}, 0.5) != 3 {
		t.Fatal("percentile")
	}
}
