package balance

import (
	"errors"
	"math"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/nao1215/gozen/internal/drive/headless"
	"github.com/nao1215/gozen/internal/model/rpg"
	"github.com/nao1215/gozen/internal/rules/battle"
	"github.com/nao1215/gozen/internal/testgame"
)

// The numbers must match gozen sim's counting before it was shared (a new game's party row, PCG(seed, i), -level and -tree).
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

func TestOddsArithmetic(t *testing.T) {
	var none Odds
	if none.WinRate() != 0 || none.AvgTurns() != 0 || none.AvgStanding() != 0 {
		t.Errorf("an empty tally is all zeros: %v %v %v", none.WinRate(), none.AvgTurns(), none.AvgStanding())
	}
	if _, _, _, ok := none.HPBand(); ok {
		t.Error("no wins, no band")
	}
	o := Odds{Battles: 8, Won: 4, Turns: 20, Standing: 6, HP: []float64{0.9, 0.1, 0.5, 0.3}}
	if o.WinRate() != 0.5 || o.AvgTurns() != 2.5 || o.AvgStanding() != 1.5 {
		t.Errorf("%v %v %v", o.WinRate(), o.AvgTurns(), o.AvgStanding())
	}
	low, mid, high, ok := o.HPBand()
	if !ok || low != 0.1 || high != 0.9 || mid < 0.3 || mid > 0.5 {
		t.Errorf("band %v %v %v %v", low, mid, high, ok)
	}
	if o.HP[0] != 0.9 {
		t.Error("HPBand sorted the tally's own list")
	}
	one := Odds{HP: []float64{0.4}}
	if l, m, h, ok := one.HPBand(); !ok || l != 0.4 || m != 0.4 || h != 0.4 {
		t.Errorf("one win: %v %v %v", l, m, h)
	}
}

// TestTryAndAddTallyEveryKindOfEnd: a battle won, lost, escaped or cut off by the turn limit goes to its own count.
func TestTryAndAddTallyEveryKindOfEnd(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/hard.yaml": hardData})
	var o Odds
	var seen int
	for _, tc := range []struct {
		troop string
		turns int
	}{{"one", 30}, {"hard", 30}, {"hard", 0}} {
		st := testgame.State(db, 4)
		b := Try(db, st, tc.troop, tc.turns, func(*battle.Battle, []battle.Step) { seen++ })
		o.Add(b)
	}
	if o.Battles != 3 || o.Won != 1 || o.Lost != 1 || o.Draw != 1 || o.Fled != 0 {
		t.Errorf("%+v", o)
	}
	if len(o.HP) != 1 || o.HP[0] <= 0 || o.HP[0] > 1 {
		t.Errorf("one win with some HP left: %v", o.HP)
	}
	if seen == 0 {
		t.Error("the observer saw no steps")
	}
	var fled Odds
	fled.Add(&battle.Battle{Result: battle.Fled, Turn: 2})
	if fled.Fled != 1 || fled.Turns != 2 {
		t.Errorf("%+v", fled)
	}
}

func TestPrepRefusesUnknownGearAndLevelsUp(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	if err := (Prep{Gear: []string{"ghost"}}).apply(db, st); err == nil {
		t.Error("unknown gear")
	}
	if err := (Prep{Level: 5, Tree: true, Gear: []string{"blade"}}).apply(db, st); err != nil {
		t.Fatal(err)
	}
	a := st.Members()[0]
	if a.Level != 5 || a.HP != a.Stat("mhp") {
		t.Errorf("level %d hp %d/%d: level-up heals", a.Level, a.HP, a.Stat("mhp"))
	}
	if a.Equip["weapon"] != "blade" {
		t.Errorf("the best gear is worn: %v", a.Equip)
	}
}

func TestSimStopsOnABadStartAndSeesEveryBattle(t *testing.T) {
	db := testgame.Load(t, nil)
	boom := errors.New("no start")
	if _, err := Sim(db, "one", 3, 10, func(int) (*rpg.State, error) { return nil, boom }, nil); !errors.Is(err, boom) {
		t.Errorf("a failing start: %v", err)
	}
	if _, err := Sim(db, "one", 3, 10, FreshStart(db, 1, nil, Prep{Gear: []string{"ghost"}}), nil); err == nil {
		t.Error("a start that cannot be prepared")
	}
	seen := map[int]bool{}
	o, err := Sim(db, "one", 4, 10, FreshStart(db, 2, nil, Prep{}), func(i int, _ *battle.Battle, _ []battle.Step) { seen[i] = true })
	if err != nil || o.Battles != 4 || len(seen) != 4 {
		t.Errorf("%+v %v %v", o, err, seen)
	}
	if math.IsNaN(o.WinRate()) {
		t.Error("NaN win rate")
	}
}
