package rpg_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// The amount bought is added to the variable written in the shop's spent. If two shops hold the
// same variable, they are counted together.
func TestBuyCountsSpent(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/shops.yaml": `
shops:
  goods: {name: Goods, goods: [potion], spent: var.spent_here}
  arms: {name: Arms, goods: [blade], spent: var.spent_here}
  other: {name: Other, goods: [potion]}
`})
	st := testgame.State(db, 1)
	st.Gold = 200
	if !st.Buy("goods", "potion", 3) {
		t.Fatal("cannot buy 3 potions with 200")
	}
	if !st.Buy("arms", "blade", 1) {
		t.Fatal("cannot buy the blade")
	}
	if !st.Buy("other", "potion", 1) {
		t.Fatal("cannot buy at the shop without spent")
	}
	if st.Gold != 200-30-100-10 {
		t.Errorf("gold = %d", st.Gold)
	}
	if got := st.Vars["spent_here"]; got != 130.0 {
		t.Errorf("spent_here = %v, want 130 (the potions and the blade, not the other shop)", got)
	}
	if st.Buy("arms", "blade", 1) { // only 50 remains
		t.Error("bought the blade with too little money")
	}
	if got := st.Vars["spent_here"]; got != 130.0 || st.Gold != 60 {
		t.Errorf("a failed purchase changed spent %v / gold %d", got, st.Gold)
	}
}

func TestShopSpentMustBeVar(t *testing.T) {
	_, err := testgame.TryLoad(map[string]string{"data/shops.yaml": `
shops:
  goods: {name: Goods, goods: [potion], spent: flag.spent}
`})
	if err == nil || !strings.Contains(err.Error(), "shops.goods.spent") {
		t.Errorf("spent: flag.spent was not reported: %v", err)
	}
}
