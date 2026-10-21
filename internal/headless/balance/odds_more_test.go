package balance

import (
	"errors"
	"math"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

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
