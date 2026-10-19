package battle_test

import (
	"math"
	"testing"

	"github.com/nao1215/gozen/battle"
	"github.com/nao1215/gozen/internal/testgame"
)

// Three multipliers whose product lands on a rounding boundary: 25 × 0.7 × 1.2 × 1.5 is
// 31.5 in exact arithmetic, and the float result depends on the order of the products.
const orderStates = `
states:
  s_a: {name: A, turns: 3, stats: {atk: 0.7}, state_resist: {sleep: 0.7}}
  s_b: {name: B, turns: 3, stats: {atk: 1.2}, state_resist: {sleep: 1.2}}
  s_c: {name: C, turns: 3, stats: {atk: 1.5}, state_resist: {sleep: 1.5}}
enemies:
  dummy:
    name: Dummy
    image: x.png
    stats: {mhp: 99, mmp: 0, atk: 25, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}
    ai: [{use: guard}]
troops:
  dummy: {members: [dummy]}
`

// State multipliers are applied in state ID order, so a stat is the same on every call
// and every run (map iteration order used to decide the rounding).
func TestStatMultipliersHaveAFixedOrder(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/order.yaml": orderStates})
	st := testgame.State(db, 1)
	b := battle.New(db, st, "dummy")
	foe := b.Foes[0]
	for _, s := range []string{"s_c", "s_a", "s_b"} {
		foe.States[s] = 3
	}
	v := 25.0
	for _, r := range []float64{0.7, 1.2, 1.5} {
		v *= r
	}
	want := int(math.Round(v))
	for range 200 {
		if got := foe.Stat("atk"); got != want {
			t.Fatalf("atk = %d, want %d", got, want)
		}
	}

	a := st.Members()[0]
	for _, s := range []string{"s_c", "s_a", "s_b"} {
		a.States[s] = 3
	}
	r := 1.0
	for _, f := range []float64{0.7, 1.2, 1.5} {
		r *= f
	}
	for range 200 {
		if got := a.StateResist("sleep"); got != r {
			t.Fatalf("state resist = %v, want %v", got, r)
		}
	}
}
