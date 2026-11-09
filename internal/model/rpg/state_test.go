package rpg_test

import (
	"testing"

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
	if empty.PartyHPRatio() != 0 || empty.PartyMPRatio() != 0 || empty.AverageLevel() != 0 {
		t.Error("an empty party must report zeros")
	}
}
