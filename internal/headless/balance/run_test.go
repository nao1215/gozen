package balance

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

const shopData = `
shops:
  smith: {goods: [blade, stick, hat, potion, key]}
`

// TestRunIsDeterministic: the same Seed and config give the same floors (same Seed -> same result).
func TestRunIsDeterministic(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/shop.yaml": shopData})
	cfg := Config{Location: "field", Floors: 4, Battles: 4, Tries: 4, Seed: 11, Bosses: map[int]string{3: "boss"}, Guards: map[int]string{2: "two"}, Shops: []string{"smith"}}
	a, err := Run(db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Run(db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("the same seed gave different curves:\n%+v\n%+v", a, b)
	}
	if len(a.Floors) != 4 {
		t.Fatalf("want 4 floors, got %d", len(a.Floors))
	}
	for i, f := range a.Floors {
		if f.Floor != i+1 {
			t.Errorf("floor %d numbered %d", i+1, f.Floor)
		}
		if f.Battles > cfg.Battles {
			t.Errorf("floor %d fought %d battles, more than the %d asked", f.Floor, f.Battles, cfg.Battles)
		}
		for _, r := range []float64{f.BossRate, f.BossRateEnd, f.GuardRate, f.GuardRateEnd, f.HPLeft, f.MPLeft} {
			if r < 0 || r > 1.0000001 {
				t.Errorf("floor %d has a rate outside 0..1: %+v", f.Floor, f)
				break
			}
		}
		if f.Lost > f.Battles {
			t.Errorf("floor %d lost more battles than it fought", f.Floor)
		}
	}
	if a.Floors[2].Boss != "boss" || a.Floors[1].Guard != "two" || a.Floors[0].Boss != "" {
		t.Errorf("boss and guard land on their floors: %+v", a.Floors)
	}
	if a.Order[0] != "scout" || a.Names["scout"] != "Scout" || a.Level["scout"] < 1 {
		t.Errorf("the party is reported: %+v %+v %+v", a.Order, a.Names, a.Level)
	}
}

// TestRunDefaultsAndBadExpressions: the zero Config means 20 floors; a broken expression is an error.
func TestRunDefaultsAndBadExpressions(t *testing.T) {
	db := testgame.Load(t, nil)
	res, err := Run(db, Config{Location: "field", Battles: 1, Seed: 1})
	if err != nil || len(res.Floors) != 20 {
		t.Fatalf("floors %d err %v, want 20 floors by default", len(res.Floors), err)
	}
	if _, err := Run(db, Config{Location: "field", RestCost: "1 +"}); err == nil {
		t.Error("a broken RestCost must fail")
	}
	if _, err := Run(db, Config{Location: "field", SceneEvent: "sub", SceneWhen: "(("}); err == nil {
		t.Error("a broken SceneWhen must fail")
	}
}

// TestRestCostSuppliesAndBackItem: resting costs gold (or goes unpaid), supplies are bought up to the stock wanted,
// and going back without the return item is stranding.
func TestRestCostSuppliesAndBackItem(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/shop.yaml": shopData})
	rich := Config{Location: "field", Floors: 3, Battles: 3, Tries: 2, Seed: 5, RestCost: "5", Supplies: map[string]int{"potion": 4, "missing": 3}, BackItem: "key"}
	res, err := Run(db, rich)
	if err != nil {
		t.Fatal(err)
	}
	spent, stranded := 0, 0
	for _, f := range res.Floors {
		spent += f.Spent
		stranded += f.Stranded
	}
	if spent == 0 {
		t.Error("resting with a cost and supplies spent no gold")
	}
	if stranded == 0 && res.Floors[0].Gold > 0 {
		t.Log("every return found a return item (the mote drops a key)")
	}
	broke := rich
	broke.RestCost = "1000000"
	res, err = Run(db, broke)
	if err != nil {
		t.Fatal(err)
	}
	unpaid := 0
	for _, f := range res.Floors {
		unpaid += f.Unpaid
		if f.Spent >= 1000000 {
			t.Errorf("paid a cost larger than the purse: %+v", f)
		}
	}
	if unpaid == 0 {
		t.Error("a rest that costs more than the purse must be counted unpaid")
	}
}

// TestGrindRaisesTheOddsOrGivesUp: with a Grind target the run fights until the boss rate is reached (at most 200 rounds).
func TestGrindRaisesTheOddsOrGivesUp(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/shop.yaml": shopData})
	res, err := Run(db, Config{Location: "field", Floors: 2, Battles: 2, Tries: 4, Seed: 3, Bosses: map[int]string{2: "boss"}, Grind: 0.9, Shops: []string{"smith"}})
	if err != nil {
		t.Fatal(err)
	}
	f := res.Floors[1]
	if f.Boss != "boss" || f.Grinding > 200 || f.Grinding < 0 {
		t.Fatalf("%+v", f)
	}
	if f.BossRateEnd < f.BossRate-1e-9 && f.Grinding == 0 {
		t.Errorf("no grinding yet the rate changed: %+v", f)
	}
	if f.Grinding > 0 && f.BossRateEnd == 0 && f.BossRate == 0 && f.Grinding < 200 {
		t.Errorf("stopped grinding at rate 0 before the limit: %+v", f)
	}
}

// TestGuardsRetreatAndGuardsOnly covers the guard (a mid-floor boss) with and without retreat.
func TestGuardsRetreatAndGuardsOnly(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/shop.yaml": shopData})
	base := Config{Location: "field", Floors: 2, Battles: 1, Tries: 3, Seed: 2, Guards: map[int]string{1: "boss", 2: "one"}, Shops: []string{"smith"}}
	retreat := base
	retreat.Retreat, retreat.Grind, retreat.NoTown = true, 0.95, false
	res, err := Run(db, retreat)
	if err != nil {
		t.Fatal(err)
	}
	f := res.Floors[0]
	if f.Guard != "boss" {
		t.Fatalf("%+v", f)
	}
	if f.GuardRate < 0.95 && !f.GuardBack {
		t.Errorf("a guard that is too hard sends the party back: %+v", f)
	}
	if f.GuardRateEnd < f.GuardRate-1e-9 && f.Grinding == 0 {
		t.Errorf("rate went down without fighting: %+v", f)
	}
	only := base
	only.GuardsOnly = true
	only.NoTown = true
	res, err = Run(db, only)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Floors {
		if f.GuardLost != 0 {
			t.Errorf("GuardsOnly measures the rate only and never fights the guard: %+v", f)
		}
	}
}

// TestWornPartyGoesBack: with RestEvery and a worn party the run goes back early; the floor counts the backs.
func TestWornPartyGoesBack(t *testing.T) {
	db := testgame.Load(t, nil)
	res, err := Run(db, Config{Location: "field", Floors: 6, Battles: 6, Tries: 2, Seed: 4, RestEvery: 3, WornHP: 0.99, WornMP: 0.99})
	if err != nil {
		t.Fatal(err)
	}
	backs, rested := 0, 0
	for _, f := range res.Floors {
		backs += f.Backs
		if f.Rested {
			rested++
		}
	}
	if backs == 0 {
		t.Error("a party worn under 99 percent never went back")
	}
	if rested == 0 {
		t.Error("nobody rested over six floors with RestEvery 3")
	}
	if !res.Floors[2].Rested {
		t.Errorf("floor 3 is a scheduled rest: %+v", res.Floors[2])
	}
}

// TestSceneEventCountsScenes: SceneEvent fires after a won battle when SceneWhen holds.
func TestSceneEventCountsScenes(t *testing.T) {
	db := testgame.Load(t, nil)
	res, err := Run(db, Config{Location: "field", Floors: 1, Battles: 3, Tries: 2, Seed: 4, SceneEvent: "sub", SceneWhen: "true"})
	if err != nil {
		t.Fatal(err)
	}
	f := res.Floors[0]
	if f.Scenes == 0 || f.Scenes > f.Battles {
		t.Errorf("scenes %d for %d battles", f.Scenes, f.Battles)
	}
}

// TestUnusedSkills: Unused lists the skills a character knows and never used.
func TestUnusedSkills(t *testing.T) {
	r := Result{
		Known: map[string][]string{"a": {"attack", "fire", "heal"}, "b": {"attack"}},
		Used:  map[string]map[string]int{"a": {"attack": 3}, "b": {"attack": 1}},
	}
	got := r.Unused()
	if len(got) != 1 || strings.Join(got["a"], ",") != "fire,heal" {
		t.Errorf("Unused = %v", got)
	}
}

// TestRatesNow: WinRateNow and DuelRateNow are fractions of tries, repeatable, and 0 for no tries.
func TestRatesNow(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 9)
	for _, f := range []func() (float64, error){
		func() (float64, error) { return WinRateNow(db, st, "one", 5, 0) },
		func() (float64, error) { return DuelRateNow(db, st, "one", []string{"scout"}, 5, 0) },
	} {
		a, err := f()
		if err != nil {
			t.Fatal(err)
		}
		b, err := f()
		if err != nil {
			t.Fatal(err)
		}
		if a != b || a < 0 || a > 1 {
			t.Errorf("rates %v %v", a, b)
		}
		if a*5 != float64(int(a*5+0.5)) {
			t.Errorf("a rate over 5 tries must be a multiple of 0.2, got %v", a)
		}
	}
	if r, err := WinRateNow(db, st, "one", 0, 10); err != nil || r != 0 {
		t.Errorf("zero tries gives %v %v", r, err)
	}
	if r, err := DuelRateNow(db, st, "one", nil, 0, 10); err != nil || r != 0 {
		t.Errorf("zero tries gives %v %v", r, err)
	}
	strong := testgame.State(db, 9)
	for _, a := range strong.Members() {
		a.GainExp(1_000_000)
	}
	if r, err := WinRateNow(db, strong, "one", 5, 30); err != nil || r != 1 {
		t.Errorf("a maxed party loses to a mote: %v", r)
	}
	// the state measured is not changed
	before, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WinRateNow(db, st, "boss", 3, 5); err != nil {
		t.Fatal(err)
	}
	after, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("measuring a win rate changed the state it measured")
	}
}
