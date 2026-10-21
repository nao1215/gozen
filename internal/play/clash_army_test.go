package play

import "testing"

func TestArmyCapAndSoldierCount(t *testing.T) {
	s := &clashScene{army: true}
	if got := s.cap(); got != defaultArmySoldiers {
		t.Fatalf("default cap %d", got)
	}
	s.maxSoldiers = 8
	if got := s.cap(); got != 8 {
		t.Fatalf("cap %d", got)
	}
	s.maxSoldiers = 40
	if got := s.cap(); got != 12 {
		t.Fatalf("cap is held to 12, got %d", got)
	}
	s.maxSoldiers = 0
	u := &clashUnit{shown: 450, per: 113} // 450 troops over 4 soldiers
	for _, c := range []struct {
		troops float64
		want   int
	}{{450, 4}, {300, 3}, {200, 2}, {50, 1}, {0, 0}} {
		u.shown = c.troops
		if got := s.count(u); got != c.want {
			t.Errorf("%v troops: %d soldiers, want %d", c.troops, got, c.want)
		}
	}
	if (&clashScene{}).cap() != maxSoldiers {
		t.Error("the ranks keep their cap")
	}
}
