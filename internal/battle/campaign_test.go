package battle_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestUnitTroops: in a campaign's battle a general's troops are self.troops, and a foe's
// are the garrison of the region marched on (ADR 0030); outside a campaign both are 0.
func TestUnitTroops(t *testing.T) {
	db := testgame.LoadCampaign(t, nil)
	st := testgame.State(db, 1)
	st.Vars["campaign_target"] = "keep"
	b := battle.New(db, st, "one")
	if v, _ := b.Party[0].Field("troops"); v != 400.0 {
		t.Fatalf("the scout's troops: %v", v)
	}
	if v, _ := b.Foes[0].Field("troops"); v != 500.0 {
		t.Fatalf("the foe's troops: %v", v)
	}
	plain := testgame.Load(t, nil)
	if v, _ := battle.New(plain, testgame.State(plain, 1), "one").Party[0].Field("troops"); v != 0.0 {
		t.Fatalf("outside a campaign: %v", v)
	}
}
